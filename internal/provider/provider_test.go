package provider

import (
	"context"
	"errors"
	"github.com/jungdosa/QuotaDock/internal/model"
	"sync/atomic"
	"testing"
	"time"
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
