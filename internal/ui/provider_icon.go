package ui

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	providerassets "github.com/jungdosa/QuotaDock/assets/providers"
	"github.com/jungdosa/QuotaDock/internal/settings"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vector"
)

// ProviderIconKind identifies the provider logo rendered in compact mode.
type ProviderIconKind string

const (
	ProviderIconClaude   ProviderIconKind = "claude"
	ProviderIconCodex    ProviderIconKind = "codex"
	ProviderIconGemini   ProviderIconKind = "antigravity-gemini"
	ProviderIconAGClaude ProviderIconKind = "antigravity-claude"
)

const providerIconSize float32 = 16
const providerIconRenderSize = 216
const providerIconSupersample = 4

var (
	providerIconResourcesOnce sync.Once
	providerIconResources     map[ProviderIconKind]fyne.Resource
	providerIconImages        map[ProviderIconKind]image.Image
	providerIconMetrics       map[ProviderIconKind]providerPathMetrics
	providerIconOfficial      map[ProviderIconKind]bool
)

// NewProviderIcon uses the official SVG path raster when it passes the
// geometry and software-raster gates. A failed logo falls back independently.
func NewProviderIcon(kind ProviderIconKind, modes ...settings.Theme) *canvas.Image {
	mode := settings.ThemeSystem
	if len(modes) > 0 {
		mode = modes[0]
	}
	icon := canvas.NewImageFromImage(providerIconImageForTheme(kind, mode))
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(providerIconSize, providerIconSize))
	return icon
}


func newNativeProviderIcon(kind ProviderIconKind) *canvas.Image {
	icon := canvas.NewImageFromResource(providerIconResource(kind))
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(providerIconSize, providerIconSize))
	return icon
}

func providerIconOfficialVerified(kind ProviderIconKind) bool {
	providerIconResource(kind)
	return providerIconOfficial[kind]
}

// providerIconNativeVerified remains false: Fyne's native SVG path still uses
// the broken oksvg route measured in Phase 3T. Phase 3U uses the direct parser.
func providerIconNativeVerified(ProviderIconKind) bool { return false }

func providerIconAsset(kind ProviderIconKind) string {
	switch kind {
	case ProviderIconClaude:
		return "claude.svg"
	case ProviderIconCodex:
		return "openai.svg"
	case ProviderIconGemini:
		return "gemini.svg"
	case ProviderIconAGClaude:
		return "claude.svg"
	default:
		return "claude-color.svg"
	}
}

func providerIconTint(kind ProviderIconKind) string {
	switch kind {
	case ProviderIconClaude:
		return "#D97757"
	case ProviderIconCodex:
		return "#1F2328"
	case ProviderIconGemini:
		return "#8E75FF"
	case ProviderIconAGClaude:
		return "#6B7280"
	default:
		return ""
	}
}

func providerIconTintForTheme(kind ProviderIconKind, mode settings.Theme, variant fyne.ThemeVariant) string {
	if kind != ProviderIconCodex {
		return providerIconTint(kind)
	}
	if mode == settings.ThemeDark || mode == settings.ThemeSystem && variant == theme.VariantDark {
		return "#E6E8EB"
	}
	return "#1F2328"
}

func providerIconResource(kind ProviderIconKind) fyne.Resource {
	providerIconResourcesOnce.Do(func() {
		providerIconResources = make(map[ProviderIconKind]fyne.Resource, 4)
		providerIconImages = make(map[ProviderIconKind]image.Image, 4)
		providerIconMetrics = make(map[ProviderIconKind]providerPathMetrics, 4)
		providerIconOfficial = make(map[ProviderIconKind]bool, 4)
		for _, candidate := range []ProviderIconKind{
			ProviderIconClaude,
			ProviderIconCodex,
			ProviderIconGemini,
			ProviderIconAGClaude,
		} {
			name := providerIconAsset(candidate)
			data, err := providerassets.Read(name)
			if err != nil {
				data = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"/>`)
			}
			imageValue, metrics, renderErr := rasterizeOfficialProvider(candidate, data)
			metrics.Asset = name
			if renderErr != nil {
				metrics.Error = renderErr.Error()
			}
			if renderErr == nil && providerPathRasterPasses(metrics) {
				providerIconImages[candidate] = imageValue
				providerIconOfficial[candidate] = true
			} else {
				providerIconImages[candidate] = rasterizeProviderMark(candidate)
			}
			providerIconMetrics[candidate] = metrics
			data = normalizeProviderSVG(data, providerIconTint(candidate))
			resourceName := "provider-" + string(candidate) + ".svg"
			providerIconResources[candidate] = fyne.NewStaticResource(resourceName, data)
		}
	})
	if resource := providerIconResources[kind]; resource != nil {
		return resource
	}
	return providerIconResources[ProviderIconClaude]
}

