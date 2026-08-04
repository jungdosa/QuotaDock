package diagnostics

import (
	"image"
	"image/color"
	"testing"
)

func filled(width, height int, fill color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, fill)
		}
	}
	return img
}

func TestBlankRatioReportsFlatImageAsFullyBlank(t *testing.T) {
	img := filled(120, 80, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	if ratio := BlankRatio(img, 4); ratio != 1 {
		t.Fatalf("flat image ratio=%v, want 1", ratio)
	}
}

// A painted window carries text, meters and icons. Even a sparse 5% of ink has
// to drop the ratio clear of the blank threshold, or the watchdog would repaint
// windows that were fine.
func TestBlankRatioFallsBelowThresholdForPaintedContent(t *testing.T) {
	background := color.RGBA{R: 240, G: 244, B: 248, A: 255}
	ink := color.RGBA{R: 30, G: 40, B: 50, A: 255}
	img := filled(200, 100, background)
	for y := 0; y < 100; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, ink)
		}
	}
	ratio := BlankRatio(img, 4)
	if ratio > 0.96 {
		t.Fatalf("painted image ratio=%v, want well below the blank threshold", ratio)
	}
}

// The title bar is a vertical gradient, so neighbouring rows differ by a step
// or two. Those steps must still count as one colour.
func TestBlankRatioToleratesGradientSteps(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		shade := uint8(240 + y%3)
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}
	if ratio := BlankRatio(img, 1); ratio != 1 {
		t.Fatalf("gradient image ratio=%v, want 1", ratio)
	}
}

func TestBlankRatioHandlesStrideLargerThanImage(t *testing.T) {
	img := filled(8, 8, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if ratio := BlankRatio(img, 512); ratio != 1 {
		t.Fatalf("oversized stride ratio=%v, want 1", ratio)
	}
	if ratio := BlankRatio(img, 0); ratio != 1 {
		t.Fatalf("zero stride ratio=%v, want 1", ratio)
	}
}

// Missing evidence must not read as "blank": returning 0 keeps the watchdog
// from repainting a window it never managed to look at.
func TestBlankRatioReturnsZeroWithoutEvidence(t *testing.T) {
	if ratio := BlankRatio(nil, 4); ratio != 0 {
		t.Fatalf("nil image ratio=%v, want 0", ratio)
	}
	if ratio := BlankRatio(image.NewRGBA(image.Rect(0, 0, 0, 0)), 4); ratio != 0 {
		t.Fatalf("empty image ratio=%v, want 0", ratio)
	}
}
