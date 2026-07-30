package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase4ETrayPromotionToggleVisibleOnlyWhenSupported(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, supported := range []bool{false, true} {
		window := test.NewWindow(nil)
		view := NewView(window.Canvas(), catalog, i18n.English, settings.Default(), Actions{TrayPromotionSupported: supported})
		window.SetContent(view.Root)

		labelFound := false
		var promotionToggle *Toggle
		walkCanvasObject(view.Settings, func(object fyne.CanvasObject) {
			switch value := object.(type) {
			case *canvas.Text:
				labelFound = labelFound || value.Text == view.text(i18n.KeyPromoteTray)
			case *Toggle:
				if value.Tooltip == view.text(i18n.KeyTooltipPromoteTray) {
					promotionToggle = value
				}
			}
		})
		if supported && (!labelFound || promotionToggle == nil) {
			t.Fatal("supported settings screen is missing the tray promotion toggle or tooltip")
		}
		if !supported && (labelFound || promotionToggle != nil) {
			t.Fatal("unsupported settings screen shows the tray promotion toggle")
		}
		window.Close()
	}
}
