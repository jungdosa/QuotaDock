package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/security"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func init() {
	// A rescan mark is cleared by a timer; no test waits for it.
	settleRescan = func(func()) {}
}

func TestConnectionGuideForProviderStates(t *testing.T) {
	providers := []model.ProviderID{model.ProviderClaude, model.ProviderCodex, model.ProviderGrok}
	cases := []struct {
		name   string
		status model.ConnectionStatus
		code   model.ErrorCode
		want   connectionGuideState
	}{
		{"not installed", model.StatusUnavailable, model.ErrCLINotInstalled, connectionGuideInstall},
		{"signed out", model.StatusLoggedOut, model.ErrNotLoggedIn, connectionGuideSignIn},
		{"outdated", model.StatusOutdated, model.ErrCLIOutdated, connectionGuideUpdate},
		{"timeout", model.StatusError, model.ErrTimeout, connectionGuideUnknown},
		{"connected", model.StatusConnected, "", connectionGuideUnknown},
	}
	for _, provider := range providers {
		for _, test := range cases {
			t.Run(string(provider)+"/"+test.name, func(t *testing.T) {
				lane := LaneState{Provider: provider, Status: test.status, Error: test.code}
				if got := connectionGuideFor(lane); got != test.want {
					t.Fatalf("guide = %v, want %v", got, test.want)
				}
			})
		}
	}
}

func TestNormalConnectionGuideShortcut(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	state := sampleState()
	state.Lanes[0].Rows = nil
	state.Lanes[0].Status = model.StatusLoggedOut
	v.SetState(state)
	v.Show(NormalScreen)
	var guide *SmallButton
	walkCanvasObject(v.Normal, func(object fyne.CanvasObject) {
		if button, ok := object.(*SmallButton); ok && button.Label == "Connection guide" {
			guide = button
		}
	})
	if guide == nil {
		t.Fatal("disconnected normal lane has no guide shortcut")
	}
	guide.Tapped(nil)
	if v.Screen() != SettingsScreen || v.openConnectionPanel.provider != model.ProviderClaude || !v.connectionCache[0].panelOpen {
		t.Fatal("shortcut did not open the Claude settings guide")
	}
	for _, screen := range []struct {
		mode   Screen
		object fyne.CanvasObject
	}{{CompactScreen, v.Compact}, {NanoScreen, v.Nano}} {
		found := false
		walkCanvasObject(screen.object, func(object fyne.CanvasObject) {
			if button, ok := object.(*SmallButton); ok && button.Label == "Connection guide" {
				found = true
			}
		})
		if found {
			t.Fatalf("%v mode contains a guide shortcut", screen.mode)
		}
	}
}

func TestCopyConnectionCommandCopiesOnlyCommand(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	copyConnectionCommand(claudeInstallCommand)
	if got := app.Clipboard().Content(); got != claudeInstallCommand {
		t.Fatalf("clipboard = %q, want %q", got, claudeInstallCommand)
	}
}

func TestClaudeAuthAlternativeOnCLIGuides(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	id := model.ProviderClaude
	closeButton := NewOutlinedSmallButton("Close", "Close", nil, v.colors)
	v.Actions.WebAuthAvailable = false
	if panel := v.connectionInstallPanel(id, closeButton); panel.authButton != nil {
		t.Fatal("Auth alternative shown without WebAuth")
	}
	v.Actions.WebAuthAvailable = true
	var signedIn model.ProviderID
	v.Actions.SignIn = func(provider model.ProviderID) { signedIn = provider }
	var saved string
	v.Actions.ConfigChanged = func(config settings.Config) { saved = config.ConnectionMethods[string(id)] }
	for _, panel := range []connectionPanelView{v.connectionInstallPanel(id, closeButton), v.connectionSignInPanel(id, closeButton)} {
		if panel.authButton == nil {
			t.Fatal("Claude CLI guide has no Auth alternative")
		}
		panel.authButton.Tapped(nil)
		if v.selectedConnectionMethod(id) != connectionMethodAuth || saved != string(connectionMethodAuth) || signedIn != id {
			t.Fatalf("Auth action method=%q saved=%q signed in=%q", v.selectedConnectionMethod(id), saved, signedIn)
		}
	}
}

func TestCodexAuthAlternativeOnlyOnSignInGuide(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	id := model.ProviderCodex
	closeButton := NewOutlinedSmallButton("Close", "Close", nil, v.colors)
	v.Actions.WebAuthAvailable = false
	var signedIn model.ProviderID
	v.Actions.SignIn = func(provider model.ProviderID) { signedIn = provider }
	if panel := v.connectionInstallPanel(id, closeButton); panel.authButton != nil {
		t.Fatal("Codex Auth alternative offered without installed CLI")
	}
	panel := v.connectionSignInPanel(id, closeButton)
	if panel.authButton == nil {
		t.Fatal("Codex sign-in guide has no Auth alternative")
	}
	panel.authButton.Tapped(nil)
	if v.selectedConnectionMethod(id) != connectionMethodAuth || signedIn != id {
		t.Fatalf("Codex Auth action method=%q signed in=%q", v.selectedConnectionMethod(id), signedIn)
	}
}

