//go:build windows

package main

import "golang.org/x/sys/windows"

// processCPUSeconds reads this process's user and kernel time. It is a plain
// call; no callback is registered.
func processCPUSeconds() (float64, bool) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user); err != nil {
		return 0, false
	}
	// FILETIME counts 100-nanosecond intervals.
	ticks := uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)
	ticks += uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime)
	return float64(ticks) / 1e7, true
}
