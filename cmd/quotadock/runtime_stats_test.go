package main

import (
	"testing"
	"time"
)

func TestCollectRuntimeStatsFields(t *testing.T) {
	started := time.Unix(1000, 0)
	now := started.Add(25 * time.Minute)
	stats := collectRuntimeStats(started, now, func() (float64, bool) { return 12.345, true })
	if stats.goroutines < 1 || stats.heapMB <= 0 || stats.uptime != 25*time.Minute || stats.cpuSeconds != 12.345 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	missing := collectRuntimeStats(started, now, func() (float64, bool) { return 0, false })
	if missing.cpuSeconds != -1 {
		t.Fatalf("unavailable CPU time must read -1, got %v", missing.cpuSeconds)
	}
	if roundTenth(12.345) != 12.3 || roundTenth(0.05) != 0.1 {
		t.Fatal("roundTenth")
	}
}
