package ui

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
	xdraw "golang.org/x/image/draw"
)

func TestPhase3TProviderNativeSVGInkCoverageColorsAndDistinctRasters(t *testing.T) {
	tests := []struct {
		kind  ProviderIconKind
		name  string
		color color.NRGBA
	}{
		{ProviderIconClaude, "Claude", color.NRGBA{R: 0xD9, G: 0x77, B: 0x57, A: 0xFF}},
		{ProviderIconAGClaude, "AG Claude", color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}},
		{ProviderIconCodex, "Codex", color.NRGBA{R: 0x1F, G: 0x23, B: 0x28, A: 0xFF}},
		{ProviderIconGemini, "Gemini", color.NRGBA{R: 0x8E, G: 0x75, B: 0xFF, A: 0xFF}},
	}
	app := test.NewApp()
	t.Cleanup(app.Quit)
	w := test.NewWindow(nil)
	w.SetPadded(false)
	defer w.Close()
	sanity := canvas.NewImageFromResource(fyne.NewStaticResource("provider-sanity.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path fill="#FFFFFF" d="M2 2h20v20H2z"/></svg>`)))
	sanity.FillMode = canvas.ImageFillContain
	sanityRender := renderNativeCanvasImage(w, sanity, providerIconRenderSize)
	if coverage, _ := nonBackgroundCoverage(sanityRender); coverage <= 0 {
		t.Fatal("Fyne native SVG software-canvas sanity mark rendered 0% ink")
	}
	renders := make(map[ProviderIconKind]image.Image, len(tests))
	for _, test := range tests {
		native := renderNativeProviderIcon(w, test.kind, providerIconRenderSize)
		nativeCoverage, background := nonBackgroundCoverage(native)
		effective := renderNativeCanvasImage(w, NewProviderIcon(test.kind), providerIconRenderSize)
		effectiveCoverage, _ := nonBackgroundCoverage(effective)
		renders[test.kind] = effective
		t.Logf("%s Fyne/oksvg diagnostic raster=%v ink=%.2f%% effective=%.2f%% (background %v direct-path=%t)", test.name, native.Bounds(), nativeCoverage, effectiveCoverage, background, providerIconOfficialVerified(test.kind))
		if effectiveCoverage <= 0 || !hasApproximateColor(effective, test.color, 3) {
			t.Fatalf("%s effective provider icon has no expected ink near %v", test.name, test.color)
		}
	}
	for leftIndex := range tests {
		for rightIndex := leftIndex + 1; rightIndex < len(tests); rightIndex++ {
			left, right := tests[leftIndex], tests[rightIndex]
			if sameRaster(renders[left.kind], renders[right.kind]) {
				t.Fatalf("%s and %s effective provider rasters are identical", left.name, right.name)
			}
		}
	}
	if sameNonBackgroundMask(renders[ProviderIconClaude], renders[ProviderIconCodex]) ||
		sameNonBackgroundMask(renders[ProviderIconClaude], renders[ProviderIconGemini]) ||
		sameNonBackgroundMask(renders[ProviderIconCodex], renders[ProviderIconGemini]) {
		t.Fatal("Claude, Codex, and Gemini effective silhouettes are not distinct")
	}
}

func renderNativeProviderIcon(window fyne.Window, kind ProviderIconKind, size int) image.Image {
	icon := newNativeProviderIcon(kind)
	return renderNativeCanvasImage(window, icon, size)
}

func renderNativeCanvasImage(window fyne.Window, icon *canvas.Image, size int) image.Image {
	backgroundColor := color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xFF}
	background := canvas.NewRectangle(backgroundColor)
	content := container.NewStack(background, icon)
	window.SetContent(content)
	window.Resize(fyne.NewSquareSize(float32(size)))
	content.Resize(fyne.NewSquareSize(float32(size)))
	icon.Refresh()
	content.Refresh()
	if icon.Image == nil {
		return window.Canvas().Capture()
	}
	result := image.NewNRGBA(icon.Image.Bounds())
	draw.Draw(result, result.Bounds(), image.NewUniform(backgroundColor), image.Point{}, draw.Src)
	draw.Draw(result, result.Bounds(), icon.Image, icon.Image.Bounds().Min, draw.Over)
	return result
}

