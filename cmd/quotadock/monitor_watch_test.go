package main

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestMonitorSignatureChanged(t *testing.T) {
	for _, tt := range []struct {
		name              string
		previous, current monitorSignature
		want              bool
	}{
		{"first observation", monitorSignature{}, monitorSignature{1, 1, 1}, false},
		{"unavailable monitor", monitorSignature{1, 1, 1}, monitorSignature{0, 1, 1}, false},
		{"unchanged", monitorSignature{1, 1, 1}, monitorSignature{1, 1, 1}, false},
		{"same scale different monitor", monitorSignature{1, 1, 1}, monitorSignature{2, 1, 1}, true},
		{"same monitor different scale", monitorSignature{1, 1, 1}, monitorSignature{1, 1.5, 1}, true},
		{"both changed", monitorSignature{1, 1, 1}, monitorSignature{2, 1.5, 1.5}, true},
		{"return to lower scale", monitorSignature{2, 1.5, 1.5}, monitorSignature{1, 1, 1}, true},
		{"canvas changes before native DPI", monitorSignature{1, 1, 1}, monitorSignature{1, 1, 1.5}, true},
		{"canvas changes after native DPI", monitorSignature{2, 1.5, 1}, monitorSignature{2, 1.5, 1.5}, true},
		{"canvas returns before native DPI", monitorSignature{2, 1.5, 1.5}, monitorSignature{2, 1.5, 1}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := monitorSignatureChanged(tt.previous, tt.current); got != tt.want {
				t.Fatalf("monitorSignatureChanged(%v, %v) = %v, want %v", tt.previous, tt.current, got, tt.want)
			}
		})
	}
}

func TestMonitorDragQuiet(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tt := range []struct {
		name     string
		lastMove int64
		want     bool
	}{
		{"never dragged", 0, true},
		{"moving now", now.UnixNano(), false},
		{"before boundary", now.Add(-250*time.Millisecond + time.Nanosecond).UnixNano(), false},
		{"at boundary", now.Add(-250 * time.Millisecond).UnixNano(), true},
		{"after boundary", now.Add(-251 * time.Millisecond).UnixNano(), true},
		{"clock moved backwards", now.Add(time.Second).UnixNano(), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := monitorDragQuiet(now, tt.lastMove); got != tt.want {
				t.Fatalf("monitorDragQuiet = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonitorWatchSharesDragEndSignature(t *testing.T) {
	var state monitorWatchState
	initial := monitorSignature{1, 1, 1}
	landed := monitorSignature{2, 1.5, 1.5}
	if _, changed := state.update(initial); changed {
		t.Fatal("initial observation requested a resize")
	}
	// The watch sees a change, but drag end handles it before the UI queue runs.
	if landed == state.snapshot() {
		t.Fatal("watch missed the monitor change")
	}
	if previous, changed := state.update(landed); !changed || previous != initial {
		t.Fatalf("drag end update = (%v, %v), want (%v, true)", previous, changed, initial)
	}
	if state.snapshot() != landed {
		t.Fatal("watch did not see the drag end signature")
	}
	if _, changed := state.update(landed); changed {
		t.Fatal("queued watch duplicated the drag end resize")
	}
	if _, changed := state.update(monitorSignature{}); changed || state.snapshot() != landed {
		t.Fatal("unavailable monitor discarded the last valid signature")
	}
	if _, changed := state.update(initial); !changed {
		t.Fatal("return to the original monitor did not request a resize")
	}
}

func TestMonitorWatchConcurrentSnapshots(t *testing.T) {
	var state monitorWatchState
	signatures := [2]monitorSignature{{1, 1, 1}, {2, 1.5, 1.5}}
	state.update(signatures[0])
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for range 1000 {
				if got := state.snapshot(); got != signatures[0] && got != signatures[1] {
					t.Errorf("partial monitor signature: %v", got)
					return
				}
			}
		})
	}
	for i := range 1000 {
		state.update(signatures[i%2])
	}
	readers.Wait()
}

func TestScaledDragOffset(t *testing.T) {
	for _, tt := range []struct {
		name                string
		offset              int
		startScale, current float32
		want                int
	}{
		{"unchanged", 37, 1.25, 1.25, 37},
		{"100 to 125 rounds down", 41, 1, 1.25, 51},
		{"100 to 150 rounds half up", 41, 1, 1.5, 62},
		{"125 to 100", 41, 1.25, 1, 33},
		{"150 to 100", 41, 1.5, 1, 27},
		{"125 to 150", 41, 1.25, 1.5, 49},
		{"150 to 125", 41, 1.5, 1.25, 34},
		{"negative offset", -41, 1, 1.5, -62},
		{"zero offset", 0, 1, 1.5, 0},
		{"missing start scale", 41, 0, 1.5, 41},
		{"missing current scale", 41, 1, 0, 41},
		{"negative scale", 41, -1, 1.5, 41},
		{"NaN start scale", 41, float32(math.NaN()), 1, 41},
		{"NaN current scale", 41, 1, float32(math.NaN()), 41},
		{"infinite start scale", 41, float32(math.Inf(1)), 1, 41},
		{"infinite current scale", 41, 1, float32(math.Inf(1)), 41},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := scaledDragOffset(tt.offset, tt.startScale, tt.current); got != tt.want {
				t.Fatalf("scaledDragOffset(%d, %v, %v) = %d, want %d", tt.offset, tt.startScale, tt.current, got, tt.want)
			}
		})
	}
}

