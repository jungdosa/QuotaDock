//go:build windows

package windows

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	currentVersionKeyPath     = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"
	notifyIconSettingsKeyPath = "Control Panel\\NotifyIconSettings"
)

func currentWindowsBuild() (int, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer key.Close()

	value, _, err := key.GetStringValue("CurrentBuild")
	if err != nil {
		return 0, err
	}
	build, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse CurrentBuild %q: %w", value, err)
	}
	return build, nil
}

func SupportsTrayIconPromotion() bool {
	build, err := currentWindowsBuild()
	if err != nil {
		slog.Debug("Windows build could not be read for tray icon promotion", "error", err)
		return false
	}
	return isWindows11Build(build)
}

func SetTrayIconPromoted(executable string, promoted bool) (TrayPromotionResult, error) {
	build, err := currentWindowsBuild()
	if err != nil {
		return TrayPromotionUnsupported, fmt.Errorf("read Windows build for tray icon promotion: %w", err)
	}
	if !isWindows11Build(build) {
		return TrayPromotionUnsupported, nil
	}

	root, err := registry.OpenKey(registry.CURRENT_USER, notifyIconSettingsKeyPath, registry.ENUMERATE_SUB_KEYS)
	if err == registry.ErrNotExist {
		return TrayPromotionEntryNotFound, nil
	}
	if err != nil {
		return TrayPromotionUnsupported, fmt.Errorf("open tray icon settings: %w", err)
	}
	defer root.Close()

	subkeys, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return TrayPromotionUnsupported, fmt.Errorf("enumerate tray icon settings: %w", err)
	}
	entries := make([]trayIconRegistryEntry, 0, len(subkeys))
	for _, subkey := range subkeys {
		entryKey, openErr := registry.OpenKey(root, subkey, registry.QUERY_VALUE)
		if openErr != nil {
			if openErr != registry.ErrNotExist {
				slog.Debug("tray icon registry entry could not be opened", "subkey", subkey, "error", openErr)
			}
			continue
		}
		executablePath, _, valueErr := entryKey.GetStringValue("ExecutablePath")
		entryKey.Close()
		if valueErr != nil {
			if valueErr != registry.ErrNotExist {
				slog.Debug("tray icon executable path could not be read", "subkey", subkey, "error", valueErr)
			}
			continue
		}
		entries = append(entries, trayIconRegistryEntry{subkey: subkey, executablePath: executablePath})
	}

	subkey, found := selectTrayIconSubkey(executable, entries)
	if !found {
		return TrayPromotionEntryNotFound, nil
	}
	entryKey, err := registry.OpenKey(root, subkey, registry.SET_VALUE)
	if err == registry.ErrNotExist {
		return TrayPromotionEntryNotFound, nil
	}
	if err != nil {
		return TrayPromotionUnsupported, fmt.Errorf("open matched tray icon settings: %w", err)
	}
	defer entryKey.Close()

	value := uint32(0)
	if promoted {
		value = 1
	}
	if err := entryKey.SetDWordValue("IsPromoted", value); err != nil {
		return TrayPromotionUnsupported, fmt.Errorf("write tray icon promotion: %w", err)
	}
	return TrayPromotionApplied, nil
}
