package main

import (
	"context"
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
var paintCheckDelays = []time.Duration{2500 * time.Millisecond, 5 * time.Second, 9 * time.Second, 15 * time.Second}

// visibilityPollInterval is how often the watchdog asks Windows whether the
// window is on screen. The app already polls foreground state at this rate for
// the idle trimmer, so the cost is known and small.
const visibilityPollInterval = time.Second

// startPaintWatch watches for the window becoming visible and checks, each
// time, that something was actually drawn.
//
// It anchors on visibility rather than process start because the autostart
// entry launches with --hidden: a start-anchored check fires while the window
// is still in the tray, finds nothing to look at, and stands down before the
// user ever opens it. That is exactly how the first version of this watchdog
// slept through the fault it was written for.
//
// Visibility is read from Windows on every tick instead of being tracked in a
// flag, because the single-instance guard restores the window with ShowWindow
// from a *second* process. The running process is never told, so any flag it
// keeps is wrong precisely in the case that matters - clicking the desktop
// icon while the app sits hidden in the tray.
//
// Capture and repaint both touch the canvas, so every step runs on the Fyne
// thread; the polling stays off it.
func startPaintWatch(ctx context.Context, visible func() bool, window fyne.Window, rebuild func()) {
	watch := &paintWatch{
		capture: func() (image.Image, bool) {
			if !visible() {
				return nil, false
			}
			return window.Canvas().Capture(), true
		},
		repairs: []paintRepair{
			// Tell Fyne the window is up. This is first because it addresses the
			// measured cause: the single-instance guard restores the window with
			// a raw Win32 ShowWindow from a second process, so Fyne still has it
			// marked hidden and skips drawing it entirely. Show() is idempotent
			// on an already-visible window, so it costs nothing when the blank
			// came from somewhere else.
			{name: "window_show", run: window.Show},
			{name: "canvas_refresh", run: func() { window.Canvas().Refresh(window.Content()) }},
			{name: "rebuild_content", run: rebuild},
		},
		logEvent: func(event string, attrs ...any) { slog.Info(event, attrs...) },
	}
	var running atomic.Bool
	var schedule func(index int)
	schedule = func(index int) {
		if index >= len(paintCheckDelays) {
			running.Store(false)
			return
		}
		diagnostics.AfterFunc(paintCheckDelays[index], "paint_watch", func() {
			fyne.Do(func() {
				if watch.step(index == len(paintCheckDelays)-1) {
					running.Store(false)
					return
				}
				schedule(index + 1)
			})
		})
	}
	arm := func() {
		// One round at a time. A window shown again while a round is still in
		// flight must not stack overlapping capture schedules.
		if !running.CompareAndSwap(false, true) {
			return
		}
		watch.reset()
		// Sync Fyne with what Windows already did. The single-instance guard
		// restores the window from a second process, so Fyne can still have it
		// marked hidden and draw nothing at all. Doing this the moment the
		// transition is seen keeps the blank period to about one poll instead
		// of waiting for the first check to diagnose it. Show is idempotent, so
		// the ordinary tray path pays nothing for it.
		fyne.Do(window.Show)
		schedule(0)
	}

	diagnostics.Go("paint_watch_visibility", func() {
		ticker := time.NewTicker(visibilityPollInterval)
		defer ticker.Stop()
		wasVisible := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nowVisible := visible()
				if nowVisible && !wasVisible {
					arm()
				}
				wasVisible = nowVisible
			}
		}
	})
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

// reset prepares a fresh round. The watchdog is armed once per show, so the
// attempt counter and the repair budget start over each time.
func (p *paintWatch) reset() {
	p.attempts = 0
	p.repaired = false
	p.finished = false
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
		// The window went back to the tray mid-round. Say so: an unexplained
		// gap in the log is what let the first version of this watchdog look
		// identical whether it skipped or never ran at all.
		p.logEvent("render.skip", "reason", "hidden", "attempt", p.attempts)
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
