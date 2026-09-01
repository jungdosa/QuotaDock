package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"

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
	// NanoCardGap separates stacked cards. It is the gap the flat layout leaves
	// between cards across, turned to run down.
	NanoCardGap float32 = 6
	// nanoBodyPadX is the inset buildNano puts either side of the cards.
	nanoBodyPadX float32 = 4
)

// NanoStackLayout runs the cards down the window, each keeping the size it has
// when they run across it.
//
// The grid layout this replaces shares the space out equally, so a card in a
// window made tall by the title strip beside it was stretched to two and a half
// times its height and the meter inside it grew with it. Standing nano up is
// meant to move the readings, not resize them, so the leftover height is simply
// left empty.
type NanoStackLayout struct{ Gap float32 }

// cardHeight is one height for every card, the way the flat grid gives every
// column one height: rows of differing height would leave the meters unaligned
// down the strip.
func (l *NanoStackLayout) cardHeight(objects []fyne.CanvasObject) float32 {
	height := float32(0)
	for _, object := range objects {
		height = max(height, object.MinSize().Height)
	}
	return height
}

func (l *NanoStackLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	height := l.cardHeight(objects)
	y := float32(0)
	for _, object := range objects {
		object.Resize(fyne.NewSize(size.Width, height))
		object.Move(fyne.NewPos(0, y))
		y += height + l.Gap
	}
}

func (l *NanoStackLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}
	width := float32(0)
	for _, object := range objects {
		width = max(width, object.MinSize().Width)
	}
	return fyne.NewSize(width, l.cardHeight(objects)*float32(len(objects))+l.Gap*float32(len(objects)-1))
}

// nanoCardWidth is the width one card is given when the cards run across the
// window. The upright layout hands a card the same number, so turning nano on
// its side moves the meters without resizing them.
func nanoCardWidth(cells int) float32 {
	if cells <= 0 {
		return NanoWidth - 2*nanoBodyPadX
	}
	body := max(NanoWidth, float32(cells)*NanoCellMinimumWidth) - 2*nanoBodyPadX
	return (body - float32(cells-1)*theme.Padding()) / float32(cells)
}

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

// nanoVerticalMinimumSize is the window vertical nano asks for: one card's
// worth of width beside the strip, and height enough for whichever of the
// readout and the strip needs more.
//
// The strip usually needs more, because seven stacked actions are taller than a
// handful of stacked cards. The readout keeps its own size regardless and the
// spare height stays empty rather than being shared out among the cards.
func (v *View) nanoVerticalMinimumSize() fyne.Size {
	cells := len(v.nanoCellStates())
	card := (&NanoStackLayout{Gap: NanoCardGap}).cardHeight(v.nanoBody.Objects)
	body := float32(cells)*card + max(0, float32(cells-1))*NanoCardGap + 2*3
	bar := float32(0)
	if v.nanoBar != nil {
		bar = v.nanoBar.MinSize().Height
	}
	// The border layout that puts the strip beside the readout leaves its own
	// padding between the two, which comes out of the readout's width. Without
	// that term here the cards would come up one padding narrower than the same
	// cards laid out flat.
	width := nanoCardWidth(cells) + 2*nanoBodyPadX + NanoBarWidth + theme.Padding()
	return fyne.NewSize(width, max(body, bar))
}
