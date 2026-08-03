package ui

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase3WCodexLogoUsesThemeNeutralTintsWithContrast(t *testing.T) {
	tests := []struct {
		name       string
		mode       settings.Theme
		variant    fyne.ThemeVariant
		colors     BrandColors
		expected   color.NRGBA
		expectedID string
	}{
		{"light", settings.ThemeLight, theme.VariantLight, LightBrandColors, color.NRGBA{R: 0x1F, G: 0x23, B: 0x28, A: 0xFF}, "#1F2328"},
		{"dark", settings.ThemeDark, theme.VariantDark, DarkBrandColors, color.NRGBA{R: 0xE6, G: 0xE8, B: 0xEB, A: 0xFF}, "#E6E8EB"},
	}
	for _, test := range tests {
		if got := providerIconTintForTheme(ProviderIconCodex, test.mode, test.variant); got != test.expectedID {
			t.Fatalf("%s Codex tint=%s, want %s", test.name, got, test.expectedID)
		}
		imageValue := providerIconImageForTheme(ProviderIconCodex, test.mode)
		if !hasApproximateColor(imageValue, test.expected, 1) {
			t.Fatalf("%s Codex raster does not contain %v", test.name, test.expected)
		}
		rowBackground := compactRowBackground(test.colors.Background, test.colors.PaletteColor("green"))
		baseContrast := phase3WContrastRatio(test.expected, test.colors.Background)
		rowContrast := phase3WContrastRatio(test.expected, color.NRGBAModel.Convert(rowBackground).(color.NRGBA))
		if baseContrast < 7 || rowContrast < 7 {
			t.Fatalf("%s Codex contrast base/row=%.2f/%.2f, want >= 7", test.name, baseContrast, rowContrast)
		}
		t.Logf("%s Codex tint=%s background=%s row-background=%s contrast base/row=%.2f:1/%.2f:1",
			test.name, test.expectedID, phase3WHex(test.colors.Background), phase3WHex(color.NRGBAModel.Convert(rowBackground).(color.NRGBA)), baseContrast, rowContrast)
	}

	for _, kind := range []ProviderIconKind{ProviderIconClaude, ProviderIconGemini, ProviderIconAGClaude} {
		if providerIconTintForTheme(kind, settings.ThemeLight, theme.VariantLight) != providerIconTint(kind) ||
			providerIconTintForTheme(kind, settings.ThemeDark, theme.VariantDark) != providerIconTint(kind) {
			t.Fatalf("%s non-Codex tint changed across themes", kind)
		}
	}
}

