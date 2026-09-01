package ui

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestLaneAtPicksTheGroupUnderThePointer(t *testing.T) {
	bounds := []laneBound{
		{id: "claude", top: 0, bottom: 40},
		{id: "codex", top: 40, bottom: 70},
		{id: "grok", top: 70, bottom: 100},
	}
	tests := []struct {
		name string
		y    float32
		want string
	}{
		{name: "inside the first", y: 10, want: "claude"},
		{name: "on a boundary belongs below", y: 40, want: "codex"},
		{name: "inside the last", y: 95, want: "grok"},
		// Overshooting the list has to land on the end group. Answering "no
		// group" would strand a drag that ran past the window edge.
		{name: "above the list", y: -30, want: "claude"},
		{name: "below the list", y: 400, want: "grok"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index := laneAt(bounds, test.y)
			if index < 0 || bounds[index].id != test.want {
				t.Fatalf("laneAt(%v) = %d, want %s", test.y, index, test.want)
			}
		})
	}
	if laneAt(nil, 10) != -1 {
		t.Fatal("an unmeasured body must not report a group")
	}
}

// A provider the user has switched off has no slot to drag over, but it still
// has a place in the order. Rewriting only the visible names keeps it there, so
// switching it back on does not find it somewhere else.
func TestApplyVisibleOrderLeavesHiddenProvidersWhereTheyWere(t *testing.T) {
	base := []string{"claude", "claude-auth", "codex", "antigravity", "grok"}
	shown := []string{"claude", "codex", "antigravity"}
	arranged := []string{"antigravity", "claude", "codex"}
	got := applyVisibleOrder(base, shown, arranged)
	// claude-auth held second place and grok last; only the three on screen move.
	want := []string{"antigravity", "claude-auth", "claude", "codex", "grok"}
	if !slices.Equal(got, want) {
		t.Fatalf("applyVisibleOrder = %v, want %v", got, want)
	}
	if !slices.Equal(base, []string{"claude", "claude-auth", "codex", "antigravity", "grok"}) {
		t.Fatalf("applyVisibleOrder modified its input: %v", base)
	}
	if applyVisibleOrder(base, shown, []string{"claude"}) != nil {
		t.Fatal("a mismatched arrangement must be refused rather than truncate the order")
	}
}

// Every pointer position has to mean one arrangement on its own. Deriving each
// one from the slots the drag started in — rather than from whatever the screen
// currently shows — is what stops a swap from being undone by the next event
// while the rebuilt body is still catching up.
func TestADragProducesTheSameOrderWhereverItPassed(t *testing.T) {
	// Each route runs on its own view: a finished drag rewrites the order, so
	// replaying on the same one would start the second route somewhere else.
	drag := func(wander bool) []string {
		view, _, _ := draggableView(t)
		surface := findReorderSurface(view.Root)
		bounds := laneGroupBounds(view.normalBody, view.normalCache.groups)
		if len(bounds) < 3 {
			t.Fatalf("the body measured %d groups, want at least three", len(bounds))
		}
		last := bounds[len(bounds)-1]
		surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, bounds[0].top+1)}})
		if wander {
			for _, bound := range bounds {
				surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, bound.top+1)}})
				surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, bound.bottom-1)}})
			}
		}
		surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, last.bottom-1)}})
		order := slices.Clone(view.dragOrder)
		surface.DragEnd()
		return order
	}

	direct, wandering := drag(false), drag(true)
	if !slices.Equal(direct, wandering) {
		t.Fatalf("the route changed the result: straight %v, wandering %v", direct, wandering)
	}
	if len(direct) == 0 || direct[len(direct)-1] != "claude" {
		t.Fatalf("dragging the first provider to the bottom did not land it there: %v", direct)
	}
}

// findReorderSurface locates the drag surface the way the driver does, by
// walking the rendered tree, so the test also proves the surface is actually
// mounted on the screen.
func findReorderSurface(object fyne.CanvasObject) *LaneReorderSurface {
	switch typed := object.(type) {
	case *LaneReorderSurface:
		return typed
	case *fyne.Container:
		for _, child := range typed.Objects {
			if found := findReorderSurface(child); found != nil {
				return found
			}
		}
	}
	return nil
}

