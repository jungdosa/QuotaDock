package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
)

func TestConnectionMethodCountsByProvider(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	v.Show(SettingsScreen)

	want := map[model.ProviderID]int{
		model.ProviderClaude:      3,
		model.ProviderCodex:       1,
		model.ProviderAntigravity: 1,
	}
	for _, row := range v.connectionCache {
		if len(row.methods) != want[row.id] {
			t.Fatalf("%s method buttons=%d, want %d", row.id, len(row.methods), want[row.id])
		}
	}
}

func TestConnectionMethodStatesAreDerived(t *testing.T) {
	tests := []struct {
		name          string
		lane          LaneState
		method        connectionMethod
		envConfigured bool
		want          connectionMethodState
	}{
		{"Claude CLI connected", LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected}, connectionMethodCLI, false, connectionMethodActive},
		{"Claude CLI missing", LaneState{Provider: model.ProviderClaude, Status: model.StatusUnavailable}, connectionMethodCLI, false, connectionMethodMissing},
		{"Claude Auth planned", LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected}, connectionMethodAuth, false, connectionMethodPlanned},
		{"Claude ENV missing", LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected}, connectionMethodOther, false, connectionMethodMissing},
		{"Claude ENV available", LaneState{Provider: model.ProviderClaude, Status: model.StatusUnavailable}, connectionMethodOther, true, connectionMethodAvailable},
		{"Codex CLI connected", LaneState{Provider: model.ProviderCodex, Status: model.StatusConnected}, connectionMethodCLI, false, connectionMethodActive},
		{"Codex CLI missing", LaneState{Provider: model.ProviderCodex, Status: model.StatusLoggedOut}, connectionMethodCLI, false, connectionMethodMissing},
		{"Antigravity IDE connected", LaneState{Provider: model.ProviderAntigravity, Status: model.StatusConnected}, connectionMethodIDE, false, connectionMethodActive},
		{"Antigravity IDE missing", LaneState{Provider: model.ProviderAntigravity, Status: model.StatusUnavailable}, connectionMethodIDE, false, connectionMethodMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := connectionMethodStateFor(test.lane, test.method, test.envConfigured); got != test.want {
				t.Fatalf("state=%v, want %v", got, test.want)
			}
		})
	}

	button := NewConnectionMethodButton("CLI", "CLI", connectionMethodMissing, nil, DarkBrandColors)
	button.Resize(fyne.NewSize(42, 22))
	renderer := button.CreateRenderer().(*connectionMethodButtonRenderer)
	if renderer.border.Visible() {
		t.Fatal("missing method rendered a solid border")
	}
	for index, dash := range renderer.dashes {
		if !dash.Visible() {
			t.Fatalf("missing method dash %d is hidden", index)
		}
	}
	button.State = connectionMethodActive
	renderer.Refresh()
	if !renderer.border.Visible() {
		t.Fatal("active method did not render a solid border")
	}
	for index, dash := range renderer.dashes {
		if dash.Visible() {
			t.Fatalf("active method dash %d remained visible", index)
		}
	}
}

func TestConnectionPanelsAreAccordion(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	v.Show(SettingsScreen)

	claude := v.connectionCache[0]
	codex := v.connectionCache[1]
	claude.methods[0].button.Tapped(nil)
	if !claude.panelOpen || codex.panelOpen || openConnectionPanelCount(v) != 1 {
		t.Fatal("opening Claude did not leave exactly one inline panel open")
	}
	codex.methods[0].button.Tapped(nil)
	if claude.panelOpen || !codex.panelOpen || openConnectionPanelCount(v) != 1 {
		t.Fatal("opening Codex did not close the previous Claude panel")
	}
	codex.methods[0].button.Tapped(nil)
	if openConnectionPanelCount(v) != 0 || v.openConnectionPanel != (connectionPanelSelection{}) {
		t.Fatal("tapping the selected method did not collapse the accordion")
	}
}

func TestClaudeAuthAndEnvironmentPanelsExposeNoLoginFlowOrToken(t *testing.T) {
	const secret = "phase4c-token-must-not-appear"
	t.Setenv(claudeOAuthTokenEnv, secret)
	v, window := newTestView(t)
	defer window.Close()
	v.Show(SettingsScreen)
	claude := v.connectionCache[0]

	claude.methods[1].button.Tapped(nil)
	if claude.methods[1].button.State != connectionMethodPlanned || claude.panelView.rescanButton != nil || claude.panelView.docsButton != nil {
		t.Fatal("Claude Auth is not a placeholder-only planned panel")
	}
	if text := connectionPanelText(claude); !strings.Contains(text, v.text(i18n.KeyConnectionAuthPlanned)) {
		t.Fatalf("Claude Auth placeholder text=%q", text)
	}

	claude.methods[2].button.Tapped(nil)
	if claude.methods[2].button.State != connectionMethodAvailable {
		t.Fatalf("configured Claude ENV state=%v, want available", claude.methods[2].button.State)
	}
	if text := connectionPanelText(claude); strings.Contains(text, secret) {
		t.Fatal("Claude ENV token value reached the inline panel")
	}
}

func openConnectionPanelCount(v *View) int {
	count := 0
	for _, row := range v.connectionCache {
		if row.panelOpen {
			count++
		}
	}
	return count
}

func connectionPanelText(row *connectionView) string {
	var values []string
	walkCanvasObject(row.panel, func(object fyne.CanvasObject) {
		switch text := object.(type) {
		case *canvas.Text:
			values = append(values, text.Text)
		case *widget.RichText:
			for _, segment := range text.Segments {
				if value, ok := segment.(*widget.TextSegment); ok {
					values = append(values, value.Text)
				}
			}
		}
	})
	return strings.Join(values, "\n")
}
