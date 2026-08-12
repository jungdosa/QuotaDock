//go:build windows

package webview

import "testing"

// This machine has the WebView2 runtime installed (confirmed in the Phase 5
// plan), so detection must find it. It is a live check, not a mock: the whole
// point of runtime detection is to reflect the real machine.
func TestDetectRuntimeReportsInstalledRuntime(t *testing.T) {
	got := DetectRuntime()
	if !got.Present || got.Version == "" {
		t.Fatalf("DetectRuntime() = %+v, want a present runtime with a version", got)
	}
	t.Logf("WebView2 runtime version %s", got.Version)
}
