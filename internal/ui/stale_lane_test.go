package ui

import (
	"context"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// flakyProvider answers a good reading until told to fail, and reports itself
// as failed on inspection once it has — the way the browser-backed account
// does after a refresh times out.
type flakyProvider struct {
	fail *model.ErrorCode
}

func (p flakyProvider) Inspect(context.Context) model.ConnectionState {
	if *p.fail != model.ErrNone {
		return model.ConnectionState{Status: model.StatusError, Error: *p.fail, ErrorKey: "error." + string(*p.fail)}
	}
	return model.ConnectionState{Status: model.StatusConnected}
}

func (p flakyProvider) Refresh(context.Context) (model.UsageSnapshot, error) {
	if code := *p.fail; code != model.ErrNone {
		return model.UsageSnapshot{}, model.SafeError{Code: code, Key: "error." + string(code)}
	}
	return model.UsageSnapshot{
		Provider: model.ProviderClaude,
		Plan:     "MAX",
		Limits:   []model.UsageLimit{{Label: "Session", UsedPercent: 42, WindowMinutes: 300, ResetsAt: time.Now().Add(time.Hour)}},
	}, nil
}

func (p flakyProvider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return p.Refresh(ctx)
}
func (p flakyProvider) Close() error { return nil }

func staleController(t *testing.T) (*Controller, *model.ErrorCode) {
	t.Helper()
	fail := new(model.ErrorCode)
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderClaude: flakyProvider{fail: fail}}}, settings.Default())
	state := controller.Refresh(context.Background())
	if lane := state.Lanes[0]; lane.Status != model.StatusConnected || len(lane.Rows) != 1 {
		t.Fatalf("the first refresh did not connect: %+v", lane)
	}
	return controller, fail
}

// One missed refresh must not blank a lane that was reading fine a minute ago.
// The rows stay, the lane stays connected, and the failure is carried as the
// error code so the state log and the connection card can still say so.
func TestATransientFailureKeepsTheLastReading(t *testing.T) {
	controller, fail := staleController(t)
	*fail = model.ErrTimeout
	lane := controller.Refresh(context.Background()).Lanes[0]
	if lane.Status != model.StatusConnected || len(lane.Rows) != 1 || lane.Rows[0].Percent != 42 {
		t.Fatalf("a timed-out refresh dropped the reading: %+v", lane)
	}
	if lane.Plan != "MAX" {
		t.Fatalf("the held reading lost its plan: %q", lane.Plan)
	}
	if lane.Error != model.ErrTimeout || lane.StaleFor != 1 {
		t.Fatalf("the held reading does not carry the failure: err=%q stale=%d", lane.Error, lane.StaleFor)
	}
}

// A provider that stays down is reported as down. Holding the numbers past
// the limit would show a reading as current long after it stopped being one.
func TestAFailureThatPersistsIsShownAfterTheLimit(t *testing.T) {
	controller, fail := staleController(t)
	*fail = model.ErrUnavailable
	var lane LaneState
	for refresh := 1; refresh <= StaleRefreshLimit; refresh++ {
		lane = controller.Refresh(context.Background()).Lanes[0]
		if lane.Status != model.StatusConnected || lane.StaleFor != refresh {
			t.Fatalf("refresh %d: status=%s stale=%d, want connected and %d", refresh, lane.Status, lane.StaleFor, refresh)
		}
	}
	lane = controller.Refresh(context.Background()).Lanes[0]
	if lane.Status == model.StatusConnected || len(lane.Rows) != 0 {
		t.Fatalf("after %d held refreshes the lane still shows a reading: %+v", StaleRefreshLimit, lane)
	}
	if lane.Error != model.ErrUnavailable {
		t.Fatalf("the persisting failure was not reported: %q", lane.Error)
	}
}

