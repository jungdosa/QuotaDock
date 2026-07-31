package windows

import (
	"strings"
	"time"
)

const windows11MinimumBuild = 22000

type TrayPromotionResult int

const (
	TrayPromotionApplied TrayPromotionResult = iota
	TrayPromotionEntryNotFound
	TrayPromotionUnsupported
)

const maxTrayPromotionAttempts = 5

var trayPromotionRetryDelays = [...]time.Duration{
	time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
}

type trayIconRegistryEntry struct {
	subkey         string
	executablePath string
}

func isWindows11Build(build int) bool {
	return build >= windows11MinimumBuild
}

func normalizeExecutablePath(value string) string {
	value = strings.ReplaceAll(value, "/", "\\")
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "\\\\?\\unc\\"):
		value = "\\\\" + value[len("\\\\?\\UNC\\"):]
	case strings.HasPrefix(lower, "\\\\?\\"):
		value = value[len("\\\\?\\"):]
	}
	return strings.ToLower(strings.TrimRight(value, "\\"))
}

func executableBaseName(value string) string {
	value = normalizeExecutablePath(value)
	if index := strings.LastIndex(value, "\\"); index >= 0 {
		return value[index+1:]
	}
	return value
}

func selectTrayIconSubkey(executable string, entries []trayIconRegistryEntry) (string, bool) {
	target := normalizeExecutablePath(executable)
	if target == "" {
		return "", false
	}

	targetName := executableBaseName(target)
	filenameMatches := make([]string, 0, 1)
	for _, entry := range entries {
		candidate := normalizeExecutablePath(entry.executablePath)
		if candidate == target {
			return entry.subkey, true
		}
		if candidate != "" && executableBaseName(candidate) == targetName {
			filenameMatches = append(filenameMatches, entry.subkey)
		}
	}
	if len(filenameMatches) == 1 {
		return filenameMatches[0], true
	}
	return "", false
}

func shouldDemoteTrayIcon(previous, current bool) bool {
	return previous && !current
}

// NextTrayIconPromotionRetry returns the next delay after completedAttempts.
// The first registry attempt is immediate, so four delayed retries cap the
// operation at five total attempts.
func NextTrayIconPromotionRetry(result TrayPromotionResult, completedAttempts int) (time.Duration, bool) {
	if result != TrayPromotionEntryNotFound || completedAttempts < 1 || completedAttempts >= maxTrayPromotionAttempts {
		return 0, false
	}
	return trayPromotionRetryDelays[completedAttempts-1], true
}

// UpdateTrayIconPromotion applies only a setting transition. In particular,
// false -> false never reaches the registry and cannot undo a manual Windows pin.
func UpdateTrayIconPromotion(executable string, previous, current bool) error {
	switch {
	case shouldDemoteTrayIcon(previous, current):
		_, err := SetTrayIconPromoted(executable, false)
		return err
	case !previous && current:
		_, err := SetTrayIconPromoted(executable, true)
		return err
	default:
		return nil
	}
}
