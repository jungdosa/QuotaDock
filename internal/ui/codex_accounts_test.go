package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func codexAccountsState() ViewState {
	state := defaultViewState()
	for i := range state.Lanes {
		lane := &state.Lanes[i]
		if !model.IsCodexAccount(lane.Provider) {
			continue
		}
		lane.Status = model.StatusConnected
		lane.Plan = "PLUS"
		lane.Credits = &model.Credits{Balance: 25, HasCredits: true}
		lane.Rows = []UsageRowState{
			{Label: "Spark", WindowMinutes: 10080, Percent: 45, ResetsAt: time.Now().Add(3 * 24 * time.Hour)},
			{Label: "Weekly", WindowMinutes: 10080, Percent: float64(model.CodexAccountIndex(lane.Provider) * 10), ResetsAt: time.Now().Add(3 * 24 * time.Hour)},
			{Label: "Spark", WindowMinutes: 300, Percent: 80, ResetsAt: time.Now().Add(2 * time.Hour)},
		}
	}
	return state
}

func TestCodexAccountsControlsBoundedAndSeparateFromClaude(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)
	for count := 1; count < 5; count++ {
		last := model.CodexAccountIDs()[count-1]
		row := connectionRowForTest(t, v, last)
		if row.addButton == nil {
			t.Fatalf("missing add at %d", count)
		}
		row.addButton.Tapped(nil)
		if v.config.CodexAccounts != count+1 || v.config.ClaudeAccounts != 1 {
			t.Fatal("wrong account count changed")
		}
	}
	last := connectionRowForTest(t, v, model.ProviderCodex5)
	if last.addButton != nil || last.removeButton == nil {
		t.Fatal("account count not bounded")
	}
	for _, id := range model.CodexAccountIDs()[1:] {
		row := connectionRowForTest(t, v, id)
		if len(row.methods) != 1 || row.methods[0].method != connectionMethodAuth || row.methods[0].button.State == connectionMethodPlanned || row.labelEntry == nil {
			t.Fatalf("incomplete account controls: %s", id)
		}
	}
	v.setAccountLabel(model.ProviderCodex5, "Spare")
	connectionRowForTest(t, v, model.ProviderCodex5).removeButton.Tapped(nil)
	if v.config.CodexAccounts != 4 {
		t.Fatal("remove failed")
	}
	connectionRowForTest(t, v, model.ProviderCodex4).addButton.Tapped(nil)
	if connectionRowForTest(t, v, model.ProviderCodex5).name.Text != "Spare" {
		t.Fatal("re-add lost alias")
	}
	if v.MinimumSize(SettingsScreen).Height != SettingsHeight {
		t.Fatal("multi-account settings exceed fixed scroll viewport")
	}
}

func TestCodexAccountsRenderWithTheirOwnLabelsColorsAndLimits(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.CodexAccounts = 2
	cfg.ShowClaude = false
	cfg.ShowAGClaude = false
	cfg.ShowAGGemini = false
	cfg.AccountLabels = map[string]string{"codex": "Work", "codex-2": "Personal"}
	v.SetConfig(cfg)
	v.SetState(codexAccountsState())
	lanes := v.visibleLanes()
	if len(lanes) != 2 || lanes[0].Name != "Work" || lanes[1].Name != "Personal" {
		t.Fatalf("lanes=%+v", lanes)
	}
	for _, lane := range lanes {
		if providerIconKind(lane, lane.Rows[0]) != ProviderIconCodex || providerColorKey(lane.Provider, lane.Rows[0]) != string(lane.Provider) {
			t.Fatal("incorrect branding")
		}
		if codexRowGroup(lane.Rows[0]) != "" {
			t.Fatal("general limit must precede named limits")
		}
		if !strings.Contains(englishUsageLabel(lane, lane.Rows[1]), "Spark") {
			t.Fatal("named limit label lost")
		}
	}
	v.Show(CompactScreen)
	if len(v.compactCache.accountHeaders) != 2 {
		t.Fatal("compact account headers missing")
	}
	cfg.AccountLabels["codex-2"] = "Home"
	v.SetConfig(cfg)
	if v.compactCache.accountHeaders[1].label.Text != "Home" {
		t.Fatal("compact header cache retained old alias")
	}
	if cells := v.nanoCellStates(); len(cells) != 2 || cells[1].key != "codex-2" || cells[1].name != "Home" || cells[1].kind != ProviderIconCodex {
		t.Fatalf("nano=%+v", cells)
	}
	tooltip := BuildTrayTooltip(v.state, v.config, i18n.English, time.Now())
	if !strings.Contains(tooltip, "Work") || !strings.Contains(tooltip, "Home") {
		t.Fatal("tooltip lost accounts")
	}
	cfg.ShowCodexCredits = false
	v.SetConfig(cfg)
	if v.laneCreditsVisible(lanes[1]) {
		t.Fatal("additional account ignored credits toggle")
	}
	cfg.ShowCodex = false
	v.SetConfig(cfg)
	if len(v.visibleLanes()) != 0 || len(v.nanoCellStates()) != 0 {
		t.Fatal("hidden Codex accounts still visible")
	}
}

