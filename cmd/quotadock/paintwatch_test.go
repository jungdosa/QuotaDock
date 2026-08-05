package main

import (
	"image"
	"image/color"
	"testing"
)

type paintEvent struct {
	name  string
	attrs map[string]any
}

func recordEvents(into *[]paintEvent) func(string, ...any) {
	return func(event string, attrs ...any) {
		fields := map[string]any{}
		for i := 0; i+1 < len(attrs); i += 2 {
			key, _ := attrs[i].(string)
			fields[key] = attrs[i+1]
		}
		*into = append(*into, paintEvent{name: event, attrs: fields})
	}
}

func flatImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 240, G: 244, B: 248, A: 255})
		}
	}
	return img
}

func paintedImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			shade := color.RGBA{R: 240, G: 244, B: 248, A: 255}
			if x < 20 {
				shade = color.RGBA{R: 20, G: 30, B: 40, A: 255}
			}
			img.Set(x, y, shade)
		}
	}
	return img
}

func TestPaintWatchAcceptsFirstPaintWithoutRepair(t *testing.T) {
	var events []paintEvent
	repaired := 0
	watch := &paintWatch{
		capture:  func() (image.Image, bool) { return paintedImage(), true },
		repairs:  []paintRepair{{name: "canvas_refresh", run: func() { repaired++ }}},
		logEvent: recordEvents(&events),
	}
	if !watch.step(false) {
		t.Fatal("watchdog did not finish on a painted window")
	}
	if repaired != 0 {
		t.Fatalf("repair ran %d times on a painted window, want 0", repaired)
	}
	if len(events) != 1 || events[0].name != "render.paint" || events[0].attrs["ok"] != true {
		t.Fatalf("events=%+v, want one render.paint ok=true", events)
	}
	if events[0].attrs["repaired"] != false {
		t.Fatalf("render.paint repaired=%v, want false", events[0].attrs["repaired"])
	}
}

func TestPaintWatchRepairsBlankWindowAndReportsRecovery(t *testing.T) {
	var events []paintEvent
	blank := true
	watch := &paintWatch{
		capture: func() (image.Image, bool) {
			if blank {
				return flatImage(), true
			}
			return paintedImage(), true
		},
		repairs:  []paintRepair{{name: "canvas_refresh", run: func() { blank = false }}},
		logEvent: recordEvents(&events),
	}
	if watch.step(false) {
		t.Fatal("watchdog finished on the first blank capture, want a retry")
	}
	if !watch.step(true) {
		t.Fatal("watchdog did not finish after the repair took effect")
	}
	if len(events) != 2 {
		t.Fatalf("events=%+v, want a render.blank then a render.paint", events)
	}
	if events[0].name != "render.blank" || events[0].attrs["action"] != "canvas_refresh" {
		t.Fatalf("first event=%+v, want render.blank action=canvas_refresh", events[0])
	}
	if events[1].name != "render.paint" || events[1].attrs["repaired"] != true {
		t.Fatalf("second event=%+v, want render.paint repaired=true", events[1])
	}
}

// Giving up has to be visible. A watchdog that goes quiet on the case it exists
// for is indistinguishable from one that never ran.
func TestPaintWatchGivesUpLoudlyAfterEveryAttempt(t *testing.T) {
	var events []paintEvent
	watch := &paintWatch{
		capture: func() (image.Image, bool) { return flatImage(), true },
		repairs: []paintRepair{
			{name: "canvas_refresh", run: func() {}},
			{name: "rebuild_content", run: func() {}},
		},
		logEvent: recordEvents(&events),
	}
	for attempt, final := range []bool{false, false, true} {
		if watch.step(final) != final {
			t.Fatalf("attempt %d finished=%v, want %v", attempt+1, !final, final)
		}
	}
	if len(events) != 3 {
		t.Fatalf("events=%+v, want three render.blank lines", events)
	}
	actions := []string{"canvas_refresh", "rebuild_content", "none"}
	for i, want := range actions {
		if events[i].name != "render.blank" || events[i].attrs["action"] != want {
			t.Fatalf("event %d=%+v, want render.blank action=%s", i, events[i], want)
		}
	}
	if events[2].attrs["ok"] != false {
		t.Fatalf("final event ok=%v, want false", events[2].attrs["ok"])
	}
}

// A window in the tray is blank for a legitimate reason, so the watchdog must
// not repair it - but it must still say that it looked and stood down. Silence
// here is what made the first version indistinguishable from one that never ran.
func TestPaintWatchSkipsHiddenWindowButSaysSo(t *testing.T) {
	var events []paintEvent
	captures, repaired := 0, 0
	watch := &paintWatch{
		capture:  func() (image.Image, bool) { captures++; return nil, false },
		repairs:  []paintRepair{{name: "canvas_refresh", run: func() { repaired++ }}},
		logEvent: recordEvents(&events),
	}
	if !watch.step(false) {
		t.Fatal("watchdog did not finish on a hidden window")
	}
	if captures != 1 || repaired != 0 {
		t.Fatalf("captures=%d repaired=%d, want 1/0", captures, repaired)
	}
	if len(events) != 1 || events[0].name != "render.skip" || events[0].attrs["reason"] != "hidden" {
		t.Fatalf("events=%+v, want one render.skip reason=hidden", events)
	}
}

// Arming again after a finished round has to start over. A hidden start ends
// the first round immediately, and the round that matters is the one armed when
// the tray finally brings the window up.
func TestPaintWatchResetAllowsANewRound(t *testing.T) {
	var events []paintEvent
	hidden := true
	watch := &paintWatch{
		capture: func() (image.Image, bool) {
			if hidden {
				return nil, false
			}
			return paintedImage(), true
		},
		logEvent: recordEvents(&events),
	}
	watch.step(false)
	hidden = false
	watch.step(false)
	if len(events) != 1 {
		t.Fatalf("events=%+v, want the finished round to stay finished without a reset", events)
	}
	watch.reset()
	if !watch.step(false) {
		t.Fatal("reset round did not run")
	}
	if len(events) != 2 || events[1].name != "render.paint" || events[1].attrs["attempt"] != 1 {
		t.Fatalf("events=%+v, want render.skip then render.paint attempt=1", events)
	}
}

func TestPaintWatchStopsAfterFinishing(t *testing.T) {
	var events []paintEvent
	watch := &paintWatch{
		capture:  func() (image.Image, bool) { return paintedImage(), true },
		logEvent: recordEvents(&events),
	}
	watch.step(false)
	watch.step(false)
	if len(events) != 1 {
		t.Fatalf("events=%+v, want a single line after the watchdog finished", events)
	}
}
