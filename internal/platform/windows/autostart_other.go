//go:build !windows

package windows

import "errors"

var errUnsupportedPlatform = errors.New("Windows desktop integration is unavailable on this platform")

type RunKey interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

type AutoStartManager struct {
	AppName, Executable string
	Portable            bool
	key                 RunKey
}

func NewAutoStartManager(appName, executable string, portable bool) *AutoStartManager {
	return &AutoStartManager{AppName: appName, Executable: executable, Portable: portable}
}
func NewAutoStartManagerWithKey(appName, executable string, portable bool, key RunKey) *AutoStartManager {
	return &AutoStartManager{AppName: appName, Executable: executable, Portable: portable, key: key}
}
func (*AutoStartManager) Enable(bool) error              { return errUnsupportedPlatform }
func (*AutoStartManager) Disable() error                 { return nil }
func (*AutoStartManager) Enabled() (bool, error)         { return false, nil }
func (*AutoStartManager) StartsMinimized() (bool, error) { return false, nil }
