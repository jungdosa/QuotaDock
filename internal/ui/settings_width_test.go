package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// The settings screen is exactly as wide as the normal widget, in every
// language and with any guide panel open: a row that outgrows the width would
// push the window wider, which is what this screen used to do.
func TestSettingsFitTheNormalWidthInEveryLanguage(t *testing.T) {
	if SettingsWidth != NormalWidth {
		t.Fatalf("settings width %.0f differs from the normal width %.0f", SettingsWidth, NormalWidth)
	}
	type condition struct {
		name   string
		status model.ConnectionStatus
		code   model.ErrorCode
		key    string
	}
	conditions := []condition{
		{"connected", model.StatusConnected, model.ErrNone, ""},
		{"not installed", model.StatusUnavailable, model.ErrCLINotInstalled, i18n.KeyErrorCLINotInstalled},
		{"signed out", model.StatusLoggedOut, model.ErrNotLoggedIn, i18n.KeyErrorNotLoggedIn},
		{"outdated", model.StatusOutdated, model.ErrCLIOutdated, i18n.KeyErrorCLIOutdated},
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range i18n.Supported {
		app := test.NewApp()
		app.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
		window := test.NewWindow(nil)
		config := settings.Default()
		config.ClaudeAccounts = 3
		config.ShowClaudeAuth = true
		config.CodexAccounts = 2
		config.ShowGrok = true
		config.AccountLabels = map[string]string{"claude": "Abcdefghij Klmnopqrs", "claude-auth": "Jungdosa.US", "codex": "Abcdefghij Klmnopqrs"}
		v := NewView(window.Canvas(), catalog, language, config, Actions{AppVersion: "0.0.0", WebAuthAvailable: true, Inspect: func(model.ProviderID) {}, SignIn: func(model.ProviderID) {}})
		window.SetContent(v.Root)
		v.Show(SettingsScreen)
		for _, state := range conditions {
			view := sampleState()
			for index := range view.Lanes {
				view.Lanes[index].Status = state.status
				view.Lanes[index].Error = state.code
				view.Lanes[index].ErrorKey = state.key
				view.Lanes[index].CLIPath = `C:\Users\someone\AppData\Local\Programs\OpenAI\Codex\bin\codex.exe`
				view.Lanes[index].CLIVersion = "0.160.1"
			}
			v.SetState(view)
			if width := v.Settings.MinSize().Width; width > SettingsWidth {
				t.Errorf("%s, lanes %s: settings need %.0f, more than %.0f", language, state.name, width, SettingsWidth)
			}
			for _, row := range v.connectionCache {
				for _, method := range connectionMethodsFor(row.id) {
					lane := v.connectionLane(row.id)
					lane.Provider = row.id
					lane.Status, lane.Error, lane.ErrorKey = state.status, state.code, state.key
					panel := v.connectionPanel(lane, method).object
					// A panel sits inside its card, the section and the page.
					if width := panel.MinSize().Width + (SettingsWidth - connectionCardInnerWidth); width > SettingsWidth {
						t.Errorf("%s, %s %s panel while %s: needs %.0f, more than %.0f", language, row.id, method, state.name, width, SettingsWidth)
					}
				}
			}
		}
		window.Close()
		app.Quit()
	}
}
