package security

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourceHasNoCredentialFileCookieDBOrTelemetryAccess(t *testing.T) {
	root := filepath.Join("..", "..")
	forbidden := []string{".codex/auth.json", ".codex\\auth.json", "browser cookie db", "cookies sqlite", "telemetry endpoint", "analytics endpoint"}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == ".cache" || name == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(data))
		for _, marker := range forbidden {
			if strings.Contains(lower, marker) {
				t.Errorf("%s contains forbidden production access marker %q", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFixturesContainNoCredentialOrAccountIdentity(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	forbidden := []string{"@", "access_token", "refresh_token", "authorization", "cookie", "password", "api_key"}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(data))
		for _, marker := range forbidden {
			if strings.Contains(lower, marker) {
				t.Errorf("fixture %s contains forbidden marker %q", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClaudeOAuthSourceDoesNotReadKeyringCookieOrLogSecrets(t *testing.T) {
	path := filepath.Join("..", "provider", "claude", "oauth.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"go-keyring", "keyring::", "wincred", "cookiejar", "cookies sqlite", "log.printf", "log.println"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Claude OAuth source contains forbidden credential/log access marker %q", forbidden)
		}
	}
}
