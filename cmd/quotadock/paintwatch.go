package main

import (
	"image"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/diagnostics"
)

// paintCheckDelays are measured from the moment the window is shown. The first
// is late enough that a healthy start has painted, the last late enough that a
// slow GPU handover is not mistaken for a fault.
var paintCheckDelays = []time.Duration{2500 * time.Millisecond, 6 * time.Second, 12 * time.Second}

// startPaintWatch schedules the startup paint checks. Capture and repaint both
// touch the canvas, so every step runs on the Fyne thread; the scheduling
// itself stays off it, matching how rounded corners and window position are
// already re-applied after startup.
func startPaintWatch(shown *atomic.Bool, window fyne.Window, rebuild func()) {
	watch := &paintWatch{
		capture: func() (image.Image, bool) {
			if !shown.Load() {
				return nil, false
			}
			return window.Canvas().Capture(), true
		},
		repairs: []paintRepair{
			{name: "canvas_refresh", run: func() { window.Canvas().Refresh(window.Content()) }},
			{name: "rebuild_content", run: rebuild},
		},
		logEvent: func(event string, attrs ...any) { slog.Info(event, attrs...) },
	}
	var schedule func(index int)
	schedule = func(index int) {
		if index >= len(paintCheckDelays) {
			return
		}
		diagnostics.AfterFunc(paintCheckDelays[index], "paint_watch", func() {
			fyne.Do(func() {
				if watch.step(index == len(paintCheckDelays)-1) {
					return
				}
				schedule(index + 1)
			})
		})
	}
	schedule(0)
}

// blankThreshold is the share of one flat colour above which a window counts as
// never painted. A real screen carries text, meters and icons, so it sits far
// below this; the margin exists for the theme-coloured background alone.
const blankThreshold = 0.995

// paintSampleStride keeps a full-window capture affordable on the UI thread.
const paintSampleStride = 4

// paintWatch answers one question after startup: did anything actually get
// drawn? The app has been observed running forty minutes with a healthy data
// layer behind a window that painted nothing, and neither Fyne nor Windows
// reported a fault, so the only reliable evidence is the pixels themselves.
//
// It is a state machine rather than a goroutine so the schedule (and the tests)
// can drive it step by step without sleeping.
type paintWatch struct {
	// capture returns the current window image. The bool reports whether
	// looking was meaningful at all: a hidden or minimised window is blank for
	// entirely legitimate reasons and must not be treated as a fault.
	capture func() (image.Image, bool)
	// repairs are escalating recovery actions, applied one per failed attempt.
	repairs []paintRepair
	logEvent func(event string, attrs ...any)

	attempts int
	repaired bool
	finished bool
}

type paintRepair struct {
	name string
	run  func()
}

// step runs one check. It returns true once the watchdog is finished, whether
// that finish was a success, a skip, or a give-up.
func (p *paintWatch) step(final bool) bool {
	if p.finished {
		return true
	}
	p.attempts++
	img, ok := p.capture()
	if !ok {
		p.finished = true
		return true
	}
	ratio := diagnostics.BlankRatio(img, paintSampleStride)
	if ratio < blankThreshold {
		p.logEvent("render.paint", "ok", true, "ratio", roundRatio(ratio), "attempt", p.attempts, "repaired", p.repaired)
		p.finished = true
		return true
	}
	// Give up rather than keep repainting: three failures mean the surface is
	// broken in a way this watchdog cannot reach, and the log line is what makes
	// the next occurrence diagnosable.
	if final {
		p.logEvent("render.blank", "ok", false, "ratio", roundRatio(ratio), "attempt", p.attempts, "action", "none")
		p.finished = true
		return true
	}
	action := "none"
	if index := p.attempts - 1; index < len(p.repairs) {
		repair := p.repairs[index]
		action = repair.name
		if repair.run != nil {
			repair.run()
			p.repaired = true
		}
	}
	p.logEvent("render.blank", "ok", false, "ratio", roundRatio(ratio), "attempt", p.attempts, "action", action)
	return false
}

// roundRatio trims the ratio to three decimals. The extra digits carry no
// meaning at this sample size and only make log lines harder to scan.
func roundRatio(ratio float64) float64 {
	return math.Round(ratio*1000) / 1000
}
