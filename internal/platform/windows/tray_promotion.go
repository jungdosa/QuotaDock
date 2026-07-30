package windows

import "strings"

const windows11MinimumBuild = 22000

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

// UpdateTrayIconPromotion applies only a setting transition. In particular,
// false -> false never reaches the registry and cannot undo a manual Windows pin.
func UpdateTrayIconPromotion(executable string, previous, current bool) error {
	switch {
	case shouldDemoteTrayIcon(previous, current):
		return SetTrayIconPromoted(executable, false)
	case !previous && current:
		return SetTrayIconPromoted(executable, true)
	default:
		return nil
	}
}
