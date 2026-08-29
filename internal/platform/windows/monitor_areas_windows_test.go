//go:build windows

package windows

import (
	"sync"
	"testing"
)

// syscall.NewCallback registrations are permanent and capped per process, so
// creating one per MonitorWorkAreas call killed the app silently after ~2000
// invocations (one per minute in production - about 33 awake hours). Calling
// past that cap proves the callback is registered exactly once: the unfixed
// code aborts this whole test process with "too many callback functions".
func TestMonitorWorkAreasSurvivesThousandsOfCalls(t *testing.T) {
	first := MonitorWorkAreas()
	for i := 0; i < 2500; i++ {
		if got := MonitorWorkAreas(); len(got) != len(first) {
			t.Fatalf("call %d returned %d areas, first returned %d", i, len(got), len(first))
		}
	}
}

func TestMonitorWorkAreasReturnsCallerOwnedCopies(t *testing.T) {
	first := MonitorWorkAreas()
	if len(first) == 0 {
		t.Skip("no monitors reported in this environment")
	}
	saved := append([]Rect{}, first...)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				MonitorWorkAreas()
			}
		}()
	}
	wg.Wait()
	for i := range saved {
		if first[i] != saved[i] {
			t.Fatalf("earlier result mutated by later calls: %v != %v", first[i], saved[i])
		}
	}
}
