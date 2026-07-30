package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

const (
	phase3SLegacyWarningOnHeight  float32 = 758.3
	phase3SLegacyWarningOffHeight float32 = 680
	phase3SLegacyLanguageWidth    float32 = 160
	phase3SLegacyDateWidth        float32 = 260
)

func TestPhase3SSettingsLayoutMetrics(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)

	usage := v.usageSettings().(*fyne.Container)
	if len(usage.Objects) != 4 {
		t.Fatalf("usage rows=%d, want 4 compact rows", len(usage.Objects))
	}
	controlStarts := make([][2]float32, 0, 3)
	for rowIndex := 0; rowIndex < 3; rowIndex++ {
		pair := usage.Objects[rowIndex].(*fyne.Container)
		pair.Resize(fyne.NewSize(560, 28))
		pair.Layout.Layout(pair.Objects, pair.Size())
		starts := [2]float32{}
		for columnIndex, object := range pair.Objects {
			row := object.(*fyne.Container)
			row.Layout.Layout(row.Objects, row.Size())
			starts[columnIndex] = row.Objects[1].Position().X
		}
		controlStarts = append(controlStarts, starts)
	}
	for rowIndex, starts := range controlStarts {
		if starts != [2]float32{120, 120} {
			t.Fatalf("usage row %d control starts=%v, want [120 120]", rowIndex, starts)
		}
	}
	rightColumnStart := usage.Objects[2].(*fyne.Container).Objects[1].Position().X + controlStarts[2][1]
	t.Logf("control start x: left=%.1f right=%.1f (560px pair), local starts=%v", controlStarts[0][0], rightColumnStart, controlStarts)

	cfg := v.config
	cfg.Language = settings.LanguageKorean
	v.SetConfig(cfg)
	display := v.displaySettings().(*fyne.Container)
	displayPair := display.Objects[0].(*fyne.Container)
	languageLayout := displayPair.Objects[0].(*fyne.Container).Layout.(*SettingRowLayout)
	dateLayout := displayPair.Objects[1].(*fyne.Container).Layout.(*SettingRowLayout)
	if languageLayout.LabelWidth+languageLayout.Gap != dateLayout.LabelWidth+dateLayout.Gap {
		t.Fatalf("select starts differ: language=%.1f date=%.1f", languageLayout.LabelWidth+languageLayout.Gap, dateLayout.LabelWidth+dateLayout.Gap)
	}
	if dateLayout.ControlWidth >= phase3SLegacyDateWidth {
		t.Fatalf("date select width did not shrink: %.1f", dateLayout.ControlWidth)
	}
	for name, rowLayout := range map[string]*SettingRowLayout{"language": languageLayout, "date/time": dateLayout} {
		if total := rowLayout.LabelWidth + rowLayout.Gap + rowLayout.ControlWidth; total > halfSettingRowWidth {
			t.Fatalf("%s select row width %.1f exceeds half-row %.1f", name, total, halfSettingRowWidth)
		}
	}
	t.Logf("select start x=%.1f; widths language %.1f->%.1f (nine endonyms), date/time %.1f->%.1f", languageLayout.LabelWidth+languageLayout.Gap, phase3SLegacyLanguageWidth, languageLayout.ControlWidth, phase3SLegacyDateWidth, dateLayout.ControlWidth)

	cfg = v.config
	cfg.WarningsEnabled = true
	v.SetConfig(cfg)
	warningOn := v.MinimumSize(SettingsScreen).Height
	cfg = v.config
	cfg.WarningsEnabled = false
	v.SetConfig(cfg)
	warningOff := v.MinimumSize(SettingsScreen).Height
	legacyDelta := phase3SLegacyWarningOnHeight - phase3SLegacyWarningOffHeight
	if warningOn > phase3SLegacyWarningOffHeight+legacyDelta/2 {
		t.Fatalf("warning-on height %.1f exceeds half-expansion target %.1f", warningOn, phase3SLegacyWarningOffHeight+legacyDelta/2)
	}
	t.Logf("warning height on %.1f->%.1f, off %.1f->%.1f, expansion %.1f->%.1f", phase3SLegacyWarningOnHeight, warningOn, phase3SLegacyWarningOffHeight, warningOff, legacyDelta, warningOn-warningOff)

	for name, colors := range map[string]BrandColors{"light": LightBrandColors, "dark": DarkBrandColors} {
		unselected := wcagContrastRatio(colors.RadioBorder, colors.Card)
		selected := wcagContrastRatio(colors.Accent, colors.Card)
		t.Logf("%s radio ring pixel contrast unselected=%.2f:1 selected=%.2f:1", name, unselected, selected)
		if unselected < 3 || selected < 3 {
			t.Fatalf("%s radio contrast unselected/selected=%.2f/%.2f, want >=3", name, unselected, selected)
		}
	}
}

