//go:build windows

package webview

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These exercise the real Edge WebView2 runtime and open real windows, so
// they stay behind an environment switch like the provider live tests.
// Set QUOTADOCK_LIVE_WEBVIEW=1 to run them.
func liveWebViewEnabled(t *testing.T) string {
	t.Helper()
	if os.Getenv("QUOTADOCK_LIVE_WEBVIEW") != "1" {
		t.Skip("set QUOTADOCK_LIVE_WEBVIEW=1 to drive the installed WebView2 runtime")
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "QuotaDock", "webview-probe")
}

func TestLiveRuntimeIsDetected(t *testing.T) {
	liveWebViewEnabled(t)
	got := DetectRuntime()
	if !got.Present || got.Version == "" {
		t.Fatalf("DetectRuntime() = %+v, want an installed runtime", got)
	}
	t.Logf("WebView2 runtime %s", got.Version)
}

// The window has to open, reach the sign-in page, and report that navigation
// back through the message channel. Only the human sign-in is out of scope.
func TestLiveSignInWindowOpensAndReportsNavigation(t *testing.T) {
	dir := liveWebViewEnabled(t)
	session := NewSession(dir)
	seen := 0
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := session.SignIn(ctx, "https://claude.ai/login", func(url string) bool {
		seen++
		return false
	})
	if seen == 0 {
		t.Fatalf("no navigation was reported (SignIn returned %v)", err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("the browser profile folder was not created: %v", statErr)
	}
}

// The hidden request must run inside the stored session and come back with a
// body. The probe profile is signed out, so only delivery is asserted - the
// response is never logged, because account replies carry personal data.
func TestLiveHiddenFetchReturnsABody(t *testing.T) {
	dir := liveWebViewEnabled(t)
	session := NewSession(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := session.Fetch(ctx, "https://claude.ai/api/organizations")
	if err != nil {
		t.Fatalf("hidden fetch failed: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("hidden fetch returned an empty body")
	}
	t.Logf("hidden fetch returned %d bytes", len(body))
}
