// Package security contains validation and redaction shared by core packages.
package security

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
)

const DefaultMaxJSONSize int64 = 1 << 20

var (
	emailPattern = regexp.MustCompile(`(?i)\b[a-z0-9.!#$%&'*+/=?^_{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\b`)
	authPattern  = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;"'}]+`)
	jsonSecret   = regexp.MustCompile(`(?i)("(?:access[_-]?token|refresh[_-]?token|csrf(?:[_-]?token)?|cookie|password|api[_-]?key)"\s*:\s*")[^"]*(")`)
	pairSecret   = regexp.MustCompile(`(?i)\b(access[_-]?token|refresh[_-]?token|csrf(?:[_-]?token)?|cookie|password|api[_-]?key|token)\s*=\s*[^\s,;&]+`)
	bearer       = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
)

func MaskSecrets(value string) string {
	value = jsonSecret.ReplaceAllString(value, `${1}[REDACTED]${2}`)
	value = authPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = pairSecret.ReplaceAllStringFunc(value, func(match string) string {
		if index := strings.Index(match, "="); index >= 0 {
			return match[:index+1] + "[REDACTED]"
		}
		return "[REDACTED]"
	})
	value = bearer.ReplaceAllString(value, "Bearer [REDACTED]")
	return emailPattern.ReplaceAllString(value, "[REDACTED_EMAIL]")
}

// External URL matching is boundary-aware suffix matching, so allowing
// github.com also allows its *.github.com subdomains.
var officialDomains = []string{"anthropic.com", "claude.ai", "claude.com", "openai.com", "chatgpt.com", "google.com", "github.com"}
var providerRequestHosts = map[string]struct{}{
	"api.anthropic.com":   {},
	"platform.claude.com": {},
}
var updateRequestHosts = map[string]struct{}{
	"api.github.com":                       {},
	"objects.githubusercontent.com":        {},
	"release-assets.githubusercontent.com": {},
}

func IsAllowedExternalURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return false
	}
	for _, domain := range officialDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// IsAllowedProviderRequestURL is intentionally narrower than the browser URL
// allowlist. Provider credentials may only be sent to exact audited hosts.
func IsAllowedProviderRequestURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return false
	}
	_, allowed := providerRequestHosts[strings.ToLower(parsed.Hostname())]
	return allowed
}

// IsAllowedUpdateURL is only for update checks and downloads that carry no
// credentials. Never use it for requests that send credentials.
func IsAllowedUpdateURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return false
	}
	_, allowed := updateRequestHosts[strings.ToLower(parsed.Hostname())]
	return allowed
}

var palette = map[string]struct{}{"slate": {}, "gray": {}, "red": {}, "orange": {}, "amber": {}, "yellow": {}, "lime": {}, "green": {}, "emerald": {}, "teal": {}, "cyan": {}, "sky": {}, "blue": {}, "indigo": {}, "violet": {}, "purple": {}}

func IsPaletteID(id string) bool { _, ok := palette[id]; return ok }
func PaletteIDs() []string {
	return []string{"slate", "gray", "red", "orange", "amber", "yellow", "lime", "green", "emerald", "teal", "cyan", "sky", "blue", "indigo", "violet", "purple"}
}

func DecodeJSONLimited(reader io.Reader, maxBytes int64, destination any) error {
	if maxBytes <= 0 {
		return errors.New("maximum JSON size must be positive")
	}
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if limited.N <= 0 {
		return fmt.Errorf("JSON exceeds %d bytes", maxBytes)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func ClampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
func ClampFloat(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
