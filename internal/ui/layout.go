package ui

import "fyne.io/fyne/v2"

type ColumnLayout struct {
	Widths []float32
	Gap    float32
	Height float32
}

// GapColumnLayout is a fixed/flexible column row whose individual gaps differ.
// It is used where visual grouping matters, such as title actions and compact
// settings controls. Widths <= 0 identify the single flexible column.
type GapColumnLayout struct {
	Widths []float32
	Gaps   []float32
	Height float32
}

func NewGapColumnLayout(widths, gaps []float32, height float32) *GapColumnLayout {
	return &GapColumnLayout{
		Widths: append([]float32(nil), widths...),
		Gaps:   append([]float32(nil), gaps...),
		Height: height,
	}
}

func (l *GapColumnLayout) gap(index int) float32 {
	if index < 0 || index >= len(l.Gaps) {
		return 0
	}
	return l.Gaps[index]
}

func (l *GapColumnLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	count := min(len(objects), len(l.Widths))
	if count == 0 {
		return
	}
	fixed := float32(0)
	flex := -1
	for index := 0; index < count; index++ {
		width := l.Widths[index]
		if width <= 0 && flex < 0 {
			flex = index
			continue
		}
		fixed += max(float32(0), width)
	}
	for index := 0; index < count-1; index++ {
		fixed += l.gap(index)
	}
	available := max(float32(0), size.Width-fixed)
	x := float32(0)
	for index, object := range objects {
		if index >= count {
			object.Hide()
			continue
		}
		width := l.Widths[index]
		if index == flex || width <= 0 {
			width = available
		}
		height := size.Height
		if l.Height > 0 && l.Height < height {
			height = l.Height
		}
		object.Move(fyne.NewPos(x, (size.Height-height)/2))
		object.Resize(fyne.NewSize(width, height))
		x += width
		if index < count-1 {
			x += l.gap(index)
		}
	}
}

func (l *GapColumnLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	count := min(len(objects), len(l.Widths))
	minimum := fyne.NewSize(0, l.Height)
	for index := 0; index < count; index++ {
		width := l.Widths[index]
		if width <= 0 {
			width = objects[index].MinSize().Width
		}
		minimum.Width += width
		minimum.Height = max(minimum.Height, objects[index].MinSize().Height)
		if index < count-1 {
			minimum.Width += l.gap(index)
		}
	}
	return minimum
}

func NewColumnLayout(widths []float32, gap, height float32) *ColumnLayout {
	return &ColumnLayout{Widths: append([]float32(nil), widths...), Gap: gap, Height: height}
}
func (l *ColumnLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	fixed := float32(0)
	flex := -1
	for i, w := range l.Widths {
		if w <= 0 {
			if flex < 0 {
				flex = i
			}
			continue
		}
		fixed += w
	}
	count := min(len(objects), len(l.Widths))
	available := size.Width - fixed - l.Gap*float32(max(0, count-1))
	if available < 0 {
		available = 0
	}
	x := float32(0)
	for i, obj := range objects {
		if i >= len(l.Widths) {
			obj.Hide()
			continue
		}
		w := l.Widths[i]
		if i == flex || w <= 0 {
			w = available
		}
		h := size.Height
		if l.Height > 0 && l.Height < h {
			h = l.Height
		}
		obj.Resize(fyne.NewSize(w, h))
		obj.Move(fyne.NewPos(x, (size.Height-h)/2))
		x += w + l.Gap
	}
}
func (l *ColumnLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := float32(0)
	height := l.Height
	for i, obj := range objects {
		if i >= len(l.Widths) {
			break
		}
		w := l.Widths[i]
		if w <= 0 {
			w = obj.MinSize().Width
		}
		width += w
		if obj.MinSize().Height > height {
			height = obj.MinSize().Height
		}
	}
	width += l.Gap * float32(max(0, min(len(objects), len(l.Widths))-1))
	return fyne.NewSize(width, height)
}

type SettingRowLayout struct {
	LabelWidth   float32
	Gap          float32
	ControlWidth float32
	Height       float32
}

type CompactRowsLayout struct {
	Gap float32
}

func (l *CompactRowsLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, object := range objects {
		height := object.MinSize().Height
		object.Move(fyne.NewPos(0, y))
		object.Resize(fyne.NewSize(size.Width, height))
		y += height + l.Gap
	}
}

