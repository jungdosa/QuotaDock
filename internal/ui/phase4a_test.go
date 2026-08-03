package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase4ANanoBarLabelsSpacingAndCenters(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	v.Show(NanoScreen)
	window.Resize(v.MinimumSize(NanoScreen))

	if NanoUsageBarHeight != 6 || NanoResetBarHeight != 2 || NanoResetGap != 1 || NanoLineGap != 2 {
		t.Fatalf("nano usage/reset-gap/reset/line-gap=%.0f/%.0f/%.0f/%.0f, want 6/1/2/2",
			NanoUsageBarHeight, NanoResetGap, NanoResetBarHeight, NanoLineGap)
	}
	for cellIndex, cell := range v.nanoCache.cells {
		var previousLine *fyne.Container
		for rowIndex, row := range cell.rows {
			if row.label.Text != "5h" && row.label.Text != "7D" {
				t.Fatalf("nano cell %d row %d label=%q", cellIndex, rowIndex, row.label.Text)
			}
			if row.bar.Height != NanoUsageBarHeight || row.bar.Size().Height != NanoUsageBarHeight {
				t.Fatalf("nano cell %d row %d usage height=%.1f/%.1f", cellIndex, rowIndex, row.bar.Height, row.bar.Size().Height)
			}
			labelCenter := row.label.Position().Y + row.label.Size().Height/2
			bundleCenter := (row.bar.Position().Y + row.reset.Position().Y + row.reset.Size().Height) / 2
			if labelCenter != bundleCenter {
				t.Fatalf("nano cell %d row %d label/bar centers=%.2f/%.2f", cellIndex, rowIndex, labelCenter, bundleCenter)
			}
			// Rows are wrapped in a Stack with their hover region (W12), so the
			// object positioned by NanoLinesLayout is the wrapper one level up.
			line := findContainerWithObject(v.Nano, findContainerWithObject(v.Nano, row.bar))
			if previousLine != nil {
				gap := line.Position().Y - (previousLine.Position().Y + previousLine.Size().Height)
				if gap != NanoLineGap {
					t.Fatalf("nano cell %d inter-line gap=%.1f, want %.1f", cellIndex, gap, NanoLineGap)
				}
			}
			previousLine = line
		}
	}
	if size := v.MinimumSize(NanoScreen); size.Width != 360 || size.Height <= 75 || size.Height > 78 {
		t.Fatalf("nano minimum size=%v, want width 360 and minimal height increase from 75 to at most 78", size)
	}
}

