// Package webview hosts QuotaDock's embedded browser auth infrastructure.
//
// The visible sign-in surface and the hidden usage fetch both run inside a
// WebView2 control so the browser session — cookies included — stays owned by
// the OS. QuotaDock never reads the cookie store: the session lives only in
// the WebView2 user-data folder, and account responses are never logged.
//
// This file is platform-agnostic. The Windows implementation lives in
// webview_windows.go; other platforms get inert stubs so the desktop entry
// point keeps building.
package webview

// Availability reports whether the embedded browser can run on this machine.
// It is a value, not a live handle, so callers can decide up front whether to
// offer the Auth connection method or fall back to CLI-only quietly.
type Availability struct {
	// Present is true when the WebView2 runtime is installed.
	Present bool
	// Version is the detected runtime version string, empty when absent.
	Version string
}

// DefaultUserDataDir is the per-user folder WebView2 keeps the browser session
// in. It sits beside the diagnostics logs, never inside the repo or settings.
const DefaultUserDataDir = "webview"
