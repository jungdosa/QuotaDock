package windows

import (
	"testing"
	"time"
)

func TestIdleTrimmerDebouncesAndRearms(t *testing.T) {
	start := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	trimmer := NewIdleTrimmer(start, 20*time.Second, 5*time.Second)
	if trimmer.ShouldTrim(start.Add(19*time.Second), true, false) {
		t.Fatal("trim fired before the foreground idle delay")
	}
	if !trimmer.ShouldTrim(start.Add(20*time.Second), true, false) {
		t.Fatal("trim did not fire after the foreground idle delay")
	}
	if trimmer.ShouldTrim(start.Add(40*time.Second), true, false) {
		t.Fatal("trim was not debounced")
	}
	trimmer.Activity(start.Add(41 * time.Second))
	if !trimmer.ShouldTrim(start.Add(61*time.Second), true, false) {
		t.Fatal("activity did not rearm idle trimming")
	}
}

func TestIdleTrimmerBackgroundGraceAndRenderGuard(t *testing.T) {
	start := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	trimmer := NewIdleTrimmer(start, time.Minute, 5*time.Second)
	if trimmer.ShouldTrim(start.Add(time.Second), false, false) {
		t.Fatal("background trim fired before its grace period")
	}
	if trimmer.ShouldTrim(start.Add(6*time.Second), false, true) {
		t.Fatal("trim fired during an active render")
	}
	if !trimmer.ShouldTrim(start.Add(7*time.Second), false, false) {
		t.Fatal("background trim did not fire after rendering ended")
	}
}
