package security

import (
	"strings"
	"testing"
)

func TestSensitiveLogMasking(t *testing.T) {
	input := `Authorization: Bearer fake-secret access_token=alpha refresh_token=beta cookie=session password=pw user@example.invalid {"csrf_token":"gamma","api_key":"delta"}`
	masked := MaskSecrets(input)
	for _, secret := range []string{"fake-secret", "alpha", "beta", "session", "pw", "user@example.invalid", "gamma", "delta"} {
		if strings.Contains(masked, secret) {
			t.Errorf("masked output retained %q: %s", secret, masked)
		}
	}
}

func TestURLAllowlist(t *testing.T) {
	for _, value := range []string{"https://openai.com/", "https://auth.openai.com/login", "https://claude.ai/", "https://code.claude.com/docs/en/quickstart", "https://developers.openai.com/codex/cli/"} {
		if !IsAllowedExternalURL(value) {
			t.Errorf("expected allowed: %s", value)
		}
	}
	for _, value := range []string{"http://openai.com", "https://openai.com.evil.invalid", "https://127.0.0.1", "https://user@openai.com"} {
		if IsAllowedExternalURL(value) {
			t.Errorf("expected rejected: %s", value)
		}
	}
}

func TestProviderRequestURLAllowlist(t *testing.T) {
	for _, value := range []string{"https://api.anthropic.com/api/oauth/usage", "https://platform.claude.com/v1/oauth/token"} {
		if !IsAllowedProviderRequestURL(value) {
			t.Errorf("expected provider request URL to be allowed: %s", value)
		}
	}
	for _, value := range []string{"http://api.anthropic.com/api/oauth/usage", "https://anthropic.com/api/oauth/usage", "https://api.anthropic.com.evil.invalid/", "https://user@platform.claude.com/"} {
		if IsAllowedProviderRequestURL(value) {
			t.Errorf("expected provider request URL to be rejected: %s", value)
		}
	}
}

func TestPaletteValidation(t *testing.T) {
	if len(PaletteIDs()) != 16 || !IsPaletteID("blue") || IsPaletteID("#00aaff") {
		t.Fatal("palette allowlist mismatch")
	}
}
