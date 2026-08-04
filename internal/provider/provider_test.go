package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

type fakeProvider struct {
	id  model.ProviderID
	err error
}

func (f fakeProvider) Inspect(context.Context) model.ConnectionState {
	return model.ConnectionState{Status: model.StatusConnected}
}
func (f fakeProvider) Refresh(context.Context) (model.UsageSnapshot, error) {
	return model.UsageSnapshot{Provider: f.id}, f.err
}
func (f fakeProvider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return f.Refresh(ctx)
}
func (f fakeProvider) Close() error { return nil }
func TestProviderSuccessFailureIsolation(t *testing.T) {
	coordinator := Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderClaude: fakeProvider{id: model.ProviderClaude, err: errors.New("safe failure")}, model.ProviderCodex: fakeProvider{id: model.ProviderCodex}, model.ProviderAntigravity: fakeProvider{id: model.ProviderAntigravity}}}
	outcomes := coordinator.RefreshAll(context.Background())
	if outcomes[model.ProviderClaude].Err == nil {
		t.Fatal("failed provider did not fail")
	}
	for _, id := range []model.ProviderID{model.ProviderCodex, model.ProviderAntigravity} {
		if outcomes[id].Err != nil || outcomes[id].Snapshot.Provider != id {
			t.Errorf("provider %s was affected: %+v", id, outcomes[id])
		}
	}
}
func TestVersionComparison(t *testing.T) {
	if !VersionAtLeast("v1.2.3", "1.2.0") || VersionAtLeast("0.9.9", "1.0.0") {
		t.Fatal("semantic version comparison failed")
	}
}

func TestSchedulerPreventsDuplicateTimers(t *testing.T) {
	var scheduler Scheduler
	var calls atomic.Int32
	if !scheduler.Start(context.Background(), 5*time.Millisecond, func(context.Context) { calls.Add(1) }) {
		t.Fatal("first timer did not start")
	}
	if scheduler.Start(context.Background(), 5*time.Millisecond, func(context.Context) { calls.Add(100) }) {
		t.Fatal("duplicate timer started")
	}
	time.Sleep(15 * time.Millisecond)
	scheduler.Stop()
	if calls.Load() == 0 || calls.Load() >= 100 {
		t.Fatalf("timer calls = %d", calls.Load())
	}
}

func TestProviderRefreshLogHasSafeCodeAndDuration(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previousLogger)

	coordinator := Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderCodex: fakeProvider{id: model.ProviderCodex, err: model.SafeError{Code: model.ErrTimeout}},
	}}
	coordinator.RefreshAll(context.Background())

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatal(err)
	}
	if record["msg"] != "provider.refresh" || record["provider"] != "codex" || record["ok"] != false {
		t.Fatalf("refresh event=%#v", record)
	}
	if record["err"] != "timeout" {
		t.Fatalf("refresh error=%#v", record)
	}
	if _, ok := record["ms"]; !ok {
		t.Fatalf("refresh duration missing: %#v", record)
	}
}
