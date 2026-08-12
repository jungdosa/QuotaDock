//go:build windows

package webview

import "github.com/wailsapp/go-webview2/webviewloader"

// DetectRuntime reports whether the Edge WebView2 runtime is installed. It
// asks the loader for the installed browser version and treats an empty
// string — the loader's "not found" signal — as absent. Any lookup error is
// also absent: a broken probe must degrade to CLI-only, never crash startup.
func DetectRuntime() Availability {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil || version == "" {
		return Availability{}
	}
	return Availability{Present: true, Version: version}
}
