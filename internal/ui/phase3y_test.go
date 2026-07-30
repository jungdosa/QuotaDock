package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase3YNormalThreeColumnGeometryAndPercentBand(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))

	if NormalWidth != 539 {
		t.Fatalf("normal width=%.0f, want 539", NormalWidth)
	}
	handles := v.normalCache.rows[0]
	row := handles.row
	if row == nil || len(row.Objects) != 3 {
		t.Fatalf("normal row=%v columns=%d, want label/meter/reset", row != nil, len(row.Objects))
	}
	columns := row.Layout.(*ColumnLayout).Widths
	wantMeterWidth := row.Size().Width - columns[0] - columns[2] - 2*NormalRowGap
	if handles.meterStack.Position().X != columns[0]+NormalRowGap || handles.meterStack.Size().Width != wantMeterWidth {
		t.Fatalf("meter stack x/width=%.1f/%.1f, want %.1f/%.1f", handles.meterStack.Position().X, handles.meterStack.Size().Width, columns[0]+NormalRowGap, wantMeterWidth)
	}
	wantResetX := columns[0] + NormalRowGap + wantMeterWidth + NormalRowGap
	if row.Objects[2].Position().X != wantResetX || row.Objects[2].Size().Width != 140 {
		t.Fatalf("reset x/width=%.1f/%.1f, want %.1f/140", row.Objects[2].Position().X, row.Objects[2].Size().Width, wantResetX)
	}
	// The meter/reset-bar bundle (W2) is centred in the region below the
	// percent band; the reset bar spans exactly the meter's x range.
	bundle := NormalMeterHeight + NormalResetBarGap + NormalResetBarHeight
	wantMeterY := NormalPercentBandHeight + (handles.meterStack.Size().Height-NormalPercentBandHeight-bundle)/2
	if handles.meter.Position().Y != wantMeterY || handles.meter.Size().Width != handles.meterStack.Size().Width {
		t.Fatalf("meter y/width=%.1f/%.1f, want %.1f/%.1f", handles.meter.Position().Y, handles.meter.Size().Width, wantMeterY, handles.meterStack.Size().Width)
	}
	if handles.resetBar == nil ||
		handles.resetBar.Position().X != handles.meter.Position().X ||
		handles.resetBar.Size().Width != handles.meter.Size().Width ||
		handles.resetBar.Position().Y != wantMeterY+NormalMeterHeight+NormalResetBarGap ||
		handles.resetBar.Height != NormalResetBarHeight {
		t.Fatalf("reset bar geometry pos=%v size=%v height=%.1f, want aligned under meter at %.1f",
			handles.resetBar.Position(), handles.resetBar.Size(), handles.resetBar.Height, wantMeterY+NormalMeterHeight+NormalResetBarGap)
	}
	percentPos, ok := objectPosition(handles.meterStack, handles.percent, fyne.NewPos(0, 0))
	if !ok {
		t.Fatal("percent number is not inside meter stack")
	}
	symbolPos, ok := objectPosition(handles.meterStack, handles.percentSymbol, fyne.NewPos(0, 0))
	if !ok {
		t.Fatal("percent symbol is not inside meter stack")
	}
	percentRight := symbolPos.X + handles.percentSymbol.Size().Width
	if percentRight != handles.meterStack.Size().Width-NormalPercentInset || percentPos.X >= symbolPos.X {
		t.Fatalf("percent right/ordering=%.1f/%t, want %.1f/true", percentRight, percentPos.X < symbolPos.X, handles.meterStack.Size().Width-NormalPercentInset)
	}
	if handles.percent.TextSize != NormalPercentTextSize || handles.percentSymbol.TextSize != NormalPercentTextSize-1 {
		t.Fatalf("percent font sizes=%.1f/%.1f, want %.1f/%.1f", handles.percent.TextSize, handles.percentSymbol.TextSize, NormalPercentTextSize, NormalPercentTextSize-1)
	}
	if v.normalCache.usageHeader.Position().X != handles.meterStack.Position().X ||
		v.normalCache.usageHeader.Size().Width != handles.meterStack.Size().Width ||
		v.normalCache.resetHeader.Position().X != row.Objects[2].Position().X ||
		v.normalCache.resetHeader.Size().Width != row.Objects[2].Size().Width {
		t.Fatal("normal Remaining/Reset headers do not share the row column geometry")
	}
	rings := 0
	walkCanvasObject(v.normalBody, func(object fyne.CanvasObject) {
		if _, ok := object.(*RasterRing); ok {
			rings++
		}
	})
	if rings != 0 {
		t.Fatalf("normal body still contains %d reset rings", rings)
	}
	t.Logf("normal width=%.0f content=%.0f label/meter/reset=%.0f/%.0f/%.0f resetX=%.0f percentRight=%.0f",
		NormalWidth, row.Size().Width, columns[0], wantMeterWidth, columns[2], wantResetX, percentRight)
}

