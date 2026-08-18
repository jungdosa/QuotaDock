package claude

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/model"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

const webUsageJSON = `{"five_hour":{"utilization":12,"resets_at":"2030-01-02T03:04:05Z"},
	"seven_day":{"utilization":34,"resets_at":"2030-01-08T00:00:00Z"}}`

// A working CLI must keep serving the lane; the browser session is a
// fallback, never a replacement.
func TestWebAuthDoesNotDisplaceAWorkingCLI(t *testing.T) {
	client := &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`),
		limits: json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":5,"resets_at":"2030-01-02T00:00:00Z"}}}`)}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// The CLI figure (5%) must win over the web figure (12%).
	if len(snapshot.Limits) == 0 || snapshot.Limits[0].UsedPercent != 5 {
		t.Fatalf("web auth displaced the CLI: %+v", snapshot.Limits)
	}
}

// With no CLI at all, a signed-in browser session serves the lane.
func TestWebAuthServesTheLaneWhenTheCLIIsMissing(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(snapshot.Limits) != 2 || snapshot.Limits[0].UsedPercent != 12 {
		t.Fatalf("web usage was not used: %+v", snapshot.Limits)
	}
	if state := provider.Inspect(context.Background()); state.Status == model.StatusConnected && state.Source == model.SourceWebSignIn {
		t.Log("inspect reports the web source")
	}
}

// A failing or unavailable web session must leave the CLI error standing so
// the lane keeps telling the user to install or sign in.
func TestWebAuthFailureKeepsTheCLIError(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	for name, fetcher := range map[string]oauthUsageFetcher{
		"unavailable": fakeOAuthFetcher{available: false},
		"fetch fails": fakeOAuthFetcher{available: true, err: errors.New("no session")},
		"bad payload": fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage("not json")}},
	} {
		t.Run(name, func(t *testing.T) {
			provider := newProvider(client, nil, "2.0.0")
			provider.SetWebAuth(fetcher)
			if _, err := provider.Refresh(context.Background()); err == nil {
				t.Fatal("a failing web session hid the CLI error")
			}
		})
	}
}

func TestChatOrganizationPicksTheAppOrgAndRejectsJunk(t *testing.T) {
	raw := `[{"uuid":"api-only","capabilities":["api"]},{"uuid":"app","capabilities":["chat","claude_max"],"rate_limit_tier":"tier_x","billing_type":"max"}]`
	org, ok := chatOrganization(raw)
	if !ok || org.UUID != "app" || org.RateLimitTier != "tier_x" || org.BillingType != "max" {
		t.Fatalf("organization = %+v ok=%t", org, ok)
	}
	for _, bad := range []string{"", "<html>signed out</html>", "[]", `[{"capabilities":["chat"]}]`, `[{"uuid":"x","capabilities":["api"]}]`} {
		if _, ok := chatOrganization(bad); ok {
			t.Fatalf("junk accepted: %q", bad)
		}
	}
}

// Refresh succeeding while Inspect still reported an error left the row
// showing a failure the user had already worked around by signing in.
func TestInspectReportsTheWebSourceWhenTheCLICannotServe(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	state := provider.Inspect(context.Background())
	if state.Status != model.StatusConnected || state.Source != model.SourceWebSignIn {
		t.Fatalf("inspect = %+v, want connected via the web sign-in", state)
	}
}

// A working CLI keeps owning the row, so the source stays empty.
func TestInspectPrefersTheCLIOverTheWebSession(t *testing.T) {
	client := &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`)}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	state := provider.Inspect(context.Background())
	if state.Status != model.StatusConnected || state.Source == model.SourceWebSignIn {
		t.Fatalf("inspect = %+v, want the CLI to own the row", state)
	}
}

// Without a usable browser session the CLI error must survive untouched.
func TestInspectKeepsTheCLIErrorWithoutASession(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: false})
	if state := provider.Inspect(context.Background()); state.Status == model.StatusConnected {
		t.Fatalf("inspect = %+v, want the CLI error preserved", state)
	}
}
