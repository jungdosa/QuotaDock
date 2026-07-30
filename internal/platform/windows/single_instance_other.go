//go:build !windows

package windows

type SingleInstanceGuard struct{}

func AcquireSingleInstance() (*SingleInstanceGuard, bool, error) {
	return &SingleInstanceGuard{}, false, nil
}

func (*SingleInstanceGuard) Close() {}

func RegisterMainWindow(uintptr) bool { return false }

func ActivateExistingWindow() bool { return false }
