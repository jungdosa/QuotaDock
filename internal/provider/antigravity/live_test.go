package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

func TestLiveAntigravityQuotaSchema(t *testing.T) {
	if os.Getenv("QUOTADOCK_LIVE_ANTIGRAVITY") != "1" {
		t.Skip("set QUOTADOCK_LIVE_ANTIGRAVITY=1 to test a running Antigravity IDE")
	}
	client := NewLocalClient()
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	running, loggedIn, err := client.Status(ctx)
	if err != nil || !running {
		t.Fatalf("verified Antigravity loopback endpoint was not available")
	}
	if !loggedIn {
		t.Skip("running Antigravity IDE is not logged in")
	}
	client.mu.Lock()
	candidate := client.current
	client.mu.Unlock()
	if candidate != nil {
		statusRaw, statusErr := requestLocal(ctx, *candidate, GetUserStatusEndpoint)
		if statusErr == nil {
			paths, schemaErr := maskedSchemaPaths(statusRaw)
			if schemaErr == nil {
				t.Logf("masked status schema only: %v", paths)
			}
		}
	}
	raw, err := client.RetrieveUserQuotaSummary(ctx)
	if err != nil {
		t.Fatalf("quota schema check failed")
	}
	snapshot, err := NormalizeQuota(raw, time.Now())
	if err != nil || snapshot.Plan == "UNKNOWN" || len(snapshot.Limits) == 0 {
		paths, schemaErr := maskedSchemaPaths(raw)
		if schemaErr == nil {
			t.Logf("masked response schema only: %v", paths)
		}
		t.Fatalf("quota normalization schema mismatch")
	}
	seenFiveHour, seenWeekly := false, false
	for _, limit := range snapshot.Limits {
		seenFiveHour = seenFiveHour || limit.WindowMinutes == 300
		seenWeekly = seenWeekly || limit.WindowMinutes == 10080
	}
	if !seenFiveHour || !seenWeekly {
		t.Fatalf("quota windows did not include both 5-hour and weekly buckets")
	}
	t.Logf("verified masked quota schema: plan=string, quotaGroups=%d", len(snapshot.Limits))
}

func maskedSchemaPaths(raw json.RawMessage) ([]string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var paths []string
	var walk func(string, any)
	walk = func(path string, current any) {
		switch typed := current.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(path+"."+key, typed[key])
			}
		case []any:
			paths = append(paths, path+":array")
			if len(typed) > 0 {
				walk(path+"[]", typed[0])
			}
		default:
			paths = append(paths, path+":"+fmt.Sprintf("%T", current))
		}
	}
	walk("$", value)
	return paths, nil
}
