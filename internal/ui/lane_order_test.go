package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// orderedState names every provider so a reordering test can tell a dropped
// lane apart from a reordered one.
func orderedState() ViewState {
	lane := func(id model.ProviderID, name string, rows ...UsageRowState) LaneState {
		return LaneState{Provider: id, Name: name, Status: model.StatusConnected, Rows: rows}
	}
	session := UsageRowState{Label: "Session", Percent: 10, WindowMinutes: 300, ResetsAt: time.Now().Add(time.Hour)}
	weekly := UsageRowState{Label: "Weekly", Percent: 20, WindowMinutes: 7 * 24 * 60, ResetsAt: time.Now().Add(72 * time.Hour)}
	gemini := UsageRowState{Label: "Gemini Session", Percent: 30, WindowMinutes: 300, ResetsAt: time.Now().Add(time.Hour)}
	agClaude := UsageRowState{Label: "Claude Session", Percent: 40, WindowMinutes: 300, ResetsAt: time.Now().Add(time.Hour)}
	return ViewState{Lanes: []LaneState{
		lane(model.ProviderClaude, "Claude", session, weekly),
		lane(model.ProviderCodex, "Codex", weekly),
		lane(model.ProviderAntigravity, "Antigravity", gemini, agClaude),
		lane(model.ProviderGrok, "Grok", weekly),
		lane(model.ProviderClaudeAuth, "Claude Auth", session, weekly),
	}}
}

func orderedView(t *testing.T, order []string) *View {
	t.Helper()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	config := settings.Default()
	config.ShowClaudeAuth = true
	config.ShowGrok = true
	config.LaneOrder = order
	config = config.Validated()
	view := NewView(nil, catalog, i18n.English, config, Actions{AppVersion: testAppVersion})
	view.state = orderedState()
	view.config = config
	return view
}

func laneIDs(lanes []LaneState) []string {
	out := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		out = append(out, string(lane.Provider))
	}
	return out
}

// The order the user drags into has to reach the window itself, not just the
// place they dragged it.
func TestVisibleLanesFollowTheConfiguredOrder(t *testing.T) {
	view := orderedView(t, []string{"grok", "antigravity", "codex", "claude-auth", "claude"})
	got := strings.Join(laneIDs(view.visibleLanes()), ",")
	want := "grok,antigravity,codex,claude-auth,claude"
	if got != want {
		t.Fatalf("visible lanes = %s, want %s", got, want)
	}
}

// Antigravity is one provider group that happens to draw two nano cells. A
// reorder must move the pair together and never interleave another provider
// between them.
func TestNanoKeepsTheAntigravityPairTogetherWhenReordered(t *testing.T) {
	view := orderedView(t, []string{"antigravity", "grok", "claude", "claude-auth", "codex"})
	keys := make([]string, 0, 6)
	for _, cell := range view.nanoCellStates() {
		keys = append(keys, cell.key)
	}
	got := strings.Join(keys, ",")
	want := "antigravity-gemini,antigravity,grok,claude,claude-auth,codex"
	if got != want {
		t.Fatalf("nano cells = %s, want %s", got, want)
	}
}

// A settings card sitting somewhere other than its lane would make the two
// screens disagree about what "second" means.
func TestConnectionCardsFollowTheConfiguredOrder(t *testing.T) {
	view := orderedView(t, []string{"codex", "claude", "grok", "claude-auth", "antigravity"})
	view.buildConnectionRows()
	ids := make([]string, 0, len(view.connectionCache))
	for _, card := range view.connectionCache {
		ids = append(ids, string(card.id))
	}
	got := strings.Join(ids, ",")
	want := "codex,claude,grok,claude-auth,antigravity"
	if got != want {
		t.Fatalf("connection cards = %s, want %s", got, want)
	}
}

// The tray tooltip is the same reading in a smaller space, so it follows the
// same order rather than the order the controller happened to build lanes in.
func TestTrayTooltipFollowsTheConfiguredOrder(t *testing.T) {
	config := settings.Default()
	config.ShowClaudeAuth = true
	config.ShowGrok = true
	config.LaneOrder = []string{"grok", "codex", "antigravity", "claude", "claude-auth"}
	tooltip := BuildTrayTooltip(orderedState(), config.Validated(), i18n.English, time.Now())
	positions := []int{
		strings.Index(tooltip, "Grok"),
		strings.Index(tooltip, "Codex"),
		strings.Index(tooltip, "Antigravity"),
	}
	for index, position := range positions {
		if position < 0 {
			t.Fatalf("tooltip is missing entry %d:\n%s", index, tooltip)
		}
		if index > 0 && position < positions[index-1] {
			t.Fatalf("tooltip ignored the configured order:\n%s", tooltip)
		}
	}
}

// A config from before the order existed, or one a hand edit left short, must
// still draw every provider the user has switched on.
func TestAnAbsentOrderStillDrawsEveryProvider(t *testing.T) {
	view := orderedView(t, nil)
	got := strings.Join(laneIDs(view.visibleLanes()), ",")
	want := "claude,claude-auth,codex,antigravity,grok"
	if got != want {
		t.Fatalf("visible lanes = %s, want the shipped order %s", got, want)
	}
}

// The window body introduces a provider with the same mark compact puts in
// front of that provider's rows. Two different logos for one provider would
// read as two different services.
func TestNormalLaneHeaderLeadsWithTheSameMarkCompactUses(t *testing.T) {
	view := orderedView(t, nil)
	for _, lane := range view.visibleLanes() {
		_, handles := view.makeLaneHeader(lane)
		if handles.icon == nil {
			t.Fatalf("%s lane header has no brand mark", lane.Provider)
		}
		if len(lane.Rows) == 0 {
			continue
		}
		want := view.rowVisual(lane, lane.Rows[0], time.Now()).iconKind
		if got := laneIconKind(lane); got != want {
			t.Fatalf("%s header mark=%s, compact first row=%s", lane.Provider, got, want)
		}
	}
}

// Antigravity draws two logos across its rows. Hiding the Gemini half has to
// move the header onto the mark that is actually left, not leave it announcing
// a reading the group no longer shows.
func TestAntigravityHeaderMarkFollowsTheHalfStillShown(t *testing.T) {
	view := orderedView(t, nil)
	find := func() LaneState {
		for _, lane := range view.visibleLanes() {
			if lane.Provider == model.ProviderAntigravity {
				return lane
			}
		}
		t.Fatal("the Antigravity lane is not visible")
		return LaneState{}
	}
	if got := laneIconKind(find()); got != ProviderIconGemini {
		t.Fatalf("both halves shown: mark=%s, want the Gemini mark", got)
	}
	config := view.config
	config.ShowAGGemini = false
	view.config = config.Validated()
	if got := laneIconKind(find()); got != ProviderIconAGClaude {
		t.Fatalf("Gemini hidden: mark=%s, want the Antigravity Claude mark", got)
	}
}