func TestAntigravityGuideRescans(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	var inspected model.ProviderID
	v.Actions.Inspect = func(id model.ProviderID) { inspected = id }
	panel := v.connectionPanel(LaneState{Provider: model.ProviderAntigravity, Status: model.StatusUnavailable}, connectionMethodIDE)
	if panel.rescanButton == nil {
		t.Fatal("Antigravity guide has no rescan button")
	}
	panel.rescanButton.Tapped(nil)
	if inspected != model.ProviderAntigravity {
		t.Fatalf("inspected = %q", inspected)
	}
}

func TestFirstRunCardWaitsAndDismisses(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	window := test.NewWindow(nil)
	defer window.Close()
	v := NewView(window.Canvas(), catalog, i18n.English, settings.Default(), Actions{FirstRun: true})
	window.SetContent(v.Root)
	if len(v.normalIntro.Objects) != 0 {
		t.Fatal("first-run card appeared before refresh")
	}
	baseHeight := v.MinimumSize(NormalScreen).Height
	state := sampleState()
	state.Lanes[0].Status = model.StatusLoggedOut
	state.Lanes[0].Rows = nil
	v.SetState(state)
	if len(v.normalIntro.Objects) != 1 || v.MinimumSize(NormalScreen).Height <= baseHeight {
		t.Fatalf("card objects=%d height=%.1f base=%.1f", len(v.normalIntro.Objects), v.MinimumSize(NormalScreen).Height, baseHeight)
	}
	cardHeight := v.MinimumSize(NormalScreen).Height
	var closeButton *SmallButton
	walkCanvasObject(v.normalIntro, func(object fyne.CanvasObject) {
		if button, ok := object.(*SmallButton); ok && button.Label == "Close" {
			closeButton = button
		}
	})
	if closeButton == nil {
		t.Fatal("first-run card cannot be dismissed")
	}
	closeButton.Tapped(nil)
	if len(v.normalIntro.Objects) != 0 || v.MinimumSize(NormalScreen).Height >= cardHeight {
		t.Fatalf("card did not reduce height from %.1f, got %.1f", cardHeight, v.MinimumSize(NormalScreen).Height)
	}
	v.SetState(state)
	if len(v.normalIntro.Objects) != 0 {
		t.Fatal("dismissed card reappeared")
	}
}

func TestFirstRunCardFitsTwelveLocales(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	window := test.NewWindow(nil)
	defer window.Close()
	v := NewView(window.Canvas(), catalog, i18n.English, settings.Default(), Actions{FirstRun: true})
	window.SetContent(v.Root)
	state := sampleState()
	state.Lanes[0].Status = model.StatusLoggedOut
	state.Lanes[0].Rows = nil
	state.Lanes[1].Status = model.StatusUnavailable
	state.Lanes[1].Error = model.ErrCLINotInstalled
	state.Lanes[1].Rows = nil
	v.SetState(state)
	for _, language := range i18n.Supported {
		config := v.config
		config.Language = settings.Language(language)
		v.SetConfig(config)
		v.Show(NormalScreen)
		window.Resize(v.MinimumSize(NormalScreen))
		if got := v.normalIntro.MinSize().Width; got > NormalWidth {
			t.Errorf("%s first-run card width %.1f exceeds widget width %.1f", language, got, NormalWidth)
		}
		closeButton := NewOutlinedSmallButton(v.text(i18n.KeyConnectionPanelClose), "", nil, v.colors)
		for _, panel := range []connectionPanelView{
			v.connectionInstallPanel(model.ProviderClaude, closeButton),
			v.connectionSignInPanel(model.ProviderClaude, closeButton),
			v.connectionUpdatePanel(LaneState{Provider: model.ProviderClaude, Status: model.StatusOutdated}, closeButton),
		} {
			if got := panel.object.MinSize().Width; got > SettingsWidth {
				t.Errorf("%s connection guide width %.1f exceeds settings width %.1f", language, got, SettingsWidth)
			}
		}
		if got := window.Canvas().Capture().Bounds().Dy(); got < int(v.MinimumSize(NormalScreen).Height) {
			t.Errorf("%s capture height %d below minimum %.1f", language, got, v.MinimumSize(NormalScreen).Height)
		}
	}
}

