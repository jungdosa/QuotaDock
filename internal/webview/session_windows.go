//go:build windows

package webview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

// The sign-in window is deliberately its own Win32 window with its own message
// pump on a dedicated OS thread. It never touches the Fyne window: the
// blank-window incident taught that reaching around Fyne's own window handling
// breaks rendering in ways that are hard to diagnose, so the two never share a
// handle or a thread.
var (
	user32              = syscall.NewLazyDLL("user32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	registerClassExW    = user32.NewProc("RegisterClassExW")
	createWindowExW     = user32.NewProc("CreateWindowExW")
	defWindowProcW      = user32.NewProc("DefWindowProcW")
	destroyWindowProc   = user32.NewProc("DestroyWindow")
	postQuitMessageProc = user32.NewProc("PostQuitMessage")
	getMessageW         = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessageW    = user32.NewProc("DispatchMessageW")
	postMessageW        = user32.NewProc("PostMessageW")
	getClientRect       = user32.NewProc("GetClientRect")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	loadCursorW         = user32.NewProc("LoadCursorW")
	getModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	ole32               = syscall.NewLazyDLL("ole32.dll")
	coInitializeEx      = ole32.NewProc("CoInitializeEx")
	coUninitialize      = ole32.NewProc("CoUninitialize")
)

// WebView2 is an apartment-threaded COM object, so the pump thread has to join
// an STA before the environment is created. Without this the environment
// creation fails outright with "CoInitialize has not been called".
const (
	coinitApartmentThreaded = 0x2
	rpcEChangedMode         = 0x80010106
)

// enterApartment joins the STA and reports whether this call owns it. Windows
// answers RPC_E_CHANGED_MODE when the thread already belongs to a different
// apartment; that is survivable, but this call must not then uninitialize it.
func enterApartment() (owned bool, err error) {
	result, _, _ := coInitializeEx.Call(0, coinitApartmentThreaded)
	switch uint32(result) {
	case 0, 1: // S_OK, S_FALSE (already initialized on this thread)
		return true, nil
	case rpcEChangedMode:
		return false, nil
	default:
		return false, fmt.Errorf("join the browser apartment: 0x%x", uint32(result))
	}
}

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	swShow             = 5
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmClose            = 0x0010
	wmApp              = 0x8000
	// wmCancelSession asks the pump thread to tear the window down from
	// another goroutine; posting a message is the only thread-safe way in.
	wmCancelSession = wmApp + 1
	idcArrow        = 32512
	smCXScreen      = 0
	smCYScreen      = 1
	cwUseDefault    = 0x80000000
)

type winRect struct{ Left, Top, Right, Bottom int32 }

type winMsg struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type wndClassExW struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}

var (
	classOnce sync.Once
	classErr  error
	className *uint16
)

// registerWindowClass installs the shared window class once per process.
func registerWindowClass() (*uint16, error) {
	classOnce.Do(func() {
		name, err := syscall.UTF16PtrFromString("QuotaDockAuthWindow")
		if err != nil {
			classErr = err
			return
		}
		instance, _, _ := getModuleHandleW.Call(0)
		cursor, _, _ := loadCursorW.Call(0, uintptr(idcArrow))
		class := wndClassExW{
			Size:      uint32(unsafe.Sizeof(wndClassExW{})),
			WndProc:   syscall.NewCallback(windowProc),
			Instance:  instance,
			Cursor:    cursor,
			ClassName: name,
		}
		if atom, _, callErr := registerClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			classErr = fmt.Errorf("register auth window class: %w", callErr)
			return
		}
		className = name
	})
	return className, classErr
}

// windows maps a live HWND to its session so the window procedure, which
// Windows calls without any user data, can find the owner.
var (
	windowsMu sync.Mutex
	windows   = map[uintptr]*Session{}
)

func windowProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmSize:
		windowsMu.Lock()
		session := windows[hwnd]
		windowsMu.Unlock()
		if session != nil && session.chromium != nil {
			session.chromium.Resize()
		}
		return 0
	case wmCancelSession, wmClose:
		destroyWindowProc.Call(hwnd)
		return 0
	case wmDestroy:
		postQuitMessageProc.Call(0)
		return 0
	}
	result, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return result
}

