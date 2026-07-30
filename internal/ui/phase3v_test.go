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
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase3VAGClaudeUsesClaudePathWithGrayTint(t *testing.T) {
	mainImage := providerIconImage(ProviderIconClaude)
	agImage := providerIconImage(ProviderIconAGClaude)
	mainMetrics := providerIconMetrics[ProviderIconClaude]
	agMetrics := providerIconMetrics[ProviderIconAGClaude]
	mainColor := color.NRGBA{R: 0xD9, G: 0x77, B: 0x57, A: 0xFF}
	agColor := color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}

	if providerIconAsset(ProviderIconAGClaude) != "claude.svg" || agMetrics.Asset != "claude.svg" ||
		!providerIconOfficialVerified(ProviderIconAGClaude) {
		t.Fatalf("AG Claude did not pass the official claude.svg path gate: asset=%q metrics=%+v",
			providerIconAsset(ProviderIconAGClaude), agMetrics)
	}
	if !sameAlphaMask(mainImage, agImage) {
		t.Fatal("main Claude and AG Claude must use the same official sunburst alpha mask")
	}
	if !hasApproximateColor(mainImage, mainColor, 3) || !hasApproximateColor(agImage, agColor, 3) {
		t.Fatalf("Claude colors main/AG do not match %v/%v", mainColor, agColor)
	}
	colorDistance := math.Sqrt(
		math.Pow(float64(mainColor.R)-float64(agColor.R), 2) +
			math.Pow(float64(mainColor.G)-float64(agColor.G), 2) +
			math.Pow(float64(mainColor.B)-float64(agColor.B), 2),
	)
	t.Logf("AG Claude asset=%s same-alpha=true ink main/AG=%.2f%%/%.2f%% bounds main/AG=%v/%v tint main/AG=#D97757/#6B7280 RGB-distance=%.2f",
		agMetrics.Asset, mainMetrics.InkCoverage, agMetrics.InkCoverage, mainMetrics.InkBounds, agMetrics.InkBounds, colorDistance)
}

func TestPhase3VCompactPercentWidthBudgetAndRightClearance(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	v.SetState(phase3VAllHundredState())

	measured100 := compactPercentTextWidth()
	percentWidth := compactPercentColumnWidth()
	if measured100+CompactPercentMargin > percentWidth {
		t.Fatalf("100%% measured %.2f + margin %.2f exceeds percent column %.2f", measured100, CompactPercentMargin, percentWidth)
	}
	for _, language := range []settings.Language{settings.LanguageEnglish, settings.LanguageKorean} {
		cfg := v.config
		cfg.Language = language
		cfg.UsageMode = settings.UsageUsed
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		window.Resize(v.MinimumSize(CompactScreen))

		budget := compactLayoutWidthBudget(v.compactCache.labelWidth)
		if budget.RequiredTotal > CompactWidth || budget.Total > CompactWidth {
			t.Fatalf("%s compact required/allocated/window widths %.2f/%.2f/%.2f", language, budget.RequiredTotal, budget.Total, CompactWidth)
		}
		for index, row := range compactTestRows(v) {
			content := compactTestRowContent(t, row)
			percentColumn := content.Objects[3]
			handles := v.compactCache.rows[index]
			if handles.number.Text != "100" || handles.symbol.Text != "%" {
				t.Fatalf("%s row %d percent=%q/%q, want 100/%%", language, index, handles.number.Text, handles.symbol.Text)
			}
			internalRightClearance := percentColumn.Size().Width - (handles.symbol.Position().X + handles.symbol.Size().Width)
			windowRightClearance := CompactPaddingRight + internalRightClearance
			if internalRightClearance < CompactPercentMargin || windowRightClearance < CompactPaddingRight+CompactPercentMargin {
				t.Fatalf("%s row %d percent right clearance internal/window=%.2f/%.2f", language, index, internalRightClearance, windowRightClearance)
			}
		}
		t.Logf("%s compact icon/label/meter-min/gaps/percent/padding-left/right required<=window: %.0f+%.0f+%.0f+%.0f+%.0f+%.0f+%.0f=%.0f<=%.0f; allocated meter/total=%.0f/%.0f; MeasureText(100%%)=%.2f margin=%.0f column=%.0f internal-right=%.0f visual-right=%.0f",
			language, budget.Icon, budget.Label, budget.MeterMinimum, budget.Gaps, budget.Percent,
			CompactPaddingLeft, CompactPaddingRight, budget.RequiredTotal, CompactWidth, budget.Meter, budget.Total,
			measured100, CompactPercentMargin, percentWidth, CompactPercentMargin, CompactPercentMargin+CompactPaddingRight)
	}
}

func TestPhase3VCompactSoftwareRenderCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3V_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3V_SCREENSHOT_DIR for Phase 3V captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	v, window := phase2DTestView(t)
	defer window.Close()
	v.SetState(phase3VAllHundredState())

	for _, themeMode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark} {
		cfg := v.config
		cfg.Theme = themeMode
		cfg.Language = settings.LanguageKorean
		cfg.UsageMode = settings.UsageUsed
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(themeMode))
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		window.Resize(v.MinimumSize(CompactScreen))
		output := window.Canvas().Capture()
		if output.Bounds().Dx() != int(CompactWidth) || output.Bounds().Dy() == 0 {
			t.Fatalf("%s compact software render size=%v", themeMode, output.Bounds())
		}
		for index, handles := range v.compactCache.rows {
			position, ok := objectPosition(v.Compact, handles.symbol, fyne.NewPos(0, 0))
			if !ok {
				t.Fatalf("%s compact row %d percent position not found", themeMode, index)
			}
			clearance := float32(output.Bounds().Dx()) - (position.X + handles.symbol.Size().Width)
			if clearance < CompactPaddingRight+CompactPercentMargin {
				t.Fatalf("%s compact row %d 100%% right clearance=%.2f", themeMode, index, clearance)
			}
		}
		path := filepath.Join(directory, string(themeMode)+"-compact.png")
		if err := writePhase3VPNG(path, output); err != nil {
			t.Fatal(err)
		}
		t.Logf("software render %s: %dx%d, all 100%% rows right-clearance >= %.0fpx",
			path, output.Bounds().Dx(), output.Bounds().Dy(), CompactPaddingRight+CompactPercentMargin)
	}
}

func phase3VAllHundredState() ViewState {
	state := DemoViewState()
	for laneIndex := range state.Lanes {
		for rowIndex := range state.Lanes[laneIndex].Rows {
			state.Lanes[laneIndex].Rows[rowIndex].Percent = 100
		}
	}
	return state
}

func writePhase3VPNG(path string, source image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = png.Encode(file, source); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
