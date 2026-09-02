package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func nanoView(t *testing.T, vertical bool) *View {
	t.Helper()
	app := test.NewApp()
	app.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	t.Cleanup(app.Quit)
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	config := settings.Default()
	config.Theme = settings.ThemeDark
	config.NanoVertical = vertical
	view := NewView(nil, catalog, i18n.English, config.Validated(), Actions{AppVersion: testAppVersion})
	window := test.NewWindow(view.Root)
	window.SetPadded(false)
	t.Cleanup(window.Close)
	view.SetState(orderedState())
	view.Show(NanoScreen)
	window.Resize(view.MinimumSize(NanoScreen))
	return view
}

// Standing nano up turns a wide short readout into a narrow tall one. If the
// window kept its old shape the cards would have nowhere to stack.
func TestStandingNanoUpMakesTheWindowTallerThanItIsWide(t *testing.T) {
	flat := nanoView(t, false).MinimumSize(NanoScreen)
	upright := nanoView(t, true).MinimumSize(NanoScreen)
	if flat.Width <= flat.Height {
		t.Fatalf("flat nano is %v, want wider than it is tall", flat)
	}
	if upright.Height <= upright.Width {
		t.Fatalf("upright nano is %v, want taller than it is wide", upright)
	}
	if upright.Width >= flat.Width {
		t.Fatalf("upright nano is %.0f wide against flat %.0f, want narrower", upright.Width, flat.Width)
	}
}

// The cards run in one direction or the other; nothing else about them changes.
// This reads the laid-out positions rather than the layout object, because the
// question is where the cards actually end up.
func TestNanoCardsRunDownTheWindowWhenUpright(t *testing.T) {
	step := func(view *View) (across, down float32, count int) {
		objects := view.nanoBody.Objects
		if len(objects) < 2 {
			t.Fatalf("nano drew %d cards, want at least two", len(objects))
		}
		first, second := objects[0].Position(), objects[1].Position()
		return second.X - first.X, second.Y - first.Y, len(objects)
	}
	flatAcross, flatDown, flatCount := step(nanoView(t, false))
	if flatAcross <= 0 || flatDown != 0 {
		t.Fatalf("flat nano steps by (%.1f, %.1f), want a step across only", flatAcross, flatDown)
	}
	upright := nanoView(t, true)
	uprightAcross, uprightDown, uprightCount := step(upright)
	if uprightDown <= 0 || uprightAcross != 0 {
		t.Fatalf("upright nano steps by (%.1f, %.1f), want a step down only", uprightAcross, uprightDown)
	}
	if uprightCount != flatCount {
		t.Fatalf("upright nano drew %d cards against the flat layout's %d", uprightCount, flatCount)
	}
	if got := len(upright.nanoCache.cells); got != uprightCount {
		t.Fatalf("upright nano cached %d cards for %d drawn", got, uprightCount)
	}
}

// The strip is the title bar stood on its end: the same actions in the same
// order. A different order here would make the two layouts two different bars.
func TestTheVerticalStripCarriesTheSameActionsInTheSameOrder(t *testing.T) {
	view := nanoView(t, true)
	if view.nanoBar == nil {
		t.Fatal("upright nano has no title strip")
	}
	if width := view.nanoBar.MinSize().Width; width > NanoBarWidth {
		t.Fatalf("the strip wants %.0f of width, more than the %.0f it is given", width, NanoBarWidth)
	}
	// The strip is walked as rendered and matched against the one list the bar
	// is built from, so the two cannot drift into different sets or orders.
	want := view.titleButtons(settings.ModeNano)
	var got []string
	var walk func(fyne.CanvasObject)
	walk = func(object fyne.CanvasObject) {
		switch typed := object.(type) {
		case *SmallButton:
			got = append(got, typed.Tooltip)
		case *fyne.Container:
			for _, child := range typed.Objects {
				walk(child)
			}
		}
	}
	walk(view.nanoBar)
	if len(got) != len(want) {
		t.Fatalf("the strip drew %d actions against the bar's %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index].Tooltip {
			t.Fatalf("action %d on the strip is %q, want %q", index, got[index], want[index].Tooltip)
		}
	}
}

// Nano is the only mode the orientation applies to, so it is the only one that
// carries the toggle. On the others it would be a control with nothing to do.
func TestOnlyNanoOffersTheOrientationToggle(t *testing.T) {
	view := nanoView(t, false)
	nano := len(view.titleButtons(settings.ModeNano))
	for _, mode := range []settings.DisplayMode{settings.ModeNormal, settings.ModeCompact} {
		if got := len(view.titleButtons(mode)); got != nano-1 {
			t.Fatalf("%s has %d actions, want one fewer than nano's %d", mode, got, nano)
		}
	}
}

// The toggle shows the layout it moves to, the way the display-mode button
// already does. Showing the current one would read as a state badge and leave
// the button looking inert.
func TestTheToggleShowsTheLayoutItMovesTo(t *testing.T) {
	view := nanoView(t, false)
	flat := nanoOrientationResource(false, view.colors)
	upright := nanoOrientationResource(true, view.colors)
	if flat.Name() != "nano-vertical.svg" {
		t.Fatalf("flat nano offers %q, want the upright mark", flat.Name())
	}
	if upright.Name() != "nano-horizontal.svg" {
		t.Fatalf("upright nano offers %q, want the flat mark", upright.Name())
	}
	if view.nanoOrientationTooltip() == nanoView(t, true).nanoOrientationTooltip() {
		t.Fatal("both layouts describe the toggle the same way")
	}
}

// The strip has to reach the bottom of the window. Its actions come to less
// than a handful of stacked cards, and on the real frame the only spare space
// in the upright window was the band of bare background beneath its last
// button, where a wrapper had pinned the strip to the height of its actions.
func TestTheVerticalStripRunsTheFullHeightOfTheWindow(t *testing.T) {
	view := nanoView(t, true)
	if view.nanoBar == nil {
		t.Fatal("upright nano has no title strip")
	}
	body := view.nanoBody.Size().Height + 2*3
	if strip := view.nanoBar.Size().Height; strip < body {
		t.Fatalf("the strip is %.1f tall against a %.1f readout, leaving %.1f of bare window beneath its actions", strip, body, body-strip)
	}
	if width := view.nanoBar.Size().Width; width != NanoBarWidth {
		t.Fatalf("the strip is %.1f wide, want %.1f", width, NanoBarWidth)
	}
}
