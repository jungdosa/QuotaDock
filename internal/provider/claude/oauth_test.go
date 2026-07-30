package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/security"
)

const syntheticUsage = `{
  "five_hour":{"utilization":12.5,"resets_at":"2030-01-01T01:00:00Z"},
  "seven_day":{"utilization":34.5,"resets_at":"2030-01-07T01:00:00Z"}
}`

type fakeOAuthFetcher struct {
	available bool
	result    oauthResult
	err       error
}

func (f fakeOAuthFetcher) Available() bool { return f.available }
func (f fakeOAuthFetcher) Fetch(context.Context) (oauthResult, error) {
	return f.result, f.err
}

func writeCredentials(t *testing.T, path, access, refresh string, expiresAt time.Time) {
	t.Helper()
	payload := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":      access,
			"refreshToken":     refresh,
			"expiresAt":        expiresAt.UnixMilli(),
			"scopes":           []string{"user:profile"},
			"rateLimitTier":    "default_claude_max_20x",
			"subscriptionType": "max",
		},
		"unrelated": map[string]any{"preserve": true},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testOAuthClient(t *testing.T, path string, handler http.HandlerFunc) (*OAuthClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := NewOAuthClient()
	client.credentialsPath = path
	client.usageURL = server.URL + "/usage"
	client.tokenURL = server.URL + "/token"
	client.getenv = func(string) string { return "" }
	client.httpClient = server.Client()
	client.httpClient.Timeout = oauthRequestTimeout
	client.allowURL = func(raw string) bool { return strings.HasPrefix(raw, server.URL+"/") }
	client.reportRefreshFailure = func() {}
	return client, server
}

func TestParseOAuthCredentialsMapsFields(t *testing.T) {
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	raw := []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"access-placeholder","refreshToken":"refresh-placeholder","expiresAt":%d,"scopes":["user:profile"],"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}}`, expires.UnixMilli()))
	credentials, err := parseOAuthCredentials(raw)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.accessToken != "access-placeholder" || credentials.refreshToken != "refresh-placeholder" || !credentials.expiresAt.Equal(expires) {
		t.Fatal("credential fields were not mapped")
	}
	if len(credentials.scopes) != 1 || credentials.subscriptionType != "max" || credentials.rateLimitTier != "default_claude_max_20x" {
		t.Fatal("credential metadata was not mapped")
	}
}

func TestNormalizeOAuthUsageFiveHourAndSevenDay(t *testing.T) {
	snapshot, err := NormalizeOAuthUsage(json.RawMessage(syntheticUsage), "", "pro", time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Provider != model.ProviderClaude || snapshot.Plan != "PRO" || len(snapshot.Limits) != 2 {
		t.Fatalf("unexpected normalized shape: provider=%s plan=%s limits=%d", snapshot.Provider, snapshot.Plan, len(snapshot.Limits))
	}
	if snapshot.Limits[0].ID != "five_hour" || snapshot.Limits[0].WindowMinutes != 300 || snapshot.Limits[1].ID != "seven_day" || snapshot.Limits[1].WindowMinutes != 10080 {
		t.Fatal("usage windows were not mapped")
	}
}

func TestNormalizeOAuthUsageFablePresentAndMissing(t *testing.T) {
	direct := json.RawMessage(`{"five_hour":{"utilization":1},"seven_day_fable":{"utilization":2}}`)
	snapshot, err := NormalizeOAuthUsage(direct, "", "pro", time.Now())
	if err != nil || len(snapshot.Limits) != 2 || snapshot.Limits[1].Label != "Fable" {
		t.Fatalf("direct Fable limit was not mapped: count=%d err=%v", len(snapshot.Limits), err)
	}
	scoped := json.RawMessage(`{"limits":[{"kind":"weekly_scoped","group":"weekly","percent":3,"scope":{"model":{"id":"claude-fable","display_name":"Fable"}}}]}`)
	snapshot, err = NormalizeOAuthUsage(scoped, "", "pro", time.Now())
	if err != nil || len(snapshot.Limits) != 1 || snapshot.Limits[0].Label != "Fable" {
		t.Fatalf("scoped Fable limit was not mapped: count=%d err=%v", len(snapshot.Limits), err)
	}
	missing := json.RawMessage(`{"five_hour":{"utilization":1},"limits":[{"kind":"weekly_scoped","group":"weekly","percent":3,"scope":{"model":{"display_name":"Other"}}}]}`)
	snapshot, err = NormalizeOAuthUsage(missing, "", "pro", time.Now())
	if err != nil || len(snapshot.Limits) != 1 {
		t.Fatalf("missing Fable created an empty row: count=%d err=%v", len(snapshot.Limits), err)
	}
}

func TestNormalizeOAuthUsageWeeklyAllOverridesLegacySevenDay(t *testing.T) {
	raw := json.RawMessage(`{"seven_day":{"utilization":100},"limits":[{"kind":"weekly_all","group":"weekly","percent":4,"resets_at":"2030-01-07T01:00:00Z"}]}`)
	snapshot, err := NormalizeOAuthUsage(raw, "", "max", time.Now())
	if err != nil || len(snapshot.Limits) != 1 {
		t.Fatalf("weekly limit normalization failed: count=%d err=%v", len(snapshot.Limits), err)
	}
	if snapshot.Limits[0].UsedPercent != 4 {
		t.Fatal("limits weekly_all did not override stale seven_day")
	}
}

func TestOAuthCredentialsExpireWithinFiveMinuteBuffer(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if !(oauthCredentials{expiresAt: now.Add(5 * time.Minute)}).expiresWithin(now, oauthExpiryBuffer) {
		t.Fatal("token at the five-minute boundary must be refreshed")
	}
	if (oauthCredentials{expiresAt: now.Add(5*time.Minute + time.Millisecond)}).expiresWithin(now, oauthExpiryBuffer) {
		t.Fatal("token outside the five-minute buffer was treated as expired")
	}
	if (oauthCredentials{}).expiresWithin(now, oauthExpiryBuffer) {
		t.Fatal("missing expiry must not force refresh")
	}
}

func TestOAuthRefreshRereadsDiskAndAvoidsDoubleRotation(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "disk-fresh-access", "disk-fresh-refresh", now.Add(time.Hour))
	var requests atomic.Int32
	client, _ := testOAuthClient(t, path, func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	})
	client.now = func() time.Time { return now }
	initial := oauthCredentials{accessToken: "stale-access", refreshToken: "stale-refresh", expiresAt: now}
	got, err := client.ensureFreshCredentials(context.Background(), initial, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.accessToken != "disk-fresh-access" || requests.Load() != 0 {
		t.Fatal("fresh disk credentials were not adopted before refresh")
	}
}

func TestOAuthRefreshPersistsCredentialsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".credentials.json")
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	writeCredentials(t, path, "old-access", "old-refresh", now)
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"scope":"user:profile user:inference"}`))
		case "/usage":
			_, _ = w.Write([]byte(syntheticUsage))
		}
	})
	client.now = func() time.Time { return now }
	if _, err := client.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		t.Fatal("persisted credentials are not valid JSON")
	}
	oauth := root["claudeAiOauth"].(map[string]any)
	if oauth["accessToken"] != "new-access" || oauth["refreshToken"] != "new-refresh" || root["unrelated"] == nil {
		t.Fatal("atomic persistence did not merge only refreshed fields")
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".credentials.json.quotadock-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary credential files remain: %v", temps)
	}
}

func TestOAuthRefreshFailureFallsBackToExistingCredentials(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "existing-access", "existing-refresh", now)
	var usageAuthorization string
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/usage":
			usageAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(syntheticUsage))
		}
	})
	client.now = func() time.Time { return now }
	warned := false
	client.reportRefreshFailure = func() { warned = true }
	result, err := client.Fetch(context.Background())
	if err != nil || len(result.raw) == 0 {
		t.Fatalf("existing credential fallback failed: %v", err)
	}
	if usageAuthorization != "Bearer existing-access" {
		t.Fatal("usage request did not fall back to the existing access credential")
	}
	if !warned {
		t.Fatal("refresh failure was hidden instead of emitting a safe warning")
	}
}

func TestOAuthUsageUnauthorizedRequiresReauthentication(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "access-placeholder", "refresh-placeholder", now.Add(time.Hour))
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	client.now = func() time.Time { return now }
	provider := newProvider(&fakeClient{}, client, "")
	_, err := provider.Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrNotLoggedIn || safe.Key != "error.not_logged_in" {
		t.Fatalf("401 did not become a safe reauthentication error: %v", err)
	}
}

func TestOAuthUsageRateLimitBackoffHonorsRetryAfter(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	current := now
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "access-placeholder", "refresh-placeholder", now.Add(time.Hour))
	var requests atomic.Int32
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, _ *http.Request) {
		requestNumber := requests.Add(1)
		if requestNumber == 2 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(syntheticUsage))
	})
	client.now = func() time.Time { return current }
	if _, err := client.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	limited, err := client.Fetch(context.Background())
	if err != nil || !limited.cached {
		t.Fatal("429 did not preserve the last successful usage")
	}
	current = current.Add(119 * time.Second)
	if cached, err := client.Fetch(context.Background()); err != nil || !cached.cached || requests.Load() != 2 {
		t.Fatal("request was sent during Retry-After backoff")
	}
	current = current.Add(2 * time.Second)
	if _, err := client.Fetch(context.Background()); err != nil || requests.Load() != 3 {
		t.Fatal("request did not resume after Retry-After backoff")
	}
	if retryAfterDuration("", now) != defaultRetryBackoff {
		t.Fatal("missing Retry-After did not use the five-minute default")
	}
}

func TestNormalizeClaudeOAuthPlanAllowlist(t *testing.T) {
	cases := []struct {
		tier         string
		subscription string
		want         model.Plan
	}{
		{"default_claude_max_20x", "", "MAX 20X"},
		{"default_claude_max_5x", "", "MAX 5X"},
		{"", "max", "MAX"},
		{"", "pro", "PRO"},
		{"", "team", "TEAM"},
		{"", "enterprise", "ENTERPRISE"},
		{"", "free", "FREE"},
		{"server-controlled-unknown", "", model.PlanUnknown},
	}
	for _, tc := range cases {
		if got := NormalizeClaudeOAuthPlan(tc.tier, tc.subscription); got != tc.want {
			t.Errorf("plan normalization = %q, want %q", got, tc.want)
		}
	}
}

func TestProviderMissingCredentialsFallsBackToAuthStatus(t *testing.T) {
	client := &fakeClient{
		version:   MinimumCLIVersion,
		auth:      json.RawMessage(`{"loggedIn":true,"subscriptionType":"max"}`),
		limitsErr: ErrRateLimitsUnavailable,
	}
	oauth := fakeOAuthFetcher{err: errOAuthCredentialsUnavailable}
	provider := newProvider(client, oauth, MinimumCLIVersion)
	snapshot, err := provider.Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrUsageUnavailable {
		t.Fatalf("missing credentials did not use auth-status fallback: %v", err)
	}
	if snapshot.Plan != "MAX" || len(snapshot.Limits) != 0 {
		t.Fatal("auth-status fallback did not retain the safe no-usage state")
	}
}

func TestOAuthSecretsNeverReachSnapshotLogOrError(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "private-access-value", "private-refresh-value", now.Add(time.Hour))
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/failure" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"access_token":"private-access-value","email":"private@example.invalid"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":6},"email":"private@example.invalid","organizationUuid":"private-org"}`))
	})
	client.now = func() time.Time { return now }
	result, err := client.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, now)
	if err != nil {
		t.Fatal(err)
	}
	public := fmt.Sprintf("snapshot=%+v error=%v", snapshot, err)
	for _, secret := range []string{"private-access-value", "private-refresh-value", "private@example.invalid", "private-org"} {
		if strings.Contains(public, secret) {
			t.Fatal("credential or account identity reached a public surface")
		}
	}

	client.lastSuccess = oauthResult{}
	client.usageURL = strings.TrimSuffix(client.usageURL, "/usage") + "/failure"
	_, failure := client.Fetch(context.Background())
	if failure == nil || strings.Contains(failure.Error(), "private") {
		t.Fatal("provider error was absent or included response or credential data")
	}
}

