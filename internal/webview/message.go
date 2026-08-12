package webview

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/jungdosa/QuotaDock/internal/security"
)

// IsAllowedNavigation gates where the sign-in window may go. It delegates to
// the security package so the allowlist has one home.
func IsAllowedNavigation(raw string) bool { return security.IsAllowedAuthWebNavigationURL(raw) }

// IsAllowedFetch gates the in-session request. It is deliberately narrower
// than navigation: this is the call that carries the session.
func IsAllowedFetch(raw string) bool { return security.IsAllowedAuthWebFetchURL(raw) }

type webMessage struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// decodeURLMessage extracts a navigation report. Anything else is ignored:
// the page can post arbitrary messages, so nothing is trusted by shape alone.
func decodeURLMessage(raw string) (string, bool) {
	var message webMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return "", false
	}
	if message.Kind != "url" || message.Value == "" {
		return "", false
	}
	return message.Value, true
}

// decodeResultMessage extracts a fetch outcome.
func decodeResultMessage(raw string) (kind, value string, ok bool) {
	var message webMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return "", "", false
	}
	switch message.Kind {
	case "body", "error":
		return message.Kind, message.Value, true
	}
	return "", "", false
}

// originOf reduces a request address to its scheme and host, which is the page
// the hidden window loads before running a same-origin request.
func originOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	return parsed.Scheme + "://" + parsed.Host + "/"
}

// SignedInAt reports whether a URL looks like the signed-in application rather
// than the login flow. It is deliberately conservative: only a known
// application path counts, so a redirect that merely passes through the host
// does not end the sign-in window early.
func SignedInAt(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.TrimSuffix(parsed.Path, "/")
	if host != "claude.ai" && host != "www.claude.ai" {
		return false
	}
	switch {
	case path == "", path == "/new", path == "/chats", path == "/recents":
		return true
	case strings.HasPrefix(path, "/chat/"), strings.HasPrefix(path, "/project"):
		return true
	}
	return false
}

func sleepShort() { time.Sleep(25 * time.Millisecond) }
func sleepLong()  { time.Sleep(400 * time.Millisecond) }
