package claude

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

func TestLiveClaudeOAuthUsageSchema(t *testing.T) {
	if os.Getenv("QUOTADOCK_LIVE_CLAUDE") != "1" {
		t.Skip("set QUOTADOCK_LIVE_CLAUDE=1 to validate the Claude OAuth usage schema")
	}
	provider := New(NewCLIClient(nil), MinimumCLIVersion)
	defer provider.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	snapshot, err := provider.Refresh(ctx)
	if err != nil {
		t.Fatal("Claude OAuth usage schema validation failed")
	}
	if snapshot.Provider != model.ProviderClaude {
		t.Fatal("Claude OAuth response did not normalize to the Claude provider")
	}
	foundSession, foundWeekly := false, false
	for _, limit := range snapshot.Limits {
		switch limit.ID {
		case "five_hour":
			foundSession = limit.WindowMinutes == 300
		case "seven_day":
			foundWeekly = limit.WindowMinutes == 10080
		}
	}
	if !foundSession || !foundWeekly {
		t.Fatal("Claude OAuth response is missing the required usage window schema")
	}
}
