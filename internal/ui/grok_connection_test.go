package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// An account that never bought credits reports a zero balance with the
// feature off; showing "Credits 0" would be noise rather than data.
func TestCreditsSurfaceHidesWhenNothingWasPurchased(t *testing.T) {
	for _, test := range []struct {
		name    string
		credits *model.Credits
		want    bool
	}{
		{"never purchased", &model.Credits{Balance: 0, HasCredits: false}, false},
		{"purchased balance", &model.Credits{Balance: 250, HasCredits: true}, true},
		{"spent down but enabled", &model.Credits{Balance: 0, HasCredits: true}, true},
		{"older CLI without the flag", &model.Credits{Balance: 12.5}, true},
		{"reset credits only", &model.Credits{ResetCredits: 3}, true},
		{"unlimited", &model.Credits{Unlimited: true}, true},
		{"claude extra usage", &model.Credits{Spend: &model.CreditSpend{Used: 0, Limit: 200, Currency: "USD"}}, true},
		{"no credits reported", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := settings.Default()
			config.ShowCodexCredits, config.ShowClaudeCredits = true, true
			view := &View{config: config}
			lane := LaneState{Provider: model.ProviderCodex, Credits: test.credits}
			if got := view.laneCreditsVisible(lane); got != test.want {
				t.Fatalf("laneCreditsVisible(%+v) = %t, want %t", test.credits, got, test.want)
			}
		})
	}
}

func TestGrokConnectionRowOffersTheCLIMethod(t *testing.T) {
	if methods := connectionMethodsFor(model.ProviderGrok); len(methods) != 1 || methods[0] != connectionMethodCLI {
		t.Fatalf("Grok connection methods = %v, want [cli]", methods)
	}
	title, body, retry := connectionHelpKeys(model.ProviderGrok)
	if title != i18n.KeyHelpGrokTitle || body != i18n.KeyHelpGrok || retry != i18n.KeyHelpGrokRetry {
		t.Fatalf("Grok help keys = %s/%s/%s", title, body, retry)
	}
	// A disconnected Grok must not read as an active CLI connection.
	disconnected := LaneState{Provider: model.ProviderGrok, Status: model.StatusLoggedOut}
	if state := connectionMethodStateFor(disconnected, connectionMethodCLI, false); state == connectionMethodActive {
		t.Fatal("disconnected Grok reported an active CLI method")
	}
}

// Grok reports no consumption yet, so its tooltip line carries the countdown
// alone instead of an invented percentage.
func TestTrayTooltipOmitsPercentForUnknownUsage(t *testing.T) {
	config := settings.Default()
	config.ShowGrok = true
	config.ShowClaude, config.ShowCodex, config.ShowAGGemini, config.ShowAGClaude = false, false, false, false
	now := time.Now()
	state := ViewState{Lanes: []LaneState{{
		Provider: model.ProviderGrok, Name: "Grok", Status: model.StatusConnected,
		Rows: []UsageRowState{{Label: "Weekly", UsageUnknown: true, WindowMinutes: 10080, ResetsAt: now.Add(48 * time.Hour)}},
	}}}
	tooltip := BuildTrayTooltip(state, config, i18n.English, now)
	if !strings.Contains(tooltip, "Grok") || strings.Contains(tooltip, "%") {
		t.Fatalf("tooltip = %q, want a Grok line with no percentage", tooltip)
	}
	config.ShowGrok = false
	if hidden := BuildTrayTooltip(state, config, i18n.English, now); strings.Contains(hidden, "Grok") {
		t.Fatalf("tooltip = %q, want no Grok line when the toggle is off", hidden)
	}
}
