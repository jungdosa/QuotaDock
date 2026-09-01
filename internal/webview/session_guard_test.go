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
	first := func(_ int, _ string) (string, bool) { return "https://evil.invalid/api", true }
	if _, err := session.Fetch(context.Background(), first); err == nil {
		t.Fatal("a disallowed address was fetched")
	}
	if err := session.SignIn(context.Background(), "https://evil.invalid/login", nil); err == nil {
		t.Fatal("a disallowed address was opened for sign-in")
	}
}

// Only the first address in a chain is fixed by the caller; a later one is
// built from a response, which makes it the one an unexpected reply could
// steer. The chain runs on the browser's pump thread where no test can reach
// it, so the decision it makes there is asserted here instead.
func TestALaterAddressInAChainFacesTheSameAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		address string
		more    bool
		allowed bool
	}{
		{name: "an allowed follow-up", address: "https://claude.ai/api/organizations/x/usage", more: true, allowed: true},
		{name: "an address off the allowed hosts", address: "https://evil.invalid/api", more: true, allowed: false},
		{name: "an empty address", address: "", more: true, allowed: false},
		{name: "the end of the chain", more: false, allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			address, more, allowed := nextRequest(func(int, string) (string, bool) {
				return test.address, test.more
			}, 1, "{}")
			if more != test.more || allowed != test.allowed {
				t.Fatalf("nextRequest(%q) = more %v, allowed %v; want %v, %v", test.address, more, allowed, test.more, test.allowed)
			}
			if more && address != test.address {
				t.Fatalf("nextRequest returned %q, want %q", address, test.address)
			}
		})
	}
	if _, more, allowed := nextRequest(nil, 0, ""); more || allowed {
		t.Fatal("a chain with no description must request nothing")
	}
}
