package ui

import (
	"testing"

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
		"claude":  "cli",
		"bogus":   "cli",
		"codex":   "teleport",
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
