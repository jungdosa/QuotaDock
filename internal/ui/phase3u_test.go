package ui

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase3USVGPathParserSupportsFullCommandGrammar(t *testing.T) {
	source := "M1.5-2.5 3e0,4E+0 l1 2 H8 v3 C8 9 9 10 10 10 s2-1 3 0 Q14 12 15 10 t2-2 A2.5 1.5 30 0119 10 a2 2 0 10-2 0 z"
	parsed, err := parseSVGPathData(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.subpaths) != 1 || len(parsed.subpaths[0].segments) < 12 || !parsed.subpaths[0].closed {
		t.Fatalf("parsed path subpaths=%d segments=%d closed=%t", len(parsed.subpaths), len(parsed.subpaths[0].segments), parsed.subpaths[0].closed)
	}
	hasArcCubic := false
	for _, segment := range parsed.subpaths[0].segments {
		if segment.command == 'C' {
			hasArcCubic = true
		}
	}
	if !hasArcCubic {
		t.Fatal("arc commands were not converted to cubic Beziers")
	}
	bounds := parsed.bounds()
	if !bounds.valid || math.IsNaN(bounds.MinX) || math.IsNaN(bounds.MaxY) {
		t.Fatalf("invalid parser bounds %+v", bounds)
	}
}

func TestPhase3UOfficialProviderPathsGeometryInkAndDistinctMasks(t *testing.T) {
	tests := []struct {
		kind  ProviderIconKind
		name  string
		asset string
		color color.NRGBA
	}{
		{ProviderIconClaude, "Claude", "claude.svg", color.NRGBA{R: 0xD9, G: 0x77, B: 0x57, A: 0xFF}},
		{ProviderIconCodex, "Codex", "openai.svg", color.NRGBA{R: 0x1F, G: 0x23, B: 0x28, A: 0xFF}},
		{ProviderIconGemini, "Gemini", "gemini.svg", color.NRGBA{R: 0x8E, G: 0x75, B: 0xFF, A: 0xFF}},
		{ProviderIconAGClaude, "AG Claude", "claude.svg", color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}},
	}
	renders := make(map[ProviderIconKind]image.Image, len(tests))
	for _, test := range tests {
		imageValue := providerIconImage(test.kind)
		metrics := providerIconMetrics[test.kind]
		renders[test.kind] = imageValue
		t.Logf("%s asset=%s paths/subpaths=%d/%d bounds=[%.3f %.3f %.3f %.3f] ink=%.2f%% quadrants=[%.2f %.2f %.2f %.2f] ink-bounds=%v evenodd-diff=%d fallback=%t",
			test.name, metrics.Asset, metrics.PathCount, metrics.SubpathCount,
			metrics.Bounds.MinX, metrics.Bounds.MinY, metrics.Bounds.MaxX, metrics.Bounds.MaxY,
			metrics.InkCoverage, metrics.Quadrants[0], metrics.Quadrants[1], metrics.Quadrants[2], metrics.Quadrants[3],
			metrics.InkBounds, metrics.EvenOddDifferentPixels, !providerIconOfficialVerified(test.kind))
		if metrics.Asset != test.asset || !providerIconOfficialVerified(test.kind) {
			t.Fatalf("%s official path gate failed: asset=%q error=%q metrics=%+v", test.name, metrics.Asset, metrics.Error, metrics)
		}
		if metrics.InkCoverage < 15 || metrics.InkCoverage > 40 || !providerPathRasterPasses(metrics) {
			t.Fatalf("%s official path metrics failed gate: %+v", test.name, metrics)
		}
		if test.kind == ProviderIconCodex && metrics.EvenOddDifferentPixels == 0 {
			t.Fatalf("%s evenodd mask unexpectedly matches non-zero rendering", test.name)
		}
		if !hasApproximateColor(imageValue, test.color, 3) {
			t.Fatalf("%s raster does not contain expected color %v", test.name, test.color)
		}
	}
	for left := 0; left < len(tests); left++ {
		for right := left + 1; right < len(tests); right++ {
			sameMask := sameAlphaMask(renders[tests[left].kind], renders[tests[right].kind])
			claudeColorPair := (tests[left].kind == ProviderIconClaude && tests[right].kind == ProviderIconAGClaude) ||
				(tests[left].kind == ProviderIconAGClaude && tests[right].kind == ProviderIconClaude)
			if claudeColorPair && !sameMask {
				t.Fatalf("%s and %s must share the official Claude silhouette", tests[left].name, tests[right].name)
			}
			if !claudeColorPair && sameMask {
				t.Fatalf("%s and %s official alpha masks are identical", tests[left].name, tests[right].name)
			}
		}
	}
}

