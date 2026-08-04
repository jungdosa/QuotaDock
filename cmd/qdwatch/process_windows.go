//go:build windows

package main

import (
	"errors"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeProcessHandle struct {
	handle windows.Handle
}

func quotaDockPIDs() ([]uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return nil, nil
		}
		return nil, err
	}
	var pids []uint32
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "quotadock.exe") {
			pids = append(pids, entry.ProcessID)
		}
		err = windows.Process32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return pids, nil
}

func openWatchedProcess(pid uint32, observedAt time.Time) *watchedProcess {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return &watchedProcess{PID: pid, Started: observedAt}
	}
	started := observedAt
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err == nil {
		started = time.Unix(0, created.Nanoseconds())
	}
	return &watchedProcess{
		PID:     pid,
		Started: started,
		Handle:  &nativeProcessHandle{handle: handle},
	}
}

func (h *nativeProcessHandle) ExitCode() (uint32, bool) {
	if h == nil || h.handle == 0 {
		return 0, false
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h.handle, &code); err != nil {
		return 0, false
	}
	return code, true
}

func (h *nativeProcessHandle) Close() error {
	if h == nil || h.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(h.handle)
	h.handle = 0
	return err
}
