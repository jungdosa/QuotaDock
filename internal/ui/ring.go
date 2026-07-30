package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"image"
	"image/color"
	"math"
	"sync"
)

const RingSize float32 = 22
const RingStroke float64 = 2.6

type RingCandidate interface {
	fyne.CanvasObject
	SetPercent(float64, color.Color, color.Color)
	CandidateName() string
}

// RasterRing is the production renderer. Despite the retained public name, it
// caches one high-resolution canvas.Image and only replaces its pixels when the
// displayed value or colors change. Normal paints therefore allocate no images.
type RasterRing struct {
	widget.BaseWidget
	mu                     sync.RWMutex
	percent                float64
	foreground, colorTrack color.Color
	cached                 image.Image
	image                  *canvas.Image
}

func NewRasterRing(percent float64, foreground, track color.Color) *RasterRing {
	percent = clampPercent(percent)
	r := &RasterRing{
		percent:    percent,
		foreground: foreground,
		colorTrack: track,
		cached:     RenderRingImage(88, 88, percent, RingStroke*4, foreground, track),
	}
	r.ExtendBaseWidget(r)
	return r
}
func (r *RasterRing) CandidateName() string { return "cached canvas.Image" }
func (r *RasterRing) SetPercent(v float64, fg, track color.Color) {
	v = clampPercent(v)
	r.mu.Lock()
	if r.percent == v && sameColor(r.foreground, fg) && sameColor(r.colorTrack, track) {
		r.mu.Unlock()
		return
	}
	r.percent = v
	r.foreground = fg
	r.colorTrack = track
	r.cached = RenderRingImage(88, 88, v, RingStroke*4, fg, track)
	cached := r.cached
	canvasImage := r.image
	r.mu.Unlock()
	if canvasImage != nil {
		canvasImage.Image = cached
		canvasImage.Refresh()
	}
}
func (r *RasterRing) CreateRenderer() fyne.WidgetRenderer {
	r.mu.Lock()
	r.image = canvas.NewImageFromImage(r.cached)
	r.image.FillMode = canvas.ImageFillContain
	r.image.ScaleMode = canvas.ImageScaleSmooth
	canvasImage := r.image
	r.mu.Unlock()
	return &singleRenderer{object: canvasImage, min: fyne.NewSize(RingSize, RingSize)}
}

// FrameRing is candidate B: 101 pre-rendered 4x frames swapped in canvas.Image.
type FrameRing struct {
	widget.BaseWidget
	frames                 [101]image.Image
	current                int
	foreground, colorTrack color.Color
	image                  *canvas.Image
}

func NewFrameRing(percent float64, foreground, track color.Color) *FrameRing {
	r := &FrameRing{foreground: foreground, colorTrack: track}
	r.buildFrames()
	r.current = int(math.Round(clampPercent(percent)))
	r.ExtendBaseWidget(r)
	return r
}
func (r *FrameRing) CandidateName() string { return "101 pre-rendered frames" }
func (r *FrameRing) buildFrames() {
	for i := range r.frames {
		r.frames[i] = RenderRingImage(88, 88, float64(i), RingStroke*4, r.foreground, r.colorTrack)
	}
}
func (r *FrameRing) SetPercent(v float64, fg, track color.Color) {
	if !sameColor(fg, r.foreground) || !sameColor(track, r.colorTrack) {
		r.foreground = fg
		r.colorTrack = track
		r.buildFrames()
	}
	r.current = int(math.Round(clampPercent(v)))
	if r.image != nil {
		r.image.Image = r.frames[r.current]
		r.image.Refresh()
	}
}
func (r *FrameRing) CreateRenderer() fyne.WidgetRenderer {
	r.image = canvas.NewImageFromImage(r.frames[r.current])
	r.image.FillMode = canvas.ImageFillContain
	r.image.ScaleMode = canvas.ImageScaleSmooth
	return &singleRenderer{object: r.image, min: fyne.NewSize(RingSize, RingSize)}
}

