package codex

import (
	"context"
	"encoding/json"
	"github.com/jungdosa/QuotaDock/internal/model"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveAppServerInitializeSchema(t *testing.T) {
	if os.Getenv("QUOTADOCK_LIVE_CODEX") != "1" {
		t.Skip("set QUOTADOCK_LIVE_CODEX=1 to test the installed Codex app-server")
	}
	transport := NewAppServerTransport(nil)
	defer transport.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	version, err := transport.Version(ctx)
	if err != nil || !versionPatternAtLeast(version, MinimumCLIVersion) {
		t.Fatalf("installed Codex version is unavailable or too old")
	}
	initialized, err := transport.Request(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "QuotaDock schema check", "version": "0.4.0"}})
	if err != nil || !json.Valid(initialized) {
		t.Fatalf("initialize schema check failed")
	}
	if err := transport.Notify(ctx, "initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized notification failed")
	}
	accountRaw, err := transport.Request(ctx, "account/read", map[string]any{})
	if err != nil {
		t.Fatalf("account/read schema check failed")
	}
	var account struct {
		Account            json.RawMessage `json:"account"`
		RequiresOpenAIAuth *bool           `json:"requiresOpenaiAuth"`
	}
	if json.Unmarshal(accountRaw, &account) != nil || account.RequiresOpenAIAuth == nil {
		t.Fatalf("account/read schema mismatch")
	}
	if len(account.Account) == 0 || string(account.Account) == "null" {
		t.Skip("installed Codex CLI is not logged in")
	}
	rateRaw, err := transport.Request(ctx, "account/rateLimits/read", map[string]any{})
	if err != nil {
		t.Fatalf("account/rateLimits/read schema check failed")
	}
	var rates struct {
		RateLimits json.RawMessage `json:"rateLimits"`
	}
	if json.Unmarshal(rateRaw, &rates) != nil || len(rates.RateLimits) == 0 {
		t.Fatalf("account/rateLimits/read schema mismatch")
	}
}

func TestLiveCodexWindowLabelsUseActualDurations(t *testing.T) {
	if os.Getenv("QUOTADOCK_LIVE_CODEX") != "1" {
		t.Skip("set QUOTADOCK_LIVE_CODEX=1 to test installed Codex window durations")
	}
	provider := New(NewAppServerTransport(nil), MinimumCLIVersion)
	defer provider.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snapshot, err := provider.Refresh(ctx)
	if err != nil {
		t.Fatal("installed Codex usage snapshot was unavailable")
	}
	if snapshot.Plan == model.PlanUnknown {
		t.Fatal("installed Codex account plan did not pass the allowlist")
	}
	if len(snapshot.Limits) == 0 {
		t.Fatal("installed Codex account returned no usage windows")
	}
	previous := 0
	for _, limit := range snapshot.Limits {
		t.Logf("Codex window label=%q windowMinutes=%d usedPercent=%.0f resetsAt=%s", limit.Label, limit.WindowMinutes, limit.UsedPercent, limit.ResetsAt.Format(time.RFC3339))
		if limit.WindowMinutes <= 0 {
			t.Fatal("installed Codex usage window has no machine-readable duration")
		}
		if previous > limit.WindowMinutes {
			t.Fatal("installed Codex usage windows are not sorted by duration")
		}
		if limit.Label != model.UsageWindowLabel(limit.WindowMinutes) {
			t.Fatal("installed Codex usage label is not derived from its duration")
		}
		if strings.Contains(limit.Label, limit.ID) || strings.Contains(strings.ToLower(limit.Label), "codex_") {
			t.Fatal("installed Codex raw limit ID leaked into a display label")
		}
		previous = limit.WindowMinutes
	}
}

func versionPatternAtLeast(version, minimum string) bool {
	return version != "" && minimum != ""
}