func nonBackgroundCoverage(source image.Image) (float64, color.NRGBA) {
	counts := make(map[color.NRGBA]int)
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			counts[color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)]++
		}
	}
	background := color.NRGBA{}
	backgroundCount := -1
	for candidate, count := range counts {
		if count > backgroundCount {
			background, backgroundCount = candidate, count
		}
	}
	pixels := bounds.Dx() * bounds.Dy()
	return float64(pixels-backgroundCount) * 100 / float64(pixels), background
}

func hasApproximateColor(source image.Image, expected color.NRGBA, tolerance uint8) bool {
	close := func(left, right uint8) bool {
		delta := int(left) - int(right)
		if delta < 0 {
			delta = -delta
		}
		return delta <= int(tolerance)
	}
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			if close(pixel.R, expected.R) && close(pixel.G, expected.G) && close(pixel.B, expected.B) {
				return true
			}
		}
	}
	return false
}

func sameRaster(left, right image.Image) bool {
	if left.Bounds() != right.Bounds() {
		return false
	}
	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(left.At(x, y)) != color.NRGBAModel.Convert(right.At(x, y)) {
				return false
			}
		}
	}
	return true
}

func sameNonBackgroundMask(left, right image.Image) bool {
	if left.Bounds() != right.Bounds() {
		return false
	}
	_, leftBackground := nonBackgroundCoverage(left)
	_, rightBackground := nonBackgroundCoverage(right)
	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			leftInk := color.NRGBAModel.Convert(left.At(x, y)).(color.NRGBA) != leftBackground
			rightInk := color.NRGBAModel.Convert(right.At(x, y)).(color.NRGBA) != rightBackground
			if leftInk != rightInk {
				return false
			}
		}
	}
	return true
}

func alphaCoverage(source image.Image) float64 {
	var ink uint64
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			ink += uint64(alpha)
		}
	}
	return float64(ink) * 100 / float64(bounds.Dx()*bounds.Dy()*0xFFFF)
}

func hasSolidColor(source image.Image, expected color.NRGBA) bool {
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			if pixel == expected {
				return true
			}
		}
	}
	return false
}

func sameAlphaMask(left, right image.Image) bool {
	if left.Bounds() != right.Bounds() {
		return false
	}
	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			_, _, _, leftAlpha := left.At(x, y).RGBA()
			_, _, _, rightAlpha := right.At(x, y).RGBA()
			if leftAlpha != rightAlpha {
				return false
			}
		}
	}
	return true
}

func TestPhase3RNanoLabelsBarsAndIconsAreVerticallyCentered(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	v.Show(NanoScreen)
	w.Resize(v.MinimumSize(NanoScreen))
	for cellIndex, cell := range v.nanoCache.cells {
		if got, want := cell.icon.Position().Y+cell.icon.Size().Height/2, (cell.background.Size().Height-6)/2; got != want {
			t.Fatalf("nano cell %d icon center=%.2f, want %.2f", cellIndex, got, want)
		}
		for rowIndex, row := range cell.rows {
			labelCenter := row.label.Position().Y + row.label.Size().Height/2
			bundleCenter := row.bar.Position().Y + (row.bar.Size().Height+1+row.reset.Size().Height)/2
			if labelCenter != bundleCenter {
				t.Fatalf("nano cell %d row %d label/bar-bundle centers=%.2f/%.2f", cellIndex, rowIndex, labelCenter, bundleCenter)
			}
		}
	}
}

