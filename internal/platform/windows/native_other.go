//go:build !windows

package windows

import "fyne.io/fyne/v2"

// WindowController keeps the desktop entry point buildable on non-Windows
// systems; Windows-only native window behavior becomes a no-op there.
type WindowController struct {
	Window fyne.Window
}

func NewWindowController(window fyne.Window) *WindowController {
	return &WindowController{Window: window}
}
func (*WindowController) Bind() error                   { return nil }
func (*WindowController) ConfigureFrameless() error     { return nil }
func (*WindowController) ApplyRoundedCorners(int) error { return nil }
func (*WindowController) MoveBy(int, int) error         { return nil }
func (*WindowController) CursorPos() (int, int, error)  { return 0, 0, nil }
func (*WindowController) MoveTo(int, int) error         { return nil }
func (*WindowController) Minimize()                     {}
func (*WindowController) IsForeground() bool            { return true }
func (*WindowController) DPIScale() float64              { return 1 }
func (*WindowController) TrimWorkingSet() error         { return nil }
func (*WindowController) SetAlwaysOnTop(bool) error     { return nil }
func (*WindowController) SetTaskbarVisible(bool) error  { return nil }
func (*WindowController) Position() (Rect, error)       { return Rect{}, errUnsupportedPlatform }
func (*WindowController) Restore(Rect) error            { return nil }
func MonitorWorkAreas() []Rect                          { return nil }