// A recovery clears the hold entirely, so the next blip gets the full limit
// again rather than the remainder of the last one.
func TestARecoveryResetsTheHold(t *testing.T) {
	controller, fail := staleController(t)
	*fail = model.ErrTimeout
	controller.Refresh(context.Background())
	controller.Refresh(context.Background())
	*fail = model.ErrNone
	lane := controller.Refresh(context.Background()).Lanes[0]
	if lane.StaleFor != 0 || lane.Error != model.ErrNone {
		t.Fatalf("a good refresh left the hold in place: stale=%d err=%q", lane.StaleFor, lane.Error)
	}
	*fail = model.ErrTimeout
	if lane = controller.Refresh(context.Background()).Lanes[0]; lane.StaleFor != 1 {
		t.Fatalf("the hold did not restart from one: %d", lane.StaleFor)
	}
}

// Needing the user is not transient. An expired sign-in shown as "still
// connected" for three minutes would hide the one thing they have to act on.
func TestASignInFailureIsShownAtOnce(t *testing.T) {
	controller, fail := staleController(t)
	*fail = model.ErrNotLoggedIn
	lane := controller.Refresh(context.Background()).Lanes[0]
	if lane.Status == model.StatusConnected || len(lane.Rows) != 0 || lane.StaleFor != 0 {
		t.Fatalf("a sign-in failure was held like a blip: %+v", lane)
	}
}

// The connection card must not claim a fresh read that did not happen.
func TestTheConnectionCardSaysWhenAReadingIsHeld(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	held := LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected, Error: model.ErrTimeout, StaleFor: 1}
	if got, want := view.connectionStatusText(held), view.text(i18n.KeyConnectionStale); got != want {
		t.Fatalf("held lane reads %q, want %q", got, want)
	}
	fresh := LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected}
	if got, want := view.connectionStatusText(fresh), view.text(i18n.KeyConnected); got != want {
		t.Fatalf("fresh lane reads %q, want %q", got, want)
	}
}

// A widget saved in nano must open in nano. Opening on the normal screen and
// switching afterwards asked the window for the normal size first, and the
// resize log showed a 539-wide request before the 136-wide one on every launch.
func TestTheViewOpensOnTheSavedScreen(t *testing.T) {
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []settings.DisplayMode{settings.ModeNormal, settings.ModeCompact, settings.ModeNano} {
		config := settings.Default()
		config.DisplayMode = mode
		view := NewView(nil, catalog, i18n.English, config, Actions{AppVersion: testAppVersion})
		if got, want := view.Screen(), ScreenForDisplayMode(mode); got != want {
			t.Fatalf("saved %s opened on screen %v, want %v", mode, got, want)
		}
	}
}

// The line between two groups gets its extra room beneath it, not above: it
// still closes the group it follows, and the next header no longer sits as
// close to it as the last row above does.
func TestTheGroupDividerLeavesRoomBeneathItself(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.Show(NormalScreen)
	window.Resize(view.MinimumSize(NormalScreen))
	objects := view.normalBody.Objects
	if len(view.normalCache.dividers) == 0 {
		t.Fatal("the sample state drew no group divider")
	}
	// The first divider is the first object that is neither a header nor a
	// row: find it by its height, which is the divider's padded height plus
	// the room beneath it.
	wantHeight := 2*CompactDividerPaddingY + 1 + NormalDividerGapBelow
	found := false
	for index := 1; index < len(objects)-1; index++ {
		if objects[index].Size().Height != wantHeight {
			continue
		}
		found = true
		gapAbove := objects[index].Position().Y - (objects[index-1].Position().Y + objects[index-1].Size().Height)
		gapBelow := objects[index+1].Position().Y - (objects[index].Position().Y + objects[index].Size().Height)
		if gapAbove != NormalBodyRowGap || gapBelow != NormalBodyRowGap {
			t.Fatalf("divider gaps above/below = %.1f/%.1f, want the body gap %.1f on both sides with the room inside the divider", gapAbove, gapBelow, NormalBodyRowGap)
		}
		break
	}
	if !found {
		t.Fatalf("no object is %.1f tall — the divider is not carrying %.1f of room beneath its line", wantHeight, NormalDividerGapBelow)
	}
}
