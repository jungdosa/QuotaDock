package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/security"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func textLabel(text string, size float32, clr color.Color, bold, mono bool) *canvas.Text {
	t := canvas.NewText(text, clr)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold, Monospace: mono}
	t.Alignment = fyne.TextAlignLeading
	return t
}
func trackedUpper(text string) string {
	return strings.ToUpper(text)
}

type Toggle struct {
	widget.BaseWidget
	Checked      bool
	OnChanged    func(bool)
	Tooltip      string
	Hovered      bool
	Colors       BrandColors
	OnHoverStart func(*Toggle)
	OnHoverEnd   func(*Toggle)
}

func NewToggle(checked bool, onChanged func(bool), colors ...BrandColors) *Toggle {
	t := &Toggle{Checked: checked, OnChanged: onChanged, Colors: optionalBrandColors(colors)}
	t.ExtendBaseWidget(t)
	return t
}
func NewTooltipToggle(checked bool, tooltip string, onChanged func(bool), colors ...BrandColors) *Toggle {
	t := NewToggle(checked, onChanged, colors...)
	t.Tooltip = tooltip
	return t
}
func (t *Toggle) Tapped(*fyne.PointEvent) {
	if t.OnHoverEnd != nil {
		t.OnHoverEnd(t)
	}
	t.SetChecked(!t.Checked)
}
func (t *Toggle) TypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeySpace || e.Name == fyne.KeyEnter {
		t.SetChecked(!t.Checked)
	}
}
func (t *Toggle) TypedRune(rune) {}
func (t *Toggle) FocusGained()   { t.Refresh() }
func (t *Toggle) FocusLost()     { t.Refresh() }
func (t *Toggle) tooltipActive() bool  { return t.Hovered }
func (t *Toggle) tooltipValue() string { return t.Tooltip }
func (t *Toggle) MouseIn(*desktop.MouseEvent) {
	t.Hovered = true
	if t.OnHoverStart != nil {
		t.OnHoverStart(t)
	}
}
func (t *Toggle) MouseMoved(*desktop.MouseEvent) {}
func (t *Toggle) MouseOut() {
	t.Hovered = false
	if t.OnHoverEnd != nil {
		t.OnHoverEnd(t)
	}
}
func (t *Toggle) SetChecked(value bool) {
	if t.Checked == value {
		return
	}
	t.Checked = value
	t.Refresh()
	if t.OnChanged != nil {
		t.OnChanged(value)
	}
}
var _ desktop.Hoverable = (*Toggle)(nil)

func (t *Toggle) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(t.Colors.Disconnected)
	track.CornerRadius = 10
	thumb := canvas.NewCircle(t.Colors.ControlThumb)
	renderer := &toggleRenderer{toggle: t, track: track, thumb: thumb, objects: []fyne.CanvasObject{track, thumb}}
	// Paint the checked state right away: without this first Refresh a toggle
	// that is already on still renders in the "off" grey until some later redraw.
	renderer.Refresh()
	return renderer
}

type toggleRenderer struct {
	toggle  *Toggle
	track   *canvas.Rectangle
	thumb   *canvas.Circle
	objects []fyne.CanvasObject
}

func (r *toggleRenderer) Destroy()                     {}
func (r *toggleRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *toggleRenderer) MinSize() fyne.Size           { return fyne.NewSize(38, 20) }
func (r *toggleRenderer) Layout(size fyne.Size) {
	r.track.Resize(fyne.NewSize(38, 20))
	r.track.Move(fyne.NewPos(0, (size.Height-20)/2))
	x := float32(3)
	if r.toggle.Checked {
		x = 21
	}
	r.thumb.Resize(fyne.NewSize(14, 14))
	r.thumb.Move(fyne.NewPos(x, (size.Height-14)/2))
}
func (r *toggleRenderer) Refresh() {
	r.track.FillColor = r.toggle.Colors.Disconnected
	if r.toggle.Checked {
		r.track.FillColor = r.toggle.Colors.ToggleOn
	}
	r.track.Refresh()
	r.thumb.Refresh()
	r.Layout(r.toggle.Size())
}

type SmallButton struct {
	widget.BaseWidget
	Label        string
	Icon         fyne.Resource
	OnTapped     func()
	Tooltip      string
	Disabled     bool
	Outlined     bool
	Primary      bool
	Hovered      bool
	Colors       BrandColors
	OnHoverStart func(*SmallButton)
	OnHoverEnd   func(*SmallButton)
}

