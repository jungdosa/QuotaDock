package ui

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/jungdosa/QuotaDock/internal/settings"
)

// laneGroupSpan marks the run of body objects one provider occupies. The body
// is a flat list of dividers, headers and rows rather than a container per
// provider, because nesting each group in its own box would insert the layout's
// padding between groups and change spacing the layout tests pin down. The span
// gives a drag the same grouping without touching the geometry.
type laneGroupSpan struct {
	id    string
	first int
	last  int
}

// laneBound is one provider's vertical extent inside the body it was measured
// from.
type laneBound struct {
	id     string
	top    float32
	bottom float32
}

// laneGroupBounds reads the spans back out of the laid-out body. It answers nil
// before the first layout, when every object still sits at the origin and no
// position would mean anything.
func laneGroupBounds(body *fyne.Container, spans []laneGroupSpan) []laneBound {
	if body == nil || len(spans) == 0 {
		return nil
	}
	bounds := make([]laneBound, 0, len(spans))
	for _, span := range spans {
		if span.first < 0 || span.first > span.last || span.last >= len(body.Objects) {
			return nil
		}
		top := body.Objects[span.first].Position().Y
		last := body.Objects[span.last]
		bottom := last.Position().Y + last.Size().Height
		if bottom <= top {
			return nil
		}
		bounds = append(bounds, laneBound{id: span.id, top: top, bottom: bottom})
	}
	return bounds
}

// laneAt reports which provider covers y. A pointer above the first group or
// below the last belongs to that end group rather than to nothing, so a drag
// that overshoots the list still lands somewhere.
func laneAt(bounds []laneBound, y float32) int {
	if len(bounds) == 0 {
		return -1
	}
	if y < bounds[0].top {
		return 0
	}
	for index, bound := range bounds {
		if y < bound.bottom {
			return index
		}
	}
	return len(bounds) - 1
}

// LaneReorderSurface turns a vertical drag over the provider list into a
// reordering. It is a sibling of the body rather than a handle inside each
// group: Fyne keeps delivering a drag to whichever object received its first
// event, and the body is rebuilt every time the order changes mid-drag, which
// would destroy a handle that lived in it.
//
// It implements Draggable and nothing else, so hovers, tooltips and taps still
// reach the rows underneath — the same arrangement nano already uses to keep
// its tooltips working beneath a drag surface.
type LaneReorderSurface struct {
	widget.BaseWidget
	OnDragStart func(y float32)
	OnDragMove  func(y float32)
	OnDragEnd   func()
	dragging    bool

	// plate is the translucent card that follows the pointer while a group is
	// being carried. Without it a reorder simply happened: the rows swapped
	// under a cursor that showed nothing being held, so there was no sign of
	// what had been picked up or where it was going.
	plateTop     float32
	plateHeight  float32
	plateVisible bool
	plateFill    color.Color
	plateStroke  color.Color
}

var _ fyne.Draggable = (*LaneReorderSurface)(nil)

func NewLaneReorderSurface(onDragStart, onDragMove func(y float32), onDragEnd func()) *LaneReorderSurface {
	s := &LaneReorderSurface{OnDragStart: onDragStart, OnDragMove: onDragMove, OnDragEnd: onDragEnd}
	s.ExtendBaseWidget(s)
	return s
}

// ShowPlate puts the carried card at top, height tall. It is called on every
// pointer move, so it repaints only when something actually changed.
func (s *LaneReorderSurface) ShowPlate(top, height float32, fill, stroke color.Color) {
	if s.plateVisible && s.plateTop == top && s.plateHeight == height {
		return
	}
	s.plateTop, s.plateHeight = top, height
	s.plateFill, s.plateStroke = fill, stroke
	s.plateVisible = true
	s.Refresh()
}

func (s *LaneReorderSurface) HidePlate() {
	if !s.plateVisible {
		return
	}
	s.plateVisible = false
	s.Refresh()
}