func TestPhase3TCompactAndNanoBarsUseRequestedThickness(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()

	v.Show(CompactScreen)
	w.Resize(v.MinimumSize(CompactScreen))
	for rowIndex, row := range v.compactCache.rows {
		if row.reset == nil {
			t.Fatalf("compact row %d has no reset bar", rowIndex)
		}
		if row.reset.Height != 2 || row.reset.Size().Height != 2 {
			t.Fatalf("compact row %d reset height property/render=%.2f/%.2f, want 2/2", rowIndex, row.reset.Height, row.reset.Size().Height)
		}
	}

	v.Show(NanoScreen)
	nanoSize := v.MinimumSize(NanoScreen)
	w.Resize(nanoSize)
	states := v.nanoCellStates()
	for cellIndex, cell := range v.nanoCache.cells {
		for rowIndex, row := range cell.rows {
			if row.reset == nil {
				t.Fatalf("nano cell %d row %d has no reset bar", cellIndex, rowIndex)
			}
			if row.bar.Height != NanoUsageBarHeight || row.bar.Size().Height != NanoUsageBarHeight {
				t.Fatalf("nano cell %d row %d usage height property/render=%.2f/%.2f, want %.0f/%.0f", cellIndex, rowIndex, row.bar.Height, row.bar.Size().Height, NanoUsageBarHeight, NanoUsageBarHeight)
			}
			if row.reset.Height != 2 || row.reset.Size().Height != 2 {
				t.Fatalf("nano cell %d row %d reset height property/render=%.2f/%.2f, want 2/2", cellIndex, rowIndex, row.reset.Height, row.reset.Size().Height)
			}
			if got, want := row.reset.Position().Y, row.bar.Position().Y+row.bar.Size().Height+1; got != want {
				t.Fatalf("nano cell %d row %d reset y=%.2f, want %.2f below usage bar", cellIndex, rowIndex, got, want)
			}
			want := v.nanoResetPercent(states[cellIndex].rows[rowIndex], time.Now())
			if math.Abs(row.reset.Value-want) > 0.01 {
				t.Fatalf("nano cell %d row %d reset progress=%.2f, want %.2f", cellIndex, rowIndex, row.reset.Value, want)
			}
		}
	}
	t.Logf("compact reset=2px; nano usage/gap/reset=%.0fpx/%.0fpx/%.0fpx; line gap=%.0fpx; nano minimum size=%.0fx%.0f (body constant %.0fpx)", NanoUsageBarHeight, NanoResetGap, NanoResetBarHeight, NanoLineGap, nanoSize.Width, nanoSize.Height, NanoBodyHeight)

	cache := v.nanoCache
	reset := cache.cells[0].rows[0].reset
	next := DemoViewState()
	next.Lanes[0].Rows[0].DisplayRemainingPercent = 37
	v.SetState(next)
	// Reset bars always show the remaining share of the window.
	if v.nanoCache != cache || v.nanoCache.cells[0].rows[0].reset != reset || reset.Value != 37 {
		t.Fatalf("nano reset cache/update=%p/%p value=%.2f, want retained pointer and 37", cache.cells[0].rows[0].reset, reset, reset.Value)
	}
}

func TestPhase3RThemeIconsUseBalancedSolidSilhouettes(t *testing.T) {
	tests := []struct {
		name   string
		mode   settings.Theme
		colors BrandColors
	}{
		{"Light sun", settings.ThemeLight, LightBrandColors},
		{"Dark moon", settings.ThemeDark, DarkBrandColors},
		{"System half-moon", settings.ThemeSystem, DarkBrandColors},
	}
	coverages := make([]float64, 0, len(tests))
	icons := make([]image.Image, 0, len(tests))
	for _, test := range tests {
		icon := rasterizeThemeModeIcon(test.mode, test.colors.Label)
		coverage := alphaCoverage(icon)
		coverages = append(coverages, coverage)
		icons = append(icons, icon)
		if coverage < 20 || coverage > 40 {
			t.Fatalf("%s coverage %.2f%% is outside solid-icon range 20–40%%", test.name, coverage)
		}
		expected := color.NRGBAModel.Convert(test.colors.Label).(color.NRGBA)
		if !hasSolidColor(icon, expected) {
			t.Fatalf("%s does not contain the title label color %v", test.name, expected)
		}
		resource := themeModeResource(test.mode, test.colors)
		if !strings.HasSuffix(resource.Name(), ".png") || len(resource.Content()) == 0 {
			t.Fatalf("%s resource=%q bytes=%d, want non-empty raster PNG", test.name, resource.Name(), len(resource.Content()))
		}
		topContrast := wcagContrastRatio(test.colors.Label, test.colors.TitleTop)
		bottomContrast := wcagContrastRatio(test.colors.Label, test.colors.TitleBottom)
		t.Logf("%s coverage=%.2f%% contrast(top/bottom)=%.2f:1/%.2f:1", test.name, coverage, topContrast, bottomContrast)
		if topContrast < 4.5 || bottomContrast < 4.5 {
			t.Fatalf("%s title contrast %.2f:1/%.2f:1, want >= 4.5:1", test.name, topContrast, bottomContrast)
		}
	}
	minimum, maximum := coverages[0], coverages[0]
	for _, coverage := range coverages[1:] {
		minimum = min(minimum, coverage)
		maximum = max(maximum, coverage)
	}
	if maximum-minimum > 12 {
		t.Fatalf("theme icon coverage spread %.2f%% is too wide for a consistent visual weight", maximum-minimum)
	}
	for left := 0; left < len(icons); left++ {
		for right := left + 1; right < len(icons); right++ {
			if sameAlphaMask(icons[left], icons[right]) {
				t.Fatalf("theme icon masks %d/%d are not distinct", left, right)
			}
		}
	}
	button := NewSmallIconButton(themeModeResource(settings.ThemeDark, DarkBrandColors), "theme", nil, DarkBrandColors)
	renderer := button.CreateRenderer().(*smallButtonRenderer)
	renderer.Layout(fyne.NewSize(24, 24))
	if renderer.icon.Size() != fyne.NewSize(16, 16) || renderer.icon.Position() != fyne.NewPos(4, 4) {
		t.Fatalf("theme titlebar icon size/position=%v/%v, want 16x16 centered at 4,4", renderer.icon.Size(), renderer.icon.Position())
	}
}

