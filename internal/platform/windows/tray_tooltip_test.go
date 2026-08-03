package windows

import "testing"

func TestTrayTooltipBeforeRegistrationIsIgnored(t *testing.T) {
	tray := &Tray{}
	tray.SetTooltip("QuotaDock")
	tray.Ready()

	var nilTray *Tray
	nilTray.SetTooltip("QuotaDock")
	nilTray.Ready()
}