func draggableView(t *testing.T) (*View, fyne.Window, *settings.Config) {
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
	config.ShowClaudeAuth = true
	config.ShowGrok = true
	config = config.Validated()
	saved := settings.Config{}
	view := NewView(nil, catalog, i18n.English, config, Actions{
		AppVersion:    testAppVersion,
		ConfigChanged: func(next settings.Config) { saved = next },
	})
	window := test.NewWindow(view.Root)
	window.SetPadded(false)
	t.Cleanup(window.Close)
	view.SetState(orderedState())
	view.Show(NormalScreen)
	window.Resize(fyne.NewSize(NormalWidth, 900))
	return view, window, &saved
}

// The end-to-end path: a drag on the window body has to rearrange the lanes as
// the pointer travels and hand the result to the settings owner when it stops.
func TestDraggingAProviderGroupReordersAndSaves(t *testing.T) {
	view, _, saved := draggableView(t)
	surface := findReorderSurface(view.Root)
	if surface == nil {
		t.Fatal("no reorder surface is mounted on the window")
	}
	bounds := laneGroupBounds(view.normalBody, view.normalCache.groups)
	if len(bounds) < 2 {
		t.Fatalf("the body measured %d groups, want the whole provider list", len(bounds))
	}
	first, second := bounds[0].id, bounds[1].id
	// Building the screens settles the controls, which reports the config once.
	// Clear that so the assertions below only see what the drag caused.
	*saved = settings.Config{}
	grab := bounds[0].top + 2
	drop := bounds[1].bottom - 2

	surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, grab)}})
	surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, drop)}})

	if got := laneIDs(view.visibleLanes()); got[0] != second || got[1] != first {
		t.Fatalf("the screen did not follow the drag: %v", got)
	}
	if saved.LaneOrder != nil {
		t.Fatal("the settings file was written before the drag finished")
	}

	surface.DragEnd()
	if len(saved.LaneOrder) == 0 {
		t.Fatal("the finished drag was never saved")
	}
	if slices.Index(saved.LaneOrder, second) > slices.Index(saved.LaneOrder, first) {
		t.Fatalf("saved order kept the old arrangement: %v", saved.LaneOrder)
	}
	if view.dragOrder != nil || view.dragLane != "" {
		t.Fatal("the drag left state behind")
	}
}

// A press that never moves far enough to leave its own group must not rewrite
// the settings file: the order did not change, and a write would churn it on
// every stray click-drag over the rows.
func TestADragInsideOneGroupSavesNothing(t *testing.T) {
	view, _, saved := draggableView(t)
	surface := findReorderSurface(view.Root)
	bounds := laneGroupBounds(view.normalBody, view.normalCache.groups)
	if len(bounds) == 0 {
		t.Fatal("the body measured no groups")
	}
	before := strings.Join(laneIDs(view.visibleLanes()), ",")
	*saved = settings.Config{}

	surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, bounds[0].top+1)}})
	surface.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, bounds[0].bottom-1)}})
	surface.DragEnd()

	if after := strings.Join(laneIDs(view.visibleLanes()), ","); after != before {
		t.Fatalf("lane order changed from %s to %s", before, after)
	}
	if saved.LaneOrder != nil {
		t.Fatalf("an unchanged order was written anyway: %v", saved.LaneOrder)
	}
}

// The surface must stay drag-only. Anything more would take taps and hovers
// away from the rows it covers, which is where the tooltips live.
func TestReorderSurfaceDoesNotSwallowTapsOrHovers(t *testing.T) {
	surface := NewLaneReorderSurface(nil, nil, nil)
	if _, tappable := interface{}(surface).(fyne.Tappable); tappable {
		t.Fatal("the reorder surface would swallow taps meant for the rows")
	}
	if _, hoverable := interface{}(surface).(interface {
		MouseIn(*fyne.PointEvent)
	}); hoverable {
		t.Fatal("the reorder surface would swallow hovers meant for the tooltips")
	}
	if size := surface.CreateRenderer().MinSize(); size.Width != 0 || size.Height != 0 {
		t.Fatalf("the reorder surface claims %v of its own, which would floor an empty list", size)
	}
}
