package grok

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

func TestLoadCredential(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	path := writeCredentialFile(t, "private-grok-token", expiresAt, credentialScopePrefix+"profile")
	credential, err := LoadCredential(path)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Key != "private-grok-token" || !credential.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("credential = %+v", credential)
	}
}

func TestLoadCredentialMissingExpiredAndWrongScope(t *testing.T) {
	tests := map[string]string{
		"missing":     filepath.Join(t.TempDir(), "missing.json"),
		"expired":     writeCredentialFile(t, "expired-token", time.Now().Add(-time.Hour), credentialScopePrefix+"profile"),
		"wrong scope": writeCredentialFile(t, "wrong-scope-token", time.Now().Add(time.Hour), "https://accounts.x.ai/sign-in"),
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := LoadCredential(path)
			if !errors.Is(err, shared.ErrNotLoggedIn) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLoadCredentialRejectsMalformedExpiryWithoutLeakingToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	secret := "malformed-expiry-secret"
	raw := fmt.Sprintf(`{"%sprofile":{"key":"%s","expires_at":"not-a-time"}}`, credentialScopePrefix, secret)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadCredential(path)
	if !errors.Is(err, errCredentialInvalid) {
		t.Fatalf("error = %v", err)
	}
	if err != nil && contains(err.Error(), secret) {
		t.Fatal("credential value leaked through parse error")
	}
}

func writeCredentialFile(t *testing.T, key string, expiresAt time.Time, scope string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	raw := fmt.Sprintf(`{"%s":{"key":"%s","expires_at":"%s"}}`, scope, key, expiresAt.UTC().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