// Session owns one sign-in window and the browser profile behind it. The
// profile lives in the user-data folder, which is the only place the browser
// session is kept; QuotaDock never reads the cookie store itself.
type Session struct {
	userDataDir string

	chromium *edge.Chromium
	hwnd     uintptr

	mu       sync.Mutex
	messages chan string
}

func NewSession(userDataDir string) *Session {
	return &Session{userDataDir: userDataDir, messages: make(chan string, 16)}
}

// reportScript posts every document's URL back to Go so navigation can be
// observed without reaching into COM for the source address.
const reportScript = `(function(){try{window.chrome.webview.postMessage(JSON.stringify({kind:"url",value:location.href}));}catch(e){}})();`

type windowOptions struct {
	title    string
	width    int32
	height   int32
	visible  bool
	startURL string
}

// open creates the window, embeds the browser, and runs the message pump on
// this goroutine. It must be called from a goroutine that owns its OS thread.
func (s *Session) open(options windowOptions, onMessage func(string) bool) error {
	// The binding's internal error path ends in os.Exit, so a browser failure
	// would take the whole widget down rather than degrading to CLI-only.
	// Refusing to start without a healthy runtime removes the realistic cause
	// (a missing or broken WebView2) before the browser is ever created.
	if !DetectRuntime().Present {
		return errors.New("the Edge WebView2 runtime is not installed")
	}
	if err := os.MkdirAll(s.userDataDir, 0o755); err != nil {
		return fmt.Errorf("prepare the browser profile folder: %w", err)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	owned, err := enterApartment()
	if err != nil {
		return err
	}
	if owned {
		defer coUninitialize.Call()
	}

	name, err := registerWindowClass()
	if err != nil {
		return err
	}
	title, err := syscall.UTF16PtrFromString(options.title)
	if err != nil {
		return err
	}
	instance, _, _ := getModuleHandleW.Call(0)
	style := uintptr(wsOverlappedWindow)
	if options.visible {
		style |= wsVisible
	}
	x, y := uintptr(cwUseDefault), uintptr(cwUseDefault)
	if options.visible {
		screenW, _, _ := getSystemMetrics.Call(smCXScreen)
		screenH, _, _ := getSystemMetrics.Call(smCYScreen)
		if screenW > 0 && screenH > 0 {
			x = uintptr((int32(screenW) - options.width) / 2)
			y = uintptr((int32(screenH) - options.height) / 2)
		}
	}
	hwnd, _, callErr := createWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(title)),
		style,
		x, y,
		uintptr(options.width), uintptr(options.height),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("create auth window: %w", callErr)
	}
	s.hwnd = hwnd
	windowsMu.Lock()
	windows[hwnd] = s
	windowsMu.Unlock()
	defer func() {
		windowsMu.Lock()
		delete(windows, hwnd)
		windowsMu.Unlock()
	}()

	chromium := edge.NewChromium()
	chromium.DataPath = s.userDataDir
	chromium.MessageCallback = func(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		if onMessage != nil && onMessage(message) {
			postMessageW.Call(hwnd, wmCancelSession, 0, 0)
		}
	}
	chromium.SetErrorCallback(func(error) {})
	if !chromium.Embed(hwnd) {
		destroyWindowProc.Call(hwnd)
		return errors.New("the embedded browser could not start")
	}
	s.chromium = chromium
	// Closing the controller ends the browser process. Without it the
	// msedgewebview2 host lingers and keeps the profile folder locked.
	defer func() {
		chromium.ShuttingDown()
		if controller := chromium.GetController(); controller != nil {
			controller.Release()
		}
		s.chromium = nil
	}()
	chromium.Init(reportScript)
	chromium.Resize()
	// WebView2 is a single-threaded COM object: every call has to happen on
	// the thread that created it. Navigation therefore starts here rather than
	// from the caller's goroutine, and later calls ride the message callback,
	// which WebView2 also raises on this thread.
	if options.startURL != "" {
		chromium.Navigate(options.startURL)
	}

	var message winMsg
	for {
		result, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

// cancel asks the pump thread to close the window. Safe from any goroutine.
func (s *Session) cancel() {
	if hwnd := s.hwnd; hwnd != 0 {
		postMessageW.Call(hwnd, wmCancelSession, 0, 0)
	}
}

func (s *Session) navigate(url string) {
	if s.chromium != nil {
		s.chromium.Navigate(url)
	}
}

// SignIn opens a visible window at startURL and returns once ready reports the
// user has arrived at a signed-in page, the user closes the window, or ctx
// ends. Navigation is watched: leaving the allowed domains closes the window
// rather than letting an unexpected host render inside the app.
func (s *Session) SignIn(ctx context.Context, startURL string, ready func(url string) bool) error {
	if !IsAllowedNavigation(startURL) {
		return fmt.Errorf("sign-in address is not allowed")
	}
	var (
		mu       sync.Mutex
		signedIn bool
		blocked  bool
	)
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("sign-in window failed: %v", recovered)
			}
		}()
		err := s.open(windowOptions{title: "QuotaDock — Sign in", width: 520, height: 680, visible: true, startURL: startURL}, func(message string) bool {
			url, ok := decodeURLMessage(message)
			if !ok {
				return false
			}
			mu.Lock()
			defer mu.Unlock()
			if !IsAllowedNavigation(url) {
				blocked = true
				return true
			}
			if ready != nil && ready(url) {
				signedIn = true
				return true
			}
			return false
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if blocked {
			return errors.New("sign-in left the allowed sites")
		}
		if !signedIn {
			return errors.New("sign-in was cancelled")
		}
		return nil
	case <-ctx.Done():
		s.cancel()
		<-done
		return ctx.Err()
	}
}

