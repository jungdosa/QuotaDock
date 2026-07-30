// Package ui implements the three QuotaDock Fyne screens from PLAN §25.5.
package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// BrandColors contains every custom canvas color that does not automatically
// follow Fyne's theme. Keeping the sets immutable lets light and dark views be
// rebuilt without package-global theme state.
type BrandColors struct {
	Background         color.NRGBA
	SettingsBackground color.NRGBA
	Card               color.NRGBA
	CardBorder         color.NRGBA
	TitleTop           color.NRGBA
	TitleBottom        color.NRGBA
	TitleDivider       color.NRGBA
	Accent             color.NRGBA
	ToggleOn           color.NRGBA
	Danger             color.NRGBA
	Label              color.NRGBA
	Secondary          color.NRGBA
	Text               color.NRGBA
	PercentNormal      color.NRGBA
	Track              color.NRGBA
	Connected          color.NRGBA
	Disconnected       color.NRGBA
	PlanChip           color.NRGBA
	PlanChipText       color.NRGBA
	ControlThumb       color.NRGBA
	IconText           color.NRGBA
	RadioBorder        color.NRGBA
	palette            map[string]color.NRGBA
}

var (
	ColorBackground    = color.NRGBA{R: 0x0C, G: 0x15, B: 0x21, A: 0xFF}
	ColorTitleTop      = color.NRGBA{R: 0x20, G: 0x36, B: 0x4F, A: 0xFF}
	ColorTitleBottom   = color.NRGBA{R: 0x18, G: 0x2B, B: 0x41, A: 0xFF}
	ColorAccent        = color.NRGBA{R: 0x6E, G: 0x9C, B: 0xE8, A: 0xFF}
	ColorDanger        = color.NRGBA{R: 0xE0, G: 0x65, B: 0x6C, A: 0xFF}
	ColorLabel         = color.NRGBA{R: 0x91, G: 0xA4, B: 0xB8, A: 0xFF}
	ColorSecondary     = color.NRGBA{R: 0x75, G: 0x88, B: 0x9C, A: 0xFF}
	ColorText          = color.NRGBA{R: 0xE6, G: 0xED, B: 0xF5, A: 0xFF}
	ColorPercentNormal = color.NRGBA{R: 0xE2, G: 0xEA, B: 0xF3, A: 0xFF}
	ColorTrack         = color.NRGBA{R: 0x25, G: 0x34, B: 0x45, A: 0xFF}
	ColorConnected     = color.NRGBA{R: 0x5F, G: 0xAF, B: 0x86, A: 0xFF}
	ColorDisconnected  = color.NRGBA{R: 0x61, G: 0x70, B: 0x82, A: 0xFF}
	ColorPlanChip      = color.NRGBA{R: 0x78, G: 0x8C, B: 0xA3, A: 0xFF}
	ColorPlanChipText  = color.NRGBA{R: 0x0C, G: 0x15, B: 0x21, A: 0xFF}
)

var darkPaletteColors = map[string]color.NRGBA{
	"slate": {0x7E, 0x8F, 0xA6, 0xFF}, "gray": {0x9A, 0xA7, 0xB7, 0xFF},
	"red": {0xE0, 0x65, 0x6C, 0xFF}, "orange": {0xDD, 0x9A, 0x63, 0xFF},
	"amber": {0xE0, 0xA9, 0x4E, 0xFF}, "yellow": {0xD7, 0xBD, 0x5A, 0xFF},
	"lime": {0x9B, 0xBE, 0x67, 0xFF}, "green": {0x5F, 0xB8, 0x8A, 0xFF},
	"emerald": {0x4F, 0xB3, 0x97, 0xFF}, "teal": {0x45, 0xA9, 0xA5, 0xFF},
	"cyan": {0x4F, 0xA6, 0xC1, 0xFF}, "sky": {0x5B, 0x9C, 0xCB, 0xFF},
	"blue": {0x5B, 0x8D, 0xEF, 0xFF}, "indigo": {0x7C, 0x8A, 0xED, 0xFF},
	"violet": {0x9B, 0x8C, 0xEC, 0xFF}, "purple": {0xA7, 0x8B, 0xD8, 0xFF},
}

var lightPaletteColors = map[string]color.NRGBA{
	"slate": {0x5F, 0x70, 0x86, 0xFF}, "gray": {0x72, 0x7E, 0x90, 0xFF},
	"red": {0xC9, 0x4F, 0x5A, 0xFF}, "orange": {0xB8, 0x6B, 0x3D, 0xFF},
	"amber": {0xA6, 0x6C, 0x16, 0xFF}, "yellow": {0x91, 0x73, 0x20, 0xFF},
	"lime": {0x69, 0x82, 0x3B, 0xFF}, "green": {0x39, 0x84, 0x5E, 0xFF},
	"emerald": {0x33, 0x86, 0x78, 0xFF}, "teal": {0x2F, 0x7E, 0x86, 0xFF},
	"cyan": {0x34, 0x7F, 0x9C, 0xFF}, "sky": {0x3E, 0x78, 0xA8, 0xFF},
	"blue": {0x3F, 0x6F, 0xBE, 0xFF}, "indigo": {0x5D, 0x63, 0xB3, 0xFF},
	"violet": {0x75, 0x60, 0xB0, 0xFF}, "purple": {0x87, 0x5B, 0xA0, 0xFF},
}

