package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

const (
	trayTooltipAppName       = "QuotaDock"
	trayTooltipMaxUTF16Units = 127
)

// BuildTrayTooltip returns the Windows tray hover summary for the current UI
// state. It deliberately accepts only view state and non-secret display
// settings, so credentials, account identifiers, and provider source details
// cannot enter the tooltip.
func BuildTrayTooltip(state ViewState, config settings.Config, systemLanguage i18n.Language, now time.Time) string {
	config = config.Validated()
	lines := make([]string, 0, len(state.Lanes))
	for _, lane := range state.Lanes {
		if lane.Status != model.StatusConnected {
			continue
		}
		row, ok := highestVisibleUsageRow(lane, config)
		if !ok {
			continue
		}
		until, _ := resetStrings(row, now, config, systemLanguage)
		countdown := strings.ReplaceAll(until, " ", "")
		if row.UsageUnknown {
			// No consumption figure to quote, so the line carries the reset
			// countdown alone rather than an invented percentage.
			lines = append(lines, fmt.Sprintf("%s · %s", lane.Name, countdown))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s%% · %s", lane.Name, formatUsagePercent(row.Percent), countdown))
	}
	return combineTrayTooltip(trayTooltipAppName, lines)
}

func highestVisibleUsageRow(lane LaneState, config settings.Config) (UsageRowState, bool) {
	switch lane.Provider {
	case model.ProviderClaude:
		if !config.ShowClaude {
			return UsageRowState{}, false
		}
	case model.ProviderCodex:
		if !config.ShowCodex {
			return UsageRowState{}, false
		}
	case model.ProviderAntigravity:
		if !config.ShowAGGemini && !config.ShowAGClaude {
			return UsageRowState{}, false
		}
	case model.ProviderGrok:
		if !config.ShowGrok {
			return UsageRowState{}, false
		}
	default:
		return UsageRowState{}, false
	}

	var selected UsageRowState
	found := false
	for _, row := range lane.Rows {
		if lane.Provider == model.ProviderAntigravity {
			gemini := antigravityRowIsGemini(row)
			if gemini && !config.ShowAGGemini || !gemini && !config.ShowAGClaude {
				continue
			}
		}
		if !found || row.Percent > selected.Percent {
			selected = row
			found = true
		}
	}
	return selected, found
}

// combineTrayTooltip is kept pure so the Windows UTF-16 boundary can be
// verified without creating a desktop application or registering a tray icon.
// Whole trailing lines are omitted instead of allowing Windows to truncate a
// line in the middle.
func combineTrayTooltip(appName string, lines []string) string {
	tooltip := ""
	for _, line := range lines {
		if line == "" {
			continue
		}
		candidate := line
		if tooltip != "" {
			candidate = tooltip + "\n" + line
		}
		if utf16CodeUnits(candidate) > trayTooltipMaxUTF16Units {
			break
		}
		tooltip = candidate
	}
	if tooltip != "" {
		return tooltip
	}
	return truncateUTF16(appName, trayTooltipMaxUTF16Units)
}

func utf16CodeUnits(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func truncateUTF16(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	var output strings.Builder
	used := 0
	for _, valueRune := range value {
		units := 1
		if valueRune > 0xffff {
			units = 2
		}
		if used+units > maximum {
			break
		}
		output.WriteRune(valueRune)
		used += units
	}
	return output.String()
}

func formatUsagePercent(percent float64) string {
	return fmt.Sprintf("%.0f", percent)
}