func (s *LaneReorderSurface) Dragged(event *fyne.DragEvent) {
	if event == nil {
		return
	}
	if !s.dragging {
		s.dragging = true
		if s.OnDragStart != nil {
			// The first event already carries the movement that started the
			// drag, so the provider the user grabbed is the one under where the
			// pointer was before it, not under where it has already reached.
			s.OnDragStart(event.Position.Y - event.Dragged.DY)
		}
	}
	if s.OnDragMove != nil {
		s.OnDragMove(event.Position.Y)
	}
}

func (s *LaneReorderSurface) DragEnd() {
	if !s.dragging {
		return
	}
	s.dragging = false
	if s.OnDragEnd != nil {
		s.OnDragEnd()
	}
}

// CreateRenderer asks for no space of its own. The surface is stacked directly
// on the body so the two share a coordinate space, and any minimum size here
// would become a floor under an empty provider list.
func (s *LaneReorderSurface) CreateRenderer() fyne.WidgetRenderer {
	plate := canvas.NewRectangle(color.Transparent)
	plate.CornerRadius = 6
	plate.StrokeWidth = 1
	plate.Hide()
	return &laneReorderRenderer{surface: s, plate: plate}
}

type laneReorderRenderer struct {
	surface *LaneReorderSurface
	plate   *canvas.Rectangle
}

func (r *laneReorderRenderer) Layout(size fyne.Size) { r.place(size) }

// place keeps the carried card inside the list. Dragging past either end would
// otherwise slide it off the window while the rows below it kept reacting.
func (r *laneReorderRenderer) place(size fyne.Size) {
	if !r.surface.plateVisible {
		r.plate.Hide()
		return
	}
	top := min(max(0, r.surface.plateTop), max(0, size.Height-r.surface.plateHeight))
	r.plate.Move(fyne.NewPos(0, top))
	r.plate.Resize(fyne.NewSize(size.Width, r.surface.plateHeight))
	r.plate.Show()
}

func (r *laneReorderRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }

func (r *laneReorderRenderer) Refresh() {
	r.plate.FillColor = r.surface.plateFill
	r.plate.StrokeColor = r.surface.plateStroke
	r.place(r.surface.Size())
	r.plate.Refresh()
}

func (r *laneReorderRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.plate} }
func (r *laneReorderRenderer) Destroy()                     {}

// newLaneReorderSurface wires a surface to this view. Both the window body and
// the compact body get one; nano does not, because its whole surface is already
// the window's own drag handle.
func (v *View) newLaneReorderSurface() *LaneReorderSurface {
	return NewLaneReorderSurface(v.beginLaneDrag, v.moveLaneDrag, v.endLaneDrag)
}

// currentLaneBounds measures the screen the drag is happening on. A drag can
// only start on a screen that draws provider groups.
func (v *View) currentLaneBounds() []laneBound {
	switch v.screen {
	case NormalScreen:
		if v.normalCache == nil {
			return nil
		}
		return laneGroupBounds(v.normalBody, v.normalCache.groups)
	case CompactScreen:
		if v.compactCache == nil {
			return nil
		}
		return laneGroupBounds(v.compactBody, v.compactCache.groups)
	}
	return nil
}

func (v *View) beginLaneDrag(y float32) {
	bounds := v.currentLaneBounds()
	index := laneAt(bounds, y)
	if index < 0 {
		return
	}
	v.dragLane = bounds[index].id
	v.dragBounds = bounds
	v.dragBase = slices.Clone(v.laneOrder())
	v.dragOrder = slices.Clone(v.dragBase)
	// The card is carried by the point it was grabbed at rather than by its top
	// edge, so it stays under the cursor instead of jumping to meet it.
	v.dragGrabOffset = y - bounds[index].top
	v.dragPlateHeight = bounds[index].bottom - bounds[index].top
	v.showLanePlate(y)
}

