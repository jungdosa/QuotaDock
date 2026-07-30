package ui

import (
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"time"
)

// DemoConfig makes visual review deterministic regardless of saved settings.
func DemoConfig(config settings.Config) settings.Config {
	config.Language = settings.LanguageKorean
	config.Theme = settings.ThemeDark
	config.UsageMode = settings.UsageUsed
	config.WarningsEnabled = true
	config.WarningPercent = 75
	config.DangerPercent = 90
	config.WarningColor = "amber"
	config.DangerColor = "red"
	config.ProviderColors = map[string]string{
		"claude":             "orange",
		"codex":              "gray",
		"antigravity":        "slate",
		"antigravity-gemini": "violet",
	}
	config.ShowClaude = true
	config.ShowCodex = true
	config.ShowAGGemini = true
	config.ShowAGClaude = true
	config.DisplayMode = settings.ModeNormal
	return config.Validated()
}

// DemoViewState returns the fixed PLAN §25.5 visual-review fixture.
// It is UI-only and deliberately does not construct or call any provider.
func DemoViewState() ViewState {
	return ViewState{
		LastRefresh: time.Date(2026, time.July, 24, 12, 33, 0, 0, time.Local),
		Lanes: []LaneState{
			{
				Provider: model.ProviderClaude,
				Name:     "Claude",
				Plan:     model.Plan("MAX 20X"),
				Status:   model.StatusConnected,
				Rows: []UsageRowState{
					demoRow("세션", 42, 300, "2h 47m", "초기화 7.24 15:20", 55.67),
					demoRow("주간", 68, 10080, "3d 6h", "초기화 7.28 09:00", 46.55),
					demoRow("Fable", 12, 10080, "3d 6h", "초기화 7.28 09:00", 46.55),
				},
			},
			{
				Provider: model.ProviderCodex,
				Name:     "Codex",
				Plan:     model.Plan("PLUS"),
				Status:   model.StatusConnected,
				Credits:  &model.Credits{Balance: 2.39},
				Rows: []UsageRowState{
					demoRow("세션", 31, 300, "1h 08m", "초기화 7.24 13:41", 22.67),
					demoRow("주간", 55, 10080, "4d 19h", "초기화 7.29 08:00", 68.78),
				},
			},
			{
				Provider: model.ProviderAntigravity,
				Name:     "Antigravity",
				Plan:     model.Plan("AI ULTRA"),
				Status:   model.StatusConnected,
				Rows: []UsageRowState{
					demoRow("Gemini Models", 8, 300, "4h 02m", "초기화 7.24 16:35", 80.67),
					demoRow("Gemini Models", 24, 10080, "5d 2h", "초기화 7.29 15:00", 72.80),
					demoRow("Claude/GPT Models", 91, 300, "0h 38m", "초기화 7.24 13:11", 12.67),
					demoRow("Claude/GPT Models", 77, 10080, "5d 2h", "초기화 7.29 15:00", 72.80),
				},
			},
		},
	}
}

func demoRow(label string, percent float64, windowMinutes int, remaining, reset string, remainingPercent float64) UsageRowState {
	return UsageRowState{
		Label:                   label,
		Percent:                 percent,
		WindowMinutes:           windowMinutes,
		DisplayOverride:         true,
		DisplayRemaining:        remaining,
		DisplayReset:            reset,
		DisplayRemainingPercent: remainingPercent,
	}
}