func normalizeProviderSVG(data []byte, tint string) []byte {
	source := strings.NewReplacer(
		`height="1em"`, `height="24"`,
		`width="1em"`, `width="24"`,
	).Replace(string(data))
	if tint != "" {
		source = strings.ReplaceAll(source, "currentColor", tint)
	}
	return []byte(source)
}

func providerIconImage(kind ProviderIconKind) image.Image {
	providerIconResource(kind)
	if icon := providerIconImages[kind]; icon != nil {
		return icon
	}
	return providerIconImages[ProviderIconClaude]
}

func providerIconImageForTheme(kind ProviderIconKind, mode settings.Theme) image.Image {
	source := providerIconImage(kind)
	if kind != ProviderIconCodex {
		return source
	}
	variant := theme.VariantLight
	if app := fyne.CurrentApp(); app != nil {
		variant = app.Settings().ThemeVariant()
	}
	tint, err := parseProviderIconColor(providerIconTintForTheme(kind, mode, variant))
	if err != nil {
		return source
	}
	result := image.NewNRGBA(source.Bounds())
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			result.SetNRGBA(x, y, color.NRGBA{R: tint.R, G: tint.G, B: tint.B, A: uint8(alpha >> 8)})
		}
	}
	return result
}

type providerPathMetrics struct {
	Asset                  string
	PathCount              int
	SubpathCount           int
	Bounds                 svgBounds
	ViewBox                svgViewBox
	EvenOdd                bool
	EvenOddDifferentPixels int
	InkCoverage            float64
	Quadrants              [4]float64
	InkBounds              image.Rectangle
	Error                  string
}

type providerSVGDocument struct {
	ViewBox  string            `xml:"viewBox,attr"`
	FillRule string            `xml:"fill-rule,attr"`
	Paths    []providerSVGPath `xml:"path"`
}

type providerSVGPath struct {
	Data     string `xml:"d,attr"`
	FillRule string `xml:"fill-rule,attr"`
}

func rasterizeOfficialProvider(kind ProviderIconKind, data []byte) (image.Image, providerPathMetrics, error) {
	metrics := providerPathMetrics{}
	document := providerSVGDocument{}
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, metrics, fmt.Errorf("parse provider SVG XML: %w", err)
	}
	viewBox, err := parseProviderViewBox(document.ViewBox)
	if err != nil {
		return nil, metrics, err
	}
	metrics.ViewBox = viewBox
	if len(document.Paths) == 0 {
		return nil, metrics, errors.New("provider SVG contains no paths")
	}
	metrics.PathCount = len(document.Paths)
	highSize := int(providerIconSize) * providerIconSupersample
	combined := image.NewAlpha(image.Rect(0, 0, highSize, highSize))
	for pathIndex, sourcePath := range document.Paths {
		parsed, parseErr := parseSVGPathData(sourcePath.Data)
		if parseErr != nil {
			return nil, metrics, fmt.Errorf("parse provider path %d: %w", pathIndex, parseErr)
		}
		metrics.SubpathCount += len(parsed.subpaths)
		metrics.Bounds.merge(parsed.bounds())
		fillRule := sourcePath.FillRule
		if fillRule == "" {
			fillRule = document.FillRule
		}
		evenOdd := strings.EqualFold(fillRule, "evenodd")
		metrics.EvenOdd = metrics.EvenOdd || evenOdd
		mask := renderSVGPathMask(parsed, viewBox, highSize, evenOdd)
		if evenOdd {
			nonZero := renderSVGPathMask(parsed, viewBox, highSize, false)
			for index := range mask.Pix {
				if mask.Pix[index] != nonZero.Pix[index] {
					metrics.EvenOddDifferentPixels++
				}
			}
		}
		compositeAlphaMask(combined, mask)
	}
	fill, err := providerIconColor(kind)
	if err != nil {
		return nil, metrics, err
	}
	highResolution := image.NewNRGBA(combined.Bounds())
	for y := combined.Bounds().Min.Y; y < combined.Bounds().Max.Y; y++ {
		for x := combined.Bounds().Min.X; x < combined.Bounds().Max.X; x++ {
			alpha := combined.AlphaAt(x, y).A
			highResolution.SetNRGBA(x, y, color.NRGBA{R: fill.R, G: fill.G, B: fill.B, A: alpha})
		}
	}
	resultSize := int(providerIconSize)
	result := image.NewNRGBA(image.Rect(0, 0, resultSize, resultSize))
	xdraw.CatmullRom.Scale(result, result.Bounds(), highResolution, highResolution.Bounds(), xdraw.Src, nil)
	measureProviderRaster(result, &metrics)
	return result, metrics, nil
}

