package webview

import "testing"

func TestOnlyWellFormedMessagesAreAccepted(t *testing.T) {
	if url, ok := decodeURLMessage(`{"kind":"url","value":"https://claude.ai/login"}`); !ok || url != "https://claude.ai/login" {
		t.Fatalf("url message = %q ok=%t", url, ok)
	}
	// The page can post anything; nothing is trusted by shape alone.
	for _, raw := range []string{"", "not json", `{"kind":"url"}`, `{"kind":"body","value":"x"}`, `["url"]`} {
		if _, ok := decodeURLMessage(raw); ok {
			t.Fatalf("accepted a bad url message: %q", raw)
		}
	}
	kind, value, ok := decodeResultMessage(`{"kind":"body","value":"{}"}`)
	if !ok || kind != "body" || value != "{}" {
		t.Fatalf("result message = %q/%q ok=%t", kind, value, ok)
	}
	if _, _, ok := decodeResultMessage(`{"kind":"url","value":"x"}`); ok {
		t.Fatal("a url message was accepted as a fetch result")
	}
}

func TestOriginIsReducedForTheHiddenWindow(t *testing.T) {
	if got := originOf("https://claude.ai/api/organizations"); got != "https://claude.ai/" {
		t.Fatalf("origin = %q", got)
	}
}

// Only the signed-in application counts, so a redirect that merely passes
// through the host does not end the sign-in window early.
func TestSignedInDetectionIsConservative(t *testing.T) {
	for _, raw := range []string{"https://claude.ai/", "https://claude.ai/new", "https://claude.ai/chat/abc"} {
		if !SignedInAt(raw) {
			t.Fatalf("signed-in page not detected: %s", raw)
		}
	}
	for _, raw := range []string{"https://claude.ai/login", "https://auth.anthropic.com/oauth", "https://claude.ai/magic-link", "https://evil.invalid/"} {
		if SignedInAt(raw) {
			t.Fatalf("wrongly treated as signed in: %s", raw)
		}
	}
}

func TestFetchScriptTargetsTheRequestedURL(t *testing.T) {
	script := fetchScript("https://claude.ai/api/organizations")
	for _, want := range []string{"https://claude.ai/api/organizations", "credentials", "postMessage"} {
		if !contains(script, want) {
			t.Fatalf("fetch script missing %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
