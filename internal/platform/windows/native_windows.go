//go:build windows

package windows

import (
	"fmt"
	"fyne.io/fyne/v2"
	fynedriver "fyne.io/fyne/v2/driver"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	gwlStyle                       = -16
	gwlExStyle                     = -20
	wsCaption                      = 0x00C00000
	wsExAppWindow                  = 0x00040000
	wsExToolWindow                 = 0x00000080
	swpNoSize                      = 0x0001
	swpNoMove                      = 0x0002
	swpNoZOrder                    = 0x0004
	swpFrameChanged                = 0x0020
	swMinimize                     = 6
	monitorDefaultToNearest        = 2
	defaultWindowDPI               = 96
	dwmwaWindowCornerPreference    = 33
	dwmWindowCornerPreferenceSmall = 3
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	gdi32                 = syscall.NewLazyDLL("gdi32.dll")
	dwmapi                = syscall.NewLazyDLL("dwmapi.dll")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	psapi                 = syscall.NewLazyDLL("psapi.dll")
	getWindowLongPtr      = user32.NewProc("GetWindowLongPtrW")
	setWindowLongPtr      = user32.NewProc("SetWindowLongPtrW")
	setWindowPos          = user32.NewProc("SetWindowPos")
	setWindowRgn          = user32.NewProc("SetWindowRgn")
	showWindow            = user32.NewProc("ShowWindow")
	getWindowRect         = user32.NewProc("GetWindowRect")
	getForegroundWindow   = user32.NewProc("GetForegroundWindow")
	getCursorPos          = user32.NewProc("GetCursorPos")
	getDPIForWindow       = user32.NewProc("GetDpiForWindow")
	enumDisplayMonitors   = user32.NewProc("EnumDisplayMonitors")
	getMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	createRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	deleteObject          = gdi32.NewProc("DeleteObject")
	dwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	getCurrentProcess     = kernel32.NewProc("GetCurrentProcess")
	setWorkingSetSize     = kernel32.NewProc("SetProcessWorkingSetSize")
	emptyWorkingSet       = psapi.NewProc("EmptyWorkingSet")
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type winPoint struct{ X, Y int32 }
type monitorInfo struct {
	Size          uint32
	Monitor, Work winRect
	Flags         uint32
}

func signedIndex(value int32) uintptr { return uintptr(value) }

type WindowController struct {
	Window fyne.Window
	HWND   uintptr
}

func NewWindowController(window fyne.Window) *WindowController {
	return &WindowController{Window: window}
}
func (c *WindowController) Bind() error {
	native, ok := c.Window.(fynedriver.NativeWindow)
	if !ok {
		return fmt.Errorf("Fyne window does not expose native context")
	}
	var bindErr error
	native.RunNative(func(value any) {
		ctx, ok := value.(fynedriver.WindowsWindowContext)
		if !ok || ctx.HWND == 0 {
			bindErr = fmt.Errorf("Windows HWND is unavailable")
			return
		}
		c.HWND = ctx.HWND
		RegisterMainWindow(c.HWND)
	})
	return bindErr
}
func (c *WindowController) bound() error {
	if c.HWND != 0 {
		return nil
	}
	return c.Bind()
}
func (c *WindowController) ConfigureFrameless() error {
	if err := c.bound(); err != nil {
		return err
	}
	style, _, _ := getWindowLongPtr.Call(c.HWND, signedIndex(gwlStyle))
	style &^= wsCaption
	setWindowLongPtr.Call(c.HWND, signedIndex(gwlStyle), style)
	setWindowPos.Call(c.HWND, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpFrameChanged)
	return nil
}
func scaledCornerRadius(radius int, dpi uint32) int {
	if radius < 1 {
		return 1
	}
	if dpi == 0 {
		dpi = defaultWindowDPI
	}
	return (radius*int(dpi) + defaultWindowDPI/2) / defaultWindowDPI
}

// ApplyRoundedCorners clips the frameless window to a small rounded region.
// SetWindowRgn takes ownership of a successfully applied region, so only failed
// calls release the region locally.
func (c *WindowController) ApplyRoundedCorners(radius int) error {
	if err := c.bound(); err != nil {
		return err
	}
	var bounds winRect
	ok, _, callErr := getWindowRect.Call(c.HWND, uintptr(unsafe.Pointer(&bounds)))
	if ok == 0 {
		return fmt.Errorf("GetWindowRect: %w", callErr)
	}

	dpi := uint32(defaultWindowDPI)
	if getDPIForWindow.Find() == nil {
		if value, _, _ := getDPIForWindow.Call(c.HWND); value > 0 {
			dpi = uint32(value)
		}
	}
	scaledRadius := scaledCornerRadius(radius, dpi)
	diameter := uintptr(scaledRadius * 2)
	width := uintptr(bounds.Right-bounds.Left) + 1
	height := uintptr(bounds.Bottom-bounds.Top) + 1
	region, _, regionErr := createRoundRectRgn.Call(0, 0, width, height, diameter, diameter)
	if region == 0 {
		return fmt.Errorf("CreateRoundRectRgn: %w", regionErr)
	}
	applied, _, applyErr := setWindowRgn.Call(c.HWND, region, 1)
	if applied == 0 {
		deleteObject.Call(region)
		return fmt.Errorf("SetWindowRgn: %w", applyErr)
	}

	if dwmSetWindowAttribute.Find() == nil {
		preference := uint32(dwmWindowCornerPreferenceSmall)
		dwmSetWindowAttribute.Call(
			c.HWND,
			dwmwaWindowCornerPreference,
			uintptr(unsafe.Pointer(&preference)),
			unsafe.Sizeof(preference),
		)
	}
	return nil
}

func (c *WindowController) MoveBy(dx, dy int) error {
	if dx == 0 && dy == 0 {
		return nil
	}
	current, err := c.Position()
	if err != nil {
		return err
	}
	return c.MoveTo(current.X+dx, current.Y+dy)
}
func (c *WindowController) CursorPos() (int, int, error) {
	var point winPoint
	ok, _, callErr := getCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	if ok == 0 {
		if callErr == syscall.Errno(0) {
			return 0, 0, fmt.Errorf("GetCursorPos failed")
		}
		return 0, 0, fmt.Errorf("GetCursorPos: %w", callErr)
	}
	return int(point.X), int(point.Y), nil
}
func (c *WindowController) MoveTo(x, y int) error {
	if err := c.bound(); err != nil {
		return err
	}
	ok, _, callErr := setWindowPos.Call(c.HWND, 0, uintptr(x), uintptr(y), 0, 0, swpNoZOrder|swpNoSize)
	if ok == 0 {
		return callErr
	}
	return nil
}
func (c *WindowController) Minimize() {
	if c.bound() != nil {
		return
	}
	showWindow.Call(c.HWND, swMinimize)
	_ = c.TrimWorkingSet()
}
func (c *WindowController) IsForeground() bool {
	if c.bound() != nil {
		return false
	}
	foreground, _, _ := getForegroundWindow.Call()
	return foreground != 0 && foreground == c.HWND
}

func (c *WindowController) DPIScale() float64 {
	if c.bound() != nil || getDPIForWindow.Find() != nil {
		return 1
	}
	dpi, _, _ := getDPIForWindow.Call(c.HWND)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / defaultWindowDPI
}

// TrimWorkingSet asks Windows to evict unused resident pages after the widget is
// minimized or hidden. SetProcessWorkingSetSize is retained as a documented
// fallback for Windows variants where EmptyWorkingSet is unavailable or fails.
func (*WindowController) TrimWorkingSet() error {
	process, _, _ := getCurrentProcess.Call()
	if process == 0 {
		return fmt.Errorf("GetCurrentProcess returned a null handle")
	}
	if emptyWorkingSet.Find() == nil {
		if ok, _, _ := emptyWorkingSet.Call(process); ok != 0 {
			return nil
		}
	}
	minusOne := ^uintptr(0)
	ok, _, callErr := setWorkingSetSize.Call(process, minusOne, minusOne)
	if ok == 0 {
		if callErr == syscall.Errno(0) {
			return fmt.Errorf("working-set trim failed")
		}
		return fmt.Errorf("working-set trim: %w", callErr)
	}
	return nil
}
func (c *WindowController) SetAlwaysOnTop(enabled bool) error {
	if err := c.bound(); err != nil {
		return err
	}
	after := uintptr(0xffffffffffffffff)
	if !enabled {
		after = uintptr(0xfffffffffffffffe)
	}
	if unsafe.Sizeof(uintptr(0)) == 4 {
		after = uintptr(uint32(after))
	}
	r, _, e := setWindowPos.Call(c.HWND, after, 0, 0, 0, 0, swpNoMove|swpNoSize)
	if r == 0 {
		return e
	}
	return nil
}
func (c *WindowController) SetTaskbarVisible(visible bool) error {
	if err := c.bound(); err != nil {
		return err
	}
	style, _, _ := getWindowLongPtr.Call(c.HWND, signedIndex(gwlExStyle))
	if visible {
		style |= wsExAppWindow
		style &^= wsExToolWindow
	} else {
		style |= wsExToolWindow
		style &^= wsExAppWindow
	}
	setWindowLongPtr.Call(c.HWND, signedIndex(gwlExStyle), style)
	setWindowPos.Call(c.HWND, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpFrameChanged)
	return nil
}
func (c *WindowController) Position() (Rect, error) {
	if err := c.bound(); err != nil {
		return Rect{}, err
	}
	var r winRect
	ok, _, e := getWindowRect.Call(c.HWND, uintptr(unsafe.Pointer(&r)))
	if ok == 0 {
		return Rect{}, e
	}
	return Rect{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}, nil
}
func (c *WindowController) Restore(saved Rect) error {
	if err := c.bound(); err != nil {
		return err
	}
	current, err := c.Position()
	if err != nil {
		return err
	}
	saved.Width, saved.Height = current.Width, current.Height
	target := RestoreRect(saved, MonitorWorkAreas())
	ok, _, callErr := setWindowPos.Call(c.HWND, 0, uintptr(target.X), uintptr(target.Y), 0, 0, swpNoZOrder|swpNoSize|swpFrameChanged)
	if ok == 0 {
		return callErr
	}
	return nil
}
func MonitorWorkAreas() []Rect {
	areas := []Rect{}
	callback := syscall.NewCallback(func(monitor, hdc, raw, data uintptr) uintptr {
		info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
		if ok != 0 {
			r := info.Work
			area := Rect{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}
			if info.Flags&1 != 0 {
				areas = append([]Rect{area}, areas...)
			} else {
				areas = append(areas, area)
			}
		}
		return 1
	})
	enumDisplayMonitors.Call(0, 0, callback, 0)
	runtime.KeepAlive(callback)
	return areas
}