var DarkBrandColors = BrandColors{
	Background:         ColorBackground,
	SettingsBackground: ColorBackground,
	Card:               ColorTitleBottom,
	CardBorder:         color.NRGBA{R: 0x42, G: 0x57, B: 0x70, A: 0xC0},
	TitleTop:           ColorTitleTop,
	TitleBottom:        ColorTitleBottom,
	TitleDivider:       color.NRGBA{R: 0x3F, G: 0x66, B: 0x8B, A: 0xD0},
	Accent:             ColorAccent,
	ToggleOn:           color.NRGBA{R: 0x60, G: 0xA5, B: 0xFA, A: 0xC8},
	Danger:             ColorDanger,
	Label:              ColorLabel,
	Secondary:          ColorSecondary,
	Text:               ColorText,
	PercentNormal:      ColorPercentNormal,
	Track:              ColorTrack,
	Connected:          ColorConnected,
	Disconnected:       ColorDisconnected,
	PlanChip:           ColorPlanChip,
	PlanChipText:       ColorPlanChipText,
	ControlThumb:       ColorText,
	IconText:           ColorBackground,
	RadioBorder:        color.NRGBA{R: 0x74, G: 0x87, B: 0x9E, A: 0xFF},
	palette:            darkPaletteColors,
}

// LightBrandColors follows the approved concept B light mockup: cool light
// surfaces, dark text, darker severity colors, and a darkened 16-color palette.
var LightBrandColors = BrandColors{
	Background:         color.NRGBA{R: 0xF1, G: 0xF5, B: 0xFA, A: 0xFF},
	SettingsBackground: color.NRGBA{R: 0xED, G: 0xF2, B: 0xF8, A: 0xFF},
	Card:               color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	CardBorder:         color.NRGBA{R: 0x87, G: 0x9B, B: 0xB2, A: 0x52},
	TitleTop:           color.NRGBA{R: 0xE3, G: 0xED, B: 0xF8, A: 0xFF},
	TitleBottom:        color.NRGBA{R: 0xD6, G: 0xE4, B: 0xF3, A: 0xFF},
	TitleDivider:       color.NRGBA{R: 0x87, G: 0xA6, B: 0xC8, A: 0xB8},
	Accent:             color.NRGBA{R: 0x2E, G: 0x6F, B: 0xD6, A: 0xFF},
	ToggleOn:           color.NRGBA{R: 0x3B, G: 0x82, B: 0xF6, A: 0xCC},
	Danger:             color.NRGBA{R: 0xC9, G: 0x4F, B: 0x5A, A: 0xFF},
	Label:              color.NRGBA{R: 0x46, G: 0x58, B: 0x6C, A: 0xFF},
	Secondary:          color.NRGBA{R: 0x66, G: 0x75, B: 0x87, A: 0xFF},
	Text:               color.NRGBA{R: 0x1D, G: 0x2B, B: 0x3A, A: 0xFF},
	PercentNormal:      color.NRGBA{R: 0x1D, G: 0x2B, B: 0x3A, A: 0xFF},
	Track:              color.NRGBA{R: 0xD7, G: 0xE1, B: 0xEC, A: 0xFF},
	Connected:          color.NRGBA{R: 0x39, G: 0x84, B: 0x5E, A: 0xFF},
	Disconnected:       color.NRGBA{R: 0x7C, G: 0x89, B: 0x99, A: 0xFF},
	PlanChip:           color.NRGBA{R: 0x59, G: 0x6D, B: 0x84, A: 0xFF},
	PlanChipText:       color.NRGBA{R: 0xF7, G: 0xFA, B: 0xFB, A: 0xFF},
	ControlThumb:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	IconText:           color.NRGBA{R: 0xF7, G: 0xFA, B: 0xFB, A: 0xFF},
	RadioBorder:        color.NRGBA{R: 0x7C, G: 0x89, B: 0x99, A: 0xFF},
	palette:            lightPaletteColors,
}

func BrandColorsForTheme(mode settings.Theme, variant fyne.ThemeVariant) BrandColors {
	if mode == settings.ThemeLight || mode == settings.ThemeSystem && variant == theme.VariantLight {
		return LightBrandColors
	}
	return DarkBrandColors
}

func (c BrandColors) PaletteColor(id string) color.Color {
	if value, ok := c.palette[id]; ok {
		return value
	}
	return c.palette["slate"]
}

func PaletteColor(id string) color.Color {
	return DarkBrandColors.PaletteColor(id)
}
