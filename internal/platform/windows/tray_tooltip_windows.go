//go:build windows

package windows

import "fyne.io/systray"

func setTrayTooltip(value string) {
	systray.SetTooltip(value)
}