func TestPhase3UCompactMeterAndWidthBudget(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	measured100 := compactPercentTextWidth()
	for _, language := range []settings.Language{settings.Language(i18n.English), settings.Language(i18n.Korean)} {
		cfg := v.config
		cfg.Language = language
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		window.Resize(v.MinimumSize(CompactScreen))
		budget := compactLayoutWidthBudget(v.compactCache.labelWidth)
		if budget.Total > CompactWidth || budget.RequiredTotal > CompactWidth || measured100+CompactPercentMargin > budget.Percent {
			t.Fatalf("%s compact budget total/required/window=%.2f/%.2f/%.2f 100%%+margin/column=%.2f+%.2f/%.2f",
				language, budget.Total, budget.RequiredTotal, CompactWidth, measured100, CompactPercentMargin, budget.Percent)
		}
		for index, row := range compactTestRows(v) {
			content := compactTestRowContent(t, row)
			meter := v.compactCache.rows[index].meter
			if !meter.SquareSegments || meter.AdaptiveMax != 32 || meter.Gap != CompactMeterGap {
				t.Fatalf("%s row %d compact meter square=%t segments=%d..%d gap=%.1f", language, index, meter.SquareSegments, meter.Segments, meter.AdaptiveMax, meter.Gap)
			}
			renderer := meter.CreateRenderer().(*meterRenderer)
			renderer.Layout(content.Objects[2].Size())
			if meter.RenderedSegments < 1 || meter.RenderedSegments > meter.AdaptiveMax {
				t.Fatalf("%s row %d rendered segments=%d", language, index, meter.RenderedSegments)
			}
			visible := renderer.Objects()[:meter.RenderedSegments]
			if visible[0].Size().Width != CompactMeterHeight || visible[0].Size().Height != CompactMeterHeight {
				t.Fatalf("%s row %d segment size=%v, want %.0f square", language, index, visible[0].Size(), CompactMeterHeight)
			}
			for segment := 1; segment < len(visible); segment++ {
				gap := visible[segment].Position().X - (visible[segment-1].Position().X + visible[segment-1].Size().Width)
				if gap < 1 {
					t.Fatalf("%s row %d segment %d gap=%.2f, want visible", language, index, segment, gap)
				}
			}
			percentColumn := content.Objects[3]
			if percentColumn.Size().Width != compactPercentColumnWidth() || percentColumn.Position().X+percentColumn.Size().Width > content.Size().Width {
				t.Fatalf("%s row %d percent x/width/content=%.2f/%.2f/%.2f", language, index, percentColumn.Position().X, percentColumn.Size().Width, content.Size().Width)
			}
		}
		t.Logf("%s compact widths icon/label/meter(min)/percent/gaps/padding/required/total/window=%.0f/%.0f/%.0f(%.0f)/%.0f/%.0f/%.0f/%.0f/%.0f/%.0f; 100%%=%.2f margin=%.0f right-padding=%.0f segments=%d",
			language, budget.Icon, budget.Label, budget.Meter, budget.MeterMinimum, budget.Percent, budget.Gaps, budget.Padding,
			budget.RequiredTotal, budget.Total, CompactWidth, measured100, CompactPercentMargin, CompactPaddingRight, v.compactCache.rows[0].meter.RenderedSegments)
	}
	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))
	for index, row := range v.normalCache.rows {
		if row.meter.Segments != 20 || row.meter.AdaptiveMax != 0 || row.meter.Gap != 2 {
			t.Fatalf("normal row %d meter changed: segments=%d adaptive=%d gap=%.1f", index, row.meter.Segments, row.meter.AdaptiveMax, row.meter.Gap)
		}
	}
}

func TestPhase3USoftwareRenderCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3U_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3U_SCREENSHOT_DIR for Phase 3U captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	v, window := phase2DTestView(t)
	defer window.Close()
	v.SetState(DemoViewState())
	for _, themeMode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark} {
		cfg := v.config
		cfg.Theme = themeMode
		cfg.Language = settings.Language(i18n.Korean)
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(themeMode))
		v.SetConfig(cfg)
		for _, entry := range []struct {
			name string
			mode Screen
		}{{"compact", CompactScreen}, {"nano", NanoScreen}} {
			v.Show(entry.mode)
			window.Resize(v.MinimumSize(entry.mode))
			output := window.Canvas().Capture()
			if output.Bounds().Dx() == 0 || output.Bounds().Dy() == 0 {
				t.Fatalf("empty %s %s software render", themeMode, entry.name)
			}
			path := filepath.Join(directory, string(themeMode)+"-"+entry.name+".png")
			file, err := os.Create(path)
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
			t.Logf("software render %s: %dx%d", path, output.Bounds().Dx(), output.Bounds().Dy())
		}
	}
}