func parseProviderViewBox(source string) (svgViewBox, error) {
	parts := strings.Fields(strings.ReplaceAll(source, ",", " "))
	if len(parts) != 4 {
		return svgViewBox{}, fmt.Errorf("invalid provider viewBox %q", source)
	}
	values := [4]float64{}
	for index, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return svgViewBox{}, fmt.Errorf("parse viewBox value %q: %w", part, err)
		}
		values[index] = value
	}
	if values[2] <= 0 || values[3] <= 0 {
		return svgViewBox{}, fmt.Errorf("provider viewBox has non-positive size %q", source)
	}
	return svgViewBox{MinX: values[0], MinY: values[1], Width: values[2], Height: values[3]}, nil
}

func providerIconColor(kind ProviderIconKind) (color.NRGBA, error) {
	return parseProviderIconColor(providerIconTint(kind))
}

func parseProviderIconColor(source string) (color.NRGBA, error) {
	if len(source) != 7 || source[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("invalid provider tint %q", source)
	}
	value, err := strconv.ParseUint(source[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("parse provider tint %q: %w", source, err)
	}
	return color.NRGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 0xFF}, nil
}

func compositeAlphaMask(destination, source *image.Alpha) {
	for index, alpha := range source.Pix {
		previous := int(destination.Pix[index])
		next := int(alpha)
		destination.Pix[index] = uint8(previous + (255-previous)*next/255)
	}
}

func measureProviderRaster(source image.Image, metrics *providerPathMetrics) {
	bounds := source.Bounds()
	inkBounds := image.Rectangle{}
	var totalAlpha uint64
	var quadrants [4]uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha16 := source.At(x, y).RGBA()
			alpha := uint64(alpha16)
			if alpha == 0 {
				continue
			}
			point := image.Pt(x, y)
			if inkBounds.Empty() {
				inkBounds = image.Rectangle{Min: point, Max: point.Add(image.Pt(1, 1))}
			} else {
				inkBounds = inkBounds.Union(image.Rectangle{Min: point, Max: point.Add(image.Pt(1, 1))})
			}
			totalAlpha += alpha
			quadrant := 0
			if x >= bounds.Min.X+bounds.Dx()/2 {
				quadrant++
			}
			if y >= bounds.Min.Y+bounds.Dy()/2 {
				quadrant += 2
			}
			quadrants[quadrant] += alpha
		}
	}
	metrics.InkBounds = inkBounds
	metrics.InkCoverage = float64(totalAlpha) * 100 / float64(bounds.Dx()*bounds.Dy()*0xFFFF)
	if totalAlpha > 0 {
		for index, alpha := range quadrants {
			metrics.Quadrants[index] = float64(alpha) * 100 / float64(totalAlpha)
		}
	}
}

func providerPathRasterPasses(metrics providerPathMetrics) bool {
	if metrics.Error != "" || metrics.PathCount == 0 || metrics.SubpathCount == 0 || !metrics.Bounds.valid {
		return false
	}
	if metrics.InkCoverage < 15 || metrics.InkCoverage > 40 || metrics.InkBounds.Empty() {
		return false
	}
	viewMaxX := metrics.ViewBox.MinX + metrics.ViewBox.Width
	viewMaxY := metrics.ViewBox.MinY + metrics.ViewBox.Height
	const tolerance = 0.05
	if metrics.Bounds.MinX < metrics.ViewBox.MinX-tolerance || metrics.Bounds.MinY < metrics.ViewBox.MinY-tolerance ||
		metrics.Bounds.MaxX > viewMaxX+tolerance || metrics.Bounds.MaxY > viewMaxY+tolerance ||
		metrics.Bounds.MaxX-metrics.Bounds.MinX < metrics.ViewBox.Width/2 ||
		metrics.Bounds.MaxY-metrics.Bounds.MinY < metrics.ViewBox.Height/2 {
		return false
	}
	center := image.Pt(int(providerIconSize)/2, int(providerIconSize)/2)
	if center.X < metrics.InkBounds.Min.X || center.X >= metrics.InkBounds.Max.X ||
		center.Y < metrics.InkBounds.Min.Y || center.Y >= metrics.InkBounds.Max.Y {
		return false
	}
	for _, share := range metrics.Quadrants {
		if share < 3 {
			return false
		}
	}
	return true
}

