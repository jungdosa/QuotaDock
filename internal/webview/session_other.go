//go:build !windows

package webview

import (
	"context"
	"errors"
)

// Session keeps the desktop entry point buildable off Windows. The embedded
// sign-in browser is WebView2, which exists only there; macOS will grow a
// WKWebView implementation when that platform lands.
type Session struct{ userDataDir string }

func NewSession(userDataDir string) *Session { return &Session{userDataDir: userDataDir} }

var errUnsupported = errors.New("the embedded sign-in browser is available on Windows only")

func (*Session) SignIn(context.Context, string, func(string) bool) error { return errUnsupported }
func (*Session) Fetch(context.Context, string) (string, error)          { return "", errUnsupported }
func (*Session) Close() error                                           { return nil }
