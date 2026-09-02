package settings

import (
	"slices"
	"strings"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/security"
)

// A file from before the account count carries only the two-account flag,
// and has to open with exactly the accounts it showed before.
func TestAccountCountIsReadFromTheOlderFlag(t *testing.T) {
	for _, test := range []struct {
		file string
		want int
	}{
		{`{"schemaVersion":6,"showClaudeAuth":true}`, 2},
		{`{"schemaVersion":6,"showClaudeAuth":false}`, 1},
		{`{"schemaVersion":6}`, 1},
	} {
		config, err := Decode(strings.NewReader(test.file))
		if err != nil {
			t.Fatal(err)
		}
		if config.ClaudeAccounts != test.want {
			t.Fatalf("%s: accounts = %d, want %d", test.file, config.ClaudeAccounts, test.want)
		}
		if config.ShowClaudeAuth != (test.want >= 2) {
			t.Fatalf("%s: showClaudeAuth = %v, want %v", test.file, config.ShowClaudeAuth, test.want >= 2)
		}
	}
}

// The count wins over the flag when both are present, and the flag is written
// back from it, so a build that only knows two accounts still sees a second.
func TestAccountCountRewritesTheOlderFlag(t *testing.T) {
	three := Config{ClaudeAccounts: 3, ShowClaudeAuth: false}.Validated()
	if three.ClaudeAccounts != 3 || !three.ShowClaudeAuth {
		t.Fatalf("three accounts validated to %d / flag %v", three.ClaudeAccounts, three.ShowClaudeAuth)
	}
	// The flag alone can raise the count to two: that is the path the button
	// that adds a second account still takes.
	raised := Config{ClaudeAccounts: 1, ShowClaudeAuth: true}.Validated()
	if raised.ClaudeAccounts != 2 || !raised.ShowClaudeAuth {
		t.Fatalf("a raised flag validated to %d / flag %v", raised.ClaudeAccounts, raised.ShowClaudeAuth)
	}
	// But the flag never lowers it: a count of two stands even with the flag
	// off, so setting the count alone means what it says. Lowering is done by
	// the controls, which set the count.
	kept := Config{ClaudeAccounts: 2, ShowClaudeAuth: false}.Validated()
	if kept.ClaudeAccounts != 2 || !kept.ShowClaudeAuth {
		t.Fatalf("a count of two with the flag off validated to %d / flag %v", kept.ClaudeAccounts, kept.ShowClaudeAuth)
	}
}

func TestAccountCountIsClampedToTheMaximum(t *testing.T) {
	if got := (Config{ClaudeAccounts: 9}).Validated().ClaudeAccounts; got != MaxClaudeAccounts {
		t.Fatalf("nine accounts validated to %d, want %d", got, MaxClaudeAccounts)
	}
	if got := (Config{ClaudeAccounts: -2}).Validated().ClaudeAccounts; got != 1 {
		t.Fatalf("a negative count validated to %d, want 1", got)
	}
}

// A user who arranged two accounts and then adds a third has to find it next
// to the other Claude accounts, not at the bottom of the list below Grok.
func TestANewClaudeAccountJoinsTheOthersInTheOrder(t *testing.T) {
	order := NormalizeLaneOrder([]string{"codex", "claude-auth", "claude", "grok", "antigravity"})
	want := []string{"codex", "claude-auth", "claude", "claude-3", "claude-4", "claude-5", "grok", "antigravity"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	// With no Claude account named at all, the accounts still arrive together.
	order = NormalizeLaneOrder([]string{"grok", "codex"})
	if index := slices.Index(order, "claude"); index < 0 || order[index+1] != "claude-auth" || order[index+2] != "claude-3" {
		t.Fatalf("accounts did not arrive together: %v", order)
	}
}

// Every account has a default colour, each a real palette entry, and none of
// them borrows another provider's hue or a severity colour.
func TestEveryClaudeAccountHasItsOwnDefaultColour(t *testing.T) {
	colors := Default().ProviderColors
	taken := map[string]string{
		colors["codex"]: "codex", colors["antigravity-gemini"]: "antigravity-gemini", colors["grok"]: "grok",
		Default().WarningColor: "warning", Default().DangerColor: "danger",
	}
	for _, id := range ClaudeAccountIDs() {
		hue, ok := colors[id]
		if !ok || !security.IsPaletteID(hue) {
			t.Fatalf("%s has no valid default colour: %q", id, hue)
		}
		if owner, clash := taken[hue]; clash && id != "claude-auth" {
			t.Fatalf("%s defaults to %s, which %s already uses", id, hue, owner)
		}
	}
	if len(ClaudeAccountIDs()) != MaxClaudeAccounts {
		t.Fatalf("%d account ids for a maximum of %d", len(ClaudeAccountIDs()), MaxClaudeAccounts)
	}
}

// Labels and sign-in routes have to be accepted for every account, or a
// third account could never be named or connected.
func TestEveryClaudeAccountAcceptsALabelAndARoute(t *testing.T) {
	config := Config{
		ClaudeAccounts:    5,
		AccountLabels:     map[string]string{"claude-3": "Work", "claude-5": "Lab"},
		ConnectionMethods: map[string]string{"claude-4": ConnectionMethodAuth, "claude-5": ConnectionMethodOther},
	}.Validated()
	if config.AccountLabels["claude-3"] != "Work" || config.AccountLabels["claude-5"] != "Lab" {
		t.Fatalf("labels were dropped: %v", config.AccountLabels)
	}
	if config.ConnectionMethods["claude-4"] != ConnectionMethodAuth || config.ConnectionMethods["claude-5"] != ConnectionMethodOther {
		t.Fatalf("routes were dropped: %v", config.ConnectionMethods)
	}
}