func TestPhase3YTitleTooltipsDelayI18NStateAndLifecycle(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	state := sampleState()
	state.LastRefresh = time.Date(2026, 7, 26, 9, 55, 14, 0, time.Local)
	v.SetState(state)
	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))

	if TooltipDelay < 300*time.Millisecond || TooltipDelay > 500*time.Millisecond {
		t.Fatalf("tooltip delay=%s, want 300-500ms", TooltipDelay)
	}
	wantTooltips := []string{
		"Switch to Compact",
		"Refresh (last checked Jul 26 9:55 AM)",
		"Theme: Dark",
		"Settings",
		"Minimize",
		"Hide to tray",
	}
	buttons := map[string]*SmallButton{}
	walkCanvasObject(v.Normal, func(object fyne.CanvasObject) {
		if button, ok := object.(*SmallButton); ok {
			buttons[button.Tooltip] = button
		}
	})
	for _, tooltip := range wantTooltips {
		if buttons[tooltip] == nil {
			t.Fatalf("normal title tooltip %q is missing", tooltip)
		}
	}

	button := buttons["Hide to tray"]
	button.MouseIn(&desktop.MouseEvent{})
	if v.tooltipTimer == nil || v.tooltipObject != nil {
		t.Fatal("hover did not schedule a delayed tooltip")
	}
	v.showTooltip(button)
	if v.tooltipObject == nil || len(v.tooltipLayer.Objects) != 1 {
		t.Fatal("scheduled tooltip did not become visible in the passive layer")
	}
	position := v.tooltipObject.Position()
	size := v.tooltipObject.Size()
	if position.X < TooltipMargin || position.Y < TooltipMargin || position.X+size.Width > v.Canvas.Size().Width-TooltipMargin {
		t.Fatalf("tooltip escaped canvas bounds: pos=%v size=%v canvas=%v", position, size, v.Canvas.Size())
	}
	var background *canvas.Rectangle
	var label *canvas.Text
	walkCanvasObject(v.tooltipObject, func(object fyne.CanvasObject) {
		switch value := object.(type) {
		case *canvas.Rectangle:
			if value.CornerRadius == 4 && value.StrokeWidth == 1 {
				background = value
			}
		case *canvas.Text:
			label = value
		}
		// The layer must stay pointer-transparent: no object in a visible
		// tooltip may accept taps, hovers, or drags, or it would swallow the
		// first click on the control underneath (W1 regression guard).
		switch object.(type) {
		case fyne.Tappable, fyne.SecondaryTappable, fyne.Draggable, desktop.Hoverable, fyne.Focusable:
			t.Fatalf("tooltip layer contains an event-consuming object: %T", object)
		}
	})
	if background == nil || label == nil || label.TextSize != TooltipTextSize || label.Text != "Hide to tray" {
		t.Fatalf("tooltip style/content background=%v label=%v", background != nil, label)
	}
	// A click on the owning button hides the tooltip immediately; the next
	// hover shows the (possibly updated) text again.
	button.Tapped(nil)
	if v.tooltipObject != nil || len(v.tooltipLayer.Objects) != 0 {
		t.Fatal("click did not hide the visible tooltip")
	}
	button.MouseOut()
	if v.tooltipObject != nil || v.tooltipTimer != nil {
		t.Fatal("MouseOut retained tooltip state")
	}

	refresh := buttons["Refresh (last checked Jul 26 9:55 AM)"]
	next := state
	next.LastRefresh = time.Date(2026, 7, 26, 10, 1, 2, 0, time.Local)
	v.SetState(next)
	if refresh.Tooltip != "Refresh (last checked Jul 26 10:01 AM)" {
		t.Fatalf("refresh tooltip did not update with state: %q", refresh.Tooltip)
	}

	v.Show(SettingsScreen)
	window.Resize(v.MinimumSize(SettingsScreen))
	settingsTooltips := map[string]bool{}
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		if value, ok := object.(*SmallButton); ok {
			settingsTooltips[value.Tooltip] = true
		}
	})
	for _, tooltip := range []string{"Back", "Theme: Dark", "Help", "Update", "Done"} {
		if !settingsTooltips[tooltip] {
			t.Fatalf("settings title tooltip %q is missing", tooltip)
		}
	}

	cfg := v.config
	cfg.Language = settings.LanguageKorean
	v.SetConfig(cfg)
	if got := v.themeTooltip(); got != "테마: 다크" {
		t.Fatalf("Korean theme tooltip=%q", got)
	}
	if got := v.displayModeTooltip(settings.ModeCompact); got != "나노 모드로 전환" {
		t.Fatalf("Korean display tooltip=%q", got)
	}
	if !strings.HasPrefix(v.refreshTooltip(), "새로고침 (마지막 조회 ") {
		t.Fatalf("Korean refresh tooltip=%q", v.refreshTooltip())
	}
}