func TestDragCrossingsShareSignatureWithoutOffsetDrift(t *testing.T) {
	var state monitorWatchState
	initial := monitorSignature{1, 1, 1}
	state.update(initial)
	for cycle := range 3 {
		for _, step := range []struct {
			signature monitorSignature
			offset    int
		}{
			{monitorSignature{1, 1, 1.5}, 62}, // Canvas can lead the native change.
			{monitorSignature{2, 1.5, 1.5}, 62},
			{monitorSignature{3, 1.25, 1.25}, 51},
			{initial, 41},
		} {
			if _, changed := state.update(step.signature); !changed {
				t.Fatalf("cycle %d: crossing to %v was missed", cycle, step.signature)
			}
			if state.snapshot() != step.signature {
				t.Fatal("watch did not see the crossing signature")
			}
			if _, changed := state.update(step.signature); changed {
				t.Fatal("watch or drag end duplicated the crossing resize")
			}
			if got := scaledDragOffset(41, initial.canvasScale, step.signature.canvasScale); got != step.offset {
				t.Fatalf("cycle %d: offset drifted to %d, want %d", cycle, got, step.offset)
			}
		}
	}
}

func TestMonitorDragActiveExpiresWithoutMovement(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tt := range []struct {
		name     string
		flagged  bool
		lastMove int64
		want     bool
	}{
		{"not dragging", false, now.UnixNano(), false},
		{"flagged without any move", true, 0, false},
		{"moving now", true, now.UnixNano(), true},
		{"held still briefly", true, now.Add(-time.Second).UnixNano(), true},
		{"drag end lost", true, now.Add(-monitorDragStaleAfter).UnixNano(), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := monitorDragActive(tt.flagged, now, tt.lastMove); got != tt.want {
				t.Fatalf("monitorDragActive = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNativeMonitorChangedIgnoresCanvasScale(t *testing.T) {
	known := monitorSignature{monitor: 1, scale: 1.5, canvasScale: 1.5}
	if nativeMonitorChanged(known, monitorSignature{monitor: 1, scale: 1.5}) {
		t.Fatal("an unchanged monitor woke the UI thread")
	}
	if !nativeMonitorChanged(known, monitorSignature{monitor: 2, scale: 1.5}) {
		t.Fatal("a monitor change was missed")
	}
	if !nativeMonitorChanged(known, monitorSignature{monitor: 1, scale: 1}) {
		t.Fatal("a DPI change was missed")
	}
}