func TestPhase4AThemeSunAndNextDisplayModeIcons(t *testing.T) {
	sun := rasterizeThemeSun(LightBrandColors.Label)
	coverage := alphaCoverage(sun)
	if coverage < 20 || coverage > 30 {
		t.Fatalf("recognizable sun coverage=%.2f%%, want 20-30%%", coverage)
	}
	_, _, _, centerAlpha := sun.At(80, 80).RGBA()
	_, _, _, rayAlpha := sun.At(80, 24).RGBA()
	_, _, _, gapAlpha := sun.At(80, 44).RGBA()
	if centerAlpha == 0 || rayAlpha == 0 || gapAlpha != 0 {
		t.Fatalf("sun center/ray/gap alpha=%d/%d/%d, want solid/solid/clear", centerAlpha, rayAlpha, gapAlpha)
	}
	for name, colors := range map[string]BrandColors{"light": LightBrandColors, "dark": DarkBrandColors} {
		top := wcagContrastRatio(colors.Label, colors.TitleTop)
		bottom := wcagContrastRatio(colors.Label, colors.TitleBottom)
		t.Logf("%s display icon contrast: TitleTop %.2f:1, TitleBottom %.2f:1", name, top, bottom)
		if top < 4.5 {
			t.Fatalf("%s display icon top contrast=%.2f:1", name, top)
		}
		if bottom < 4.5 {
			t.Fatalf("%s display icon bottom contrast=%.2f:1", name, bottom)
		}
	}

	icons := []struct {
		mode      settings.DisplayMode
		name      string
		geometry  string
		paint     string
		outerArea float32
	}{
		{settings.ModeNormal, "display-normal.svg", "x='1.5' y='2' width='13' height='12' rx='2'", "fill='none' stroke='#46586c' stroke-width='1.8'", 13 * 12},
		{settings.ModeCompact, "display-compact.svg", "x='2' y='4' width='12' height='8' rx='2'", "fill='none' stroke='#46586c' stroke-width='1.6'", 12 * 8},
		// W5: the nano icon is a line, not a filled bar, and two pixels
		// narrower than the rectangles above it.
		{settings.ModeNano, "display-nano.svg", "d='M2 8h12'", "fill='none' stroke='#46586c' stroke-width='2' stroke-linecap='round'", 12 * 2},
	}
	for _, icon := range icons {
		resource := displayModeIconResource(icon.mode, LightBrandColors)
		content := string(resource.Content())
		if resource.Name() != icon.name || !strings.Contains(content, icon.geometry) || !strings.Contains(content, icon.paint) {
			t.Fatalf("%s icon name/content=%q/%q", icon.mode, resource.Name(), content)
		}
	}
	if !(icons[0].outerArea > icons[1].outerArea && icons[1].outerArea > icons[2].outerArea) {
		t.Fatal("normal/compact/nano icon outer-area hierarchy is not descending")
	}

	button := NewSmallIconButton(displayModeIconResource(settings.ModeNormal, LightBrandColors), "Normal", nil, LightBrandColors)
	renderer := button.CreateRenderer().(*smallButtonRenderer)
	renderer.Layout(fyne.NewSize(24, TitleBarHeight))
	if renderer.icon.Size() != fyne.NewSize(16, 16) || renderer.icon.Position() != fyne.NewPos(4, 11) {
		t.Fatalf("display icon slot size/position=%v/%v, want 16x16 centered at (4,11) in 24x38", renderer.icon.Size(), renderer.icon.Position())
	}

	v, window := newTestView(t)
	defer window.Close()
	english := map[settings.DisplayMode]string{
		settings.ModeNormal:  "Switch to Compact",
		settings.ModeCompact: "Switch to Nano",
		settings.ModeNano:    "Switch to Normal",
	}
	for current, want := range english {
		if got := v.displayModeTooltip(current); got != want {
			t.Fatalf("English current %s tooltip=%q, want %q", current, got, want)
		}
		if got := displayModeResource(current, LightBrandColors).Name(); got != "display-"+string(settings.NextDisplayMode(current))+".svg" {
			t.Fatalf("current %s cycle icon=%q", current, got)
		}
	}
	cfg := v.config
	cfg.Language = settings.Language(i18n.Korean)
	v.SetConfig(cfg)
	korean := map[settings.DisplayMode]string{
		settings.ModeNormal:  "컴팩트 모드로 전환",
		settings.ModeCompact: "나노 모드로 전환",
		settings.ModeNano:    "일반 모드로 전환",
	}
	for current, want := range korean {
		if got := v.displayModeTooltip(current); got != want {
			t.Fatalf("Korean current %s tooltip=%q, want %q", current, got, want)
		}
	}
}

func TestPhase4ANormalSeverityUsesMeterWithoutLabelStripe(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	cfg := v.config
	cfg.WarningPercent = 75
	cfg.DangerPercent = 90
	v.SetConfig(cfg)
	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))

	foundWarning := false
	for index, handles := range v.normalCache.rows {
		if handles.row.Objects[0] != handles.label {
			t.Fatalf("normal row %d label is still wrapped by a stripe container", index)
		}
		warning := v.colors.PaletteColor(v.config.WarningColor)
		if sameColor(handles.meter.Active, warning) {
			foundWarning = true
			if !sameColor(handles.percent.Color, warning) {
				t.Fatal("warning severity no longer colors the meter and percent")
			}
		}
	}
	if !foundWarning {
		t.Fatal("warning normal row was not found")
	}
	statusRow, status := v.makeNormalStatusRow(LaneState{})
	if statusRow.(*fyne.Container).Objects[0] != status {
		t.Fatal("normal status label is still wrapped by a stripe container")
	}
}

