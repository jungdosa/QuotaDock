//go:build windows

package windows

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
	"strings"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

type RunKey interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}
type registryRunKey struct{}

func (registryRunKey) Get(name string) (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	return value, err
}
func (registryRunKey) Set(name, value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(name, value)
}
func (registryRunKey) Delete(name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	err = key.DeleteValue(name)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

type AutoStartManager struct {
	AppName, Executable string
	Portable            bool
	key                 RunKey
}

func NewAutoStartManager(appName, executable string, portable bool) *AutoStartManager {
	return &AutoStartManager{AppName: appName, Executable: executable, Portable: portable, key: registryRunKey{}}
}
func NewAutoStartManagerWithKey(appName, executable string, portable bool, key RunKey) *AutoStartManager {
	return &AutoStartManager{AppName: appName, Executable: executable, Portable: portable, key: key}
}
func (m *AutoStartManager) quotedExecutable() string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(m.Executable, "\"", ""))
}
func (m *AutoStartManager) command(startMinimized bool) string {
	if startMinimized {
		return m.quotedExecutable() + " --hidden"
	}
	return m.quotedExecutable()
}

// Enable writes the Run entry, adding --hidden only when the user asked to
// start in the tray. Calling it again with a different choice rewrites the
// entry, which is what keeps the registry in step with the setting.
func (m *AutoStartManager) Enable(startMinimized bool) error {
	if m.Portable {
		return ErrPortableAutoStart
	}
	command := m.command(startMinimized)
	current, err := m.key.Get(m.AppName)
	if err == nil && current == command {
		return nil
	}
	return m.key.Set(m.AppName, command)
}
func (m *AutoStartManager) Disable() error { return m.key.Delete(m.AppName) }

// Enabled asks whether an entry for this executable exists at all, not whether
// it matches one particular flag combination. Comparing the whole command
// string would report autostart as off for anyone whose registry still carries
// the older always-hidden form.
func (m *AutoStartManager) Enabled() (bool, error) {
	if m.Portable {
		return false, ErrPortableAutoStart
	}
	value, err := m.key.Get(m.AppName)
	if err != nil {
		return false, err
	}
	return value == m.quotedExecutable() || strings.HasPrefix(value, m.quotedExecutable()+" "), nil
}

// StartsMinimized reports whether the stored entry carries --hidden, so a
// startup reconciliation can tell a stale entry from a current one.
func (m *AutoStartManager) StartsMinimized() (bool, error) {
	value, err := m.key.Get(m.AppName)
	if err != nil {
		return false, err
	}
	return strings.Contains(value, " --hidden"), nil
}