func TestPhase3YSeverityAndProviderUseSharedSixteenColorPopup(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	v.Show(SettingsScreen)
	window.Resize(v.MinimumSize(SettingsScreen))

	var warning *PaletteButton
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		if button, ok := object.(*PaletteButton); ok && button.ID == v.config.WarningColor {
			warning = button
		}
	})
	if warning == nil || warning.OnShowPalette == nil {
		t.Fatal("warning threshold does not use the shared popup controller")
	}
	warning.ShowPalette()
	thresholdSize := v.palettePopup.Size()
	thresholdSwatches := palettePopupSwatches(v.palettePopup)
	if len(thresholdSwatches) != 16 {
		t.Fatalf("threshold palette swatches=%d, want 16", len(thresholdSwatches))
	}
	for _, swatch := range thresholdSwatches {
		if swatch.Reset {
			t.Fatal("threshold palette unexpectedly contains provider reset swatch")
		}
	}
	var purple *PaletteSwatch
	for _, swatch := range thresholdSwatches {
		if sameColor(swatch.Fill, v.colors.PaletteColor("purple")) {
			purple = swatch
		}
	}
	if purple == nil {
		t.Fatal("threshold palette is missing purple from the shared 16 colors")
	}
	purple.Tapped(nil)
	if v.config.WarningColor != "purple" || v.palettePopup != nil {
		t.Fatalf("warning palette selection color/popup=%q/%v", v.config.WarningColor, v.palettePopup != nil)
	}

	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))
	v.normalCache.rows[0].meter.Tapped(nil)
	providerSwatches := palettePopupSwatches(v.palettePopup)
	if len(providerSwatches) != 17 || v.palettePopup.Size() != thresholdSize {
		t.Fatalf("provider palette swatches/size=%d/%v, threshold size=%v", len(providerSwatches), v.palettePopup.Size(), thresholdSize)
	}
}

func palettePopupSwatches(popup *widget.PopUp) []*PaletteSwatch {
	result := []*PaletteSwatch{}
	if popup == nil {
		return result
	}
	walkCanvasObject(popup.Content, func(object fyne.CanvasObject) {
		if swatch, ok := object.(*PaletteSwatch); ok {
			result = append(result, swatch)
		}
	})
	return result
}

func TestPhase3YTranslatedButtonsUseMeasuredWidths(t *testing.T) {
	for _, entry := range []struct {
		label   string
		minimum float32
		outline bool
	}{
		{"Update", 68, false},
		{"업데이트", 68, false},
		{"Done", 48, false},
		{"완료", 48, false},
		{"Test connection", 92, true},
		{"연결 테스트", 92, true},
		{"Install guide", 74, true},
		{"설치 안내", 74, true},
	} {
		button := NewSmallButton(entry.label, entry.label, nil)
		if entry.outline {
			button = NewOutlinedSmallButton(entry.label, entry.label, nil)
		}
		width := buttonWidthFor(button, entry.minimum)
		measured := fyne.MeasureText(entry.label, map[bool]float32{false: 13, true: SettingsTextSize}[entry.outline], fyne.TextStyle{Bold: true}).Width
		if width < entry.minimum || width < measured+2*ButtonLabelPadding {
			t.Fatalf("button %q width=%.1f, minimum=%.1f measured+padding=%.1f", entry.label, width, entry.minimum, measured+2*ButtonLabelPadding)
		}
		wrapper := connectionButton(button, entry.minimum)
		if wrapper.MinSize().Width != width || wrapper.MinSize().Height != SmallButtonHeight {
			t.Fatalf("button %q wrapper=%v, want %.1fx%.1f", entry.label, wrapper.MinSize(), width, SmallButtonHeight)
		}
		t.Logf("button %q measured=%.1f padded-width=%.1f minimum=%.1f", entry.label, measured, width, entry.minimum)
	}

	v, window := newTestView(t)
	defer window.Close()
	for _, language := range []settings.Language{settings.LanguageEnglish, settings.LanguageKorean} {
		cfg := v.config
		cfg.Language = language
		v.SetConfig(cfg)
		v.Show(SettingsScreen)
		window.Resize(v.MinimumSize(SettingsScreen))
		var titleRow *fyne.Container
		walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
			candidate, ok := object.(*fyne.Container)
			if ok && len(candidate.Objects) == 7 {
				if _, ok = candidate.Layout.(*GapColumnLayout); ok {
					titleRow = candidate
				}
			}
		})
		if titleRow == nil {
			t.Fatalf("%s settings title row is missing", language)
		}
		widths := titleRow.Layout.(*GapColumnLayout).Widths
		update := titleRow.Objects[5].(*SmallButton)
		done := titleRow.Objects[6].(*SmallButton)
		if widths[5] != buttonWidthFor(update, 68) || widths[6] != buttonWidthFor(done, 48) {
			t.Fatalf("%s settings button widths=%.1f/%.1f", language, widths[5], widths[6])
		}
		t.Logf("%s settings update/done widths=%.1f/%.1f", language, widths[5], widths[6])
	}
}