func rasterizeProviderMark(kind ProviderIconKind) image.Image {
	switch kind {
	case ProviderIconCodex:
		return rasterizeCodexMark()
	case ProviderIconGemini:
		return rasterizeGeminiMark()
	case ProviderIconAGClaude:
		return rasterizeClaudeMark(color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF})
	default:
		return rasterizeClaudeMark(color.NRGBA{R: 0xD9, G: 0x77, B: 0x57, A: 0xFF})
	}
}

func rasterizeClaudeMark(fill color.NRGBA) image.Image {
	destination := image.NewRGBA(image.Rect(0, 0, providerIconRenderSize, providerIconRenderSize))
	raster := vector.NewRasterizer(providerIconRenderSize, providerIconRenderSize)
	drawRadialPolygon(raster, 12, 10.5, 5, -math.Pi/2)
	raster.Draw(destination, destination.Bounds(), image.NewUniform(fill), image.Point{})
	return destination
}

func rasterizeCodexMark() image.Image {
	bounds := image.Rect(0, 0, providerIconRenderSize, providerIconRenderSize)
	outer := image.NewAlpha(bounds)
	raster := vector.NewRasterizer(providerIconRenderSize, providerIconRenderSize)
	drawRadialPolygon(raster, 6, 10.2, 7.2, -math.Pi/2)
	raster.Draw(outer, bounds, image.NewUniform(color.Alpha{A: 0xFF}), image.Point{})

	hole := image.NewAlpha(bounds)
	raster.Reset(providerIconRenderSize, providerIconRenderSize)
	drawRegularPolygon(raster, 6, 3.25, math.Pi/6)
	raster.Draw(hole, bounds, image.NewUniform(color.Alpha{A: 0xFF}), image.Point{})

	destination := image.NewRGBA(bounds)
	for y := 0; y < providerIconRenderSize; y++ {
		for x := 0; x < providerIconRenderSize; x++ {
			alpha := outer.AlphaAt(x, y).A
			cutout := hole.AlphaAt(x, y).A
			if cutout >= alpha {
				continue
			}
			destination.Set(x, y, color.NRGBA{R: 0x1F, G: 0x23, B: 0x28, A: alpha - cutout})
		}
	}
	return destination
}

func rasterizeGeminiMark() image.Image {
	destination := image.NewRGBA(image.Rect(0, 0, providerIconRenderSize, providerIconRenderSize))
	raster := vector.NewRasterizer(providerIconRenderSize, providerIconRenderSize)
	scale := float32(providerIconRenderSize) / 24
	raster.MoveTo(12*scale, 1*scale)
	raster.CubeTo(13*scale, 6.7*scale, 17.3*scale, 11*scale, 23*scale, 12*scale)
	raster.CubeTo(17.3*scale, 13*scale, 13*scale, 17.3*scale, 12*scale, 23*scale)
	raster.CubeTo(11*scale, 17.3*scale, 6.7*scale, 13*scale, 1*scale, 12*scale)
	raster.CubeTo(6.7*scale, 11*scale, 11*scale, 6.7*scale, 12*scale, 1*scale)
	raster.ClosePath()
	raster.Draw(destination, destination.Bounds(), image.NewUniform(color.NRGBA{R: 0x8E, G: 0x75, B: 0xFF, A: 0xFF}), image.Point{})
	return destination
}

func drawRadialPolygon(raster *vector.Rasterizer, lobes int, outerRadius, innerRadius, rotation float64) {
	points := lobes * 2
	for index := 0; index < points; index++ {
		radius := outerRadius
		if index%2 == 1 {
			radius = innerRadius
		}
		angle := rotation + float64(index)*math.Pi/float64(lobes)
		x := float32(12+math.Cos(angle)*radius) * float32(providerIconRenderSize) / 24
		y := float32(12+math.Sin(angle)*radius) * float32(providerIconRenderSize) / 24
		if index == 0 {
			raster.MoveTo(x, y)
		} else {
			raster.LineTo(x, y)
		}
	}
	raster.ClosePath()
}

func drawRegularPolygon(raster *vector.Rasterizer, sides int, radius, rotation float64) {
	scale := float32(providerIconRenderSize) / 24
	for index := 0; index < sides; index++ {
		angle := rotation + float64(index)*2*math.Pi/float64(sides)
		x := float32(12+math.Cos(angle)*radius) * scale
		y := float32(12+math.Sin(angle)*radius) * scale
		if index == 0 {
			raster.MoveTo(x, y)
		} else {
			raster.LineTo(x, y)
		}
	}
	raster.ClosePath()
}
