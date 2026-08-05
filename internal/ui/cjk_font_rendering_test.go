package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestCJKLocaleTextInkCaptures(t *testing.T) {
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	outputDirectory := os.Getenv("QUOTADOCK_PHASE4K_SCREENSHOT_DIR")
	if outputDirectory != "" {
		if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	glyphSamples := map[i18n.Language]string{
		i18n.Japanese:           "日月火水木金土",
		i18n.ChineseSimplified:  "语言设置连接",
		i18n.ChineseTraditional: "語言設定連線",
	}
	for _, language := range []i18n.Language{i18n.Japanese, i18n.ChineseSimplified, i18n.ChineseTraditional} {
		t.Run(string(language), func(t *testing.T) {
			app := test.NewApp()
			app.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
			t.Cleanup(app.Quit)

			background := canvas.NewRectangle(DarkBrandColors.Background)
			sample := i18n.Endonym(language) + " · " +
				catalog.Text(language, i18n.English, i18n.KeySettingsTitle) + " · " +
				catalog.Text(language, i18n.English, i18n.KeyHelpTitle)
			text := canvas.NewText(sample, DarkBrandColors.Text)
			text.TextSize = 18
			content := container.NewStack(background, container.NewPadded(text))
			window := test.NewWindow(content)
			window.SetPadded(false)
			window.Resize(content.MinSize())
			t.Cleanup(window.Close)

			capture := window.Canvas().Capture()
			ratio := phase4KTextInkRatio(capture, DarkBrandColors.Background)
			t.Logf("%s CJK text ink ratio=%.4f size=%v", language, ratio, capture.Bounds().Size())
			// Isolating a text-only surface prevents icons and meters from
			// inflating the measurement. Missing glyphs render below this gate.
			if ratio < 0.01 || ratio > 0.35 {
				t.Fatalf("%s CJK text ink ratio %.4f is outside 0.01..0.35", language, ratio)
			}

			// A missing-font tofu box has ink too, so compare isolated glyph masks.
			// Every chosen rune is visually distinct; identical masks mean fallback failed.
			signatures := make(map[uint64]struct{})
			glyphCount := 0
			for _, glyph := range glyphSamples[language] {
				glyphCount++
				glyphBackground := canvas.NewRectangle(DarkBrandColors.Background)
				glyphText := canvas.NewText(string(glyph), DarkBrandColors.Text)
				glyphText.TextSize = 24
				glyphContent := container.NewStack(glyphBackground, container.NewCenter(glyphText))
				glyphWindow := test.NewWindow(glyphContent)
				glyphWindow.SetPadded(false)
				glyphWindow.Resize(fyne.NewSize(48, 48))
				glyphCapture := glyphWindow.Canvas().Capture()
				glyphWindow.Close()
				if glyphRatio := phase4KTextInkRatio(glyphCapture, DarkBrandColors.Background); glyphRatio < 0.01 {
					t.Fatalf("%s glyph %q ink ratio %.4f is too low", language, glyph, glyphRatio)
				}
				signatures[phase4KGlyphInkSignature(glyphCapture, DarkBrandColors.Background)] = struct{}{}
			}
			if len(signatures) != glyphCount {
				t.Fatalf("%s has %d distinct glyph masks for %d runes; system font fallback produced tofu", language, len(signatures), glyphCount)
			}

			if outputDirectory != "" {
				file, err := os.Create(filepath.Join(outputDirectory, string(language)+"-text-ink.png"))
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(file, capture); err != nil {
					file.Close()
					t.Fatal(err)
				}
				if err = file.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func phase4KTextInkRatio(value image.Image, background color.Color) float64 {
	base := color.NRGBAModel.Convert(background).(color.NRGBA)
	ink := 0
	total := value.Bounds().Dx() * value.Bounds().Dy()
	if total == 0 {
		return 0
	}
	for y := value.Bounds().Min.Y; y < value.Bounds().Max.Y; y++ {
		for x := value.Bounds().Min.X; x < value.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(value.At(x, y)).(color.NRGBA)
			delta := absColorDifference(pixel.R, base.R) +
				absColorDifference(pixel.G, base.G) +
				absColorDifference(pixel.B, base.B)
			if pixel.A > 0x20 && delta > 24 {
				ink++
			}
		}
	}
	return float64(ink) / float64(total)
}

func phase4KGlyphInkSignature(value image.Image, background color.Color) uint64 {
	base := color.NRGBAModel.Convert(background).(color.NRGBA)
	const offset64 = uint64(1469598103934665603)
	const prime64 = uint64(1099511628211)
	hash := offset64
	for y := value.Bounds().Min.Y; y < value.Bounds().Max.Y; y++ {
		for x := value.Bounds().Min.X; x < value.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(value.At(x, y)).(color.NRGBA)
			delta := absColorDifference(pixel.R, base.R) +
				absColorDifference(pixel.G, base.G) +
				absColorDifference(pixel.B, base.B)
			if pixel.A > 0x20 && delta > 24 {
				hash ^= 1
			}
			hash *= prime64
		}
	}
	return hash
}

func absColorDifference(left, right uint8) int {
	if left > right {
		return int(left - right)
	}
	return int(right - left)
}
