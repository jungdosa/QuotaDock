package main

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/jungdosa/QuotaDock/internal/diagnostics"
)

// runtimeStatsInterval spaces the long-run health record. A widget that stays
// up for days once climbed to a quarter of a core while a fresh process used
// almost nothing; one line every ten minutes is enough to see what grows.
const runtimeStatsInterval = 10 * time.Minute

type runtimeStats struct {
	goroutines int
	cpuSeconds float64
	heapMB     float64
	uptime     time.Duration
}

func collectRuntimeStats(started, now time.Time, cpu func() (float64, bool)) runtimeStats {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	stats := runtimeStats{
		goroutines: runtime.NumGoroutine(),
		heapMB:     float64(memory.HeapAlloc) / (1 << 20),
		uptime:     now.Sub(started),
		cpuSeconds: -1,
	}
	if seconds, ok := cpu(); ok {
		stats.cpuSeconds = seconds
	}
	return stats
}

func (s runtimeStats) log() {
	slog.Info("runtime.stats",
		"goroutines", s.goroutines,
		"cpu_s", roundTenth(s.cpuSeconds),
		"heap_mb", roundTenth(s.heapMB),
		"uptime_s", int64(s.uptime/time.Second))
}

func roundTenth(value float64) float64 {
	return float64(int64(value*10+0.5)) / 10
}

func startRuntimeStats(ctx context.Context, started time.Time) {
	diagnostics.Go("runtime_stats", func() {
		ticker := time.NewTicker(runtimeStatsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				collectRuntimeStats(started, now, processCPUSeconds).log()
			}
		}
	})
}