func TestPhase3WCompactPercentPartsFontsBaselineAndClearance(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	state := DemoViewState()
	state.Lanes[0].Rows[0].Percent = 100
	state.Lanes[0].Rows[1].Percent = 99
	v.SetState(state)
	v.Show(CompactScreen)
	window.Resize(v.MinimumSize(CompactScreen))

	if CompactPercentTextSize != 12 || CompactHundredTextSize != 11 || CompactSymbolTextSize != 9 {
		t.Fatalf("compact percent constants normal/hundred/symbol=%.1f/%.1f/%.1f, want 12/11/9", CompactPercentTextSize, CompactHundredTextSize, CompactSymbolTextSize)
	}
	for index, want := range []struct {
		number string
		size   float32
	}{{"100", CompactHundredTextSize}, {"99", CompactPercentTextSize}} {
		handles := v.compactCache.rows[index]
		if handles.number.Text != want.number || handles.symbol.Text != "%" ||
			handles.number.TextSize != want.size || handles.symbol.TextSize != CompactSymbolTextSize {
			t.Fatalf("row %d percent=%q/%q fonts=%.1f/%.1f", index, handles.number.Text, handles.symbol.Text, handles.number.TextSize, handles.symbol.TextSize)
		}
		numberBaseline := handles.number.Position().Y + handles.number.Size().Height
		symbolBaseline := handles.symbol.Position().Y + handles.symbol.Size().Height
		gap := handles.symbol.Position().X - (handles.number.Position().X + handles.number.Size().Width)
		rightClearance := handles.percent.Size().Width - (handles.symbol.Position().X + handles.symbol.Size().Width)
		if math.Abs(float64(numberBaseline-symbolBaseline)) > 0.01 || math.Abs(float64(gap)) > 0.01 {
			t.Fatalf("row %d baseline number/symbol=%.2f/%.2f gap=%.2f", index, numberBaseline, symbolBaseline, gap)
		}
		if handles.number.Position().X < 0 || rightClearance < CompactPercentMargin {
			t.Fatalf("row %d percent clips: number-x=%.2f right-clearance=%.2f", index, handles.number.Position().X, rightClearance)
		}
	}

	hundredWidth := fyne.MeasureText("100", CompactHundredTextSize, fyne.TextStyle{Bold: true}).Width +
		fyne.MeasureText("%", CompactSymbolTextSize, fyne.TextStyle{Bold: true}).Width
	ninetyNineWidth := fyne.MeasureText("99", CompactPercentTextSize, fyne.TextStyle{Bold: true}).Width +
		fyne.MeasureText("%", CompactSymbolTextSize, fyne.TextStyle{Bold: true}).Width
	columnWidth := compactPercentColumnWidth()
	if columnWidth != float32(math.Ceil(float64(max(hundredWidth, ninetyNineWidth)+CompactPercentMargin))) ||
		compactPercentTextWidth()+CompactPercentMargin > columnWidth {
		t.Fatalf("percent widths 100/99/max+margin/column=%.2f/%.2f/%.2f+%.2f/%.2f",
			hundredWidth, ninetyNineWidth, compactPercentTextWidth(), CompactPercentMargin, columnWidth)
	}
	t.Logf("percent fonts normal/100-number/symbol=%.0f/%.0f/%.0fpt widths 100/99/max=%.2f/%.2f/%.2f column=%.0f margin=%.0f baseline-gap=0",
		CompactPercentTextSize, CompactHundredTextSize, CompactSymbolTextSize,
		hundredWidth, ninetyNineWidth, compactPercentTextWidth(), columnWidth, CompactPercentMargin)
}

func TestPhase3WCompactMetersStaySquareAcrossDynamicLabelWidths(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	for _, language := range []settings.Language{settings.Language(i18n.Korean), settings.Language(i18n.English)} {
		cfg := v.config
		cfg.Language = language
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		window.Resize(v.MinimumSize(CompactScreen))

		rows := compactTestRows(v)
		for index, handles := range v.compactCache.rows {
			content := compactTestRowContent(t, rows[index])
			meterStack := content.Objects[2]
			renderer := handles.meter.CreateRenderer().(*meterRenderer)
			renderer.Layout(fyne.NewSize(meterStack.Size().Width, CompactMeterHeight))
			if !handles.meter.SquareSegments || handles.meter.RenderedWidth != CompactMeterHeight {
				t.Fatalf("%s row %d square=%t width/height=%.2f/%.2f", language, index, handles.meter.SquareSegments, handles.meter.RenderedWidth, handles.meter.Height)
			}
			expectedCount := int(math.Round(float64((meterStack.Size().Width + CompactMeterGap) / (CompactMeterHeight + CompactMeterGap))))
			if handles.meter.RenderedSegments != expectedCount || handles.meter.RenderedGap < 1 {
				t.Fatalf("%s row %d meter width=%.2f segments=%d want=%d rendered-gap=%.2f", language, index, meterStack.Size().Width, handles.meter.RenderedSegments, expectedCount, handles.meter.RenderedGap)
			}
			visible := renderer.Objects()[:handles.meter.RenderedSegments]
			for segment, object := range visible {
				if object.Size() != fyne.NewSquareSize(CompactMeterHeight) {
					t.Fatalf("%s row %d segment %d size=%v", language, index, segment, object.Size())
				}
			}
		}
		first := v.compactCache.rows[0].meter
		t.Logf("%s label=%.0f meter=%.0f segment=%.0fx%.0f count=%d target-gap=%.0f rendered-gap=%.2f trailing=%.2f (>=1px gap at 1x)",
			language, v.compactCache.labelWidth, compactLayoutWidthBudget(v.compactCache.labelWidth).Meter,
			first.RenderedWidth, first.Height, first.RenderedSegments, first.Gap, first.RenderedGap, first.RenderedTrailing)
	}
}

