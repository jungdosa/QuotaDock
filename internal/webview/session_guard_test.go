//go:build windows

package webview

import (
	"context"
	"testing"
)

// Both entry points reject a disallowed address before the browser is ever
// created, so this needs no runtime and always runs.
func TestSessionRefusesAddressesOutsideTheAllowlist(t *testing.T) {
	session := NewSession(t.TempDir())
	if _, err := session.Fetch(context.Background(), "https://evil.invalid/api"); err == nil {
		t.Fatal("a disallowed address was fetched")
	}
	if err := session.SignIn(context.Background(), "https://evil.invalid/login", nil); err == nil {
		t.Fatal("a disallowed address was opened for sign-in")
	}
}