// ExtensionRing is candidate C, implemented as an x/fyne-style extension widget.
// fyne.io/x/fyne v2026-07 has no arc/progress-ring widget, so the experiment uses
// the same BaseWidget extension contract with vector line segments.
type ExtensionRing struct {
	widget.BaseWidget
	percent                float64
	foreground, colorTrack color.Color
	segments               int
}

func NewExtensionRing(percent float64, foreground, track color.Color) *ExtensionRing {
	r := &ExtensionRing{percent: percent, foreground: foreground, colorTrack: track, segments: 72}
	r.ExtendBaseWidget(r)
	return r
}
func (r *ExtensionRing) CandidateName() string { return "extension widget (vector segments)" }
func (r *ExtensionRing) SetPercent(v float64, fg, track color.Color) {
	r.percent = clampPercent(v)
	r.foreground = fg
	r.colorTrack = track
	r.Refresh()
}
func (r *ExtensionRing) CreateRenderer() fyne.WidgetRenderer {
	lines := make([]fyne.CanvasObject, r.segments)
	for i := range lines {
		line := canvas.NewLine(r.colorTrack)
		line.StrokeWidth = float32(RingStroke)
		lines[i] = line
	}
	return &extensionRingRenderer{ring: r, objects: lines}
}

type extensionRingRenderer struct {
	ring    *ExtensionRing
	objects []fyne.CanvasObject
}

func (r *extensionRingRenderer) Destroy()                     {}
func (r *extensionRingRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *extensionRingRenderer) MinSize() fyne.Size           { return fyne.NewSize(RingSize, RingSize) }
func (r *extensionRingRenderer) Layout(size fyne.Size) {
	cx, cy := size.Width/2, size.Height/2
	radius := float32(math.Min(float64(size.Width), float64(size.Height)))/2 - float32(RingStroke)
	for i, o := range r.objects {
		a := -math.Pi/2 + 2*math.Pi*float64(i)/float64(r.ring.segments)
		dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
		line := o.(*canvas.Line)
		line.Position1 = fyne.NewPos(cx+dx*(radius-float32(RingStroke)/2), cy+dy*(radius-float32(RingStroke)/2))
		line.Position2 = fyne.NewPos(cx+dx*(radius+float32(RingStroke)/2), cy+dy*(radius+float32(RingStroke)/2))
	}
}
func (r *extensionRingRenderer) Refresh() {
	on := int(math.Round(clampPercent(r.ring.percent) / 100 * float64(r.ring.segments)))
	for i, o := range r.objects {
		line := o.(*canvas.Line)
		line.StrokeColor = r.ring.colorTrack
		if i < on {
			line.StrokeColor = r.ring.foreground
		}
		line.Refresh()
	}
}

type singleRenderer struct {
	object fyne.CanvasObject
	min    fyne.Size
}

func (r *singleRenderer) Destroy()                     {}
func (r *singleRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.object} }
func (r *singleRenderer) MinSize() fyne.Size           { return r.min }
func (r *singleRenderer) Layout(size fyne.Size)        { r.object.Resize(size) }
func (r *singleRenderer) Refresh()                     { canvas.Refresh(r.object) }

func RenderRingImage(w, h int, percent, stroke float64, foreground, track color.Color) image.Image {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w-1)/2, float64(h-1)/2
	radius := math.Min(float64(w), float64(h))/2 - stroke/2 - 0.5
	start := -math.Pi / 2
	end := start + 2*math.Pi*clampPercent(percent)/100
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			dist := math.Hypot(dx, dy)
			coverage := math.Max(0, math.Min(1, stroke/2+0.75-math.Abs(dist-radius)))
			if coverage == 0 {
				continue
			}
			angle := math.Atan2(dy, dx)
			if angle < start {
				angle += 2 * math.Pi
			}
			c := track
			if percent >= 100 || angle <= end {
				c = foreground
			}
			img.SetNRGBA(x, y, withAlpha(c, coverage))
		}
	}
	return img
}
func withAlpha(c color.Color, coverage float64) color.NRGBA {
	r, g, b, a := c.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(float64(a>>8) * coverage)}
}
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