func TestOAuthRequestAllowlistAndRequiredHeaders(t *testing.T) {
	for _, allowed := range []string{claudeUsageURL, claudeTokenURL} {
		if !security.IsAllowedProviderRequestURL(allowed) {
			t.Fatal("audited Claude OAuth host was not allowlisted")
		}
	}
	for _, rejected := range []string{"https://anthropic.com/api/oauth/usage", "https://api.anthropic.com.evil.invalid/api/oauth/usage", "http://platform.claude.com/v1/oauth/token"} {
		if security.IsAllowedProviderRequestURL(rejected) {
			t.Fatal("unaudited Claude OAuth host was allowlisted")
		}
	}

	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "old-access", "old-refresh", now)
	var refreshSeen, usageSeen bool
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-beta") != claudeOAuthBeta || r.Header.Get("Accept") != "application/json" {
			t.Error("required OAuth headers are missing")
		}
		switch r.URL.Path {
		case "/token":
			refreshSeen = true
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["grant_type"] != "refresh_token" || body["client_id"] != claudeOAuthClientID {
				t.Error("refresh request body is invalid")
			}
			_, _ = w.Write([]byte(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":3600}`))
		case "/usage":
			usageSeen = true
			if r.Header.Get("Authorization") != "Bearer fresh-access" {
				t.Error("usage request did not use the refreshed bearer credential")
			}
			_, _ = w.Write([]byte(syntheticUsage))
		}
	})
	client.now = func() time.Time { return now }
	if _, err := client.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !refreshSeen || !usageSeen {
		t.Fatal("refresh and usage requests were not both observed")
	}
}
