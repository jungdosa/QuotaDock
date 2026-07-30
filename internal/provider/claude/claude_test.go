package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jungdosa/QuotaDock/internal/model"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeClient struct {
	version    string
	versionErr error
	path       string
	pathErr    error
	auth       json.RawMessage
	authErr    error
	limits     json.RawMessage
	limitsErr  error
}

func (f *fakeClient) Version(context.Context) (string, error)             { return f.version, f.versionErr }
func (f *fakeClient) ExecutablePath() (string, error)                     { return f.path, f.pathErr }
func (f *fakeClient) AuthStatus(context.Context) (json.RawMessage, error) { return f.auth, f.authErr }
func (f *fakeClient) RateLimits(context.Context) (json.RawMessage, error) {
	return f.limits, f.limitsErr
}
func (f *fakeClient) Close() error { return nil }

func TestCLIInstallationLoginAndVersionStates(t *testing.T) {
	cases := []struct {
		name   string
		client *fakeClient
		want   model.ConnectionStatus
	}{{"CLI missing", &fakeClient{versionErr: shared.ErrNotInstalled}, model.StatusUnavailable}, {"installed logged out", &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":false}`)}, model.StatusLoggedOut}, {"normal login", &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`)}, model.StatusConnected}, {"CLI too old", &fakeClient{version: "1.0.0"}, model.StatusOutdated}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := newProvider(tc.client, nil, "2.0.0").Inspect(context.Background())
			if got.Status != tc.want {
				t.Fatalf("status = %s, want %s", got.Status, tc.want)
			}
		})
	}
}

func TestInspectExposesSafeCLIDiagnostics(t *testing.T) {
	client := &fakeClient{version: "2.1.215", path: `/opt/claude/bin/claude`, auth: json.RawMessage(`{"loggedIn":true}`)}
	state := newProvider(client, nil, "2.0.0").Inspect(context.Background())
	if state.CLIPath != `/opt/claude/bin/claude` || state.CLIVersion != "2.1.215" {
		t.Fatalf("CLI diagnostics=%+v", state)
	}
}

func TestNormalUsageResponseAndMissingFields(t *testing.T) {
	auth := json.RawMessage(`{"loggedIn":true,"email":"person@example.invalid","orgId":"org-masked","subscriptionType":"max_5x"}`)
	limits := fixture(t, "claude-rate-limits.json")
	client := &fakeClient{version: "2.1.215", auth: auth, limits: limits}
	provider := newProvider(client, nil, "2.0.0")
	provider.now = func() time.Time { return time.Unix(100, 0) }
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan != "MAX 5X" || len(snapshot.Limits) != 3 || snapshot.Limits[0].UsedPercent != 25 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if strings.Contains(fmt.Sprintf("%+v", snapshot), "person@example.invalid") || strings.Contains(fmt.Sprintf("%+v", snapshot), "org-masked") {
		t.Fatal("Claude account identity reached snapshot")
	}
	partial := json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":12}}}`)
	got, err := NormalizeRateLimits(partial, "pro", time.Now())
	if err != nil || len(got.Limits) != 1 || !got.Limits[0].ResetsAt.IsZero() {
		t.Fatalf("partial response = %+v, %v", got, err)
	}
}

func TestClaudeFableLimitPresentAndMissing(t *testing.T) {
	withFable, err := NormalizeRateLimits(fixture(t, "claude-rate-limits.json"), "pro", time.Now())
	if err != nil || len(withFable.Limits) != 3 {
		t.Fatalf("Fable present = %+v, %v", withFable, err)
	}
	without, err := NormalizeRateLimits(json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":1},"seven_day":{"used_percentage":2}}}`), "pro", time.Now())
	if err != nil || len(without.Limits) != 2 {
		t.Fatalf("Fable missing created row: %+v, %v", without, err)
	}
}

func TestClaudeFailureIsSafe(t *testing.T) {
	client := &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true}`), limitsErr: errors.New("Authorization: Bearer should-not-leak")}
	_, err := newProvider(client, nil, "2.0.0").Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Key != "error.unavailable" {
		t.Fatalf("unsafe error = %v", err)
	}
}

func TestLoggedInWithoutRateLimitsReturnsUsageUnavailableAndPlan(t *testing.T) {
	client := &fakeClient{
		version:   "2.1.215",
		auth:      json.RawMessage(`{"loggedIn":true,"subscriptionType":"max"}`),
		limitsErr: ErrRateLimitsUnavailable,
	}
	snapshot, err := newProvider(client, nil, MinimumCLIVersion).Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrUsageUnavailable || safe.Key != "error.usage_unavailable" {
		t.Fatalf("missing usage error = %v", err)
	}
	if snapshot.Plan != "MAX" || snapshot.Provider != model.ProviderClaude || len(snapshot.Limits) != 0 {
		t.Fatalf("logged-in unavailable snapshot = %+v", snapshot)
	}
}
func fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
