package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"golang.org/x/image/vector"
)

const themeIconRenderSize = 160

// themeModeResource provides solid, hand-drawn silhouettes for the 16px
// title-bar theme control. Rendering at 8x the 20x20 design grid preserves
// filled edges when Fyne scales the resource down to its fixed 16px slot.
func themeModeResource(mode settings.Theme, colors BrandColors) fyne.Resource {
	icon := rasterizeThemeModeIcon(mode, colors.Label)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, icon); err != nil {
		return fyne.NewStaticResource("theme-"+string(mode)+".png", nil)
	}
	return fyne.NewStaticResource("theme-"+string(mode)+".png", encoded.Bytes())
}

func rasterizeThemeModeIcon(mode settings.Theme, fill color.Color) image.Image {
	switch mode {
	case settings.ThemeDark:
		return rasterizeThemeMoon(fill)
	case settings.ThemeSystem:
		return rasterizeThemeSystem(fill)
	default:
		return rasterizeThemeSun(fill)
	}
}

func rasterizeThemeSun(fill color.Color) image.Image {
	bounds := image.Rect(0, 0, themeIconRenderSize, themeIconRenderSize)
	destination := image.NewRGBA(bounds)
	raster := vector.NewRasterizer(themeIconRenderSize, themeIconRenderSize)
	// A separate centre disc and eight short, rounded rays read as a sun at
	// 16px. The previous alternating-radius polygon collapsed into a sharp star.
	drawThemeCircle(raster, 10, 10, 4)
	for index := 0; index < 8; index++ {
		drawThemeSunRay(raster, -math.Pi/2+float64(index)*math.Pi/4)
	}
	raster.Draw(destination, bounds, image.NewUniform(fill), image.Point{})
	return destination
}

func rasterizeThemeMoon(fill color.Color) image.Image {
	outer := themeCircleMask(9.1, 10.2, 7.5)
	cutout := themeCircleMask(14.8, 7.8, 6.8)
	return themeMaskedImage(fill, outer, func(x, y int, alpha uint8) uint8 {
		removed := cutout.AlphaAt(x, y).A
		if removed >= alpha {
			return 0
		}
		return alpha - removed
	})
}

func rasterizeThemeSystem(fill color.Color) image.Image {
	outer := themeCircleMask(10, 10, 7.3)
	inner := themeCircleMask(10, 10, 4.7)
	centerX := themeIconRenderSize / 2
	return themeMaskedImage(fill, outer, func(x, y int, alpha uint8) uint8 {
		if x < centerX {
			return alpha
		}
		removed := inner.AlphaAt(x, y).A
		if removed >= alpha {
			return 0
		}
		return alpha - removed
	})
}

func themeCircleMask(centerX, centerY, radius float64) *image.Alpha {
	bounds := image.Rect(0, 0, themeIconRenderSize, themeIconRenderSize)
	mask := image.NewAlpha(bounds)
	raster := vector.NewRasterizer(themeIconRenderSize, themeIconRenderSize)
	drawThemeCircle(raster, centerX, centerY, radius)
	raster.Draw(mask, bounds, image.NewUniform(color.Alpha{A: 0xFF}), image.Point{})
	return mask
}

func themeMaskedImage(fill color.Color, mask *image.Alpha, adjust func(int, int, uint8) uint8) image.Image {
	bounds := mask.Bounds()
	destination := image.NewNRGBA(bounds)
	tint := color.NRGBAModel.Convert(fill).(color.NRGBA)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			alpha := adjust(x, y, mask.AlphaAt(x, y).A)
			alpha = uint8(uint16(alpha) * uint16(tint.A) / 0xFF)
			if alpha != 0 {
				destination.SetNRGBA(x, y, color.NRGBA{R: tint.R, G: tint.G, B: tint.B, A: alpha})
			}
		}
	}
	return destination
}

func drawThemeSunRay(raster *vector.Rasterizer, angle float64) {
	const (
		innerRadius = 5.4
		outerRadius = 8.2
		halfWidth   = 0.8
	)
	unitX, unitY := math.Cos(angle), math.Sin(angle)
	perpendicularX, perpendicularY := -unitY*halfWidth, unitX*halfWidth
	startX, startY := 10+unitX*innerRadius, 10+unitY*innerRadius
	endX, endY := 10+unitX*outerRadius, 10+unitY*outerRadius
	scale := float32(themeIconRenderSize) / 20
	raster.MoveTo(float32(startX+perpendicularX)*scale, float32(startY+perpendicularY)*scale)
	raster.LineTo(float32(endX+perpendicularX)*scale, float32(endY+perpendicularY)*scale)
	raster.LineTo(float32(endX-perpendicularX)*scale, float32(endY-perpendicularY)*scale)
	raster.LineTo(float32(startX-perpendicularX)*scale, float32(startY-perpendicularY)*scale)
	raster.ClosePath()
	drawThemeCircle(raster, startX, startY, halfWidth)
	drawThemeCircle(raster, endX, endY, halfWidth)
}

func drawThemeCircle(raster *vector.Rasterizer, centerX, centerY, radius float64) {
	const circleBezier = 0.5522847498307936
	scale := float32(themeIconRenderSize) / 20
	cx := float32(centerX) * scale
	cy := float32(centerY) * scale
	r := float32(radius) * scale
	k := r * circleBezier
	raster.MoveTo(cx+r, cy)
	raster.CubeTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	raster.CubeTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	raster.CubeTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	raster.CubeTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	raster.ClosePath()
}
