// Command quotadock is the QuotaDock Windows desktop widget.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	platform "github.com/jungdosa/QuotaDock/internal/platform/windows"
	"github.com/jungdosa/QuotaDock/internal/provider"
	agprovider "github.com/jungdosa/QuotaDock/internal/provider/antigravity"
	claudeprovider "github.com/jungdosa/QuotaDock/internal/provider/claude"
	codexprovider "github.com/jungdosa/QuotaDock/internal/provider/codex"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"github.com/jungdosa/QuotaDock/internal/ui"
	updater "github.com/jungdosa/QuotaDock/internal/update"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"time"
)

var version = "0.7.17"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("QuotaDock stopped", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(192 << 20)
	instance, alreadyRunning, err := platform.AcquireSingleInstance()
	if err != nil {
		return err
	}
	if alreadyRunning {
		platform.ActivateExistingWindow()
		return nil
	}
	defer instance.Close()
	hidden, portable, demo := false, false, false
	for _, arg := range args {
		switch arg {
		case "--hidden":
			hidden = true
		case "--portable":
			portable = true
		case "--demo":
			demo = true
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	auto := platform.NewAutoStartManager("QuotaDock", executable, portable)
	trayPromotionSupported := platform.SupportsTrayIconPromotion()
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate user config: %w", err)
	}
	settingsPath := filepath.Join(configDir, "QuotaDock", "settings.json")
	cfg := loadSettings(settingsPath, !demo)
	if demo {
		cfg = ui.DemoConfig(cfg)
	}
	catalog, err := i18n.Load()
	if err != nil {
		return err
	}
	systemLanguage := i18n.MatchSystemLanguage(lang.SystemLocale().String())
	a := app.NewWithID("com.jungdosa.quotadock")
	app.SetMetadata(withFyneDoMigration(a.Metadata()))
	a.SetIcon(appIcon())
	a.Settings().SetTheme(ui.NewBrandTheme(cfg.Theme))
	desktopDriver, ok := a.Driver().(desktop.Driver)
	if !ok {
		return fmt.Errorf("Fyne desktop driver is unavailable")
	}
	w := desktopDriver.CreateSplashWindow()
	w.SetTitle("QuotaDock")
	configureMainWindow(w)
	processLog := func(message string) { slog.Debug("provider process output", "message", message) }
	coordinator := provider.Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderClaude:      claudeprovider.New(claudeprovider.NewCLIClient(processLog), claudeprovider.MinimumCLIVersion),
		model.ProviderCodex:       codexprovider.New(codexprovider.NewAppServerTransport(processLog), codexprovider.MinimumCLIVersion),
		model.ProviderAntigravity: agprovider.New(agprovider.NewLocalClient()),
	}}
	controller := ui.NewController(coordinator, cfg)
	native := platform.NewWindowController(w)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idleTrimmer := platform.NewIdleTrimmer(time.Now(), platform.DefaultIdleTrimDelay, platform.DefaultBackgroundTrimDelay)
	markActivity := func() { idleTrimmer.Activity(time.Now()) }
	var refreshing atomic.Bool
	var rendering atomic.Bool
	var scheduler provider.Scheduler
	var view *ui.View
	var tray *platform.Tray
	var lifecycle *platform.Lifecycle
	var trayPromotionRetryTimer *time.Timer
	stopTrayPromotionRetries := func() {
		if trayPromotionRetryTimer != nil {
			trayPromotionRetryTimer.Stop()
			trayPromotionRetryTimer = nil
		}
	}
	defer stopTrayPromotionRetries()
	updates := &updateController{
		rootContext: ctx,
		window:      w,
		catalog:     catalog,
		language: func() (i18n.Language, i18n.Language) {
			return i18n.Language(cfg.Language), systemLanguage
		},
		checker: updater.Checker{
			Fetcher:        updater.NewHTTPReleaseFetcher(version, nil),
			CurrentVersion: version,
		},
		flow: updater.Flow{
			Portable: auto.Portable,
			Installer: &updater.Installer{
				Version:  version,
				Launcher: updater.ProcessLauncher{},
			},
			OpenRelease: func(raw string) error { return platform.OpenAllowedURL(a, raw) },
		},
		quit: func() {
			if lifecycle != nil {
				lifecycle.ExitRequested()
			}
		},
	}
	hideWindow := func() {
		w.Hide()
		if trimErr := native.TrimWorkingSet(); trimErr != nil {
			slog.Debug("working set was not trimmed", "error", trimErr)
		} else {
			idleTrimmer.MarkTrimmed()
		}
	}
	savePosition := func() {
		if demo {
			return
		}
		position, posErr := native.Position()
		if posErr == nil {
			cfg.WindowX = position.X
			cfg.WindowY = position.Y
			cfg.WindowPositioned = true
			_ = saveSettings(settingsPath, cfg, true)
		}
	}
	refreshWindowCorners := func() {
		apply := func() {
			if cornerErr := native.ApplyRoundedCorners(int(ui.WindowCornerRadius)); cornerErr != nil {
				slog.Debug("rounded window region was not applied", "error", cornerErr)
			}
		}
		apply()
		time.AfterFunc(100*time.Millisecond, func() { fyne.Do(apply) })
	}
	fitWindowToScreen := func() {
		position, positionErr := native.Position()
		if positionErr != nil {
			return
		}
		fitted := platform.FitToWorkArea(position, platform.MonitorWorkAreas())
		if fitted == position {
			return
		}
		if moveErr := native.MoveTo(fitted.X, fitted.Y); moveErr != nil {
			slog.Debug("window could not be fitted to the work area", "error", moveErr)
		}
	}
	var rememberedWidgetPosition platform.Rect
	var widgetPositionRemembered bool
	var restoreWidgetPosition platform.Rect
	var restoreWidgetPositionOnResize bool
	resizeWindow := func(size fyne.Size) {
		w.Resize(size)
		refreshWindowCorners()
		positionToRestore := restoreWidgetPosition
		shouldRestorePosition := restoreWidgetPositionOnResize
		restoreWidgetPositionOnResize = false
		applyPosition := func() {
			if shouldRestorePosition {
				if moveErr := native.MoveTo(positionToRestore.X, positionToRestore.Y); moveErr != nil {
					slog.Debug("widget position could not be restored", "error", moveErr)
				}
			}
			fitWindowToScreen()
		}
		applyPosition()
		time.AfterFunc(100*time.Millisecond, func() { fyne.Do(applyPosition) })
	}
	applyScreen := func(screen ui.Screen) {
		current := view.Screen()
		if current != ui.SettingsScreen && screen == ui.SettingsScreen {
			if position, positionErr := native.Position(); positionErr == nil {
				rememberedWidgetPosition = position
				widgetPositionRemembered = true
			}
		}
		if current == ui.SettingsScreen && screen != ui.SettingsScreen && widgetPositionRemembered {
			restoreWidgetPosition = rememberedWidgetPosition
			restoreWidgetPositionOnResize = true
			widgetPositionRemembered = false
		}
		view.Show(screen)
		resizeWindow(view.MinimumSize(screen))
	}
	refresh := func() {
		if !refreshing.CompareAndSwap(false, true) {
			return
		}
		markActivity()
		fyne.Do(func() { view.SetRefreshing(true) })
		if demo {
			view.SetState(ui.DemoViewState())
			time.AfterFunc(350*time.Millisecond, func() {
				fyne.Do(func() {
					view.SetRefreshing(false)
					refreshing.Store(false)
				})
			})
			return
		}
		go func() {
			refreshCtx, stop := context.WithTimeout(ctx, 12*time.Second)
			defer stop()
			state := controller.Refresh(refreshCtx)
			rendering.Store(true)
			fyne.DoAndWait(func() {
				view.SetState(state)
				view.SetRefreshing(false)
			})
			rendering.Store(false)
			refreshing.Store(false)
			debug.FreeOSMemory()
		}()
	}
	applyConfig := func(next settings.Config) {
		previous := cfg
		cfg = next.Validated()
		controller.SetConfig(cfg)
		a.Settings().SetTheme(ui.NewBrandTheme(cfg.Theme))
		_ = native.SetAlwaysOnTop(cfg.AlwaysOnTop)
		_ = native.SetTaskbarVisible(cfg.ShowInTaskbar)
		if !demo && trayPromotionSupported && tray != nil {
			if !cfg.PromoteTrayIcon {
				stopTrayPromotionRetries()
			}
			if promotionErr := platform.UpdateTrayIconPromotion(executable, previous.PromoteTrayIcon, cfg.PromoteTrayIcon); promotionErr != nil {
				slog.Warn("tray icon promotion was not updated", "error", promotionErr)
			}
		}
		refreshWindowCorners()
		if !demo && cfg.AutoStart != previous.AutoStart {
			if cfg.AutoStart {
				if autoErr := auto.Enable(); autoErr != nil {
					slog.Warn("automatic start was not enabled", "error", autoErr)
				}
			} else if autoErr := auto.Disable(); autoErr != nil {
				slog.Warn("automatic start was not disabled", "error", autoErr)
			}
		}
		if saveErr := saveSettings(settingsPath, cfg, !demo); saveErr != nil {
			slog.Warn("settings save failed", "error", saveErr)
		}
		if tray != nil {
			tray.Update(i18n.Language(cfg.Language), systemLanguage, cfg.DisplayMode)
		}
		scheduler.Stop()
		if !demo {
			scheduler.Start(ctx, time.Duration(cfg.RefreshSeconds)*time.Second, func(context.Context) { refresh() })
		}
	}
	setDisplayMode := func(mode settings.DisplayMode) {
		cfg.DisplayMode = mode
		applyConfig(cfg)
		applyScreen(ui.ScreenForDisplayMode(mode))
	}
	runConnectionAction := func(id model.ProviderID, reconnect bool) {
		implementation := coordinator.Providers[id]
		if implementation == nil {
			return
		}
		if demo {
			refresh()
			return
		}
		go func() {
			connectCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			defer stop()
			if reconnect {
				_, _ = implementation.Reconnect(connectCtx)
			} else {
				_ = implementation.Inspect(connectCtx)
			}
			refresh()
		}()
	}
	actions := ui.Actions{AppVersion: version, TrayPromotionSupported: trayPromotionSupported, BeginWindowDrag: func() (int, int, error) {
		cursorX, cursorY, cursorErr := native.CursorPos()
		if cursorErr != nil {
			return 0, 0, cursorErr
		}
		position, positionErr := native.Position()
		if positionErr != nil {
			return 0, 0, positionErr
		}
		return cursorX - position.X, cursorY - position.Y, nil
	}, MoveWindow: func(grabOffsetX, grabOffsetY int) error {
		markActivity()
		cursorX, cursorY, cursorErr := native.CursorPos()
		if cursorErr != nil {
			return cursorErr
		}
		return native.MoveTo(cursorX-grabOffsetX, cursorY-grabOffsetY)
	}, EndWindowDrag: func() {
		position, positionErr := native.Position()
		if positionErr != nil {
			slog.Warn("window position could not be read after dragging", "error", positionErr)
			return
		}
		if restoreErr := native.Restore(position); restoreErr != nil {
			slog.Warn("window position could not be corrected after dragging", "error", restoreErr)
		}
	}, ToggleCompact: func() {
		setDisplayMode(settings.NextDisplayMode(cfg.DisplayMode))
	}, SetDisplayMode: setDisplayMode, OpenContextMenu: func(position fyne.Position) {
		if tray == nil {
			return
		}
		position.Y += ui.TitleBarHeight
		widget.NewPopUpMenu(tray.Menu(), w.Canvas()).ShowAtPosition(position)
	}, Refresh: refresh, ResizeWindow: resizeWindow, OpenSettings: func() { applyScreen(ui.SettingsScreen) }, Minimize: func() {
		native.Minimize()
		idleTrimmer.MarkTrimmed()
	}, Close: hideWindow, CloseSettings: func() {
		applyScreen(ui.ScreenForDisplayMode(cfg.DisplayMode))
	}, ConfigChanged: applyConfig, Activity: markActivity,
		Inspect:     func(id model.ProviderID) { runConnectionAction(id, false) },
		Reconnect:   func(id model.ProviderID) { runConnectionAction(id, true) },
		CheckUpdate: func() { updates.Check(true) },
		OpenURL:     func(raw string) error { return platform.OpenAllowedURL(a, raw) },
	}
	actions.DemoMode = demo
	view = ui.NewView(w.Canvas(), catalog, systemLanguage, cfg, actions)
	updates.preparePrompt = func() {
		w.Show()
		applyScreen(ui.SettingsScreen)
	}
	a.Settings().AddListener(func(fyne.Settings) {
		go func() {
			fyne.Do(func() { view.RefreshTheme() })
		}()
	})
	if demo {
		view.SetState(ui.DemoViewState())
	}
	w.SetContent(view.Root)
	applyScreen(ui.ScreenForDisplayMode(cfg.DisplayMode))
	lifecycle = &platform.Lifecycle{Hide: func() { savePosition(); hideWindow() }, Quit: func() {
		savePosition()
		cancel()
		stopTrayPromotionRetries()
		scheduler.Stop()
		_ = controller.Close()
		a.Quit()
	}}
	w.SetCloseIntercept(lifecycle.CloseRequested)
	tray, err = platform.NewTray(a, w, catalog, i18n.Language(cfg.Language), systemLanguage,
		func() {
			markActivity()
			fyne.Do(w.Show)
		},
		func() {
			markActivity()
			fyne.Do(func() {
				w.Show()
				applyScreen(ui.SettingsScreen)
			})
		},
		func(mode settings.DisplayMode) { fyne.Do(func() { setDisplayMode(mode) }) },
		func() { fyne.Do(lifecycle.ExitRequested) },
	)
	if err != nil {
		return err
	}
	if !demo && trayPromotionSupported && cfg.PromoteTrayIcon {
		var applyTrayIconPromotion func(int)
		applyTrayIconPromotion = func(attempt int) {
			if ctx.Err() != nil || !cfg.PromoteTrayIcon {
				return
			}
			result, promotionErr := platform.SetTrayIconPromoted(executable, true)
			if promotionErr != nil {
				slog.Warn("tray icon promotion was not applied after tray registration", "error", promotionErr)
				return
			}
			delay, retry := platform.NextTrayIconPromotionRetry(result, attempt)
			if !retry {
				return
			}
			trayPromotionRetryTimer = time.AfterFunc(delay, func() {
				if ctx.Err() != nil {
					return
				}
				fyne.Do(func() {
					trayPromotionRetryTimer = nil
					applyTrayIconPromotion(attempt + 1)
				})
			})
		}
		applyTrayIconPromotion(1)
	}
	// Fyne installs its tray toggle intercept in SetSystemTrayWindow. Restore
	// QuotaDock's lifecycle intercept so close still saves position and trims.
	w.SetCloseIntercept(lifecycle.CloseRequested)
	tray.Update(i18n.Language(cfg.Language), systemLanguage, cfg.DisplayMode)
	w.Show()
	if err := native.Bind(); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		wasForeground := native.IsForeground()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				foreground := native.IsForeground()
				if foreground && !wasForeground {
					idleTrimmer.Activity(now)
				}
				wasForeground = foreground
				if !idleTrimmer.ShouldTrim(now, foreground, refreshing.Load() || rendering.Load()) {
					continue
				}
				debug.FreeOSMemory()
				if trimErr := native.TrimWorkingSet(); trimErr != nil {
					slog.Debug("idle working set was not trimmed", "error", trimErr)
				}
			}
		}
	}()
	_ = native.SetAlwaysOnTop(cfg.AlwaysOnTop)
	_ = native.SetTaskbarVisible(cfg.ShowInTaskbar)
	if cfg.WindowPositioned {
		size := view.MinimumSize(view.Screen())
		_ = native.Restore(platform.Rect{X: cfg.WindowX, Y: cfg.WindowY, Width: int(size.Width), Height: int(size.Height)})
	}
	refreshWindowCorners()
	if hidden {
		hideWindow()
	}
	if !demo {
		scheduler.Start(ctx, time.Duration(cfg.RefreshSeconds)*time.Second, func(context.Context) { refresh() })
		refresh()
		go func() { fyne.Do(func() { updates.Check(false) }) }()
	}
	a.Run()
	return nil
}
func configureMainWindow(window fyne.Window) {
	window.SetFixedSize(true)
	window.SetPadded(false)
}

func withFyneDoMigration(metadata fyne.AppMetadata) fyne.AppMetadata {
	migrations := make(map[string]bool, len(metadata.Migrations)+1)
	for name, enabled := range metadata.Migrations {
		migrations[name] = enabled
	}
	migrations["fyneDo"] = true
	metadata.Migrations = migrations
	return metadata
}

func loadSettings(path string, persist bool) settings.Config {
	cfg, err := settings.Load(path)
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return cfg
		}
	}
	cfg = settings.Default()
	if saveErr := saveSettings(path, cfg, persist); saveErr != nil {
		slog.Warn("default settings could not be saved", "error", saveErr)
	}
	return cfg
}
func saveSettings(path string, cfg settings.Config, persist bool) error {
	if !persist {
		return nil
	}
	return settings.Save(path, cfg)
}

//go:embed icon.svg
var iconSVG []byte

func appIcon() fyne.Resource {
	return fyne.NewStaticResource("quotadock.svg", iconSVG)
}
