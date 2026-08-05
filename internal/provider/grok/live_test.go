package grok

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveGrokBilling talks to the real endpoint with the real CLI token. It is
// opt-in because it needs a logged-in Grok CLI and network access, and because
// a live call is the only way to catch the failure mode fixtures cannot model:
// the endpoint changing shape under a heuristic parser.
//
//	QUOTADOCK_LIVE_GROK=1 go test ./internal/provider/grok -run TestLiveGrokBilling -v
//
// It prints the window but never the token.
func TestLiveGrokBilling(t *testing.T) {
	if os.Getenv("QUOTADOCK_LIVE_GROK") != "1" {
		t.Skip("set QUOTADOCK_LIVE_GROK=1 to run the live Grok billing check")
	}
	provider := New(nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	snapshot, err := provider.Refresh(ctx)
	if err != nil {
		t.Fatalf("live refresh failed: %v", err)
	}
	if len(snapshot.Limits) == 0 {
		t.Fatal("live response produced no lane: the window failed range validation")
	}
	limit := snapshot.Limits[0]
	t.Logf("window=%d분  라벨=%q  리셋=%s (지금부터 %s 뒤)",
		limit.WindowMinutes, limit.Label,
		limit.ResetsAt.Local().Format("2006-01-02 15:04:05"),
		time.Until(limit.ResetsAt).Round(time.Minute))

	if !limit.ResetsAt.After(time.Now()) {
		t.Fatalf("reset time is in the past: %s", limit.ResetsAt)
	}
}