func TestCodexSignInDisablesConflictingControls(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.CodexAccounts = 2
	v.SetConfig(cfg)
	v.Show(SettingsScreen)
	v.SetSigningIn(model.ProviderCodex2, true)
	row := connectionRowForTest(t, v, model.ProviderCodex2)
	if !row.testButton.Disabled || !row.reconnect.Disabled || !row.removeButton.Disabled {
		t.Fatal("busy controls remain enabled")
	}
	v.SetSigningIn(model.ProviderCodex2, false)
	row = connectionRowForTest(t, v, model.ProviderCodex2)
	if row.testButton.Disabled || row.reconnect.Disabled || row.removeButton.Disabled {
		t.Fatal("controls did not recover")
	}
}

type heldAccountProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p heldAccountProvider) Inspect(context.Context) model.ConnectionState {
	return model.ConnectionState{Status: model.StatusConnected}
}
func (p heldAccountProvider) Refresh(context.Context) (model.UsageSnapshot, error) {
	close(p.started)
	<-p.release
	return model.UsageSnapshot{Provider: model.ProviderCodex2, Limits: []model.UsageLimit{{Label: "Weekly", UsedPercent: 88, WindowMinutes: 10080}}}, nil
}
func (p heldAccountProvider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return p.Refresh(ctx)
}
func (p heldAccountProvider) Close() error { return nil }

func TestAccountIdentityResetRejectsAnInflightOldReading(t *testing.T) {
	p := heldAccountProvider{started: make(chan struct{}), release: make(chan struct{})}
	c := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderCodex2: p}}, settings.Default())
	done := make(chan ViewState, 1)
	go func() { done <- c.Refresh(context.Background()) }()
	<-p.started
	c.ForgetAccount(model.ProviderCodex2)
	close(p.release)
	state := <-done
	for _, lane := range state.Lanes {
		if lane.Provider == model.ProviderCodex2 && (len(lane.Rows) != 0 || lane.Credits != nil || lane.Status == model.StatusConnected) {
			t.Fatal("old identity was republished")
		}
	}
}

func TestCodexAccountRenderCaptures(t *testing.T) {
	output := os.Getenv("QD_CODEX_CAPTURE_DIR")
	if output == "" {
		t.Skip("optional rendered review artifacts")
	}
	if err := os.MkdirAll(output, 0o700); err != nil {
		t.Fatal(err)
	}
	v, w := newTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.CodexAccounts = 2
	cfg.Language = settings.Language(i18n.Korean)
	cfg.AccountLabels = map[string]string{"codex": "업무", "codex-2": "개인"}
	v.SetConfig(cfg)
	v.SetState(codexAccountsState())
	for _, screen := range []struct {
		name   string
		screen Screen
	}{{"normal", NormalScreen}, {"compact", CompactScreen}, {"nano", NanoScreen}, {"connections", SettingsScreen}} {
		v.Show(screen.screen)
		w.Resize(v.MinimumSize(screen.screen))
		if screen.screen == SettingsScreen {
			v.settingsScroll.ScrollToBottom()
			v.toggleConnectionPanel(model.ProviderCodex2, connectionMethodAuth)
		}
		f, err := os.Create(filepath.Join(output, "codex-accounts-"+screen.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, w.Canvas().Capture())
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
