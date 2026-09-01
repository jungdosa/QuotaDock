package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

const (
	// NanoBarWidth is the thickness of the vertical title strip. It matches the
	// height the horizontal bar has always had, so the frame reads as the same
	// bar stood on end rather than a narrower second thing.
	NanoBarWidth float32 = TitleBarHeight
	// NanoBarButtonSize is the square each title action occupies down the strip,
	// the same 24 they occupy across the horizontal bar.
	NanoBarButtonSize float32 = 24
	NanoBarButtonGap  float32 = 2
	// NanoBarLetterHeight is the line the stacked app name gives each character.
	// It is tighter than the character's own measured height: at full leading
	// the name read as a list of letters rather than one word running down.
	NanoBarLetterHeight float32 = 11
	// NanoBarNameGap separates the actions from the name below them.
	NanoBarNameGap float32 = 10
	// NanoVerticalCellWidth is the reading area beside the strip. Vertical nano
	// gives a card the whole width instead of splitting it between providers, so
	// the meters are longer here than in the horizontal layout.
	NanoVerticalCellWidth float32 = 168
)

// VerticalTextLayout stacks single characters down a column, each centred in
// the width it is given. Fyne cannot rotate a canvas.Text, so a name that has
// to run down a strip the width of a button is stacked instead of turned.
type VerticalTextLayout struct{ LineHeight float32 }

func (l *VerticalTextLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, object := range objects {
		object.Resize(fyne.NewSize(size.Width, l.LineHeight))
		object.Move(fyne.NewPos(0, y))
		y += l.LineHeight
	}
}

func (l *VerticalTextLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := float32(0)
	for _, object := range objects {
		width = max(width, object.MinSize().Width)
	}
	return fyne.NewSize(width, l.LineHeight*float32(len(objects)))
}

// NanoBarButtonsLayout runs the title actions down the strip, each one a square
// centred in the strip's width.
type NanoBarButtonsLayout struct {
	Size float32
	Gap  float32
}

func (l *NanoBarButtonsLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x := (size.Width - l.Size) / 2
	y := float32(0)
	for _, object := range objects {
		object.Resize(fyne.NewSize(l.Size, l.Size))
		object.Move(fyne.NewPos(x, y))
		y += l.Size + l.Gap
	}
}

func (l *NanoBarButtonsLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(l.Size, 0)
	}
	return fyne.NewSize(l.Size, float32(len(objects))*l.Size+float32(len(objects)-1)*l.Gap)
}

// nanoOrientationResource draws the layout the button switches to, the way the
// display-mode button already shows the mode it moves to rather than the one in
// use: upright bars while nano is lying flat, a stack of lines while it stands.
func nanoOrientationResource(vertical bool, colors BrandColors) fyne.Resource {
	ink := colorHex(colors.Label)
	if vertical {
		return fyne.NewStaticResource("nano-horizontal.svg", []byte(fmt.Sprintf(
			"<svg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'>"+
				"<path d='M3 5h10M3 8h10M3 11h10' fill='none' stroke='%s' stroke-width='1.6' stroke-linecap='round'/></svg>", ink)))
	}
	return fyne.NewStaticResource("nano-vertical.svg", []byte(fmt.Sprintf(
		"<svg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'>"+
			"<path d='M5 3v10M8 3v10M11 3v10' fill='none' stroke='%s' stroke-width='1.6' stroke-linecap='round'/></svg>", ink)))
}

func (v *View) nanoOrientationTooltip() string {
	key := i18n.KeyNanoVertical
	if v.config.NanoVertical {
		key = i18n.KeyNanoHorizontal
	}
	return v.text(key)
}

func (v *View) toggleNanoOrientation() {
	config := v.config
	config.NanoVertical = !config.NanoVertical
	v.SetConfig(config)
}

// verticalTitleName stacks the application name down the strip. The version is
// dropped here rather than stacked after it: seven more characters would push
// the strip past the height of the readout it labels, and the version is on
// every other screen already.
func (v *View) verticalTitleName() fyne.CanvasObject {
	name := v.text(i18n.KeyAppTitle)
	letters := make([]fyne.CanvasObject, 0, len(name))
	for _, letter := range name {
		if letter == ' ' {
			continue
		}
		text := textLabel(string(letter), TitleVersionTextSize, v.colors.Secondary, false, false)
		text.Alignment = fyne.TextAlignCenter
		letters = append(letters, text)
	}
	return container.New(&VerticalTextLayout{LineHeight: NanoBarLetterHeight}, letters...)
}

// windowTitleVertical is the title bar stood on its end for vertical nano: the
// same actions in the same order running down the right edge, with the app name
// beneath them. The strip is a drag handle over its whole length, the way the
// horizontal bar is.
func (v *View) windowTitleVertical() *fyne.Container {
	buttons := v.titleButtons(settings.ModeNano)
	objects := make([]fyne.CanvasObject, 0, len(buttons))
	for _, button := range buttons {
		objects = append(objects, v.bindTitleButton(button))
	}
	actions := container.New(&NanoBarButtonsLayout{Size: NanoBarButtonSize, Gap: NanoBarButtonGap}, objects...)
	content := container.NewVBox(
		actions,
		container.New(layout.NewCustomPaddedLayout(NanoBarNameGap, 0, 0, 0), v.verticalTitleName()),
	)
	// The gradient runs across the strip rather than down it, so the bar keeps
	// the same light-to-dark direction it has when it lies flat.
	gradient := canvas.NewLinearGradient(v.colors.TitleTop, v.colors.TitleBottom, 90)
	divider := canvas.NewRectangle(v.colors.TitleDivider)
	divider.SetMinSize(fyne.NewSize(1, 1))
	dividerOverlay := container.NewBorder(nil, nil, divider, nil)
	background := container.NewStack(gradient, dividerOverlay)
	drag := NewDragSurface(v.Actions.BeginWindowDrag, v.Actions.MoveWindow, v.Actions.EndWindowDrag)
	return container.NewStack(background, drag, container.New(layout.NewCustomPaddedLayout(4, 4, 0, 0), content))
}

// nanoVerticalMinimumSize is the window vertical nano asks for: the strip and
// the reading area side by side, tall enough for whichever of the two needs
// more room.
func (v *View) nanoVerticalMinimumSize() fyne.Size {
	cells := len(v.nanoCellStates())
	body := float32(cells)*NanoBodyHeight + max(0, float32(cells-1))*NanoLineGap + 6
	bar := float32(0)
	if v.nanoBar != nil {
		bar = v.nanoBar.MinSize().Height
	}
	return fyne.NewSize(NanoVerticalCellWidth+NanoBarWidth, max(body, bar))
}