func (l *CompactRowsLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	size := fyne.NewSize(0, 0)
	for _, object := range objects {
		minimum := object.MinSize()
		size.Width = max(size.Width, minimum.Width)
		size.Height += minimum.Height
	}
	size.Height += l.Gap * float32(max(0, len(objects)-1))
	return size
}

type CompactMeterLayout struct {
	MeterHeight float32
	BarHeight   float32
	Gap         float32
}

type CompactPercentLayout struct {
	OffsetY    float32
	RightInset float32
	Gap        float32
}

func (l *CompactPercentLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	minimums := make([]fyne.Size, len(objects))
	maximumHeight := float32(0)
	totalWidth := l.Gap * float32(max(0, len(objects)-1))
	for index, object := range objects {
		minimums[index] = object.MinSize()
		maximumHeight = max(maximumHeight, minimums[index].Height)
		totalWidth += minimums[index].Width
	}
	x := max(float32(0), size.Width-l.RightInset-totalWidth)
	baseline := (size.Height-maximumHeight)/2 + l.OffsetY + maximumHeight
	for index, object := range objects {
		minimum := minimums[index]
		object.Move(fyne.NewPos(x, baseline-minimum.Height))
		object.Resize(minimum)
		x += minimum.Width + l.Gap
	}
}

func (l *CompactPercentLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}
	minimum := fyne.NewSize(l.RightInset+l.Gap*float32(max(0, len(objects)-1)), 0)
	for _, object := range objects {
		size := object.MinSize()
		minimum.Width += size.Width
		minimum.Height = max(minimum.Height, size.Height)
	}
	return minimum
}

type NanoLinesLayout struct {
	Gap float32
}

type NanoUsageLayout struct {
	LabelWidth  float32
	Gap         float32
	BarHeight   float32
	ResetHeight float32
	ResetGap    float32
}

func (l *NanoUsageLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	labelHeight := min(size.Height, objects[0].MinSize().Height)
	objects[0].Move(fyne.NewPos(0, (size.Height-labelHeight)/2))
	objects[0].Resize(fyne.NewSize(l.LabelWidth, labelHeight))
	barX := l.LabelWidth + l.Gap
	barHeight := min(size.Height, l.BarHeight)
	resetHeight := float32(0)
	bundleHeight := barHeight
	if len(objects) > 2 {
		resetHeight = min(size.Height, l.ResetHeight)
		bundleHeight += l.ResetGap + resetHeight
	}
	barY := max(float32(0), (size.Height-bundleHeight)/2)
	objects[1].Move(fyne.NewPos(barX, barY))
	objects[1].Resize(fyne.NewSize(max(float32(0), size.Width-barX), barHeight))
	if len(objects) < 3 {
		return
	}
	objects[2].Move(fyne.NewPos(barX, barY+barHeight+l.ResetGap))
	objects[2].Resize(fyne.NewSize(max(float32(0), size.Width-barX), resetHeight))
}

func (l *NanoUsageLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	height := l.BarHeight
	if len(objects) > 2 {
		height = l.BarHeight + l.ResetGap + l.ResetHeight
	}
	// The line height is pinned to the reference metric instead of the live
	// label: the nano label font grew for readability, but the row must
	// not push the window taller. The descender-free labels ("7D", "5h") fit
	// the tighter box without visible clipping.
	height = max(height, nanoRowLineHeight())
	return fyne.NewSize(l.LabelWidth+l.Gap+1, height)
}

func nanoRowLineHeight() float32 {
	return fyne.MeasureText("7D", nanoLabelReferenceTextSize, fyne.TextStyle{Bold: true}).Height
}

type NanoCellLayout struct {
	IconSize float32
	Gap      float32
}

func (l *NanoCellLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	iconSize := min(l.IconSize, size.Height)
	objects[0].Move(fyne.NewPos(0, (size.Height-iconSize)/2))
	objects[0].Resize(fyne.NewSize(iconSize, iconSize))
	contentX := l.IconSize + l.Gap
	objects[1].Move(fyne.NewPos(contentX, 0))
	objects[1].Resize(fyne.NewSize(max(float32(0), size.Width-contentX), size.Height))
}

