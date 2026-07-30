package windows

import (
	"errors"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

type Tray struct {
	host                                        desktop.App
	menu                                        *fyne.Menu
	show, settings, normal, compact, nano, quit *fyne.MenuItem
	language, system                            i18n.Language
	catalog                                     *i18n.Catalog
}

func NewTray(app fyne.App, window fyne.Window, catalog *i18n.Catalog, language, system i18n.Language, onShow, onSettings func(), onMode func(settings.DisplayMode), onQuit func()) (*Tray, error) {
	host, ok := app.(desktop.App)
	if !ok {
		return nil, errors.New("desktop tray is unavailable")
	}
	t := &Tray{host: host, catalog: catalog, language: language, system: system}
	t.show = fyne.NewMenuItem("", onShow)
	t.settings = fyne.NewMenuItem("", onSettings)
	t.normal = fyne.NewMenuItem("", func() { onMode(settings.ModeNormal) })
	t.compact = fyne.NewMenuItem("", func() { onMode(settings.ModeCompact) })
	t.nano = fyne.NewMenuItem("", func() { onMode(settings.ModeNano) })
	t.quit = fyne.NewMenuItem("", onQuit)
	t.quit.IsQuit = true
	t.menu = fyne.NewMenu("QuotaDock", t.show, fyne.NewMenuItemSeparator(), t.settings, t.normal, t.compact, t.nano, fyne.NewMenuItemSeparator(), t.quit)
	t.apply(settings.ModeNormal, false)
	host.SetSystemTrayMenu(t.menu)
	host.SetSystemTrayWindow(window)
	return t, nil
}
func (t *Tray) apply(mode settings.DisplayMode, refresh bool) {
	t.show.Label = t.catalog.Text(t.language, t.system, i18n.KeyTrayShow)
	t.settings.Label = t.catalog.Text(t.language, t.system, i18n.KeyTraySettings)
	t.normal.Label = t.catalog.Text(t.language, t.system, i18n.KeyTrayNormal)
	t.compact.Label = t.catalog.Text(t.language, t.system, i18n.KeyTrayCompact)
	t.nano.Label = t.catalog.Text(t.language, t.system, i18n.KeyTrayNano)
	t.quit.Label = t.catalog.Text(t.language, t.system, i18n.KeyTrayQuit)
	t.normal.Checked = mode == settings.ModeNormal
	t.compact.Checked = mode == settings.ModeCompact
	t.nano.Checked = mode == settings.ModeNano
	if refresh {
		t.host.SetSystemTrayMenu(t.menu)
	}
}
func (t *Tray) Update(language, system i18n.Language, mode settings.DisplayMode) {
	t.language = language
	t.system = system
	t.apply(mode, true)
}
func (t *Tray) Menu() *fyne.Menu { return t.menu }
