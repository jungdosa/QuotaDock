package grok

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	shared "github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/security"
)

const credentialScopePrefix = "https://auth.x.ai::"

var (
	errCredentialInvalid     = errors.New("Grok credential file is invalid")
	errCredentialUnavailable = errors.New("Grok credential file is unavailable")
)

// Credential is the CLI token this provider borrows. QuotaDock never rotates
// it: the Grok CLI owns the refresh, and racing it would break both.
type Credential struct {
	Key       string
	ExpiresAt time.Time
}

type credentialEntry struct {
	Key       string `json:"key"`
	ExpiresAt string `json:"expires_at"`
}

// DefaultCredentialPath returns the Grok CLI credential path for this user.
func DefaultCredentialPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".grok", "auth.json")
}

func LoadCredential(path string) (Credential, error) {
	if strings.TrimSpace(path) == "" {
		return Credential{}, shared.ErrNotLoggedIn
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credential{}, shared.ErrNotLoggedIn
	}
	if err != nil {
		return Credential{}, errCredentialUnavailable
	}
	if len(raw) == 0 || len(raw) > int(security.DefaultMaxJSONSize) {
		return Credential{}, errCredentialInvalid
	}

	var entries map[string]credentialEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return Credential{}, errCredentialInvalid
	}
	scopes := make([]string, 0, len(entries))
	for scope := range entries {
		if strings.HasPrefix(scope, credentialScopePrefix) {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)

	foundInvalid := false
	foundExpired := false
	for _, scope := range scopes {
		entry := entries[scope]
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.ExpiresAt))
		if err != nil {
			foundInvalid = true
			continue
		}
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(time.Now()) {
			foundExpired = true
			continue
		}
		return Credential{Key: key, ExpiresAt: expiresAt}, nil
	}
	if foundInvalid && !foundExpired {
		return Credential{}, errCredentialInvalid
	}
	return Credential{}, shared.ErrNotLoggedIn
}