func TestPhase3WCompactProviderDividersOnlyBetweenGroups(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	for _, mode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark} {
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(mode))
		cfg := v.config
		cfg.Theme = mode
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		window.Resize(v.MinimumSize(CompactScreen))

		// The caption strip lives in the header wrap above the padded body (W3),
		// so the body holds only rows and dividers.
		if len(v.compactCache.dividers) != 2 || len(v.compactBody.Objects) != len(v.compactCache.rows)+2 {
			t.Fatalf("%s dividers/body/rows=%d/%d/%d, want 2/%d/%d", mode, len(v.compactCache.dividers), len(v.compactBody.Objects), len(v.compactCache.rows), len(v.compactCache.rows)+2, len(v.compactCache.rows))
		}
		wantIndexes := []int{3, 6}
		for dividerIndex, objectIndex := range wantIndexes {
			divider := v.compactBody.Objects[objectIndex].(*fyne.Container)
			line := v.compactCache.dividers[dividerIndex]
			if len(divider.Objects) != 1 || divider.Objects[0] != line || line.Size().Height != 1 ||
				line.Position().X != CompactDividerInset || line.Size().Width != divider.Size().Width-2*CompactDividerInset {
				t.Fatalf("%s divider %d geometry container=%v line pos/size=%v/%v", mode, dividerIndex, divider.Size(), line.Position(), line.Size())
			}
			got := color.NRGBAModel.Convert(line.FillColor).(color.NRGBA)
			if got.A != CompactDividerAlpha || got.R != v.colors.Text.R || got.G != v.colors.Text.G || got.B != v.colors.Text.B {
				t.Fatalf("%s divider %d color=%v", mode, dividerIndex, got)
			}
		}
		if len(v.compactHeaderWrap.Objects) != 1 || v.compactHeaderWrap.Objects[0] != v.compactCache.columnHeader ||
			len(v.compactBody.Objects[0].(*fyne.Container).Objects) != 2 || len(v.compactBody.Objects[len(v.compactBody.Objects)-1].(*fyne.Container).Objects) != 2 {
			t.Fatalf("%s divider appeared before first or after last group", mode)
		}
		t.Logf("%s dividers color=%s alpha=%d/255=%.1f%% inset=%.0f line=1px padding-y=%.0f positions=after-Claude(index 4), after-Codex(index 7)",
			mode, phase3WHex(v.colors.Text), CompactDividerAlpha, float64(CompactDividerAlpha)*100/255, CompactDividerInset, CompactDividerPaddingY)
	}
}

func TestPhase3WNanoUsageAndResetBarsHaveOnePixelGapAndCenteredBundle(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	v.Show(NanoScreen)
	window.Resize(v.MinimumSize(NanoScreen))

	for cellIndex, cell := range v.nanoCache.cells {
		for rowIndex, row := range cell.rows {
			gap := row.reset.Position().Y - (row.bar.Position().Y + row.bar.Size().Height)
			bundleCenter := row.bar.Position().Y + (row.bar.Size().Height+gap+row.reset.Size().Height)/2
			labelCenter := row.label.Position().Y + row.label.Size().Height/2
			if gap != 1 {
				t.Fatalf("nano cell %d row %d usage/reset gap=%.2f, want 1", cellIndex, rowIndex, gap)
			}
			if labelCenter != bundleCenter {
				t.Fatalf("nano cell %d row %d label/bundle centers=%.2f/%.2f", cellIndex, rowIndex, labelCenter, bundleCenter)
			}
			// The row line is wrapped in a Stack with its hover region (W12);
			// the lines container sits one level above that wrapper.
			line := findContainerWithObject(v.Nano, row.bar)
			wrapper := findContainerWithObject(v.Nano, line)
			stack := findContainerWithObject(v.Nano, wrapper)
			linesLayout, ok := stack.Layout.(*NanoLinesLayout)
			if !ok || linesLayout.Gap != NanoLineGap {
				t.Fatalf("nano cell %d row %d inter-line gap changed: layout=%T gap=%v", cellIndex, rowIndex, stack.Layout, linesLayout)
			}
		}
	}
	t.Logf("nano usage/gap/reset=%.0fpx/%.0fpx/%.0fpx; bundle centered on 5h/7D labels; inter-line gap=%.0fpx", NanoUsageBarHeight, NanoResetGap, NanoResetBarHeight, NanoLineGap)
}

