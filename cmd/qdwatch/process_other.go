//go:build !windows

package main

import "time"

func quotaDockPIDs() ([]uint32, error) {
	return nil, nil
}

func openWatchedProcess(pid uint32, observedAt time.Time) *watchedProcess {
	return &watchedProcess{PID: pid, Started: observedAt}
}
