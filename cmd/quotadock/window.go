package main

import (
	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/diagnostics"
	platform "github.com/jungdosa/QuotaDock/internal/platform/windows"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"github.com/jungdosa/QuotaDock/internal/ui"
	"log/slog"
	"time"
)

type windowShell struct {
	window       fyne.Window
	native       *platform.WindowController
	idleTrimmer  *platform.IdleTrimmer
	cfg          *settings.Config
	settingsPath string
	demo         bool
	workAreas    []platform.Rect
}

func (shell *windowShell) show() {
	shell.window.Show()
}

func (shell *windowShell) hide() {
	shell.window.Hide()
	if trimErr := shell.native.TrimWorkingSet(); trimErr != nil {
		slog.Debug("working set was not trimmed", "error", trimErr)
	} else {
		shell.idleTrimmer.MarkTrimmed()
	}
}

func (shell *windowShell) savePosition() {
	if shell.demo {
		return
	}
	position, posErr := shell.native.Position()
	if posErr == nil {
		shell.cfg.WindowX = position.X
		shell.cfg.WindowY = position.Y
		shell.cfg.WindowPositioned = true
		_ = saveSettings(shell.settingsPath, *shell.cfg, true)
	}
}

func (shell *windowShell) refreshCorners() {
	apply := func() {
		if cornerErr := shell.native.ApplyRoundedCorners(int(ui.WindowCornerRadius)); cornerErr != nil {
			slog.Debug("rounded window region was not applied", "error", cornerErr)
		}
	}
	apply()
	diagnostics.AfterFunc(100*time.Millisecond, "rounded_corners", func() { fyne.Do(apply) })
}

func (shell *windowShell) fitToAreas(areas []platform.Rect, reason string) {
	position, positionErr := shell.native.Position()
	if positionErr != nil {
		return
	}
	fitted := platform.FitToWorkArea(position, areas)
	if fitted == position {
		return
	}
	if moveErr := shell.native.MoveTo(fitted.X, fitted.Y); moveErr == nil {
		slog.Debug("window.fit", "reason", reason, "from", rectValue(position), "to", rectValue(fitted))
	}
}

func (shell *windowShell) fitToScreen() {
	shell.fitToAreas(platform.MonitorWorkAreas(), "resize")
}

func (shell *windowShell) checkDisplayChange() {
	current := platform.MonitorWorkAreas()
	previous := shell.workAreas
	if platform.WorkAreasEqual(previous, current) {
		return
	}
	shell.workAreas = append([]platform.Rect(nil), current...)
	slog.Info("display.change", "before", len(previous), "after", len(current), "areas", areasValue(current))
	position, positionErr := shell.native.Position()
	if positionErr != nil {
		return
	}
	_, fitted, shouldFit := platform.DisplayChange(previous, current, position)
	if !shouldFit {
		return
	}
	if moveErr := shell.native.MoveTo(fitted.X, fitted.Y); moveErr == nil {
		slog.Debug("window.fit", "reason", "display_change", "from", rectValue(position), "to", rectValue(fitted))
	}
}

// Rectangles reach the diagnostic log as plain integer arrays. The log writer
// only accepts values it can serialize, so window geometry is flattened here
// rather than handed over as a struct.

func rectValue(rect platform.Rect) []int {
	return []int{rect.X, rect.Y, rect.Width, rect.Height}
}

func areasValue(areas []platform.Rect) [][]int {
	value := make([][]int, 0, len(areas))
	for _, area := range areas {
		value = append(value, rectValue(area))
	}
	return value
}