func TestPhase3WUsageHeadersAlignLocalizeAndUpdateImmediately(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()

	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))
	firstNormalRow := v.normalCache.rows[0].row
	if len(v.normalHeaderWrap.Objects) != 1 || v.normalHeaderWrap.Objects[0] != v.normalCache.columnHeader || firstNormalRow == nil {
		t.Fatal("normal column header strip is not mounted in the header wrap")
	}
	if v.normalCache.usageHeader.Text != "Usage" || v.normalCache.resetHeader.Text != "Reset" {
		t.Fatalf("English normal headers=%q/%q, want Usage/Reset", v.normalCache.usageHeader.Text, v.normalCache.resetHeader.Text)
	}
	if v.normalCache.usageHeader.TextSize != UsageHeaderTextSize || v.normalCache.resetHeader.TextSize != UsageHeaderTextSize {
		t.Fatalf("normal header fonts=%.1f/%.1f, want %.1f", v.normalCache.usageHeader.TextSize, v.normalCache.resetHeader.TextSize, UsageHeaderTextSize)
	}
	if v.normalCache.usageHeader.Position().X != firstNormalRow.Objects[1].Position().X ||
		v.normalCache.resetHeader.Position().X != firstNormalRow.Objects[2].Position().X {
		t.Fatalf("normal header x usage/reset=%.1f/%.1f, columns=%.1f/%.1f",
			v.normalCache.usageHeader.Position().X, v.normalCache.resetHeader.Position().X,
			firstNormalRow.Objects[1].Position().X, firstNormalRow.Objects[2].Position().X)
	}
	if !sameColor(v.normalCache.usageHeader.Color, v.colors.Secondary) || !sameColor(v.normalCache.resetHeader.Color, v.colors.Secondary) {
		t.Fatal("normal headers do not use Secondary color")
	}
	normalUsageX := v.normalCache.usageHeader.Position().X
	normalResetX := v.normalCache.resetHeader.Position().X

	v.Show(CompactScreen)
	window.Resize(v.MinimumSize(CompactScreen))
	firstCompactRow := compactTestRowContent(t, compactTestRows(v)[0])
	if len(v.compactHeaderWrap.Objects) != 1 || v.compactHeaderWrap.Objects[0] != v.compactCache.columnHeader || v.compactCache.usageHeader.Text != "Usage" {
		t.Fatalf("compact header wrap=%d text=%q", len(v.compactHeaderWrap.Objects), v.compactCache.usageHeader.Text)
	}
	if v.compactCache.resetHeader.Text != "Reset" || v.compactCache.resetHeader.Alignment != fyne.TextAlignTrailing {
		t.Fatalf("compact reset header=%q alignment=%v, want Reset trailing", v.compactCache.resetHeader.Text, v.compactCache.resetHeader.Alignment)
	}
	if v.compactCache.usageHeader.Position().X != firstCompactRow.Objects[2].Position().X ||
		v.compactCache.usageHeader.Size().Width != firstCompactRow.Objects[2].Size().Width ||
		v.compactCache.usageHeader.Alignment != fyne.TextAlignCenter {
		t.Fatalf("compact header/meter geometry pos=%v/%v size=%v/%v alignment=%v",
			v.compactCache.usageHeader.Position(), firstCompactRow.Objects[2].Position(),
			v.compactCache.usageHeader.Size(), firstCompactRow.Objects[2].Size(), v.compactCache.usageHeader.Alignment)
	}
	if !sameColor(v.compactCache.usageHeader.Color, v.colors.Secondary) {
		t.Fatal("compact header does not use Secondary color")
	}
	compactUsageX := v.compactCache.usageHeader.Position().X
	compactUsageWidth := v.compactCache.usageHeader.Size().Width

	normalCache := v.normalCache
	compactCache := v.compactCache
	cfg := v.config
	cfg.UsageMode = settings.UsageRemaining
	v.SetConfig(cfg)
	if v.normalCache != normalCache || v.compactCache != compactCache {
		t.Fatal("usage-mode-only change rebuilt stable normal/compact caches")
	}
	if v.normalCache.usageHeader.Text != "Remaining" || v.compactCache.usageHeader.Text != "Remaining" {
		t.Fatalf("immediate English remaining headers normal/compact=%q/%q", v.normalCache.usageHeader.Text, v.compactCache.usageHeader.Text)
	}

	cfg = v.config
	cfg.Language = settings.Language(i18n.Korean)
	v.SetConfig(cfg)
	if v.normalCache.usageHeader.Text != "잔여량" || v.compactCache.usageHeader.Text != "잔여량" || v.normalCache.resetHeader.Text != "재설정" {
		t.Fatalf("Korean remaining/reset headers normal/compact/reset=%q/%q/%q", v.normalCache.usageHeader.Text, v.compactCache.usageHeader.Text, v.normalCache.resetHeader.Text)
	}
	nanoCache := v.nanoCache
	cfg = v.config
	cfg.UsageMode = settings.UsageUsed
	v.SetConfig(cfg)
	if v.normalCache.usageHeader.Text != "사용량" || v.compactCache.usageHeader.Text != "사용량" || v.nanoCache != nanoCache {
		t.Fatalf("immediate Korean usage headers normal/compact=%q/%q nano-cache-stable=%t", v.normalCache.usageHeader.Text, v.compactCache.usageHeader.Text, v.nanoCache == nanoCache)
	}
	for cellIndex, cell := range v.nanoCache.cells {
		for rowIndex, row := range cell.rows {
			if row.label.Text != "5h" && row.label.Text != "7D" {
				t.Fatalf("nano cell %d row %d gained header label %q", cellIndex, rowIndex, row.label.Text)
			}
		}
	}

	if v.normalCache.columnHeader.MinSize().Height < UsageHeaderRowHeight || v.compactCache.columnHeader.MinSize().Height < UsageHeaderRowHeight ||
		v.MinimumSize(NormalScreen).Height != v.Normal.MinSize().Height || v.MinimumSize(CompactScreen).Height != v.Compact.MinSize().Height {
		t.Fatalf("header/window heights normal=%v/%v compact=%v/%v header-min=%.1f/%.1f",
			v.MinimumSize(NormalScreen), v.Normal.MinSize(), v.MinimumSize(CompactScreen), v.Compact.MinSize(),
			v.normalCache.columnHeader.MinSize().Height, v.compactCache.columnHeader.MinSize().Height)
	}
	t.Logf("headers 10pt Secondary; normal x meter/reset=%.0f/%.0f; compact meter x/width=%.0f/%.0f trailing; i18n Usage/Remaining/Reset and 사용량/잔여량/재설정 update immediately",
		normalUsageX, normalResetX, compactUsageX, compactUsageWidth)
}

