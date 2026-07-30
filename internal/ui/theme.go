package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	fontassets "github.com/jungdosa/QuotaDock/assets/fonts"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type BrandTheme struct {
	base     fyne.Theme
	font     fyne.Resource
	boldFont fyne.Resource
	mode     settings.Theme
}

var (
	unicodeFontOnce     sync.Once
	unicodeFont         fyne.Resource
	unicodeBoldFontOnce sync.Once
	unicodeBoldFont     fyne.Resource
)

func NewBrandTheme(mode settings.Theme) *BrandTheme {
	base := theme.DefaultTheme()
	if mode == settings.ThemeLight {
		base = theme.LightTheme()
	} else if mode == settings.ThemeDark {
		base = theme.DarkTheme()
	}
	font := fontassets.PretendardRegular()
	if font == nil {
		font = systemUnicodeFont()
	}
	boldFont := fontassets.PretendardBold()
	if boldFont == nil {
		boldFont = systemUnicodeBoldFont()
	}
	if boldFont == nil {
		boldFont = font
	}
	return &BrandTheme{base: base, font: font, boldFont: boldFont, mode: mode}
}
func (t *BrandTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	colors := BrandColorsForTheme(t.mode, variant)
	switch name {
	case theme.ColorNameBackground:
		return colors.Background
	case theme.ColorNamePrimary:
		return colors.Accent
	case theme.ColorNameForeground:
		return colors.Text
	case theme.ColorNameInputBackground:
		return colors.TitleTop
	case theme.ColorNameButton:
		return colors.TitleTop
	case theme.ColorNameSeparator:
		return colors.Track
	case theme.ColorNameDisabled:
		return colors.Secondary
	}
	return t.base.Color(name, variant)
}
func (t *BrandTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Bold && t.boldFont != nil {
		return t.boldFont
	}
	if t.font != nil {
		return t.font
	}
	return t.base.Font(style)
}
func (t *BrandTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return t.base.Icon(name) }
func (t *BrandTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 12
	case theme.SizeNameCaptionText:
		return 10
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInputBorder:
		return 1
	}
	return t.base.Size(name)
}

func currentBrandColors(mode settings.Theme) BrandColors {
	variant := theme.VariantDark
	if current := fyne.CurrentApp(); current != nil && current.Settings() != nil {
		variant = current.Settings().ThemeVariant()
	}
	return BrandColorsForTheme(mode, variant)
}

func systemUnicodeFont() fyne.Resource {
	unicodeFontOnce.Do(func() {
		unicodeFont = loadFirstFont(unicodeFontCandidates())
	})
	return unicodeFont
}

func systemUnicodeBoldFont() fyne.Resource {
	unicodeBoldFontOnce.Do(func() {
		unicodeBoldFont = loadFirstFont(unicodeBoldFontCandidates())
	})
	return unicodeBoldFont
}

func loadFirstFont(candidates []string) fyne.Resource {
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return fyne.NewStaticResource(filepath.Base(path), data)
		}
	}
	return nil
}

func unicodeFontCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		windowsDir := os.Getenv("WINDIR")
		if windowsDir == "" {
			windowsDir = "C:\\Windows"
		}
		return []string{filepath.Join(windowsDir, "Fonts", "malgun.ttf"), filepath.Join(windowsDir, "Fonts", "malgunsl.ttf")}
	case "darwin":
		return []string{"/System/Library/Fonts/AppleSDGothicNeo.ttc"}
	default:
		return []string{"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", "/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc"}
	}
}

func unicodeBoldFontCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		windowsDir := os.Getenv("WINDIR")
		if windowsDir == "" {
			windowsDir = "C:\\Windows"
		}
		return []string{filepath.Join(windowsDir, "Fonts", "malgunbd.ttf")}
	case "darwin":
		// AppleSDGothicNeo is a collection containing its weights. If it is
		// unavailable, NewBrandTheme falls back to the regular face.
		return []string{"/System/Library/Fonts/AppleSDGothicNeo.ttc"}
	default:
		return []string{"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc", "/usr/share/fonts/truetype/noto/NotoSansCJK-Bold.ttc"}
	}
}
