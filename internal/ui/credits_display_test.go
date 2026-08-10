package ui

import (
	"testing"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestCreditsTextFormatsSpendBalanceAndUnlimited(t *testing.T) {
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	view := &View{Catalog: catalog, SystemLanguage: i18n.English, config: settings.Default()}

	tests := []struct {
		name    string
		credits *model.Credits
		want    string
	}{
		{
			name:    "spend",
			credits: &model.Credits{Spend: &model.CreditSpend{Used: 47.61, Limit: 100, Currency: "USD", Percent: 48}},
			want:    "Credits $47.61 / $100",
		},
		{
			name:    "balance",
			credits: &model.Credits{Balance: 2.39},
			want:    "Credits 2.39",
		},
		{
			name:    "unlimited",
			credits: &model.Credits{Unlimited: true, Spend: &model.CreditSpend{Used: 47.61, Limit: 100, Currency: "USD"}},
			want:    "Unlimited credits",
		},
		{
			name:    "reset credits appended",
			credits: &model.Credits{Balance: 2.39, ResetCredits: 3},
			want:    "Credits 2.39 · Reset credits: 3",
		},
		{
			name:    "zero reset credits hidden",
			credits: &model.Credits{Balance: 2.39, ResetCredits: 0},
			want:    "Credits 2.39",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := view.creditsText(test.credits); got != test.want {
				t.Fatalf("creditsText() = %q, want %q", got, test.want)
			}
		})
	}

	view.config.Language = settings.Language(i18n.German)
	eur := &model.Credits{Spend: &model.CreditSpend{Used: 47.61, Limit: 100, Currency: "EUR"}}
	if got := view.creditsText(eur); got != "Guthaben 47,61 EUR / 100 EUR" {
		t.Fatalf("non-USD creditsText() = %q", got)
	}
}

func TestCodexResetCreditsFollowExistingCreditsToggle(t *testing.T) {
	config := settings.Default()
	config.ShowCodexCredits = true
	view := &View{config: config}
	lane := LaneState{Provider: model.ProviderCodex, Credits: &model.Credits{ResetCredits: 3}}
	if !view.laneCreditsVisible(lane) {
		t.Fatal("Codex reset credits should be visible with ShowCodexCredits enabled")
	}

	view.config.ShowCodexCredits = false
	if view.laneCreditsVisible(lane) {
		t.Fatal("Codex reset credits should be hidden with ShowCodexCredits disabled")
	}
}