func TestPhase3WSoftwareRenderCapturesShowUsageHeaders(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3W_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3W_SCREENSHOT_DIR for Phase 3W captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	v, window := phase2DTestView(t)
	defer window.Close()
	v.SetState(phase3VAllHundredState())
	for _, mode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark} {
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(mode))
		cfg := v.config
		cfg.Theme = mode
		cfg.Language = settings.Language(i18n.Korean)
		cfg.UsageMode = settings.UsageUsed
		v.SetConfig(cfg)
		for _, screen := range []struct {
			name   string
			mode   Screen
			width  float32
			root   func() *fyne.Container
			labels func() []fyne.CanvasObject
		}{
			{"normal", NormalScreen, NormalWidth, func() *fyne.Container { return v.Normal }, func() []fyne.CanvasObject {
				return []fyne.CanvasObject{v.normalCache.usageHeader, v.normalCache.resetHeader}
			}},
			{"compact", CompactScreen, CompactWidth, func() *fyne.Container { return v.Compact }, func() []fyne.CanvasObject {
				return []fyne.CanvasObject{v.compactCache.usageHeader}
			}},
		} {
			v.Show(screen.mode)
			window.Resize(v.MinimumSize(screen.mode))
			output := window.Canvas().Capture()
			if output.Bounds().Dx() != int(screen.width) || output.Bounds().Dy() != int(math.Ceil(float64(v.MinimumSize(screen.mode).Height))) {
				t.Fatalf("%s %s software render size=%v minimum=%v", mode, screen.name, output.Bounds(), v.MinimumSize(screen.mode))
			}
			ink := 0
			for labelIndex, label := range screen.labels() {
				labelInk, ok := phase3WHeaderInk(output, screen.root(), label, v.colors.Background)
				if !ok || labelInk == 0 {
					t.Fatalf("%s %s header %d is not visibly rasterized: found=%t ink=%d", mode, screen.name, labelIndex, ok, labelInk)
				}
				ink += labelInk
			}
			if screen.mode == CompactScreen {
				for index, handles := range v.compactCache.rows {
					position, ok := objectPosition(v.Compact, handles.symbol, fyne.NewPos(0, 0))
					if !ok {
						t.Fatalf("%s row %d percent symbol not found", mode, index)
					}
					clearance := float32(output.Bounds().Dx()) - (position.X + handles.symbol.Size().Width)
					if clearance < CompactPaddingRight+CompactPercentMargin {
						t.Fatalf("%s row %d right clearance=%.2f", mode, index, clearance)
					}
				}
			}
			path := filepath.Join(directory, string(mode)+"-"+screen.name+".png")
			if err := writePhase3VPNG(path, output); err != nil {
				t.Fatal(err)
			}
			t.Logf("software render %s: %dx%d header-ink=%d pixels", path, output.Bounds().Dx(), output.Bounds().Dy(), ink)
		}
	}
}

