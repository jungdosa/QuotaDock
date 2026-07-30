//go:build windows

package windows

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	QuotaDockMutexName                    = "QuotaDock_SingleInstance"
	quotaDockWindowTitle                  = "QuotaDock"
	quotaDockWindowProperty               = "QuotaDock_MainWindow"
	errorAlreadyExists      syscall.Errno = 183
	swRestore                             = 9
	swpShowWindow                         = 0x0040
)

var (
	createMutexW        = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW")
	closeHandle         = syscall.NewLazyDLL("kernel32.dll").NewProc("CloseHandle")
	findWindowW         = syscall.NewLazyDLL("user32.dll").NewProc("FindWindowW")
	showWindowInstance  = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")
	setForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("SetForegroundWindow")
	getWindowThreadID   = syscall.NewLazyDLL("user32.dll").NewProc("GetWindowThreadProcessId")
	attachThreadInput   = syscall.NewLazyDLL("user32.dll").NewProc("AttachThreadInput")
	bringWindowToTop    = syscall.NewLazyDLL("user32.dll").NewProc("BringWindowToTop")
	setWindowPosSingle  = syscall.NewLazyDLL("user32.dll").NewProc("SetWindowPos")
	getCurrentThreadID  = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	setWindowProperty   = syscall.NewLazyDLL("user32.dll").NewProc("SetPropW")
	getWindowProperty   = syscall.NewLazyDLL("user32.dll").NewProc("GetPropW")
	enumWindowsInstance = syscall.NewLazyDLL("user32.dll").NewProc("EnumWindows")
)

type SingleInstanceGuard struct {
	handle uintptr
}

func AcquireSingleInstance() (*SingleInstanceGuard, bool, error) {
	return acquireSingleInstance(QuotaDockMutexName)
}

func acquireSingleInstance(name string) (*SingleInstanceGuard, bool, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, fmt.Errorf("encode single-instance mutex name: %w", err)
	}
	handle, _, callErr := createMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		return nil, false, fmt.Errorf("create single-instance mutex: %w", callErr)
	}
	if callErr == errorAlreadyExists {
		closeHandle.Call(handle)
		return nil, true, nil
	}
	return &SingleInstanceGuard{handle: handle}, false, nil
}

func (g *SingleInstanceGuard) Close() {
	if g == nil || g.handle == 0 {
		return
	}
	closeHandle.Call(g.handle)
	g.handle = 0
}

func RegisterMainWindow(hwnd uintptr) bool {
	property, err := syscall.UTF16PtrFromString(quotaDockWindowProperty)
	if err != nil || hwnd == 0 {
		return false
	}
	ok, _, _ := setWindowProperty.Call(hwnd, uintptr(unsafe.Pointer(property)), 1)
	return ok != 0
}

func findExistingWindow() uintptr {
	property, err := syscall.UTF16PtrFromString(quotaDockWindowProperty)
	if err == nil {
		var found uintptr
		callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
			value, _, _ := getWindowProperty.Call(hwnd, uintptr(unsafe.Pointer(property)))
			if value != 0 {
				found = hwnd
				return 0
			}
			return 1
		})
		enumWindowsInstance.Call(callback, 0)
		runtime.KeepAlive(callback)
		if found != 0 {
			return found
		}
	}
	title, err := syscall.UTF16PtrFromString(quotaDockWindowTitle)
	if err != nil {
		return 0
	}
	hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	return hwnd
}

func ActivateExistingWindow() bool {
	hwnd := findExistingWindow()
	if hwnd == 0 {
		return false
	}
	showWindowInstance.Call(hwnd, swRestore)
	foreground, _, _ := getForegroundWindow.Call()
	currentThread, _, _ := getCurrentThreadID.Call()
	foregroundThread := uintptr(0)
	if foreground != 0 {
		foregroundThread, _, _ = getWindowThreadID.Call(foreground, 0)
	}
	attached := foregroundThread != 0 && currentThread != foregroundThread
	if attached {
		attachThreadInput.Call(currentThread, foregroundThread, 1)
		defer attachThreadInput.Call(currentThread, foregroundThread, 0)
	}
	bringWindowToTop.Call(hwnd)
	setWindowPosSingle.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	setForegroundWindow.Call(hwnd)
	active, _, _ := getForegroundWindow.Call()
	return active == hwnd
}
