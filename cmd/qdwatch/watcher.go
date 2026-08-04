package main

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	appmetadata "github.com/jungdosa/QuotaDock"
	"github.com/jungdosa/QuotaDock/internal/diagnostics"
	platform "github.com/jungdosa/QuotaDock/internal/platform/windows"
)

const (
	pollInterval   = time.Second
	eventWindow    = 2 * time.Minute
	eventTimeLayout = "2006-01-02T15:04:05.000Z07:00"
)

type processHandle interface {
	ExitCode() (uint32, bool)
	Close() error
}

type watchedProcess struct {
	PID     uint32
	Started time.Time
	Handle  processHandle
}

type pendingEventQuery struct {
	gone time.Time
	due  time.Time
}

func run(ctx context.Context) error {
	directory, err := diagnostics.LocalDataDirectory()
	if err != nil {
		return err
	}
	runID, err := diagnostics.NewRunID()
	if err != nil {
		return err
	}
	fileLogger, err := diagnostics.NewWatchLogger(directory, appmetadata.Version(), runID)
	if err != nil {
		return err
	}
	defer fileLogger.Close()
	logger := fileLogger.Logger()

	areas := platform.MonitorWorkAreas()
	initialPIDs, initialProcessErr := quotaDockPIDs()
	slices.Sort(initialPIDs)
	var tracked *watchedProcess
	if initialProcessErr == nil && len(initialPIDs) > 0 {
		tracked = openWatchedProcess(initialPIDs[0], time.Now())
	}
	startAttrs := []any{
		"interval_ms", pollInterval.Milliseconds(),
		"monitors", len(areas),
		"areas", areaValues(areas),
	}
	if tracked != nil {
		startAttrs = append(startAttrs, "pid", tracked.PID)
	}
	logger.Info("watch.start", startAttrs...)
	if initialProcessErr != nil {
		logger.Info("watch.sysevent", "ok", false, "err", "process_query_failed")
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var pending []pendingEventQuery
	var eventQueries sync.WaitGroup
	processQueryFailed := initialProcessErr != nil

	poll := func(now time.Time) {
		currentAreas := platform.MonitorWorkAreas()
		if !platform.WorkAreasEqual(areas, currentAreas) {
			logger.Info("watch.display", "before", len(areas), "after", len(currentAreas), "areas", areaValues(currentAreas))
			areas = append([]platform.Rect(nil), currentAreas...)
		}

		pids, processErr := quotaDockPIDs()
		if processErr != nil {
			if !processQueryFailed {
				logger.Info("watch.sysevent", "ok", false, "err", "process_query_failed")
			}
			processQueryFailed = true
		} else {
			processQueryFailed = false
			slices.Sort(pids)
			if tracked == nil && len(pids) > 0 {
				tracked = openWatchedProcess(pids[0], now)
			}
			if tracked != nil && !slices.Contains(pids, tracked.PID) {
				attrs := []any{
					"last_pid", tracked.PID,
					"uptime_s", uptimeSeconds(tracked.Started, now),
				}
				if tracked.Handle != nil {
					if exitCode, ok := tracked.Handle.ExitCode(); ok {
						attrs = append(attrs, "exit_code", exitCode)
					}
					_ = tracked.Handle.Close()
				}
				logger.Info("watch.gone", attrs...)
				pending = append(pending, pendingEventQuery{gone: now, due: now.Add(eventWindow)})
				tracked = nil
			}
		}

		remaining := pending[:0]
		for _, query := range pending {
			if now.Before(query.due) {
				remaining = append(remaining, query)
				continue
			}
			gone := query.gone
			eventQueries.Add(1)
			diagnostics.Go("watch_event_query", func() {
				defer eventQueries.Done()
				recordSystemEvents(ctx, logger, gone)
			})
		}
		pending = remaining
	}

	poll(time.Now())
	for {
		select {
		case <-ctx.Done():
			if tracked != nil && tracked.Handle != nil {
				_ = tracked.Handle.Close()
			}
			eventQueries.Wait()
			return nil
		case now := <-ticker.C:
			poll(now)
		}
	}
}

func uptimeSeconds(started, ended time.Time) int64 {
	if started.IsZero() || ended.Before(started) {
		return 0
	}
	return int64(ended.Sub(started) / time.Second)
}

func areaValues(areas []platform.Rect) [][]int {
	values := make([][]int, 0, len(areas))
	for _, area := range areas {
		values = append(values, []int{area.X, area.Y, area.Width, area.Height})
	}
	return values
}

func recordSystemEvents(ctx context.Context, logger *slog.Logger, gone time.Time) {
	from := gone.Add(-eventWindow)
	to := gone.Add(eventWindow)
	events, err := querySystemEvents(ctx, from, to)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Info("watch.sysevent", "ok", false, "err", "event_query_failed")
		return
	}
	// Record the query outcome even when it matched nothing. Without this line
	// "collected nothing" and "never ran" are indistinguishable in the log,
	// which is exactly the ambiguity a diagnostic tool must not have.
	logger.Info(
		"watch.sysevent",
		"ok", true,
		"count", len(events),
		"from", from.Format(eventTimeLayout),
		"to", to.Format(eventTimeLayout),
	)
	for _, event := range events {
		logger.Info(
			"watch.sysevent",
			"ok", true,
			"log", event.Log,
			"provider", event.Provider,
			"event_id", event.EventID,
			"event_time", event.Time.Format(eventTimeLayout),
			"kind", event.Kind,
		)
	}
}
