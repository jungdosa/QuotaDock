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
func (m *AutoStartManager) command() string {
	return fmt.Sprintf("\"%s\" --hidden", strings.ReplaceAll(m.Executable, "\"", ""))
}
func (m *AutoStartManager) Enable() error {
	if m.Portable {
		return ErrPortableAutoStart
	}
	current, err := m.key.Get(m.AppName)
	if err == nil && current == m.command() {
		return nil
	}
	return m.key.Set(m.AppName, m.command())
}
func (m *AutoStartManager) Disable() error { return m.key.Delete(m.AppName) }
func (m *AutoStartManager) Enabled() (bool, error) {
	if m.Portable {
		return false, ErrPortableAutoStart
	}
	value, err := m.key.Get(m.AppName)
	if err != nil {
		return false, err
	}
	return value == m.command(), nil
}