func (l *NanoCellLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	content := fyne.NewSize(1, 1)
	if len(objects) > 1 {
		content = objects[1].MinSize()
	}
	return fyne.NewSize(l.IconSize+l.Gap+content.Width, max(l.IconSize, content.Height))
}

func (l *NanoLinesLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	height := (size.Height - l.Gap*float32(len(objects)-1)) / float32(len(objects))
	y := float32(0)
	for _, object := range objects {
		object.Move(fyne.NewPos(0, y))
		object.Resize(fyne.NewSize(size.Width, height))
		y += height + l.Gap
	}
}

func (l *NanoLinesLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	size := fyne.NewSize(1, 0)
	for _, object := range objects {
		minimum := object.MinSize()
		size.Width = max(size.Width, minimum.Width)
		size.Height += minimum.Height
	}
	size.Height += l.Gap * float32(max(0, len(objects)-1))
	return size
}

func (l *CompactMeterLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	total := l.MeterHeight + l.Gap + l.BarHeight
	y := max(float32(0), (size.Height-total)/2)
	objects[0].Move(fyne.NewPos(0, y))
	objects[0].Resize(fyne.NewSize(size.Width, l.MeterHeight))
	objects[1].Move(fyne.NewPos(0, y+l.MeterHeight+l.Gap))
	objects[1].Resize(fyne.NewSize(size.Width, l.BarHeight))
}

func (l *CompactMeterLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := float32(1)
	for _, object := range objects {
		width = max(width, object.MinSize().Width)
	}
	return fyne.NewSize(width, l.MeterHeight+l.Gap+l.BarHeight)
}

func (l *SettingRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 1 {
		return
	}
	h := l.Height
	if h <= 0 {
		h = size.Height
	}
	objects[0].Move(fyne.NewPos(0, (size.Height-h)/2))
	objects[0].Resize(fyne.NewSize(l.LabelWidth, h))
	if len(objects) > 1 {
		w := l.ControlWidth
		if w <= 0 {
			w = objects[1].MinSize().Width
		}
		maxW := size.Width - l.LabelWidth - l.Gap
		if w > maxW {
			w = maxW
		}
		objects[1].Move(fyne.NewPos(l.LabelWidth+l.Gap, (size.Height-h)/2))
		objects[1].Resize(fyne.NewSize(w, h))
	}
}
func (l *SettingRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	w := l.LabelWidth
	h := l.Height
	if len(objects) > 1 {
		cw := l.ControlWidth
		if cw <= 0 {
			cw = objects[1].MinSize().Width
		}
		w += l.Gap + cw
		if objects[1].MinSize().Height > h {
			h = objects[1].MinSize().Height
		}
	}
	return fyne.NewSize(w, h)
}

// NormalMeterStackLayout stacks a percentage band directly above a meter so the
// number reads as a label attached to the bar's right end rather than a separate
// column. Objects are [percent band, meter, reset bar]: the reset bar
// shares the meter's exact x and width so the two lengths compare directly.
type NormalMeterStackLayout struct {
	PercentHeight float32
	MeterHeight   float32
	BarHeight     float32
	Gap           float32
}

func (l *NormalMeterStackLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	band := min(l.PercentHeight, size.Height)
	objects[0].Resize(fyne.NewSize(size.Width, band))
	objects[0].Move(fyne.NewPos(0, 0))
	region := max(float32(0), size.Height-band)
	if len(objects) < 3 {
		objects[1].Resize(fyne.NewSize(size.Width, region))
		objects[1].Move(fyne.NewPos(0, band))
		return
	}
	bundle := l.MeterHeight + l.Gap + l.BarHeight
	top := band + max(float32(0), (region-bundle)/2)
	objects[1].Resize(fyne.NewSize(size.Width, l.MeterHeight))
	objects[1].Move(fyne.NewPos(0, top))
	objects[2].Resize(fyne.NewSize(size.Width, l.BarHeight))
	objects[2].Move(fyne.NewPos(0, top+l.MeterHeight+l.Gap))
}

func (l *NormalMeterStackLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := float32(0)
	height := l.PercentHeight
	for _, object := range objects {
		width = max(width, object.MinSize().Width)
	}
	if len(objects) > 2 {
		height += l.MeterHeight + l.Gap + l.BarHeight
	} else if len(objects) > 1 {
		height += objects[1].MinSize().Height
	}
	return fyne.NewSize(width, height)
}
