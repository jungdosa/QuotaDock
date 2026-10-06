package claude

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

func TestOAuthProfileOverridesStaleFileTier(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "file-access", "file-refresh", now.Add(24*time.Hour))
	// The fixture helper writes 20x; simulate an old credential snapshot.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("default_claude_max_20x"), []byte("default_claude_max_5x"), 1)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	usageCalls, profileCalls := 0, 0
	client, server := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer file-access" || r.Header.Get("anthropic-beta") != claudeOAuthBeta {
			t.Error("profile and usage must use the same bearer and OAuth beta header")
		}
		switch r.URL.Path {
		case "/usage":
			usageCalls++
			_, _ = w.Write([]byte(syntheticUsage))
		case "/profile":
			profileCalls++
			_, _ = w.Write([]byte(`{"organization":{"rate_limit_tier":"default_claude_max_20x","organization_type":"claude_max"},"email":"private@example.invalid","name":"private-name","id":"private-uuid"}`))
		default:
			t.Error("unexpected request path")
		}
	})
	client.profileURL = server.URL + "/profile"
	client.now = func() time.Time { return now }

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for range 2 {
		result, err := client.FetchFrom(context.Background(), OAuthCredentialFile)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, now)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Plan != model.Plan("MAX 20X") || result.subscriptionType != "claude_max" {
			t.Fatalf("profile tier was not applied: plan=%q type=%q", snapshot.Plan, result.subscriptionType)
		}
		for _, secret := range []string{"private@example.invalid", "private-name", "private-uuid"} {
			if strings.Contains(fmt.Sprint(result), secret) || strings.Contains(logs.String(), secret) {
				t.Fatal("unrelated profile identity field escaped")
			}
		}
	}
	if usageCalls != 2 || profileCalls != 1 {
		t.Fatalf("usage=%d profile=%d, want 2 and 1", usageCalls, profileCalls)
	}
}

func TestOAuthProfileFailureFallsBackAndWaitsSixHours(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "file-access", "file-refresh", now.Add(24*time.Hour))
	profileCalls := 0
	client, server := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/profile" {
			profileCalls++
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(syntheticUsage))
	})
	client.profileURL = server.URL + "/profile"
	client.now = func() time.Time { return now }
	for range 2 {
		result, err := client.FetchFrom(context.Background(), OAuthCredentialFile)
		if err != nil {
			t.Fatal(err)
		}
		if result.rateLimitTier != "default_claude_max_20x" || result.subscriptionType != "max" {
			t.Fatal("profile failure did not preserve file metadata")
		}
	}
	if profileCalls != 1 {
		t.Fatalf("profile calls=%d, want 1", profileCalls)
	}
	now = now.Add(6 * time.Hour)
	if _, err := client.FetchFrom(context.Background(), OAuthCredentialFile); err != nil {
		t.Fatal(err)
	}
	if profileCalls != 2 {
		t.Fatalf("profile calls after six hours=%d, want 2", profileCalls)
	}
}

func TestOAuthProfileIsSkippedWithoutSuccessfulUsage(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "file-access", "file-refresh", now.Add(24*time.Hour))
	profileCalls := 0
	client, server := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/profile" {
			profileCalls++
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	client.profileURL = server.URL + "/profile"
	client.now = func() time.Time { return now }
	if _, err := client.FetchFrom(context.Background(), OAuthCredentialFile); err == nil {
		t.Fatal("usage failure was lost")
	}
	if profileCalls != 0 {
		t.Fatalf("profile calls=%d, want 0", profileCalls)
	}
}

func TestOAuthProfileIsSkippedDuringRateLimitBackoff(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "file-access", "file-refresh", now.Add(24*time.Hour))
	usageCalls, profileCalls := 0, 0
	client, server := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/profile" {
			profileCalls++
			_, _ = w.Write([]byte(`{"organization":{"rate_limit_tier":"default_claude_max_20x"}}`))
			return
		}
		usageCalls++
		if usageCalls > 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(syntheticUsage))
	})
	client.profileURL = server.URL + "/profile"
	client.now = func() time.Time { return now }
	if _, err := client.FetchFrom(context.Background(), OAuthCredentialFile); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Hour)
	for range 2 {
		result, err := client.FetchFrom(context.Background(), OAuthCredentialFile)
		if err != nil || !result.cached {
			t.Fatal("rate limit did not use cached usage")
		}
	}
	if usageCalls != 2 || profileCalls != 1 {
		t.Fatalf("usage=%d profile=%d, want 2 and 1", usageCalls, profileCalls)
	}
}