// showLanePlate draws the carried card under the pointer. The colours are the
// accent at two strengths: enough to read as lifted off the list, faint enough
// that the rows it covers stay legible underneath.
func (v *View) showLanePlate(y float32) {
	surface := v.laneReorderSurfaceFor(v.screen)
	if surface == nil {
		return
	}
	accent := v.colors.Accent
	red, green, blue, _ := accent.RGBA()
	fill := color.NRGBA{R: uint8(red >> 8), G: uint8(green >> 8), B: uint8(blue >> 8), A: 38}
	stroke := color.NRGBA{R: uint8(red >> 8), G: uint8(green >> 8), B: uint8(blue >> 8), A: 150}
	surface.ShowPlate(y-v.dragGrabOffset, v.dragPlateHeight, fill, stroke)
}

func (v *View) laneReorderSurfaceFor(screen Screen) *LaneReorderSurface {
	switch screen {
	case NormalScreen:
		return v.normalReorder
	case CompactScreen:
		return v.compactReorder
	}
	return nil
}

// moveLaneDrag places the grabbed provider in the slot the pointer is over.
//
// The slots are the ones measured when the drag began, and they are not
// measured again while it runs. Re-measuring would read the body mid-rebuild:
// each swap replaces every row, and until the layout catches up the old extents
// still name the old occupants, so the very next pointer event would swap the
// pair straight back and the group would flicker between two places. Deriving
// every arrangement from the starting slots instead makes each pointer position
// mean exactly one arrangement, whatever the screen has drawn so far.
func (v *View) moveLaneDrag(y float32) {
	if v.dragLane == "" {
		return
	}
	slot := laneAt(v.dragBounds, y)
	if slot < 0 {
		return
	}
	shown := make([]string, 0, len(v.dragBounds))
	for _, bound := range v.dragBounds {
		shown = append(shown, bound.id)
	}
	from := slices.Index(shown, v.dragLane)
	if from < 0 {
		return
	}
	// The carried card follows every pointer move, even the ones too small to
	// change the arrangement, so the drag feels continuous rather than stepped.
	v.showLanePlate(y)
	arranged := slices.Insert(slices.Delete(slices.Clone(shown), from, from+1), slot, v.dragLane)
	order := applyVisibleOrder(v.dragBase, shown, arranged)
	if order == nil || slices.Equal(order, v.dragOrder) {
		return
	}
	v.dragOrder = order
	v.renderCurrentScreen()
}

// applyVisibleOrder rewrites base so the providers on screen appear in the
// arranged sequence while the ones the user has switched off keep the places
// they held. A hidden provider dragged over would otherwise jump the moment it
// was switched back on.
func applyVisibleOrder(base, shown, arranged []string) []string {
	if len(shown) != len(arranged) {
		return nil
	}
	onScreen := make(map[string]struct{}, len(shown))
	for _, id := range shown {
		onScreen[id] = struct{}{}
	}
	next := make([]string, 0, len(base))
	cursor := 0
	for _, id := range base {
		if _, visible := onScreen[id]; !visible {
			next = append(next, id)
			continue
		}
		if cursor >= len(arranged) {
			return nil
		}
		next = append(next, arranged[cursor])
		cursor++
	}
	if cursor != len(arranged) {
		return nil
	}
	return next
}

// endLaneDrag writes the arrangement the drag settled on. Nothing reaches the
// settings file until here: a swap happens every time the pointer crosses a
// group, and saving each one would write the file many times a second.
func (v *View) endLaneDrag() {
	if surface := v.laneReorderSurfaceFor(v.screen); surface != nil {
		surface.HidePlate()
	}
	order := v.dragOrder
	v.dragOrder, v.dragLane, v.dragBase, v.dragBounds = nil, "", nil, nil
	v.dragGrabOffset, v.dragPlateHeight = 0, 0
	if order == nil {
		return
	}
	if slices.Equal(settings.NormalizeLaneOrder(v.config.LaneOrder), order) {
		return
	}
	config := v.config
	config.LaneOrder = order
	v.SetConfig(config)
}
