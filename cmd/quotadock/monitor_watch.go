package main

import (
	"math"
	"sync"
	"time"
)

const (
	monitorWatchInterval   = 150 * time.Millisecond
	monitorDragQuietPeriod = 250 * time.Millisecond
)

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
