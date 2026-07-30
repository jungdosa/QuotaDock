package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

func TestPhase3ZSettingsTitleHierarchyAndSectionTypography(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	bar := v.settingsTitleBar(NewSmallIconButton(theme.NavigateBackIcon(), "Back", nil, v.colors), defaultAppVersion, nil)
	bar.Resize(fyne.NewSize(SettingsWidth, TitleBarHeight))

	var titleRow *fyne.Container
	walkCanvasObject(bar, func(object fyne.CanvasObject) {
		candidate, ok := object.(*fyne.Container)
		if ok && len(candidate.Objects) == 7 {
			if _, ok = candidate.Layout.(*GapColumnLayout); ok {
				titleRow = candidate
			}
		}
	})
	if titleRow == nil {
		t.Fatal("settings title row with grouped actions is missing")
	}
	layout := titleRow.Layout.(*GapColumnLayout)
	wantGaps := []float32{6, 6, 6, 8, 8, 6}
	for index, want := range wantGaps {
		if layout.Gaps[index] != want {
			t.Fatalf("title gap %d=%.1f, want %.1f", index, layout.Gaps[index], want)
		}
	}
	if layout.Widths[4] != 1 || titleRow.Objects[4].MinSize().Height != 16 {
		t.Fatalf("title separator width/height=%.1f/%.1f, want 1/16", layout.Widths[4], titleRow.Objects[4].MinSize().Height)
	}
	themeButton := titleRow.Objects[2].(*SmallButton)
	help := titleRow.Objects[3].(*SmallButton)
	update := titleRow.Objects[5].(*SmallButton)
	done := titleRow.Objects[6].(*SmallButton)
	if themeButton.Outlined || help.Outlined || update.Disabled || !update.Outlined || !done.Primary {
		t.Fatalf("title hierarchy theme/help/update/done=%+v/%+v/%+v/%+v", themeButton, help, update, done)
	}
	updateRenderer := update.CreateRenderer().(*smallButtonRenderer)
	doneRenderer := done.CreateRenderer().(*smallButtonRenderer)
	if updateRenderer.bg.StrokeWidth != 1 || doneRenderer.bg.StrokeWidth != 1 ||
		!sameColor(doneRenderer.bg.FillColor, v.colors.Accent) || !sameColor(doneRenderer.label.Color, v.colors.IconText) {
		t.Fatal("settings update/done painted hierarchy is incorrect")
	}

	section := v.section("General behavior", canvas.NewRectangle(v.colors.Card))
	var heading *canvas.Text
	walkCanvasObject(section, func(object fyne.CanvasObject) {
		if text, ok := object.(*canvas.Text); ok && text.Text == "GENERAL BEHAVIOR" {
			heading = text
		}
	})
	if trackedUpper("일반 동작") != "일반 동작" || heading == nil || heading.TextSize != 11 || !heading.TextStyle.Bold {
		t.Fatalf("section typography Korean/English=%q/%v", trackedUpper("일반 동작"), heading)
	}
}

func TestPhase3ZThresholdDisplayAndConnectionGeometry(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	v.Show(SettingsScreen)
	window.Resize(v.MinimumSize(SettingsScreen))

	usage := v.usageSettings().(*fyne.Container)
	thresholdPair := usage.Objects[3].(*fyne.Container)
	thresholdStack := thresholdPair.Objects[1].(*fyne.Container)
	stackLayout, ok := thresholdStack.Layout.(*CompactRowsLayout)
	if !ok || stackLayout.Gap != 0 || len(thresholdStack.Objects) != 2 {
		t.Fatalf("threshold stack layout=%T gap/rows=%v/%d", thresholdStack.Layout, stackLayout, len(thresholdStack.Objects))
	}
	for index, object := range thresholdStack.Objects {
		row := object.(*fyne.Container)
		rowLayout := row.Layout.(*GapColumnLayout)
		wantWidths := []float32{48, 76, 34, 10, 22}
		wantGaps := []float32{6, 8, 2, 8}
		for column, want := range wantWidths {
			if rowLayout.Widths[column] != want {
				t.Fatalf("threshold %d width %d=%.1f, want %.1f", index, column, rowLayout.Widths[column], want)
			}
		}
		for gap, want := range wantGaps {
			if rowLayout.Gaps[gap] != want {
				t.Fatalf("threshold %d gap %d=%.1f, want %.1f", index, gap, rowLayout.Gaps[gap], want)
			}
		}
	}

	display := v.displaySettings().(*fyne.Container)
	if len(display.Objects) != 1 {
		t.Fatalf("display rows=%d, want one two-column row", len(display.Objects))
	}
	displayPair := display.Objects[0].(*fyne.Container)
	if len(displayPair.Objects) != 2 {
		t.Fatalf("display columns=%d, want language/date-time", len(displayPair.Objects))
	}
	for index, object := range displayPair.Objects {
		if _, ok := object.(*fyne.Container).Layout.(*SettingRowLayout); !ok {
			t.Fatalf("display column %d layout=%T", index, object.(*fyne.Container).Layout)
		}
	}

	var actionX float32
	for index, handles := range v.connectionCache {
		rowLayout, ok := handles.actionRow.Layout.(*GapColumnLayout)
		if !ok || len(rowLayout.Gaps) != 2 || rowLayout.Gaps[0] != 6 || rowLayout.Gaps[1] != 6 || rowLayout.Height != SmallButtonHeight {
			t.Fatalf("connection %d action layout=%+v", index, rowLayout)
		}
		position, found := objectPosition(v.Settings, handles.actionRow, fyne.NewPos(0, 0))
		if !found {
			t.Fatalf("connection %d action row position not found", index)
		}
		if index == 0 {
			actionX = position.X
		} else if position.X != actionX {
			t.Fatalf("connection action x row %d=%.1f, want %.1f", index, position.X, actionX)
		}
		if !handles.testButton.Outlined || !handles.reconnect.Outlined || !handles.helpButton.Outlined {
			t.Fatalf("connection %d secondary button styles differ", index)
		}
	}

	if CompactIconWidth != 16 || CompactLabelTextSize != 12 || CompactPercentTextSize != 12 ||
		CompactHundredTextSize != 11 || CompactSymbolTextSize != 9 {
		t.Fatalf("compact retained sizes icon/label/number/100/symbol=%.0f/%.0f/%.0f/%.0f/%.0f",
			CompactIconWidth, CompactLabelTextSize, CompactPercentTextSize, CompactHundredTextSize, CompactSymbolTextSize)
	}
}
