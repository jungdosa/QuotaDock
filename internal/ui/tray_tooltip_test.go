package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestBuildTrayTooltipThreeProviders(t *testing.T) {
	state := ViewState{Lanes: []LaneState{
		trayTooltipTestLane(model.ProviderClaude, "Claude",
			trayTooltipTestRow(17, "4h 10m"),
			trayTooltipTestRow(42, "2h 40m"),
		),
		trayTooltipTestLane(model.ProviderCodex, "Codex",
			trayTooltipTestRow(31, "1h 08m"),
		),
		trayTooltipTestLane(model.ProviderAntigravity, "Antigravity",
			trayTooltipTestRow(8, "4h 02m"),
			trayTooltipTestRow(24, "5d 2h"),
		),
	}}

	got := BuildTrayTooltip(state, settings.Default(), i18n.English, time.Time{})
	want := "Claude 42% · 2h40m\nCodex 31% · 1h08m\nAntigravity 24% · 5d2h"
	if got != want {
		t.Fatalf("BuildTrayTooltip()=%q, want %q", got, want)
	}
}

func TestBuildTrayTooltipDropsTrailingLinesOverUTF16Limit(t *testing.T) {
	state := ViewState{Lanes: []LaneState{
		trayTooltipTestLane(model.ProviderClaude, strings.Repeat("長", 110),
			trayTooltipTestRow(42, "2h 40m"),
		),
		trayTooltipTestLane(model.ProviderCodex, "Codex",
			trayTooltipTestRow(31, "1h 08m"),
		),
		trayTooltipTestLane(model.ProviderAntigravity, "Antigravity",
			trayTooltipTestRow(24, "5d 2h"),
		),
	}}

	got := BuildTrayTooltip(state, settings.Default(), i18n.Japanese, time.Time{})
	if units := utf16CodeUnits(got); units > trayTooltipMaxUTF16Units {
		t.Fatalf("tooltip UTF-16 units=%d, want <=%d", units, trayTooltipMaxUTF16Units)
	}
	if strings.Contains(got, "Codex") || strings.Contains(got, "Antigravity") || strings.Count(got, "\n") != 0 {
		t.Fatalf("overflow retained a trailing provider line: %q", got)
	}
}

func TestBuildTrayTooltipOmitsDisconnectedProvidersAndFallsBackToAppName(t *testing.T) {
	state := ViewState{Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Status: model.StatusUnavailable, Source: "private@example.com"},
		trayTooltipTestLane(model.ProviderCodex, "Codex",
			trayTooltipTestRow(31, "1h 08m"),
		),
		{Provider: model.ProviderAntigravity, Name: "Antigravity", Status: model.StatusError},
	}}
	state.Lanes[1].Source = "private@example.com"
	if got, want := BuildTrayTooltip(state, settings.Default(), i18n.English, time.Time{}), "Codex 31% · 1h08m"; got != want {
		t.Fatalf("disconnected provider filtering=%q, want %q", got, want)
	}

	state.Lanes[1].Status = model.StatusUnavailable
	if got := BuildTrayTooltip(state, settings.Default(), i18n.English, time.Time{}); got != trayTooltipAppName {
		t.Fatalf("all-disconnected tooltip=%q, want %q", got, trayTooltipAppName)
	}
}

func TestTrayTooltipUTF16CodeUnitMeasurement(t *testing.T) {
	if got, want := utf16CodeUnits("사용😀량"), 5; got != want {
		t.Fatalf("UTF-16 units=%d, want %d for CJK plus surrogate pair", got, want)
	}
	value := strings.Repeat("界", 126) + "😀"
	truncated := truncateUTF16(value, trayTooltipMaxUTF16Units)
	if got := utf16CodeUnits(truncated); got != 126 {
		t.Fatalf("surrogate-safe truncated units=%d, want 126", got)
	}
	fallback := combineTrayTooltip(strings.Repeat("界", 128), nil)
	if got := utf16CodeUnits(fallback); got != trayTooltipMaxUTF16Units {
		t.Fatalf("CJK fallback units=%d, want %d", got, trayTooltipMaxUTF16Units)
	}
}

func trayTooltipTestLane(id model.ProviderID, name string, rows ...UsageRowState) LaneState {
	return LaneState{Provider: id, Name: name, Status: model.StatusConnected, Rows: rows}
}

func trayTooltipTestRow(percent float64, remaining string) UsageRowState {
	return UsageRowState{Percent: percent, DisplayOverride: true, DisplayRemaining: remaining}
}
