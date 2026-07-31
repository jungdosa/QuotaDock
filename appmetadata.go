package quotadock

import (
	_ "embed"
	"strings"
)

//go:embed FyneApp.toml
var fyneAppTOML string

// Version returns the application version declared by FyneApp.toml.
func Version() string {
	for _, line := range strings.Split(fyneAppTOML, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != "Version" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
	}
	return ""
}