// Every guide's docs button opens its address through the browser allowlist,
// which refuses silently; an address outside it makes the button look dead.
func TestEveryInstallDocsAddressIsOpenable(t *testing.T) {
	for _, id := range []model.ProviderID{model.ProviderClaude, model.ProviderCodex, model.ProviderGrok} {
		if address := connectionInstallURL(id); !security.IsAllowedExternalURL(address) {
			t.Errorf("%s docs address %q is not on the browser allowlist", id, address)
		}
	}
}

// The sign-in step hands over the CLI's sign-in command, not the bare program
// name, and a tapped rescan button shows that it is working.
func TestSignInGuideCopiesTheSignInCommandAndRescanShowsProgress(t *testing.T) {
	for id, want := range map[model.ProviderID]string{model.ProviderClaude: "claude auth login", model.ProviderCodex: "codex login", model.ProviderGrok: "grok login"} {
		if got := connectionSignInCommand(id); got != want {
			t.Errorf("%s sign-in command = %q, want %q", id, got, want)
		}
	}
	v, window := newTestView(t)
	defer window.Close()
	inspected := 0
	v.Actions.Inspect = func(model.ProviderID) { inspected++ }
	before := v.connectionRescanButton(model.ProviderGrok)
	before.Tapped(nil)
	if inspected != 1 {
		t.Fatalf("rescan ran %d inspections", inspected)
	}
	after := v.connectionRescanButton(model.ProviderGrok)
	if after.Label == before.Label || !after.Disabled {
		t.Fatalf("rescan button shows no progress: label %q disabled %v", after.Label, after.Disabled)
	}
}

// Adding an account puts its card directly below the card whose button was
// pressed, even when the saved order had parked the hidden account elsewhere.
func TestAddedAccountAppearsBelowTheLastShownOne(t *testing.T) {
	cfg := settings.Default()
	cfg.ClaudeAccounts = 2
	cfg.ShowClaudeAuth = true
	cfg.LaneOrder = []string{"claude", "claude-auth", "codex", "codex-2", "codex-3", "codex-4", "codex-5", "claude-4", "claude-5", "antigravity", "grok", "claude-3"}
	adjustAccountCount(&cfg, model.ProviderClaudeAuth, 1)
	if cfg.ClaudeAccounts != 3 {
		t.Fatalf("claude accounts = %d", cfg.ClaudeAccounts)
	}
	if got := cfg.LaneOrder[:3]; got[0] != "claude" || got[1] != "claude-auth" || got[2] != "claude-3" {
		t.Fatalf("third account was not placed after the second: %v", cfg.LaneOrder)
	}
	if len(cfg.LaneOrder) != len(settings.DefaultLaneOrder()) {
		t.Fatalf("order lost or repeated an entry: %v", cfg.LaneOrder)
	}
	adjustAccountCount(&cfg, model.ProviderCodex, 1)
	if cfg.CodexAccounts != 2 || cfg.LaneOrder[3] != "codex" || cfg.LaneOrder[4] != "codex-2" {
		t.Fatalf("second Codex account misplaced: %d %v", cfg.CodexAccounts, cfg.LaneOrder)
	}
	before := append([]string(nil), cfg.LaneOrder...)
	adjustAccountCount(&cfg, model.ProviderClaude3, -1)
	for index := range before {
		if cfg.LaneOrder[index] != before[index] {
			t.Fatalf("removing an account reordered the cards: %v", cfg.LaneOrder)
		}
	}
}

// A signed-in Auth account says so in its panel and offers to sign in again;
// one that is not signed in keeps the plain sign-in button and no notice.
func TestAuthPanelShowsSignedInStateForBothFamilies(t *testing.T) {
	for _, id := range []model.ProviderID{model.ProviderClaude, model.ProviderCodex} {
		v, window := newTestView(t)
		v.Actions.WebAuthAvailable = true
		v.Actions.SignIn = func(model.ProviderID) {}
		config := v.config
		config.ConnectionMethods = map[string]string{string(id): settings.ConnectionMethodAuth}
		v.SetConfig(config)
		for _, signedIn := range []bool{true, false} {
			lane := LaneState{Provider: id, Status: model.StatusLoggedOut}
			if signedIn {
				lane.Status = model.StatusConnected
			}
			labels := map[string]bool{}
			notice := false
			walkCanvasObject(v.connectionPanel(lane, connectionMethodAuth).object, func(object fyne.CanvasObject) {
				switch value := object.(type) {
				case *SmallButton:
					labels[value.Label] = true
				case *canvas.Text:
					if strings.Contains(value.Text, "Signed in.") {
						notice = true
					}
				}
			})
			if notice != signedIn || labels["Sign in again"] != signedIn || labels["Sign in"] == signedIn {
				t.Fatalf("%s signedIn=%v: notice=%v buttons=%v", id, signedIn, notice, labels)
			}
		}
		window.Close()
	}
}