// Fetch runs a same-session request in a hidden window and returns the body.
// The response is never logged: account pages carry personal data.
func (s *Session) Fetch(ctx context.Context, requestURL string) (string, error) {
	if !IsAllowedFetch(requestURL) {
		return "", fmt.Errorf("request address is not allowed")
	}
	var (
		mu     sync.Mutex
		body   string
		failed string
	)
	done := make(chan error, 1)
	requested := false
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("fetch window failed: %v", recovered)
			}
		}()
		err := s.open(windowOptions{title: "QuotaDock", width: 480, height: 360, visible: false, startURL: originOf(requestURL)}, func(message string) bool {
			// This callback runs on the pump thread, which is the only thread
			// allowed to call the browser, so the request is issued from here
			// once the origin has loaded and its cookies are in scope.
			if _, ok := decodeURLMessage(message); ok {
				if !requested && s.chromium != nil {
					requested = true
					s.chromium.Eval(fetchScript(requestURL))
				}
				return false
			}
			kind, value, ok := decodeResultMessage(message)
			if !ok {
				return false
			}
			mu.Lock()
			defer mu.Unlock()
			switch kind {
			case "body":
				body = value
			case "error":
				failed = value
			default:
				return false
			}
			return true
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			return "", err
		}
		mu.Lock()
		defer mu.Unlock()
		if failed != "" {
			return "", errors.New("the request did not complete")
		}
		if body == "" {
			return "", errors.New("the request returned nothing")
		}
		return body, nil
	case <-ctx.Done():
		s.cancel()
		<-done
		return "", ctx.Err()
	}
}

// Close releases the window if one is still open.
func (s *Session) Close() error {
	s.cancel()
	return nil
}

// fetchScript performs the request with the session's own cookies and posts
// the result back through the same channel the URL reporter uses.
func fetchScript(requestURL string) string {
	escaped := strings.ReplaceAll(requestURL, `"`, `\"`)
	return `(function(){fetch("` + escaped + `",{credentials:"include",headers:{"Accept":"application/json"}})` +
		`.then(function(r){return r.text();})` +
		`.then(function(t){window.chrome.webview.postMessage(JSON.stringify({kind:"body",value:t}));})` +
		`.catch(function(e){window.chrome.webview.postMessage(JSON.stringify({kind:"error",value:String(e)}));});})();`
}
