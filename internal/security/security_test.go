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
	for _, value := range []string{"https://openai.com/", "https://auth.openai.com/login", "https://claude.ai/", "https://code.claude.com/docs/en/quickstart", "https://developers.openai.com/codex/cli/", "https://github.com/jungdosa/QuotaDock/releases/latest"} {
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
	for _, value := range []string{"https://api.anthropic.com/api/oauth/usage", "https://platform.claude.com/v1/oauth/token", "https://grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig"} {
		if !IsAllowedProviderRequestURL(value) {
			t.Errorf("expected provider request URL to be allowed: %s", value)
		}
	}
	for _, value := range []string{"http://api.anthropic.com/api/oauth/usage", "https://anthropic.com/api/oauth/usage", "https://api.anthropic.com.evil.invalid/", "https://user@platform.claude.com/", "https://api.grok.com/", "https://grok.com.evil.invalid/"} {
		if IsAllowedProviderRequestURL(value) {
			t.Errorf("expected provider request URL to be rejected: %s", value)
		}
	}
}

func TestUpdateURLAllowlist(t *testing.T) {
	for _, value := range []string{
		"https://api.github.com/repos/jungdosa/QuotaDock/releases/latest",
		"https://objects.githubusercontent.com/github-production-release-asset/file",
		"https://release-assets.githubusercontent.com/github-production-release-asset/file",
		"https://api.github.com:443/repos/jungdosa/QuotaDock/releases/latest",
		// A release asset's browser_download_url is served from github.com itself.
		"https://github.com/jungdosa/QuotaDock/releases/download/v0.7.10/QuotaDock-0.7.10-win-x64-Setup.exe",
	} {
		if !IsAllowedUpdateURL(value) {
			t.Errorf("expected update URL to be allowed: %s", value)
		}
	}
	for _, value := range []string{
		"https://api.github.com.evil.com/",
		"https://evil.com/api.github.com",
		"http://api.github.com/repos/jungdosa/QuotaDock/releases/latest",
		"https://api.github.com:444/repos/jungdosa/QuotaDock/releases/latest",
		"https://user@api.github.com/repos/jungdosa/QuotaDock/releases/latest",
		"https://github.com.evil.com/jungdosa/QuotaDock/releases/download/v1/x.exe",
		"https://gist.github.com/jungdosa/QuotaDock/releases/download/v1/x.exe",
	} {
		if IsAllowedUpdateURL(value) {
			t.Errorf("expected update URL to be rejected: %s", value)
		}
	}
}

func TestPaletteValidation(t *testing.T) {
	if len(PaletteIDs()) != 16 || !IsPaletteID("blue") || IsPaletteID("#00aaff") {
		t.Fatal("palette allowlist mismatch")
	}
}

func TestModulePathsSurviveMasking(t *testing.T) {
	// A real crash stack. The email pattern used to swallow "fyne.io/fyne/v2@v2.8.0"
	// and leave "[REDACTED_EMAIL]/internal/driver/glfw/...", which made panics
	// impossible to read.
	stack := `fyne.io/fyne/v2/internal/driver/glfw.getMonitorScale
	fyne.io/fyne/v2@v2.8.0/internal/driver/glfw/window_desktop.go:297
github.com/go-gl/glfw/v3.4/glfw.PollEvents
	github.com/go-gl/glfw/v3.4@v3.4.0-20240506104042-037f3cc74f2a/v3.4/glfw/window.go:1010
github.com/jungdosa/QuotaDock/cmd/quotadock/main.go:609`

	masked := MaskSecrets(stack)
	for _, keep := range []string{
		"fyne.io/fyne/v2@v2.8.0",
		"github.com/go-gl/glfw/v3.4@v3.4.0-20240506104042-037f3cc74f2a",
		"window_desktop.go:297",
	} {
		if !strings.Contains(masked, keep) {
			t.Errorf("module path %q was redacted out of the stack: %s", keep, masked)
		}
	}
	if strings.Contains(masked, "[REDACTED_EMAIL]") {
		t.Errorf("stack trace should contain no addresses to redact: %s", masked)
	}
}

func TestEmailsAreStillMasked(t *testing.T) {
	// The module-path exemption must not become a way to smuggle an address past
	// the redactor, so check the shapes that sit closest to the exemption.
	for _, address := range []string{
		"user@example.invalid",
		"first.last+tag@sub.example.invalid",
		"someone@v2.example.invalid",
		"a/b@example.invalid",
	} {
		masked := MaskSecrets("contact " + address + " for details")
		if strings.Contains(masked, "@example.invalid") {
			t.Errorf("address %q survived masking: %s", address, masked)
		}
	}
}
