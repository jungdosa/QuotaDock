package ui

import (
	"strings"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// A config written before the selector shipped must behave exactly as it did.
func TestAbsentConnectionMethodKeepsProviderDefault(t *testing.T) {
	config := settings.Default().Validated()
	if config.ConnectionMethods != nil {
		t.Fatalf("default config carries connection methods: %v", config.ConnectionMethods)
	}
	for id, want := range map[model.ProviderID]connectionMethod{
		model.ProviderClaude:      connectionMethodCLI,
		model.ProviderCodex:       connectionMethodCLI,
		model.ProviderAntigravity: connectionMethodIDE,
		model.ProviderGrok:        connectionMethodCLI,
	} {
		if got := selectedConnectionMethod(config, id); got != want {
			t.Fatalf("%s default method = %q, want %q", id, got, want)
		}
	}
}

func TestClaudeAccountAddButtonAndCLIExclusivity(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.Actions.WebAuthAvailable = true
	view.Show(SettingsScreen)

	root := connectionRowForTest(t, view, model.ProviderClaude)
	if root.addButton == nil || view.config.ShowClaudeAuth {
		t.Fatal("single Claude account did not expose the add button")
	}
	root.addButton.Tapped(nil)
	if !view.config.ShowClaudeAuth {
		t.Fatal("add account button did not enable Claude Auth")
	}
	auth := connectionRowForTest(t, view, model.ProviderClaudeAuth)
	if auth.methods[0].button.State != connectionMethodOccupied || !strings.Contains(auth.methods[0].button.Tooltip, view.text(i18n.KeyConnectionCLIInUse)) {
		t.Fatalf("second-account CLI state/tooltip = %v / %q", auth.methods[0].button.State, auth.methods[0].button.Tooltip)
	}
	auth.methods[0].button.Tapped(nil)
	if view.config.ConnectionMethods[string(model.ProviderClaudeAuth)] == string(connectionMethodCLI) {
		t.Fatal("occupied CLI method was selectable")
	}

	view.setConnectionMethod(model.ProviderClaude, connectionMethodAuth)
	auth = connectionRowForTest(t, view, model.ProviderClaudeAuth)
	if auth.methods[0].button.State != connectionMethodAvailable {
		t.Fatalf("released CLI state = %v, want available", auth.methods[0].button.State)
	}
	view.setConnectionMethod(model.ProviderClaudeAuth, connectionMethodCLI)
	if got := view.config.ConnectionMethods[string(model.ProviderClaudeAuth)]; got != string(connectionMethodCLI) {
		t.Fatalf("second account method = %q, want cli", got)
	}
	root = connectionRowForTest(t, view, model.ProviderClaude)
	if root.methods[0].button.State != connectionMethodOccupied {
		t.Fatalf("root CLI state = %v after transfer, want occupied", root.methods[0].button.State)
	}
}

func TestClaudeAuthAndOtherMethodsMayOverlap(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	view.setConnectionMethod(model.ProviderClaude, connectionMethodOther)
	view.setConnectionMethod(model.ProviderClaudeAuth, connectionMethodOther)
	if view.config.ConnectionMethods["claude"] != "other" || view.config.ConnectionMethods["claude-auth"] != "other" {
		t.Fatalf("overlapping Other methods were rejected: %v", view.config.ConnectionMethods)
	}
	view.setConnectionMethod(model.ProviderClaude, connectionMethodAuth)
	view.setConnectionMethod(model.ProviderClaudeAuth, connectionMethodAuth)
	if view.config.ConnectionMethods["claude"] != "auth" || view.config.ConnectionMethods["claude-auth"] != "auth" {
		t.Fatalf("overlapping Auth methods were rejected: %v", view.config.ConnectionMethods)
	}
}

func TestAccountLabelEntriesPersistUserAliasesOnly(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	view.Show(SettingsScreen)
	root := connectionRowForTest(t, view, model.ProviderClaude)
	auth := connectionRowForTest(t, view, model.ProviderClaudeAuth)
	root.labelEntry.SetText("Work")
	auth.labelEntry.SetText("Personal")
	if view.config.AccountLabels["claude"] != "Work" || view.config.AccountLabels["claude-auth"] != "Personal" {
		t.Fatalf("account labels were not stored: %v", view.config.AccountLabels)
	}
}

func connectionRowForTest(t *testing.T, view *View, id model.ProviderID) *connectionView {
	t.Helper()
	for _, row := range view.connectionCache {
		if row.id == id {
			return row
		}
	}
	t.Fatalf("connection row %s not found", id)
	return nil
}

func TestStoredConnectionMethodIsHonoredWhenAvailable(t *testing.T) {
	config := settings.Default()
	config.ConnectionMethods = map[string]string{"claude": "other"}
	config = config.Validated()
	if got := selectedConnectionMethod(config, model.ProviderClaude); got != connectionMethodOther {
		t.Fatalf("stored method = %q, want other", got)
	}
}

// A method the provider does not offer must fall back rather than leave the
// row with no active route at all.
func TestUnavailableStoredMethodFallsBackToDefault(t *testing.T) {
	config := settings.Default()
	config.ConnectionMethods = map[string]string{"codex": "auth", "grok": "ide"}
	config = config.Validated()
	if got := selectedConnectionMethod(config, model.ProviderCodex); got != connectionMethodCLI {
		t.Fatalf("Codex fallback = %q, want cli", got)
	}
	if got := selectedConnectionMethod(config, model.ProviderGrok); got != connectionMethodCLI {
		t.Fatalf("Grok fallback = %q, want cli", got)
	}
}

func TestValidatedDropsUnknownProvidersAndMethods(t *testing.T) {
	config := settings.Default()
	config.ConnectionMethods = map[string]string{
		"claude": "cli",
		"bogus":  "cli",
		"codex":  "teleport",
	}
	config = config.Validated()
	if len(config.ConnectionMethods) != 1 || config.ConnectionMethods["claude"] != "cli" {
		t.Fatalf("validated methods = %v, want only claude:cli", config.ConnectionMethods)
	}
}

// Only the picked route reads as active; other working routes read as
// available so the row shows which one is actually in use.
func TestOnlySelectedMethodReadsAsActive(t *testing.T) {
	lane := LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected}
	if got := connectionMethodStateSelected(lane, connectionMethodCLI, true, connectionMethodCLI); got != connectionMethodActive {
		t.Fatalf("selected CLI = %v, want active", got)
	}
	if got := connectionMethodStateSelected(lane, connectionMethodCLI, true, connectionMethodOther); got != connectionMethodAvailable {
		t.Fatalf("unselected but working CLI = %v, want available", got)
	}
	// Selecting something does not invent readiness for a planned method.
	if got := connectionMethodStateSelected(lane, connectionMethodAuth, true, connectionMethodAuth); got != connectionMethodPlanned {
		t.Fatalf("planned auth = %v, want planned", got)
	}
}

func TestSelectingAnUnofferedMethodIsIgnored(t *testing.T) {
	view := &View{config: settings.Default()}
	view.state = ViewState{Lanes: []LaneState{{Provider: model.ProviderCodex, Status: model.StatusConnected}}}
	view.setConnectionMethod(model.ProviderCodex, connectionMethodAuth)
	if view.config.ConnectionMethods["codex"] != "" {
		t.Fatalf("unoffered method was stored: %v", view.config.ConnectionMethods)
	}
}
