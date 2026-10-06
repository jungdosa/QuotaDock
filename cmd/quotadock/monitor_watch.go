package main

import (
	"math"
	"sync"
	"time"
)

const (
	monitorWatchInterval   = 150 * time.Millisecond
	monitorDragQuietPeriod = 250 * time.Millisecond
	monitorUncloakDelay    = 160 * time.Millisecond
	monitorCloakLimit      = 600 * time.Millisecond
)

// monitorCloakState is owned by the UI thread. The generation rejects old
// crossings; the reservation rejects a timer superseded by a later resize.
// started remains fixed across crossings so frequent movement cannot keep the
// window cloaked beyond one limit interval.
type monitorCloakState struct {
	active      bool
	started     time.Time
	lastResize  time.Time
	generation  uint64
	reservation uint64
	generations int
}

func (s *monitorCloakState) begin(now time.Time, cloakSucceeded bool) (uint64, bool) {
	if !s.active {
		if !cloakSucceeded {
			return 0, false
		}
		s.active = true
		s.started = now
		s.generations = 0
	}
	s.generation++
	s.generations++
	return s.generation, true
}

func (s *monitorCloakState) resized(now time.Time, generation uint64) (uint64, bool) {
	if !s.active || generation != s.generation {
		return 0, false
	}
	s.lastResize = now
	s.reservation++
	return s.reservation, true
}

func (s *monitorCloakState) ready(now time.Time, generation, reservation uint64) bool {
	return s.active && generation == s.generation && reservation == s.reservation &&
		!now.Before(s.lastResize.Add(monitorUncloakDelay))
}

func (s *monitorCloakState) forceReady(now time.Time) bool {
	return s.active && !now.Before(s.started.Add(monitorCloakLimit))
}

func (s *monitorCloakState) finish(now time.Time) (time.Duration, int, bool) {
	if !s.active {
		return 0, 0, false
	}
	elapsed, generations := now.Sub(s.started), s.generations
	s.active = false
	return elapsed, generations, true
}

type monitorSignature struct {
	monitor     uintptr
	scale       float64
	canvasScale float32
}

func scaledDragOffset(offset int, startScale, currentScale float32) int {
	// Always convert the original physical offset, so repeated crossings do
	// not accumulate rounding error. An unavailable scale keeps that offset.
	if startScale <= 0 || currentScale <= 0 ||
		math.IsNaN(float64(startScale)) || math.IsNaN(float64(currentScale)) ||
		math.IsInf(float64(startScale), 0) || math.IsInf(float64(currentScale), 0) {
		return offset
	}
	return int(math.Round(float64(offset) * float64(currentScale) / float64(startScale)))
}

func monitorSignatureChanged(previous, current monitorSignature) bool {
	// The first valid observation establishes a baseline, not a move.
	return previous.monitor != 0 && current.monitor != 0 && previous != current
}

func monitorDragQuiet(now time.Time, lastMove int64) bool {
	return lastMove == 0 || now.Sub(time.Unix(0, lastMove)) >= monitorDragQuietPeriod
}

// monitorDragStaleAfter bounds how long a drag flag is trusted without any
// movement. A drag-end event that never arrives would otherwise leave every
// later resize skipping its position fit for the rest of the session.
const monitorDragStaleAfter = 2 * time.Second

func monitorDragActive(flagged bool, now time.Time, lastMove int64) bool {
	return flagged && lastMove != 0 && now.Sub(time.Unix(0, lastMove)) < monitorDragStaleAfter
}

// nativeMonitorChanged lets the polling goroutine skip the UI thread unless
// Windows reports a different monitor or DPI. A canvas-only change is caught
// by the drag callbacks, which read the canvas scale on the UI thread.
func nativeMonitorChanged(known, observed monitorSignature) bool {
	return known.monitor != observed.monitor || known.scale != observed.scale
}

// Polling and drag callbacks share one signature. Only UI callbacks commit
// changes, after re-reading the window and canvas to avoid stale samples.
type monitorWatchState struct {
	mu      sync.Mutex
	current monitorSignature
}

func (s *monitorWatchState) snapshot() monitorSignature {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *monitorWatchState) update(next monitorSignature) (monitorSignature, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.current
	if next.monitor == 0 {
		return previous, false
	}
	s.current = next
	return previous, monitorSignatureChanged(previous, next)
}
