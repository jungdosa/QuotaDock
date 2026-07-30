package windows

import (
	"sync"
	"time"
)

const (
	DefaultIdleTrimDelay       = 20 * time.Second
	DefaultBackgroundTrimDelay = 5 * time.Second
)

// IdleTrimmer debounces working-set trims across both foreground inactivity
// and a short background grace period. Activity rearms exactly one later trim.
type IdleTrimmer struct {
	mu              sync.Mutex
	idleAfter       time.Duration
	backgroundAfter time.Duration
	lastActivity    time.Time
	backgroundSince time.Time
	trimmed         bool
}

func NewIdleTrimmer(now time.Time, idleAfter, backgroundAfter time.Duration) *IdleTrimmer {
	if idleAfter <= 0 {
		idleAfter = DefaultIdleTrimDelay
	}
	if backgroundAfter <= 0 {
		backgroundAfter = DefaultBackgroundTrimDelay
	}
	return &IdleTrimmer{idleAfter: idleAfter, backgroundAfter: backgroundAfter, lastActivity: now}
}

func (t *IdleTrimmer) Activity(now time.Time) {
	t.mu.Lock()
	t.lastActivity = now
	t.trimmed = false
	t.mu.Unlock()
}

func (t *IdleTrimmer) MarkTrimmed() {
	t.mu.Lock()
	t.trimmed = true
	t.mu.Unlock()
}

func (t *IdleTrimmer) ShouldTrim(now time.Time, foreground, rendering bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if foreground {
		t.backgroundSince = time.Time{}
	} else if t.backgroundSince.IsZero() {
		t.backgroundSince = now
	}
	if rendering || t.trimmed {
		return false
	}
	idle := now.Sub(t.lastActivity) >= t.idleAfter
	background := !foreground && now.Sub(t.backgroundSince) >= t.backgroundAfter
	if !idle && !background {
		return false
	}
	t.trimmed = true
	return true
}
