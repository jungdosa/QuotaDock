package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"image/color"
	"math"
)

type SegmentedMeter struct {
	widget.BaseWidget
	Segments           int
	AdaptiveMax        int
	SegmentTargetWidth float32
	SquareSegments     bool
	RenderedSegments   int
	RenderedWidth      float32
	RenderedGap        float32
	RenderedTrailing   float32
	Value              float64
	Active             color.Color
	Track              color.Color
	Height             float32
	Gap                float32
	OnTapped           func()
}

func NewSegmentedMeter(segments int, value float64, active color.Color, track ...color.Color) *SegmentedMeter {
	if segments <= 0 {
		segments = 20
	}
	trackColor := color.Color(ColorTrack)
	if len(track) > 0 {
		trackColor = track[0]
	}
	m := &SegmentedMeter{Segments: segments, Value: value, Active: active, Track: trackColor, Height: 11, Gap: 2}
	m.ExtendBaseWidget(m)
	return m
}
func (m *SegmentedMeter) SetAdaptiveSegments(maximum int, targetWidth float32) {
	if maximum < m.Segments {
		maximum = m.Segments
	}
	m.AdaptiveMax = maximum
	m.SegmentTargetWidth = targetWidth
}
func (m *SegmentedMeter) SetSquareSegments(maximum int) {
	if maximum < 1 {
		maximum = 1
	}
	m.SquareSegments = true
	m.AdaptiveMax = maximum
	m.SegmentTargetWidth = m.Height
}
func (m *SegmentedMeter) SetValue(value float64, active color.Color) {
	value = clampPercent(value)
	if m.Value == value && sameColor(m.Active, active) {
		return
	}
	m.Value = value
	m.Active = active
	m.Refresh()
}
func (m *SegmentedMeter) Tapped(*fyne.PointEvent) {
	if m.OnTapped != nil {
		m.OnTapped()
	}
}
func (m *SegmentedMeter) CreateRenderer() fyne.WidgetRenderer {
	count := m.Segments
	if m.AdaptiveMax > count {
		count = m.AdaptiveMax
	}
	objects := make([]fyne.CanvasObject, count)
	for i := range objects {
		objects[i] = canvas.NewRectangle(m.Track)
	}
	return &meterRenderer{meter: m, objects: objects}
}

var _ fyne.Tappable = (*SegmentedMeter)(nil)

type meterRenderer struct {
	meter   *SegmentedMeter
	objects []fyne.CanvasObject
}

func (r *meterRenderer) Destroy()                     {}
func (r *meterRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *meterRenderer) MinSize() fyne.Size {
	if r.meter.SquareSegments {
		return fyne.NewSize(r.meter.Height, r.meter.Height)
	}
	segmentWidth := float32(4)
	if r.meter.AdaptiveMax > r.meter.Segments {
		segmentWidth = 1
	}
	return fyne.NewSize(float32(r.meter.Segments)*segmentWidth+r.meter.Gap*float32(r.meter.Segments-1), r.meter.Height)
}
func (r *meterRenderer) Layout(size fyne.Size) {
	count := r.meter.Segments
	width := float32(0)
	gap := r.meter.Gap
	if r.meter.SquareSegments {
		target := r.meter.Height + r.meter.Gap
		if target > 0 {
			count = int(math.Round(float64((size.Width + r.meter.Gap) / target)))
		}
		count = min(len(r.objects), max(1, count))
		width = r.meter.Height
		if count > 1 {
			gap = (size.Width - width*float32(count)) / float32(count-1)
			for count > 1 && gap < 1 {
				count--
				gap = (size.Width - width*float32(count)) / float32(count-1)
			}
		} else {
			gap = 0
		}
	} else if r.meter.AdaptiveMax > count && r.meter.SegmentTargetWidth > 0 {
		fitted := int((size.Width + r.meter.Gap) / (r.meter.SegmentTargetWidth + r.meter.Gap))
		count = min(r.meter.AdaptiveMax, max(count, fitted))
	}
	r.meter.RenderedSegments = count
	if !r.meter.SquareSegments {
		width = (size.Width - gap*float32(count-1)) / float32(count)
		if width < 1 {
			width = 1
		}
	}
	r.meter.RenderedWidth = width
	r.meter.RenderedGap = gap
	r.meter.RenderedTrailing = max(float32(0), size.Width-(width*float32(count)+gap*float32(max(0, count-1))))
	for i, o := range r.objects {
		if i >= count {
			o.Hide()
			continue
		}
		o.Show()
		o.Move(fyne.NewPos(float32(i)*(width+gap), (size.Height-r.meter.Height)/2))
		o.Resize(fyne.NewSize(width, r.meter.Height))
	}
	r.Refresh()
}
func (r *meterRenderer) Refresh() {
	count := r.meter.RenderedSegments
	if count == 0 {
		count = r.meter.Segments
	}
	on := int(math.Ceil(clampPercent(r.meter.Value) / 100 * float64(count)))
	for i, o := range r.objects {
		rect := o.(*canvas.Rectangle)
		rect.FillColor = r.meter.Track
		if i < on {
			rect.FillColor = r.meter.Active
		}
		rect.Refresh()
	}
}
func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// SlimProgressBar renders a thin progress track in compact and nano views.
// It is retained by the view caches and updated in place.
type SlimProgressBar struct {
	widget.BaseWidget
	Value  float64
	Active color.Color
	Track  color.Color
	Height float32
}

func NewSlimProgressBar(value float64, active, track color.Color) *SlimProgressBar {
	bar := &SlimProgressBar{Value: clampPercent(value), Active: active, Track: track, Height: 1}
	bar.ExtendBaseWidget(bar)
	return bar
}

func (b *SlimProgressBar) SetValue(value float64, active, track color.Color) {
	value = clampPercent(value)
	if b.Value == value && sameColor(b.Active, active) && sameColor(b.Track, track) {
		return
	}
	b.Value = value
	b.Active = active
	b.Track = track
	b.Refresh()
}

func (b *SlimProgressBar) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(b.Track)
	active := canvas.NewRectangle(b.Active)
	return &slimProgressRenderer{bar: b, track: track, active: active, objects: []fyne.CanvasObject{track, active}}
}

type slimProgressRenderer struct {
	bar     *SlimProgressBar
	track   *canvas.Rectangle
	active  *canvas.Rectangle
	objects []fyne.CanvasObject
}

func (r *slimProgressRenderer) Destroy()                     {}
func (r *slimProgressRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *slimProgressRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, r.bar.Height) }
func (r *slimProgressRenderer) Layout(size fyne.Size) {
	r.track.Resize(fyne.NewSize(size.Width, r.bar.Height))
	r.active.Resize(fyne.NewSize(size.Width*float32(clampPercent(r.bar.Value)/100), r.bar.Height))
}
func (r *slimProgressRenderer) Refresh() {
	r.track.FillColor = r.bar.Track
	r.active.FillColor = r.bar.Active
	r.track.Refresh()
	r.active.Refresh()
	r.Layout(r.bar.Size())
}