func TestPhase3SCustomRadioAndSettingsButtons(t *testing.T) {
	selected := "Remaining"
	radio := NewRadioGroup([]string{"Remaining", "Usage"}, selected, func(value string) { selected = value }, DarkBrandColors)
	renderer := radio.CreateRenderer().(*radioGroupRenderer)
	radio.Resize(renderer.MinSize())
	renderer.Layout(radio.Size())
	renderer.Refresh()
	if !renderer.dots[0].Visible() || renderer.dots[1].Visible() {
		t.Fatal("custom radio selected dot visibility is incorrect")
	}
	secondLabelX := radio.optionWidths()[0] + radioOptionGap + radioDiameter + radioLabelGap + 2
	radio.Tapped(&fyne.PointEvent{Position: fyne.NewPos(secondLabelX, radioHeight/2)})
	if selected != "Usage" || radio.Selected != "Usage" {
		t.Fatalf("full option click selected=%q/%q, want Usage", selected, radio.Selected)
	}

	v, w := newTestView(t)
	defer w.Close()
	updateTapped := false
	v.Actions.CheckUpdate = func() { updateTapped = true }
	bar := v.settingsTitleBar(NewSmallIconButton(nil, "back", nil, v.colors), defaultAppVersion, nil)
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
		t.Fatal("settings title row was not found")
	}
	help := titleRow.Objects[3].(*SmallButton)
	update := titleRow.Objects[5].(*SmallButton)
	done := titleRow.Objects[6].(*SmallButton)
	if help.Icon == nil || help.Outlined || update.Disabled || !update.Outlined || done.Disabled || !done.Primary {
		t.Fatalf("title actions help/update/done=%+v/%+v/%+v", help, update, done)
	}
	separator := titleRow.Objects[4]
	leftGap := separator.Position().X - (help.Position().X + help.Size().Width)
	rightGap := update.Position().X - (separator.Position().X + separator.Size().Width)
	buttonGap := done.Position().X - (update.Position().X + update.Size().Width)
	if leftGap != 8 || rightGap != 8 || buttonGap != 6 {
		t.Fatalf("title action gaps=%.1f/%.1f/%.1f, want 8/8/6", leftGap, rightGap, buttonGap)
	}
	update.Tapped(nil)
	if !updateTapped {
		t.Fatal("enabled update button did not invoke its action")
	}
	updateRenderer := update.CreateRenderer().(*smallButtonRenderer)
	updateRenderer.Refresh()
	if updateRenderer.bg.StrokeWidth != 1 {
		t.Fatalf("update box stroke=%.1f, want 1", updateRenderer.bg.StrokeWidth)
	}

	v.Show(SettingsScreen)
	for index, handles := range v.connectionCache {
		actual := make([]*SmallButton, 0, len(handles.actionRow.Objects))
		for _, object := range handles.actionRow.Objects {
			wrapper := object.(*fyne.Container)
			actual = append(actual, wrapper.Objects[0].(*SmallButton))
		}
		expected := []*SmallButton{handles.testButton, handles.reconnect, handles.helpButton}
		if len(actual) != len(expected) {
			t.Fatalf("connection %d action count=%d, want %d", index, len(actual), len(expected))
		}
		for actionIndex := range expected {
			if actual[actionIndex] != expected[actionIndex] || !actual[actionIndex].Outlined {
				t.Fatalf("connection %d action %d order/style mismatch", index, actionIndex)
			}
		}
	}

	button := NewOutlinedSmallButton("Test", "Test", nil, DarkBrandColors)
	buttonRenderer := button.CreateRenderer().(*smallButtonRenderer)
	buttonRenderer.Refresh()
	normalFill := buttonRenderer.bg.FillColor
	button.MouseIn(&desktop.MouseEvent{})
	buttonRenderer.Refresh()
	if sameColor(normalFill, buttonRenderer.bg.FillColor) {
		t.Fatal("outlined connection button hover did not change its fill")
	}
}

func TestPhase3SSoftwareRenderCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3S_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3S_SCREENSHOT_DIR for Phase 3S software-canvas captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name     string
		theme    settings.Theme
		warnings bool
	}{
		{"settings-light-warning-off", settings.ThemeLight, false},
		{"settings-light-warning-on", settings.ThemeLight, true},
		{"settings-dark-warning-off", settings.ThemeDark, false},
		{"settings-dark-warning-on", settings.ThemeDark, true},
	} {
		t.Run(entry.name, func(t *testing.T) {
			a := test.NewApp()
			a.Settings().SetTheme(NewBrandTheme(entry.theme))
			catalog, err := i18n.Load()
			if err != nil {
				t.Fatal(err)
			}
			window := test.NewWindow(nil)
			window.SetPadded(false)
			config := DemoConfig(settings.Default())
			config.Theme = entry.theme
			config.Language = settings.LanguageKorean
			config.WarningsEnabled = entry.warnings
			view := NewView(window.Canvas(), catalog, i18n.Korean, config, Actions{DemoMode: true})
			window.SetContent(view.Root)
			view.SetState(DemoViewState())
			view.Show(SettingsScreen)
			window.Resize(view.MinimumSize(SettingsScreen))
			output := window.Canvas().Capture()
			path := filepath.Join(directory, entry.name+".png")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(file, output); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
			t.Logf("software canvas capture %s: %dx%d", path, output.Bounds().Dx(), output.Bounds().Dy())
			window.Close()
			a.Quit()
		})
	}
}
