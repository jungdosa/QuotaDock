package windows

import "testing"

func TestFitToWorkAreaCorrectsRightOverflow(t *testing.T) {
	window := Rect{X: 1700, Y: 100, Width: 560, Height: 700}
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}

	got := FitToWorkArea(window, []Rect{area})

	if got != (Rect{X: 1360, Y: 100, Width: 560, Height: 700}) {
		t.Fatalf("right overflow correction = %+v", got)
	}
}

func TestFitToWorkAreaCorrectsBottomOverflow(t *testing.T) {
	window := Rect{X: 100, Y: 900, Width: 560, Height: 700}
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}

	got := FitToWorkArea(window, []Rect{area})

	if got != (Rect{X: 100, Y: 340, Width: 560, Height: 700}) {
		t.Fatalf("bottom overflow correction = %+v", got)
	}
}

func TestFitToWorkAreaCorrectsRightAndBottomOverflow(t *testing.T) {
	window := Rect{X: 1700, Y: 900, Width: 560, Height: 700}
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}

	got := FitToWorkArea(window, []Rect{area})

	if got != (Rect{X: 1360, Y: 340, Width: 560, Height: 700}) {
		t.Fatalf("corner overflow correction = %+v", got)
	}
}

func TestFitToWorkAreaLeavesContainedWindowUnchanged(t *testing.T) {
	window := Rect{X: 200, Y: 100, Width: 560, Height: 700}
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}

	if got := FitToWorkArea(window, []Rect{area}); got != window {
		t.Fatalf("contained window changed: %+v", got)
	}
}

func TestFitToWorkAreaAlignsOversizedWindowToTopLeft(t *testing.T) {
	window := Rect{X: 500, Y: 300, Width: 2200, Height: 1200}
	area := Rect{X: 100, Y: 50, Width: 1920, Height: 1040}

	got := FitToWorkArea(window, []Rect{area})

	if got.X != area.X || got.Y != area.Y {
		t.Fatalf("oversized window position = %+v", got)
	}
	if got.Width != window.Width || got.Height != window.Height {
		t.Fatalf("oversized window size changed: %+v", got)
	}
}

func TestFitToWorkAreaUsesLargestOverlapWithNegativeMonitorCoordinates(t *testing.T) {
	areas := []Rect{
		{X: 0, Y: 0, Width: 1920, Height: 1040},
		{X: -1600, Y: -200, Width: 1600, Height: 900},
	}
	window := Rect{X: -500, Y: 500, Width: 560, Height: 500}

	got := FitToWorkArea(window, areas)
	want := Rect{X: -560, Y: 200, Width: 560, Height: 500}
	if got != want {
		t.Fatalf("multi-monitor correction = %+v, want %+v", got, want)
	}
}

func TestFitToWorkAreaWithoutAreasReturnsInput(t *testing.T) {
	window := Rect{X: 1700, Y: 900, Width: 560, Height: 700}

	if got := FitToWorkArea(window, nil); got != window {
		t.Fatalf("window without work areas changed: %+v", got)
	}
}

func TestFitToWorkAreaAlwaysPreservesSize(t *testing.T) {
	area := Rect{X: -1600, Y: -200, Width: 1600, Height: 900}
	windows := []Rect{
		{X: -1800, Y: -400, Width: 560, Height: 700},
		{X: -200, Y: 500, Width: 560, Height: 700},
		{X: 0, Y: 0, Width: 2200, Height: 1200},
	}

	for _, window := range windows {
		got := FitToWorkArea(window, []Rect{area})
		if got.Width != window.Width || got.Height != window.Height {
			t.Errorf("window size changed from %+v to %+v", window, got)
		}
	}
}

func TestDisplayChangeFitsWindowOnlyWhenAllWorkAreasAreOutside(t *testing.T) {
	previous := []Rect{
		{X: 0, Y: 0, Width: 1920, Height: 1040},
		{X: 1920, Y: 0, Width: 1920, Height: 1040},
	}
	current := []Rect{{X: 0, Y: 0, Width: 1920, Height: 1040}}
	window := Rect{X: 2200, Y: 100, Width: 560, Height: 300}

	changed, fitted, shouldFit := DisplayChange(previous, current, window)

	if !changed || !shouldFit {
		t.Fatalf("display change changed=%v shouldFit=%v", changed, shouldFit)
	}
	if fitted != (Rect{X: 1360, Y: 100, Width: 560, Height: 300}) {
		t.Fatalf("fitted window=%+v", fitted)
	}
}

func TestDisplayChangeLeavesWindowAloneWhenAreasAreUnchanged(t *testing.T) {
	areas := []Rect{
		{X: 0, Y: 0, Width: 1920, Height: 1040},
		{X: -1600, Y: 0, Width: 1600, Height: 900},
	}
	reordered := []Rect{areas[1], areas[0]}
	window := Rect{X: -1400, Y: 50, Width: 560, Height: 300}

	changed, fitted, shouldFit := DisplayChange(areas, reordered, window)

	if changed || shouldFit || fitted != window {
		t.Fatalf("unchanged areas changed=%v shouldFit=%v fitted=%+v", changed, shouldFit, fitted)
	}
}

func TestDisplayChangeDoesNotMoveOverlappingWindow(t *testing.T) {
	previous := []Rect{{X: 0, Y: 0, Width: 1920, Height: 1040}}
	current := []Rect{{X: 0, Y: 0, Width: 1280, Height: 720}}
	window := Rect{X: 1279, Y: 100, Width: 560, Height: 300}

	changed, fitted, shouldFit := DisplayChange(previous, current, window)

	if !changed || shouldFit || fitted != window {
		t.Fatalf("overlapping window changed=%v shouldFit=%v fitted=%+v", changed, shouldFit, fitted)
	}
}
