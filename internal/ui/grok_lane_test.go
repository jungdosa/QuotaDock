package ui

import (
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func grokWeeklyRow() UsageRowState {
	return UsageRowState{Label: "Weekly", UsageUnknown: true, WindowMinutes: 10080, ResetsAt: time.Now().Add(48 * time.Hour)}
}

func TestGrokLaneVisibilityFollowsToggle(t *testing.T) {
	config := settings.Default()
	config.ShowGrok = false
	view := &View{config: config}
	view.state = ViewState{Lanes: []LaneState{{Provider: model.ProviderGrok, Name: "Grok", Status: model.StatusConnected, Rows: []UsageRowState{grokWeeklyRow()}}}}
	if lanes := view.visibleLanes(); len(lanes) != 0 {
		t.Fatalf("Grok lane visible with the toggle off: %d lanes", len(lanes))
	}
	view.config.ShowGrok = true
	lanes := view.visibleLanes()
	if len(lanes) != 1 || lanes[0].Provider != model.ProviderGrok {
		t.Fatalf("Grok lane missing with the toggle on: %+v", lanes)
	}
}

func TestUnknownUsageRendersDashWithoutUnit(t *testing.T) {
	if number, symbol := usagePercentTexts(rowVisual{percent: 49}); number != "49" || symbol != "%" {
		t.Fatalf("known usage renders %q %q", number, symbol)
	}
	if number, symbol := usagePercentTexts(rowVisual{usageUnknown: true}); number != "–" || symbol != "" {
		t.Fatalf("unknown usage renders %q %q", number, symbol)
	}
}

// The remaining-mode flip turns 0% used into 100% remaining; an unknown row
// must not claim either number.
func TestUnknownUsageKeepsMeterEmptyInRemainingMode(t *testing.T) {
	config := settings.Default()
	config.ShowGrok = true
	config.UsageMode = settings.UsageRemaining
	view := &View{config: config, colors: LightBrandColors, SystemLanguage: i18n.English}
	visual := view.rowVisual(LaneState{Provider: model.ProviderGrok, Name: "Grok"}, grokWeeklyRow(), time.Now())
	if !visual.usageUnknown || visual.percent != 0 {
		t.Fatalf("unknown row visual = unknown=%t percent=%.1f, want empty meter", visual.usageUnknown, visual.percent)
	}
}

func TestGrokNanoCellAppearsWithToggle(t *testing.T) {
	config := settings.Default()
	config.ShowClaude, config.ShowCodex, config.ShowAGGemini, config.ShowAGClaude = false, false, false, false
	config.ShowGrok = true
	view := &View{config: config}
	view.state = ViewState{Lanes: []LaneState{{Provider: model.ProviderGrok, Name: "Grok", Status: model.StatusConnected, Rows: []UsageRowState{grokWeeklyRow()}}}}
	cells := view.nanoCellStates()
	if len(cells) != 1 || cells[0].key != "grok" || cells[0].kind != ProviderIconGrok {
		t.Fatalf("nano cells = %+v, want a single grok cell", cells)
	}
	if len(cells[0].rows) != 1 || !cells[0].rows[0].available || cells[0].rows[0].label != "7D" {
		t.Fatalf("grok nano rows = %+v, want one available 7D row", cells[0].rows)
	}
	if value, _ := view.nanoUsageValue(cells[0].rows[0], LightBrandColors.PaletteColor("sky")); value != 0 {
		t.Fatalf("grok nano usage bar = %.1f, want 0 (usage unknown)", value)
	}
}