func phase3WHeaderInk(source image.Image, root *fyne.Container, target fyne.CanvasObject, background color.NRGBA) (int, bool) {
	position, ok := objectPosition(root, target, fyne.NewPos(0, 0))
	if !ok {
		return 0, false
	}
	bounds := source.Bounds()
	left := max(bounds.Min.X, int(math.Floor(float64(position.X))))
	top := max(bounds.Min.Y, int(math.Floor(float64(position.Y))))
	right := min(bounds.Max.X, int(math.Ceil(float64(position.X+target.Size().Width))))
	bottom := min(bounds.Max.Y, int(math.Ceil(float64(position.Y+target.Size().Height))))
	ink := 0
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			difference := max(int(pixel.R)-int(background.R), int(background.R)-int(pixel.R)) +
				max(int(pixel.G)-int(background.G), int(background.G)-int(pixel.G)) +
				max(int(pixel.B)-int(background.B), int(background.B)-int(pixel.B))
			if difference > 12 {
				ink++
			}
		}
	}
	return ink, true
}

func phase3WHex(value color.NRGBA) string {
	return "#" + phase3WHexByte(value.R) + phase3WHexByte(value.G) + phase3WHexByte(value.B)
}

func phase3WHexByte(value uint8) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[value>>4], digits[value&0x0F]})
}

func phase3WContrastRatio(foreground, background color.NRGBA) float64 {
	left := phase3WLuminance(foreground)
	right := phase3WLuminance(background)
	if left < right {
		left, right = right, left
	}
	return (left + 0.05) / (right + 0.05)
}

func phase3WLuminance(value color.NRGBA) float64 {
	channel := func(component uint8) float64 {
		linear := float64(component) / 255
		if linear <= 0.04045 {
			return linear / 12.92
		}
		return math.Pow((linear+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(value.R) + 0.7152*channel(value.G) + 0.0722*channel(value.B)
}