func NewSmallButton(label, tooltip string, onTapped func(), colors ...BrandColors) *SmallButton {
	b := &SmallButton{Label: label, Tooltip: tooltip, OnTapped: onTapped, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewSmallIconButton(icon fyne.Resource, tooltip string, onTapped func(), colors ...BrandColors) *SmallButton {
	b := &SmallButton{Icon: icon, Tooltip: tooltip, OnTapped: onTapped, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewOutlinedSmallButton(label, tooltip string, onTapped func(), colors ...BrandColors) *SmallButton {
	b := &SmallButton{Label: label, Tooltip: tooltip, OnTapped: onTapped, Outlined: true, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewOutlinedSmallIconButton(icon fyne.Resource, tooltip string, onTapped func(), colors ...BrandColors) *SmallButton {
	b := &SmallButton{Icon: icon, Tooltip: tooltip, OnTapped: onTapped, Outlined: true, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewDisabledSmallButton(label, tooltip string, colors ...BrandColors) *SmallButton {
	b := &SmallButton{Label: label, Tooltip: tooltip, Disabled: true, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewDisabledOutlinedSmallButton(label, tooltip string, colors ...BrandColors) *SmallButton {
	b := &SmallButton{Label: label, Tooltip: tooltip, Disabled: true, Outlined: true, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewPrimarySmallButton(label, tooltip string, onTapped func(), colors ...BrandColors) *SmallButton {
	b := &SmallButton{Label: label, Tooltip: tooltip, OnTapped: onTapped, Primary: true, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func (b *SmallButton) SetDisabled(disabled bool) {
	if b.Disabled == disabled {
		return
	}
	b.Disabled = disabled
	b.Refresh()
}
func (b *SmallButton) SetIcon(icon fyne.Resource) {
	b.Icon = icon
	b.Refresh()
}

// tooltipActive / tooltipValue let SmallButton own the shared passive tooltip.
func (b *SmallButton) tooltipActive() bool  { return b.Hovered }
func (b *SmallButton) tooltipValue() string { return b.Tooltip }

func (b *SmallButton) Tapped(*fyne.PointEvent) {
	if b.OnHoverEnd != nil {
		b.OnHoverEnd(b)
	}
	if !b.Disabled && b.OnTapped != nil {
		b.OnTapped()
	}
}
func (b *SmallButton) TypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyEnter || e.Name == fyne.KeySpace {
		b.Tapped(nil)
	}
}
func (b *SmallButton) TypedRune(rune) {}
func (b *SmallButton) FocusGained()   { b.Refresh() }
func (b *SmallButton) FocusLost()     { b.Refresh() }
func (b *SmallButton) MouseIn(*desktop.MouseEvent) {
	b.Hovered = true
	b.Refresh()
	if b.OnHoverStart != nil {
		b.OnHoverStart(b)
	}
}
func (b *SmallButton) MouseMoved(*desktop.MouseEvent) {}
func (b *SmallButton) MouseOut() {
	b.Hovered = false
	b.Refresh()
	if b.OnHoverEnd != nil {
		b.OnHoverEnd(b)
	}
}
func (b *SmallButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 4
	labelColor := b.Colors.Label
	if b.Disabled {
		labelColor = b.Colors.Secondary
		bg.StrokeColor = b.Colors.Secondary
		bg.StrokeWidth = 1
	}
	labelSize := float32(13)
	if b.Outlined {
		labelSize = SettingsTextSize
	}
	label := textLabel(b.Label, labelSize, labelColor, true, false)
	label.Alignment = fyne.TextAlignCenter
	icon := canvas.NewImageFromResource(b.Icon)
	icon.FillMode = canvas.ImageFillContain
	if b.Icon == nil {
		icon.Hide()
	} else {
		label.Hide()
	}
	renderer := &smallButtonRenderer{button: b, bg: bg, label: label, icon: icon, objects: []fyne.CanvasObject{bg, label, icon}}
	renderer.Refresh()
	return renderer
}

type smallButtonRenderer struct {
	button  *SmallButton
	bg      *canvas.Rectangle
	label   *canvas.Text
	icon    *canvas.Image
	objects []fyne.CanvasObject
}

func (r *smallButtonRenderer) Destroy()                     {}
func (r *smallButtonRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *smallButtonRenderer) MinSize() fyne.Size           { return fyne.NewSize(24, 24) }

// SmallButtonHeight keeps every title-bar and card button on one vertical
// centre line: the painted box never stretches to the full cell height, so an
// outlined button sits level with plain text buttons next to it.
const SmallButtonHeight float32 = 24

// smallButtonLabelOpticalLift recentres label ink inside the button box.
// Pretendard's em-box descent outweighs its cap overshoot, so a mathematically
// centred em-box reads about one pixel low; half a pixel of lift balances the
// measured cap ink for both Latin and CJK labels.
const smallButtonLabelOpticalLift float32 = 0.5

func (r *smallButtonRenderer) Layout(size fyne.Size) {
	boxHeight := min(size.Height, SmallButtonHeight)
	boxTop := (size.Height - boxHeight) / 2
	r.bg.Resize(fyne.NewSize(size.Width, boxHeight))
	r.bg.Move(fyne.NewPos(0, boxTop))
	// Keep the label sized to its own text height and centre it with Move only.
	// Sizing it to the full box height made Fyne centre the text a second time
	// inside those bounds, pushing every label below the box's centre line.
	// The half-pixel optical lift recentres Pretendard's cap ink, whose em-box
	// carries more descent than cap overshoot (measured on Latin and CJK).
	labelHeight := r.label.MinSize().Height
	r.label.Resize(fyne.NewSize(size.Width, labelHeight))
	r.label.Move(fyne.NewPos(0, boxTop+(boxHeight-labelHeight)/2-smallButtonLabelOpticalLift))
	r.icon.Resize(fyne.NewSize(16, 16))
	r.icon.Move(fyne.NewPos((size.Width-16)/2, boxTop+(boxHeight-16)/2))
}
func (r *smallButtonRenderer) Refresh() {
	r.label.Text = r.button.Label
	r.icon.Resource = r.button.Icon
	if r.button.Icon == nil {
		r.icon.Hide()
		r.label.Show()
	} else {
		r.icon.Show()
		r.label.Hide()
	}
	if r.button.Disabled {
		r.label.Color = r.button.Colors.Secondary
		r.bg.FillColor = color.Transparent
		if r.button.Outlined {
			r.bg.StrokeColor = buttonAlpha(r.button.Colors.Secondary, 0x70)
			r.bg.StrokeWidth = 1
		} else {
			r.bg.StrokeColor = color.Transparent
			r.bg.StrokeWidth = 0
		}
		r.icon.Translucency = 0.35
	} else if r.button.Primary {
		r.label.Color = r.button.Colors.IconText
		r.bg.FillColor = r.button.Colors.Accent
		r.bg.StrokeColor = r.button.Colors.Accent
		r.bg.StrokeWidth = 1
		r.icon.Translucency = 0
		if r.button.Hovered {
			r.bg.FillColor = buttonAlpha(r.button.Colors.Accent, 0xD8)
		}
	} else if r.button.Outlined {
		r.label.Color = r.button.Colors.Text
		r.bg.FillColor = buttonAlpha(r.button.Colors.TitleTop, 0xB8)
		r.bg.StrokeColor = r.button.Colors.CardBorder
		r.bg.StrokeWidth = 1
		r.icon.Translucency = 0
		if r.button.Hovered {
			r.bg.FillColor = buttonAlpha(r.button.Colors.ToggleOn, 0x78)
			r.bg.StrokeColor = r.button.Colors.Accent
		}
	} else {
		r.label.Color = r.button.Colors.Label
		r.bg.FillColor = color.Transparent
		r.bg.StrokeColor = color.Transparent
		r.bg.StrokeWidth = 0
		r.icon.Translucency = 0
		if r.button.Hovered {
			r.bg.FillColor = buttonAlpha(r.button.Colors.ToggleOn, 0x50)
		}
	}
	r.label.Refresh()
	r.icon.Refresh()
	r.bg.Refresh()
}

func buttonAlpha(value color.NRGBA, alpha uint8) color.NRGBA {
	value.A = alpha
	return value
}

type connectionMethodState uint8

const (
	connectionMethodActive connectionMethodState = iota
	connectionMethodAvailable
	connectionMethodMissing
	connectionMethodPlanned
)

func (s connectionMethodState) dashed() bool {
	return s == connectionMethodMissing || s == connectionMethodPlanned
}

// ConnectionMethodButton is the settings-only method segment. Its four visual
// states are supplied by the connection card and are never persisted.
type ConnectionMethodButton struct {
	widget.BaseWidget
	Label        string
	OnTapped     func()
	Tooltip      string
	State        connectionMethodState
	Hovered      bool
	Colors       BrandColors
	OnHoverStart func(*ConnectionMethodButton)
	OnHoverEnd   func(*ConnectionMethodButton)
}

func NewConnectionMethodButton(label, tooltip string, state connectionMethodState, onTapped func(), colors ...BrandColors) *ConnectionMethodButton {
	button := &ConnectionMethodButton{Label: label, Tooltip: tooltip, State: state, OnTapped: onTapped, Colors: optionalBrandColors(colors)}
	button.ExtendBaseWidget(button)
	return button
}

func (b *ConnectionMethodButton) SetPresentation(state connectionMethodState, tooltip string) {
	if b.State == state && b.Tooltip == tooltip {
		return
	}
	b.State = state
	b.Tooltip = tooltip
	b.Refresh()
}

func (b *ConnectionMethodButton) tooltipActive() bool  { return b.Hovered }
func (b *ConnectionMethodButton) tooltipValue() string { return b.Tooltip }
func (b *ConnectionMethodButton) Tapped(*fyne.PointEvent) {
	if b.OnHoverEnd != nil {
		b.OnHoverEnd(b)
	}
	if b.OnTapped != nil {
		b.OnTapped()
	}
}
func (b *ConnectionMethodButton) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyEnter || event.Name == fyne.KeySpace {
		b.Tapped(nil)
	}
}
func (b *ConnectionMethodButton) TypedRune(rune) {}
func (b *ConnectionMethodButton) FocusGained()   { b.Refresh() }
func (b *ConnectionMethodButton) FocusLost()     { b.Refresh() }
func (b *ConnectionMethodButton) MouseIn(*desktop.MouseEvent) {
	b.Hovered = true
	b.Refresh()
	if b.OnHoverStart != nil {
		b.OnHoverStart(b)
	}
}
func (b *ConnectionMethodButton) MouseMoved(*desktop.MouseEvent) {}
func (b *ConnectionMethodButton) MouseOut() {
	b.Hovered = false
	b.Refresh()
	if b.OnHoverEnd != nil {
		b.OnHoverEnd(b)
	}
}

func (b *ConnectionMethodButton) CreateRenderer() fyne.WidgetRenderer {
	background := canvas.NewRectangle(color.Transparent)
	background.CornerRadius = 4
	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 4
	label := textLabel(b.Label, SettingsTextSize, b.Colors.Text, true, false)
	dot := canvas.NewCircle(b.Colors.Disconnected)
	dashes := make([]*canvas.Line, 12)
	objects := []fyne.CanvasObject{background, border}
	for index := range dashes {
		dashes[index] = canvas.NewLine(b.Colors.Disconnected)
		dashes[index].StrokeWidth = 1
		objects = append(objects, dashes[index])
	}
	objects = append(objects, dot, label)
	renderer := &connectionMethodButtonRenderer{button: b, background: background, border: border, dashes: dashes, dot: dot, label: label, objects: objects}
	renderer.Refresh()
	return renderer
}

type connectionMethodButtonRenderer struct {
	button     *ConnectionMethodButton
	background *canvas.Rectangle
	border     *canvas.Rectangle
	dashes     []*canvas.Line
	dot        *canvas.Circle
	label      *canvas.Text
	objects    []fyne.CanvasObject
}

func (r *connectionMethodButtonRenderer) Destroy()                     {}
func (r *connectionMethodButtonRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *connectionMethodButtonRenderer) MinSize() fyne.Size           { return fyne.NewSize(32, 22) }
func (r *connectionMethodButtonRenderer) Layout(size fyne.Size) {
	r.background.Resize(size)
	r.border.Resize(size)
	r.layoutDashes(size)

	labelSize := r.label.MinSize()
	contentWidth := labelSize.Width
	hasDot := r.button.State != connectionMethodPlanned
	if hasDot {
		contentWidth += 10
	}
	x := (size.Width - contentWidth) / 2
	if hasDot {
		r.dot.Resize(fyne.NewSize(6, 6))
		r.dot.Move(fyne.NewPos(x, (size.Height-6)/2))
		x += 10
	}
	r.label.Resize(labelSize)
	r.label.Move(fyne.NewPos(x, (size.Height-labelSize.Height)/2-smallButtonLabelOpticalLift))
}

func (r *connectionMethodButtonRenderer) layoutDashes(size fyne.Size) {
	horizontalSpan := max(float32(0), size.Width-2)
	horizontalCell := horizontalSpan / 4
	horizontalDash := horizontalCell * 0.58
	index := 0
	for _, y := range []float32{1, size.Height - 1} {
		for segment := 0; segment < 4; segment++ {
			start := float32(1) + float32(segment)*horizontalCell
			r.dashes[index].Position1 = fyne.NewPos(start, y)
			r.dashes[index].Position2 = fyne.NewPos(start+horizontalDash, y)
			index++
		}
	}
	verticalSpan := max(float32(0), size.Height-2)
	verticalCell := verticalSpan / 2
	verticalDash := verticalCell * 0.58
	for _, x := range []float32{1, size.Width - 1} {
		for segment := 0; segment < 2; segment++ {
			start := float32(1) + float32(segment)*verticalCell
			r.dashes[index].Position1 = fyne.NewPos(x, start)
			r.dashes[index].Position2 = fyne.NewPos(x, start+verticalDash)
			index++
		}
	}
}

func (r *connectionMethodButtonRenderer) Refresh() {
	accent := r.button.Colors.Disconnected
	labelColor := r.button.Colors.Secondary
	fill := color.NRGBA{A: 0}
	switch r.button.State {
	case connectionMethodActive:
		accent = r.button.Colors.Connected
		labelColor = r.button.Colors.Text
		fill = buttonAlpha(r.button.Colors.Connected, 0x24)
	case connectionMethodAvailable:
		accent = r.button.Colors.Accent
		labelColor = r.button.Colors.Text
	case connectionMethodPlanned:
		accent = buttonAlpha(r.button.Colors.Secondary, 0x68)
		labelColor = buttonAlpha(r.button.Colors.Secondary, 0x88)
	}
	if r.button.Hovered {
		fill = buttonAlpha(accent, 0x30)
	}
	r.background.FillColor = fill
	r.border.FillColor = color.Transparent
	r.border.StrokeColor = accent
	if r.button.State.dashed() {
		r.border.StrokeWidth = 0
		r.border.Hide()
	} else {
		r.border.StrokeWidth = 1
		r.border.Show()
	}
	for _, dash := range r.dashes {
		dash.StrokeColor = accent
		if r.button.State.dashed() {
			dash.Show()
		} else {
			dash.Hide()
		}
		dash.Refresh()
	}
	r.label.Text = r.button.Label
	r.label.Color = labelColor
	r.dot.FillColor = accent
	if r.button.State == connectionMethodPlanned {
		r.dot.Hide()
	} else {
		r.dot.Show()
	}
	r.background.Refresh()
	r.border.Refresh()
	r.dot.Refresh()
	r.label.Refresh()
	r.Layout(r.button.Size())
}

// TooltipRegion is a transparent hover target that owns the shared passive
// tooltip for a value rendered next to (or underneath) it. It deliberately
// implements only desktop.Hoverable — never Tappable — so taps keep falling
// through to the surface below it (W11 reset times, W12 nano rows).
type TooltipRegion struct {
	widget.BaseWidget
	Hovered bool
	value   string
	view    *View
}

func NewTooltipRegion(view *View, value string) *TooltipRegion {
	region := &TooltipRegion{view: view, value: value}
	region.ExtendBaseWidget(region)
	return region
}

func (r *TooltipRegion) SetValue(value string) { r.value = value }
func (r *TooltipRegion) tooltipActive() bool   { return r.Hovered }
func (r *TooltipRegion) tooltipValue() string  { return r.value }
func (r *TooltipRegion) MouseIn(*desktop.MouseEvent) {
	r.Hovered = true
	if r.view != nil {
		r.view.scheduleAnchorTooltip(r)
	}
}
func (r *TooltipRegion) MouseMoved(*desktop.MouseEvent) {}
func (r *TooltipRegion) MouseOut() {
	r.Hovered = false
	if r.view != nil {
		r.view.dismissAnchorTooltip(r)
	}
}
func (r *TooltipRegion) CreateRenderer() fyne.WidgetRenderer {
	return &singleRenderer{object: canvas.NewRectangle(color.Transparent), min: fyne.NewSize(1, 1)}
}

var _ desktop.Hoverable = (*TooltipRegion)(nil)

type RadioGroup struct {
	widget.BaseWidget
	Options   []string
	Selected  string
	OnChanged func(string)
	Colors    BrandColors
}

func NewRadioGroup(options []string, selected string, onChanged func(string), colors ...BrandColors) *RadioGroup {
	radio := &RadioGroup{
		Options:   append([]string(nil), options...),
		Selected:  selected,
		OnChanged: onChanged,
		Colors:    optionalBrandColors(colors),
	}
	radio.ExtendBaseWidget(radio)
	return radio
}

func (r *RadioGroup) SetSelected(value string) {
	if r.Selected == value || !containsString(r.Options, value) {
		return
	}
	r.Selected = value
	r.Refresh()
	if r.OnChanged != nil {
		r.OnChanged(value)
	}
}

func (r *RadioGroup) Tapped(event *fyne.PointEvent) {
	if len(r.Options) == 0 {
		return
	}
	x := float32(0)
	if event != nil {
		x = event.Position.X
	}
	for index, width := range r.optionWidths() {
		if x < width || index == len(r.Options)-1 {
			r.SetSelected(r.Options[index])
			return
		}
		x -= width + radioOptionGap
	}
}

func (r *RadioGroup) TypedKey(event *fyne.KeyEvent) {
	if len(r.Options) == 0 {
		return
	}
	index := 0
	for i, option := range r.Options {
		if option == r.Selected {
			index = i
			break
		}
	}
	switch event.Name {
	case fyne.KeyLeft, fyne.KeyUp:
		index = (index - 1 + len(r.Options)) % len(r.Options)
	case fyne.KeyRight, fyne.KeyDown:
		index = (index + 1) % len(r.Options)
	case fyne.KeySpace, fyne.KeyEnter:
	default:
		return
	}
	r.SetSelected(r.Options[index])
}

func (r *RadioGroup) TypedRune(rune) {}
func (r *RadioGroup) FocusGained()   { r.Refresh() }
func (r *RadioGroup) FocusLost()     { r.Refresh() }

const (
	radioDiameter  float32 = 14
	radioDotSize   float32 = 6
	radioLabelGap  float32 = 6
	radioOptionGap float32 = 14
	radioHeight    float32 = 24
)

func (r *RadioGroup) optionWidths() []float32 {
	widths := make([]float32, len(r.Options))
	for index, option := range r.Options {
		widths[index] = radioDiameter + radioLabelGap + fyne.MeasureText(option, SettingsTextSize, fyne.TextStyle{}).Width
	}
	return widths
}

func (r *RadioGroup) CreateRenderer() fyne.WidgetRenderer {
	rings := make([]*canvas.Circle, len(r.Options))
	dots := make([]*canvas.Circle, len(r.Options))
	labels := make([]*canvas.Text, len(r.Options))
	objects := make([]fyne.CanvasObject, 0, len(r.Options)*3)
	for index, option := range r.Options {
		rings[index] = canvas.NewCircle(r.Colors.Card)
		dots[index] = canvas.NewCircle(r.Colors.Accent)
		labels[index] = textLabel(option, SettingsTextSize, r.Colors.Label, false, false)
		objects = append(objects, rings[index], dots[index], labels[index])
	}
	renderer := &radioGroupRenderer{radio: r, rings: rings, dots: dots, labels: labels, objects: objects}
	renderer.Refresh()
	return renderer
}

type radioGroupRenderer struct {
	radio   *RadioGroup
	rings   []*canvas.Circle
	dots    []*canvas.Circle
	labels  []*canvas.Text
	objects []fyne.CanvasObject
}

func (r *radioGroupRenderer) Destroy()                     {}
func (r *radioGroupRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *radioGroupRenderer) MinSize() fyne.Size {
	width := float32(0)
	for _, optionWidth := range r.radio.optionWidths() {
		width += optionWidth
	}
	width += radioOptionGap * float32(max(0, len(r.radio.Options)-1))
	return fyne.NewSize(width, radioHeight)
}
func (r *radioGroupRenderer) Layout(size fyne.Size) {
	x := float32(0)
	for index, optionWidth := range r.radio.optionWidths() {
		y := (size.Height - radioDiameter) / 2
		r.rings[index].Move(fyne.NewPos(x, y))
		r.rings[index].Resize(fyne.NewSquareSize(radioDiameter))
		dotOffset := (radioDiameter - radioDotSize) / 2
		r.dots[index].Move(fyne.NewPos(x+dotOffset, y+dotOffset))
		r.dots[index].Resize(fyne.NewSquareSize(radioDotSize))
		// Centre the label on the ring's centre line rather than on the row, so
		// text and circle read as one baseline even when the row is taller.
		labelX := x + radioDiameter + radioLabelGap
		labelHeight := r.labels[index].MinSize().Height
		ringCentre := y + radioDiameter/2
		r.labels[index].Move(fyne.NewPos(labelX, ringCentre-labelHeight/2))
		r.labels[index].Resize(fyne.NewSize(optionWidth-radioDiameter-radioLabelGap, labelHeight))
		x += optionWidth + radioOptionGap
	}
}
func (r *radioGroupRenderer) Refresh() {
	for index, option := range r.radio.Options {
		selected := option == r.radio.Selected
		r.rings[index].FillColor = r.radio.Colors.Card
		r.rings[index].StrokeColor = r.radio.Colors.RadioBorder
		r.rings[index].StrokeWidth = 1.8
		r.labels[index].Color = r.radio.Colors.Label
		if selected {
			r.rings[index].StrokeColor = r.radio.Colors.Accent
			r.rings[index].StrokeWidth = 2
			r.labels[index].Color = r.radio.Colors.Text
			r.dots[index].Show()
		} else {
			r.dots[index].Hide()
		}
		r.rings[index].Refresh()
		r.dots[index].Refresh()
		r.labels[index].Refresh()
	}
	r.Layout(r.radio.Size())
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func displayModeResource(mode settings.DisplayMode, colors BrandColors) fyne.Resource {
	return displayModeIconResource(settings.NextDisplayMode(mode), colors)
}

func displayModeIconResource(mode settings.DisplayMode, colors BrandColors) fyne.Resource {
	ink := colorHex(colors.Label)
	name := "display-normal.svg"
	rect := fmt.Sprintf("<rect x='2' y='2' width='12' height='12' rx='2' fill='none' stroke='%s' stroke-width='1.6'/>", ink)
	switch mode {
	case settings.ModeCompact:
		name = "display-compact.svg"
		rect = fmt.Sprintf("<rect x='2' y='4' width='12' height='8' rx='2' fill='none' stroke='%s' stroke-width='1.6'/>", ink)
	case settings.ModeNano:
		// A thin line (W5): the filled bar read as a second rectangle next to
		// the compact icon. Shorter than the rectangles above it so the size
		// hierarchy stays visible at a glance.
		name = "display-nano.svg"
		rect = fmt.Sprintf("<path d='M3 8h10' fill='none' stroke='%s' stroke-width='2' stroke-linecap='round'/>", ink)
	}
	svg := fmt.Sprintf("<svg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'>%s</svg>", rect)
	return fyne.NewStaticResource(name, []byte(svg))
}


func busyResource(colors BrandColors) fyne.Resource {
	stroke := colorHex(colors.Accent)
	svg := fmt.Sprintf("<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20' viewBox='0 0 20 20'><circle cx='10' cy='10' r='6.5' fill='none' stroke='%s' stroke-opacity='.28' stroke-width='2'/><path d='M10 3.5a6.5 6.5 0 0 1 6.5 6.5' fill='none' stroke='%s' stroke-width='2.2' stroke-linecap='round'/></svg>", stroke, stroke)
	return fyne.NewStaticResource("refresh-busy.svg", []byte(svg))
}

func colorHex(value color.Color) string {
	r, g, b, _ := value.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

type PaletteSwatch struct {
	widget.BaseWidget
	Fill     color.Color
	Selected bool
	Reset    bool
	Colors   BrandColors
	OnTapped func()
}

func NewPaletteSwatch(fill color.Color, selected, reset bool, onTapped func(), colors BrandColors) *PaletteSwatch {
	s := &PaletteSwatch{Fill: fill, Selected: selected, Reset: reset, Colors: colors, OnTapped: onTapped}
	s.ExtendBaseWidget(s)
	return s
}
func (s *PaletteSwatch) Tapped(*fyne.PointEvent) {
	if s.OnTapped != nil {
		s.OnTapped()
	}
}
func (s *PaletteSwatch) CreateRenderer() fyne.WidgetRenderer {
	box := canvas.NewRectangle(s.Fill)
	box.CornerRadius = 6
	mark := textLabel("↺", 14, s.Colors.IconText, true, false)
	mark.Alignment = fyne.TextAlignCenter
	if !s.Reset {
		mark.Hide()
	}
	return &paletteSwatchRenderer{swatch: s, box: box, mark: mark, objects: []fyne.CanvasObject{box, mark}}
}

type paletteSwatchRenderer struct {
	swatch  *PaletteSwatch
	box     *canvas.Rectangle
	mark    *canvas.Text
	objects []fyne.CanvasObject
}

func (r *paletteSwatchRenderer) Destroy()                     {}
func (r *paletteSwatchRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *paletteSwatchRenderer) MinSize() fyne.Size           { return fyne.NewSize(28, 28) }
func (r *paletteSwatchRenderer) Layout(size fyne.Size) {
	r.box.Move(fyne.NewPos(2, 2))
	r.box.Resize(fyne.NewSize(size.Width-4, size.Height-4))
	// Full-size bounds centre the glyph on both axes; adding a manual offset on
	// top of that pushed it below centre (same double-centering as W10).
	r.mark.Move(fyne.NewPos(0, 0))
	r.mark.Resize(size)
}
func (r *paletteSwatchRenderer) Refresh() {
	r.box.FillColor = r.swatch.Fill
	r.box.StrokeColor = r.swatch.Colors.CardBorder
	r.box.StrokeWidth = 1
	if r.swatch.Selected {
		r.box.StrokeColor = r.swatch.Colors.Accent
		r.box.StrokeWidth = 2
	}
	if r.swatch.Reset {
		r.mark.Show()
	} else {
		r.mark.Hide()
	}
	r.box.Refresh()
	r.mark.Refresh()
}

var _ fyne.Tappable = (*PaletteSwatch)(nil)

type PaletteButton struct {
	widget.BaseWidget
	ID            string
	Canvas        fyne.Canvas
	OnChanged     func(string)
	OnShowPalette func()
	popup         *widget.PopUpMenu
	Colors        BrandColors
	Compact       bool
}

func NewPaletteButton(id string, c fyne.Canvas, onChanged func(string), colors ...BrandColors) *PaletteButton {
	if !security.IsPaletteID(id) {
		id = "slate"
	}
	b := &PaletteButton{ID: id, Canvas: c, OnChanged: onChanged, Colors: optionalBrandColors(colors)}
	b.ExtendBaseWidget(b)
	return b
}
func NewPaletteDotButton(id string, c fyne.Canvas, onChanged func(string), colors ...BrandColors) *PaletteButton {
	button := NewPaletteButton(id, c, onChanged, colors...)
	button.Compact = true
	return button
}
func (b *PaletteButton) Tapped(*fyne.PointEvent) { b.ShowPalette() }
func (b *PaletteButton) TypedRune(rune)          {}
func (b *PaletteButton) TypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyEnter || e.Name == fyne.KeySpace {
		b.ShowPalette()
	}
}
func (b *PaletteButton) FocusGained() { b.Refresh() }
func (b *PaletteButton) FocusLost()   { b.Refresh() }
func (b *PaletteButton) ShowPalette() {
	if b.OnShowPalette != nil {
		b.OnShowPalette()
		return
	}
	if b.Canvas == nil {
		return
	}
	items := make([]*fyne.MenuItem, 0, 16)
	for _, id := range security.PaletteIDs() {
		value := id
		item := fyne.NewMenuItemWithIcon(strings.Title(value), colorResource(value, b.Colors), func() { b.SetID(value) })
		items = append(items, item)
	}
	b.popup = widget.NewPopUpMenu(fyne.NewMenu("", items...), b.Canvas)
	b.popup.ShowAtRelativePosition(fyne.NewPos(0, b.Size().Height), b)
}
func (b *PaletteButton) Dismiss() {
	if b.popup != nil {
		b.popup.Hide()
		b.popup = nil
	}
}
func (b *PaletteButton) SetID(id string) {
	if !security.IsPaletteID(id) {
		return
	}
	b.ID = id
	b.Dismiss()
	b.Refresh()
	if b.OnChanged != nil {
		b.OnChanged(id)
	}
}
func (b *PaletteButton) CreateRenderer() fyne.WidgetRenderer {
	swatch := canvas.NewRectangle(b.Colors.PaletteColor(b.ID))
	swatch.CornerRadius = 3
	label := textLabel(b.ID, SettingsTextSize, b.Colors.Text, false, true)
	if b.Compact {
		label.Hide()
	}
	return &paletteRenderer{button: b, swatch: swatch, label: label, objects: []fyne.CanvasObject{swatch, label}}
}

type paletteRenderer struct {
	button  *PaletteButton
	swatch  *canvas.Rectangle
	label   *canvas.Text
	objects []fyne.CanvasObject
}

func (r *paletteRenderer) Destroy()                     {}
func (r *paletteRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *paletteRenderer) MinSize() fyne.Size {
	if r.button.Compact {
		return fyne.NewSize(24, 24)
	}
	return fyne.NewSize(90, 24)
}
func (r *paletteRenderer) Layout(size fyne.Size) {
	if r.button.Compact {
		r.swatch.Move(fyne.NewPos((size.Width-12)/2, (size.Height-12)/2))
		r.swatch.Resize(fyne.NewSize(12, 12))
		return
	}
	r.swatch.Move(fyne.NewPos(3, (size.Height-16)/2))
	r.swatch.Resize(fyne.NewSize(16, 16))
	r.label.Move(fyne.NewPos(25, (size.Height-r.label.MinSize().Height)/2))
	r.label.Resize(fyne.NewSize(size.Width-25, size.Height))
}
func (r *paletteRenderer) Refresh() {
	r.swatch.FillColor = r.button.Colors.PaletteColor(r.button.ID)
	r.swatch.Refresh()
	r.label.Text = r.button.ID
	r.label.Refresh()
}
func colorResource(id string, colors ...BrandColors) fyne.Resource {
	c := optionalBrandColors(colors).PaletteColor(id)
	r, g, b, _ := c.RGBA()
	svg := fmt.Sprintf("<svg xmlns='http://www.w3.org/2000/svg' width='16' height='16'><rect width='16' height='16' rx='4' fill='#%02x%02x%02x'/></svg>", r>>8, g>>8, b>>8)
	return fyne.NewStaticResource(id+".svg", []byte(svg))
}

func optionalBrandColors(colors []BrandColors) BrandColors {
	if len(colors) > 0 {
		return colors[0]
	}
	return DarkBrandColors
}

var _ fyne.Focusable = (*Toggle)(nil)
var _ fyne.Focusable = (*SmallButton)(nil)
var _ fyne.Focusable = (*PaletteButton)(nil)
var _ = theme.ColorNameBackground
