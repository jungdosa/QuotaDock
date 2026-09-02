// Command quotadock is the QuotaDock Windows desktop widget.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	appmetadata "github.com/jungdosa/QuotaDock"
	"github.com/jungdosa/QuotaDock/internal/diagnostics"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	platform "github.com/jungdosa/QuotaDock/internal/platform/windows"
	"github.com/jungdosa/QuotaDock/internal/provider"
	agprovider "github.com/jungdosa/QuotaDock/internal/provider/antigravity"
	claudeprovider "github.com/jungdosa/QuotaDock/internal/provider/claude"
	codexprovider "github.com/jungdosa/QuotaDock/internal/provider/codex"
	grokprovider "github.com/jungdosa/QuotaDock/internal/provider/grok"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"github.com/jungdosa/QuotaDock/internal/ui"
	updater "github.com/jungdosa/QuotaDock/internal/update"
	"github.com/jungdosa/QuotaDock/internal/webview"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"time"
)

var version string

func init() {
	if version == "" {
		version = appmetadata.Version()
	}
}

func main() {
	diagnosticRuntime, err := diagnostics.NewDefault(version)
	if err != nil {
		os.Exit(1)
	}
	restoreDiagnostics := diagnosticRuntime.Install()
	defer diagnosticRuntime.Close()
	defer restoreDiagnostics()
	defer diagnostics.Recover("main")
	if err := run(os.Args[1:], diagnosticRuntime); err != nil {
		_ = diagnosticRuntime.RecordFailure("main", err)
		os.Exit(1)
	}
}
func run(args []string, diagnosticRuntime *diagnostics.Runtime) error {
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
	if err := diagnosticRuntime.BeginSession(); err != nil {
		return err
	}
	if err := diagnosticRuntime.CaptureRuntimeFatal(); err != nil {
		return err
	}
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
	// Existing installs carry a Run entry that always said --hidden, written
	// before the choice existed. Rewrite it once so the registry matches the
	// setting instead of waiting for the user to toggle something.
	reconcileAutoStart := func(cfg settings.Config) {
		if portable || !cfg.AutoStart {
			return
		}
		if enabled, err := auto.Enabled(); err != nil || !enabled {
			return
		}
		if minimized, err := auto.StartsMinimized(); err == nil && minimized == cfg.StartMinimized {
			return
		}
		if err := auto.Enable(cfg.StartMinimized); err != nil {
			slog.Warn("automatic start entry was not reconciled", "error", err)
		}
	}
	trayPromotionSupported := platform.SupportsTrayIconPromotion()
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate user config: %w", err)
	}
	settingsPath := filepath.Join(configDir, "QuotaDock", "settings.json")
	cfg := loadSettings(settingsPath, !demo)
	if demo {
		cfg = ui.DemoConfig(cfg)
	} else {
		reconcileAutoStart(cfg)
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
	// Provider stdout/stderr can contain response bodies, account data, and
	// absolute CLI paths. Structured provider events below are the safe record.
	processLog := func(string) {}
	claudeProvider := claudeprovider.New(claudeprovider.NewCLIClient(processLog), claudeprovider.MinimumCLIVersion)
	// The embedded-browser sign-in stores its session under the local data
	// folder, beside the diagnostics logs. It is a fallback: the Claude
	// provider only consults it when the CLI path is unavailable.
	var claudeWebAuth *claudeprovider.WebAuthFetcher
	// The third Claude account onward each get a browser profile folder of
	// their own beside the first: a profile takes one writer, and a browser
	// session is one account, so two accounts cannot share a folder.
	accountWebAuth := make(map[model.ProviderID]*claudeprovider.WebAuthFetcher)
	if dataDir, dirErr := diagnostics.LocalDataDirectory(); dirErr == nil {
		claudeWebAuth = claudeprovider.NewWebAuthFetcher(filepath.Join(dataDir, webview.DefaultUserDataDir))
		claudeProvider.SetWebAuth(claudeWebAuth)
		for _, id := range model.ClaudeAccountIDs() {
			if index := model.ClaudeAccountIndex(id); index >= 3 {
				fetcher := claudeprovider.NewWebAuthFetcher(filepath.Join(dataDir, fmt.Sprintf("%s-%d", webview.DefaultUserDataDir, index)))
				accountWebAuth[id] = fetcher
				claudeProvider.SetAccountWebAuth(id, fetcher)
			}
		}
	}
	claudeProvider.SetSourceMode(cfg.ConnectionMethods[string(model.ProviderClaude)])
	coordinator := provider.Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderClaude:      claudeProvider,
		model.ProviderCodex:       codexprovider.New(codexprovider.NewAppServerTransport(processLog), codexprovider.MinimumCLIVersion),
		model.ProviderAntigravity: agprovider.New(agprovider.NewLocalClient()),
		model.ProviderGrok:        grokprovider.New(nil, ""),
	}}
	controller := ui.NewController(coordinator, cfg)
	native := platform.NewWindowController(w)
	workAreas := platform.MonitorWorkAreas()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idleTrimmer := platform.NewIdleTrimmer(time.Now(), platform.DefaultIdleTrimDelay, platform.DefaultBackgroundTrimDelay)
	markActivity := func() { idleTrimmer.Activity(time.Now()) }
	shell := &windowShell{
		window:       w,
		native:       native,
		idleTrimmer:  idleTrimmer,
		cfg:          &cfg,
		settingsPath: settingsPath,
		demo:         demo,
		workAreas:    workAreas,
	}
	var refreshing atomic.Bool
	var rendering atomic.Bool
	var alwaysOnTop atomic.Bool
	var scheduler provider.Scheduler
	var view *ui.View
	var tray *platform.Tray
	setTrayTooltip := func(state ui.ViewState) {
		if tray == nil {
			return
		}
		tray.SetTooltip(ui.BuildTrayTooltip(state, cfg, systemLanguage, time.Now()))
	}
	var lifecycle *platform.Lifecycle
	var exitReason atomic.Value
	exitReason.Store("exit_requested")
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
				exitReason.Store("update_install")
				lifecycle.ExitRequested()
			}
		},
	}
	var rememberedWidgetPosition platform.Rect
	var widgetPositionRemembered bool
	var restoreWidgetPosition platform.Rect
	var restoreWidgetPositionOnResize bool
	resizeWindow := func(size fyne.Size) {
		// A fixed-size window has its OS size limits pinned to its current
		// size, and Fyne's Resize asks GLFW for the new size without lifting
		// them first — so a request to shrink is clamped to the old frame until
		// the next layout pass re-pins the limits. For that interval the window
		// kept its old outline with nothing drawn inside it. Unpinning around
		// the resize lets the new size take at once; the calls queue on the
		// main thread in order, so the window is never resizable in between.
		w.SetFixedSize(false)
		w.Resize(size)
		w.SetFixedSize(true)
		shell.refreshCorners()
		positionToRestore := restoreWidgetPosition
		shouldRestorePosition := restoreWidgetPositionOnResize
		restoreWidgetPositionOnResize = false
		applyPosition := func() {
			if shouldRestorePosition {
				if moveErr := native.MoveTo(positionToRestore.X, positionToRestore.Y); moveErr != nil {
					slog.Debug("widget position could not be restored", "error", moveErr)
				}
			}
			shell.fitToScreen()
		}
		applyPosition()
		// The window is fixed-size, so Fyne pins the OS size limits to the
		// content's minimum — and it moves those limits on its next layout pass,
		// not at the moment the content changes. A resize that shrinks the window
		// right after a rebuild is clamped by the limits the old content left
		// behind: standing nano back up flat kept the upright height, and standing
		// it up kept most of the flat width. Asking again once the pass has run
		// lets the smaller size take.
		diagnostics.AfterFunc(100*time.Millisecond, "window_position", func() {
			fyne.Do(func() {
				w.Resize(size)
				applyPosition()
				// What the window actually settled at, against what was asked
				// for and what the content needs. A gap between the three is the
				// OS frame disagreeing with the layout, which no unit test can
				// see: the test driver has no frame.
				canvas := w.Canvas().Size()
				minimum := w.Content().MinSize()
				slog.Info("window.resize",
					"requested_w", size.Width, "requested_h", size.Height,
					"canvas_w", canvas.Width, "canvas_h", canvas.Height,
					"min_w", minimum.Width, "min_h", minimum.Height,
				)
			})
		})
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
			state := ui.DemoViewState()
			view.SetState(state)
			setTrayTooltip(state)
			diagnostics.AfterFunc(350*time.Millisecond, "demo_refresh", func() {
				fyne.Do(func() {
					view.SetRefreshing(false)
					refreshing.Store(false)
				})
			})
			return
		}
		diagnostics.Go("provider_refresh", func() {
			refreshCtx, stop := context.WithTimeout(ctx, 12*time.Second)
			defer stop()
			state := controller.Refresh(refreshCtx)
			rendering.Store(true)
			fyne.DoAndWait(func() {
				view.SetState(state)
				setTrayTooltip(state)
				view.SetRefreshing(false)
			})
			rendering.Store(false)
			refreshing.Store(false)
			debug.FreeOSMemory()
		})
	}
	scheduledRefresh := func(context.Context) {
		shell.checkDisplayChange()
		refresh()
	}
	applyConfig := func(next settings.Config) {
		previous := cfg
		cfg = next.Validated()
		claudeSourceChanged := cfg.ConnectionMethods[string(model.ProviderClaude)] != previous.ConnectionMethods[string(model.ProviderClaude)]
		claudeProvider.SetSourceMode(cfg.ConnectionMethods[string(model.ProviderClaude)])
		controller.SetConfig(cfg)
		a.Settings().SetTheme(ui.NewBrandTheme(cfg.Theme))
		_ = native.SetAlwaysOnTop(cfg.AlwaysOnTop)
		alwaysOnTop.Store(cfg.AlwaysOnTop)
		_ = native.SetTaskbarVisible(cfg.ShowInTaskbar)
		if !demo && trayPromotionSupported && tray != nil {
			if !cfg.PromoteTrayIcon {
				stopTrayPromotionRetries()
			}
			if promotionErr := platform.UpdateTrayIconPromotion(executable, previous.PromoteTrayIcon, cfg.PromoteTrayIcon); promotionErr != nil {
				slog.Warn("tray icon promotion was not updated", "error", promotionErr)
			}
		}
		shell.refreshCorners()
		// The minimized choice is part of the registered command, so a change to
		// either setting has to rewrite the Run entry.
		if !demo && (cfg.AutoStart != previous.AutoStart || cfg.StartMinimized != previous.StartMinimized) {
			if cfg.AutoStart {
				if autoErr := auto.Enable(cfg.StartMinimized); autoErr != nil {
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
			scheduler.Start(ctx, time.Duration(cfg.RefreshSeconds)*time.Second, scheduledRefresh)
		}
		if claudeSourceChanged && !demo {
			refresh()
		}
	}
	setDisplayMode := func(mode settings.DisplayMode) {
		cfg.DisplayMode = mode
		applyConfig(cfg)
		applyScreen(ui.ScreenForDisplayMode(mode))
	}
	runSignIn := func(id model.ProviderID) {
		if demo || !model.IsClaudeAccount(id) {
			return
		}
		// The first two accounts sign in to the shared profile; every later
		// one opens the window on its own.
		fetcher := claudeWebAuth
		if own, ok := accountWebAuth[id]; ok {
			fetcher = own
		}
		if fetcher == nil {
			return
		}
		diagnostics.Go("web_signin", func() {
			// A sign-in can wait on the user typing credentials, so it gets a
			// generous ceiling well beyond a normal request.
			signInCtx, stop := context.WithTimeout(ctx, 5*time.Minute)
			defer stop()
			err := fetcher.SignIn(signInCtx)
			slog.Info("web.signin", "provider", string(id), "ok", err == nil)
			if err == nil {
				refresh()
			}
		})
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
		diagnostics.Go("connection_action", func() {
			connectCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			defer stop()
			started := time.Now()
			if reconnect {
				_, reconnectErr := implementation.Reconnect(connectCtx)
				code := model.ErrNone
				var safe model.SafeError
				if errors.As(reconnectErr, &safe) {
					code = safe.Code
				} else if errors.Is(reconnectErr, context.DeadlineExceeded) {
					code = model.ErrTimeout
				} else if reconnectErr != nil {
					code = model.ErrUnavailable
				}
				slog.Info("session.reconnect", "provider", string(id), "ok", reconnectErr == nil, "err", string(code), "ms", time.Since(started).Milliseconds())
			} else {
				_ = implementation.Inspect(connectCtx)
			}
			refresh()
		})
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
	}, Close: shell.hide, CloseSettings: func() {
		applyScreen(ui.ScreenForDisplayMode(cfg.DisplayMode))
	}, ConfigChanged: applyConfig, Activity: markActivity,
		Inspect:     func(id model.ProviderID) { runConnectionAction(id, false) },
		Reconnect:   func(id model.ProviderID) { runConnectionAction(id, true) },
		SignIn:      runSignIn,
		CheckUpdate: func() { updates.Check(true) },
		OpenURL:     func(raw string) error { return platform.OpenAllowedURL(a, raw) },
	}
	actions.DemoMode = demo
	// The Auth method only offers itself where the embedded browser can run.
	actions.WebAuthAvailable = !demo && claudeWebAuth != nil && webview.DetectRuntime().Present
	view = ui.NewView(w.Canvas(), catalog, systemLanguage, cfg, actions)
	updates.preparePrompt = func() {
		shell.show()
		applyScreen(ui.SettingsScreen)
	}
	a.Settings().AddListener(func(fyne.Settings) {
		diagnostics.Go("theme_refresh", func() {
			fyne.Do(func() { view.RefreshTheme() })
		})
	})
	if demo {
		state := ui.DemoViewState()
		view.SetState(state)
		setTrayTooltip(state)
	}
	w.SetContent(view.Root)
	applyScreen(ui.ScreenForDisplayMode(cfg.DisplayMode))
	lifecycle = &platform.Lifecycle{Hide: func() { shell.savePosition(); shell.hide() }, Quit: func() {
		shell.savePosition()
		cancel()
		stopTrayPromotionRetries()
		scheduler.Stop()
		_ = controller.Close()
		_ = diagnosticRuntime.EndSession(exitReason.Load().(string))
		a.Quit()
	}}
	w.SetCloseIntercept(lifecycle.CloseRequested)
	tray, err = platform.NewTray(a, w, catalog, i18n.Language(cfg.Language), systemLanguage,
		func() {
			markActivity()
			fyne.Do(shell.show)
		},
		func() {
			markActivity()
			fyne.Do(func() {
				shell.show()
				applyScreen(ui.SettingsScreen)
			})
		},
		func(mode settings.DisplayMode) { fyne.Do(func() { setDisplayMode(mode) }) },
		func() {
			exitReason.Store("tray_quit")
			fyne.Do(lifecycle.ExitRequested)
		},
	)
	if err != nil {
		return err
	}
	if demo {
		setTrayTooltip(ui.DemoViewState())
	} else {
		setTrayTooltip(controller.State())
	}
	a.Lifecycle().SetOnStarted(tray.Ready)
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
			trayPromotionRetryTimer = diagnostics.AfterFunc(delay, "tray_promotion_retry", func() {
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
	shell.show()
	if err := native.Bind(); err != nil {
		return err
	}
	shell.workAreas = platform.MonitorWorkAreas()
	effectiveLanguage := i18n.Language(cfg.Language)
	if cfg.Language == settings.LanguageSystem {
		effectiveLanguage = systemLanguage
	}
	windowsBuild, _ := platform.CurrentWindowsBuild()
	diagnosticRuntime.LogStart(
		"windows_build", windowsBuild,
		"monitors", len(shell.workAreas),
		"dpi_scale", native.DPIScale(),
		"mode", string(cfg.DisplayMode),
		"language", string(effectiveLanguage),
	)
	diagnostics.Go("idle_trimmer", func() {
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
	})
	alwaysOnTop.Store(cfg.AlwaysOnTop)
	if !demo {
		// Always-on-top yields to fullscreen surfaces: while a borderless
		// window covers the widget's monitor (a video or a game — foreground
		// or not), drop directly beneath it, and rejoin the topmost band once
		// it is gone. Z-order only; hiding the window is off-limits since the
		// blank-window bug taught Fyne must not be bypassed with ShowWindow.
		diagnostics.Go("fullscreen_yield", func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			var yieldedTo uintptr
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !alwaysOnTop.Load() {
						yieldedTo = 0
						continue
					}
					cover, coverErr := native.FullscreenCover()
					if coverErr != nil {
						continue
					}
					if cover != 0 {
						if cover == yieldedTo {
							continue
						}
						if lowerErr := native.LowerBelow(cover); lowerErr == nil {
							yieldedTo = cover
							slog.Info("window.yield", "reason", "fullscreen_cover")
						}
					} else if yieldedTo != 0 {
						if raiseErr := native.RaiseTopmost(); raiseErr == nil {
							yieldedTo = 0
							slog.Info("window.yield", "reason", "cover_gone")
						}
					}
				}
			}
		})
	}
	_ = native.SetAlwaysOnTop(cfg.AlwaysOnTop)
	_ = native.SetTaskbarVisible(cfg.ShowInTaskbar)
	if cfg.WindowPositioned {
		size := view.MinimumSize(view.Screen())
		_ = native.Restore(platform.Rect{X: cfg.WindowX, Y: cfg.WindowY, Width: int(size.Width), Height: int(size.Height)})
	}
	shell.refreshCorners()
	if hidden {
		shell.hide()
	}
	if !demo {
		scheduler.Start(ctx, time.Duration(cfg.RefreshSeconds)*time.Second, scheduledRefresh)
		refresh()
		diagnostics.Go("startup_update_check", func() { fyne.Do(func() { updates.Check(false) }) })
	}
	startPaintWatch(ctx, native.IsVisible, w, func() { view.Show(view.Screen()) })
	a.Run()
	return diagnosticRuntime.EndSession("run_return")
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