func TestPhase4BSoftwareRenderCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE4B_SCREENSHOT_DIR")
	if directory == "" {
		directory = os.Getenv("QUOTADOCK_PHASE4A_SCREENSHOT_DIR")
	}
	if directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct {
		name   string
		theme  settings.Theme
		colors BrandColors
	}{
		{"light", settings.ThemeLight, LightBrandColors},
		{"dark", settings.ThemeDark, DarkBrandColors},
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
			cfg := DemoConfig(settings.Default())
			cfg.Theme = entry.theme
			view := NewView(window.Canvas(), catalog, i18n.English, cfg, Actions{})
			window.SetContent(view.Root)
			view.SetState(DemoViewState())
			view.Show(NanoScreen)
			window.Resize(view.MinimumSize(NanoScreen))
			if directory != "" {
				writePhase4BCapture(t, filepath.Join(directory, "nano-"+entry.name+".png"), window.Canvas().Capture())
			}

			iconButtons := make([]fyne.CanvasObject, 0, 3)
			for _, icon := range []struct {
				mode                   settings.DisplayMode
				name                   string
				minimumInk, maximumInk float64
			}{
				{settings.ModeNormal, "normal", 30, 42},
				{settings.ModeCompact, "compact", 20, 30},
				// W5: the nano icon is a thin line, so its ink share is far
				// below the rectangle outlines above it.
				{settings.ModeNano, "nano", 6, 16},
			} {
				button := NewSmallIconButton(displayModeIconResource(icon.mode, entry.colors), icon.name, nil, entry.colors)
				background := canvas.NewRectangle(entry.colors.TitleTop)
				window.SetContent(container.NewStack(background, button))
				window.Resize(fyne.NewSize(24, 24))
				capture := window.Canvas().Capture()
				coverage := effectiveInkCoverage(capture, entry.colors.TitleTop, entry.colors.Label, 16*16)
				centerInk := effectiveInkAt(capture.At(12, 12), entry.colors.TitleTop, entry.colors.Label)
				t.Logf("%s %s display icon ink coverage=%.2f%% center=%.2f", entry.name, icon.name, coverage, centerInk)
				if coverage < icon.minimumInk || coverage > icon.maximumInk {
					t.Fatalf("%s %s display icon ink coverage=%.2f%%, want %.0f-%.0f%%", entry.name, icon.name, coverage, icon.minimumInk, icon.maximumInk)
				}
				if icon.mode == settings.ModeNano && centerInk < 0.95 {
					t.Fatalf("%s nano display icon center ink=%.2f, want filled", entry.name, centerInk)
				}
				if icon.mode != settings.ModeNano && centerInk > 0.05 {
					t.Fatalf("%s %s display icon center ink=%.2f, want hollow", entry.name, icon.name, centerInk)
				}
				iconButtons = append(iconButtons, button)
			}

			buttons := container.NewHBox(iconButtons...)
			background := canvas.NewRectangle(entry.colors.TitleTop)
			gallery := container.NewStack(background, container.NewCenter(buttons))
			window.SetContent(gallery)
			window.Resize(fyne.NewSize(104, TitleBarHeight))
			iconCapture := window.Canvas().Capture()
			if ink := pixelsDifferentFrom(iconCapture, entry.colors.TitleTop); ink < 100 {
				t.Fatalf("%s display icon comparison has only %d non-background pixels", entry.name, ink)
			} else {
				t.Logf("%s display icon comparison ink=%dpx", entry.name, ink)
			}
			if directory != "" {
				writePhase4BCapture(t, filepath.Join(directory, "display-icons-"+entry.name+".png"), iconCapture)
			}
			window.Close()
			a.Quit()
		})
	}
}

func effectiveInkCoverage(source image.Image, background, foreground color.Color, slotPixels int) float64 {
	total := float64(0)
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			total += effectiveInkAt(source.At(x, y), background, foreground)
		}
	}
	return total * 100 / float64(slotPixels)
}

func effectiveInkAt(pixel, background, foreground color.Color) float64 {
	p := color.NRGBAModel.Convert(pixel).(color.NRGBA)
	bg := color.NRGBAModel.Convert(background).(color.NRGBA)
	fg := color.NRGBAModel.Convert(foreground).(color.NRGBA)
	delta := [3]float64{float64(fg.R) - float64(bg.R), float64(fg.G) - float64(bg.G), float64(fg.B) - float64(bg.B)}
	paint := [3]float64{float64(p.R) - float64(bg.R), float64(p.G) - float64(bg.G), float64(p.B) - float64(bg.B)}
	denominator := delta[0]*delta[0] + delta[1]*delta[1] + delta[2]*delta[2]
	if denominator == 0 {
		return 0
	}
	alpha := (paint[0]*delta[0] + paint[1]*delta[1] + paint[2]*delta[2]) / denominator
	return max(float64(0), min(float64(1), alpha))
}

func pixelsDifferentFrom(source image.Image, background color.Color) int {
	count := 0
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			if !sameColor(source.At(x, y), background) {
				count++
			}
		}
	}
	return count
}

func writePhase4BCapture(t *testing.T, path string, source image.Image) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err = png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
	t.Logf("software canvas capture %s: %dx%d", path, source.Bounds().Dx(), source.Bounds().Dy())
}