func TestPhase3RSettingsRadioAndInlineWarningSliders(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)
	expanded := v.MinimumSize(SettingsScreen)
	radios, sliders, scrolls := 0, 0, 0
	radioLabels := false
	labels := map[string]bool{}
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		switch value := object.(type) {
		case *RadioGroup:
			radios++
			if len(value.Options) != 2 {
				t.Fatalf("display method radio is not a two-option custom group: %+v", value)
			}
			radioLabels = value.Options[0] == "Remaining" && value.Options[1] == "Usage"
		case *widget.Slider:
			sliders++
		case *container.Scroll:
			scrolls++
		case *canvas.Text:
			labels[value.Text] = true
		}
	})
	if radios != 1 || !radioLabels || sliders != 2 || scrolls != 0 || !labels["Warning"] || !labels["Danger"] {
		t.Fatalf("expanded settings radios=%d sliders=%d scrolls=%d labels=%v", radios, sliders, scrolls, labels)
	}

	cfg := v.config
	cfg.WarningsEnabled = false
	v.SetConfig(cfg)
	collapsed := v.MinimumSize(SettingsScreen)
	sliders = 0
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		if _, ok := object.(*widget.Slider); ok {
			sliders++
		}
	})
	if sliders != 0 || expanded.Height < collapsed.Height {
		t.Fatalf("collapsed settings sliders=%d heights=%.1f/%.1f", sliders, expanded.Height, collapsed.Height)
	}

	cfg = v.config
	cfg.Language = "ko"
	v.SetConfig(cfg)
	if v.text(i18n.KeyUsageRemaining) != "잔여량" || v.text(i18n.KeyUsageUsed) != "사용량" {
		t.Fatalf("Korean usage labels=%q/%q", v.text(i18n.KeyUsageRemaining), v.text(i18n.KeyUsageUsed))
	}
}

func TestPhase3RVisualReviewCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3R_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3R_SCREENSHOT_DIR for Phase 3R captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	v, w := phase2DTestView(t)
	defer w.Close()
	cfg := DemoConfig(v.config)
	cfg.Language = settings.Language(i18n.Korean)
	v.SetConfig(cfg)
	v.SetState(DemoViewState())

	save := func(name string, scale int) {
		t.Helper()
		source := w.Canvas().Capture()
		output := source
		if scale > 1 {
			output = image.NewRGBA(image.Rect(0, 0, source.Bounds().Dx()*scale, source.Bounds().Dy()*scale))
			xdraw.NearestNeighbor.Scale(output.(drawImage), output.Bounds(), source, source.Bounds(), xdraw.Src, nil)
		}
		file, err := os.Create(filepath.Join(directory, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, output); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	v.Show(CompactScreen)
	w.Resize(v.MinimumSize(CompactScreen))
	save("compact-18px-3x", 3)
	v.Show(NanoScreen)
	w.Resize(v.MinimumSize(NanoScreen))
	save("nano-alignment-18px-3x", 3)
	for _, mode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark, settings.ThemeSystem} {
		cfg = v.config
		cfg.Theme = mode
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		w.Resize(v.MinimumSize(CompactScreen))
		save("theme-icon-"+string(mode)+"-3x", 3)
	}
	v.Show(SettingsScreen)
	w.Resize(v.MinimumSize(SettingsScreen))
	save("settings-warning-on", 1)
	cfg = v.config
	cfg.WarningsEnabled = false
	v.SetConfig(cfg)
	w.Resize(v.MinimumSize(SettingsScreen))
	save("settings-warning-off", 1)
}

type drawImage interface {
	image.Image
	Set(x, y int, c color.Color)
}
