package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
)

type fakeTransport struct {
	version   string
	path      string
	responses map[string]json.RawMessage
	fail      map[string]error
	calls     []string
	handler   func(json.RawMessage)
	session   int
	active    bool
	discarded int
}

func (f *fakeTransport) Version(context.Context) (string, error) { return f.version, nil }
func (f *fakeTransport) ExecutablePath() (string, error)         { return f.path, nil }
func (f *fakeTransport) Request(_ context.Context, method string, _ any) (json.RawMessage, error) {
	f.ensureSession()
	f.calls = append(f.calls, method)
	if err := f.fail[method]; err != nil {
		return nil, err
	}
	return f.responses[method], nil
}
func (f *fakeTransport) Notify(_ context.Context, method string, _ any) error {
	f.ensureSession()
	f.calls = append(f.calls, method)
	return f.fail[method]
}
func (f *fakeTransport) SetRateLimitsUpdatedHandler(handler func(json.RawMessage)) {
	f.handler = handler
}
func (f *fakeTransport) ensureSession() {
	if !f.active {
		f.session++
		f.active = true
	}
}
func (f *fakeTransport) Invalidate() {
	if f.active {
		f.active = false
		f.discarded++
	}
}
func (f *fakeTransport) Close() error {
	f.active = false
	return nil
}
func workingTransport(t *testing.T) *fakeTransport {
	return &fakeTransport{version: "0.145.0", path: `/opt/codex/bin/codex`, responses: map[string]json.RawMessage{"initialize": json.RawMessage(`{"capabilities":{}}`), "account/read": json.RawMessage(`{"account":{"loggedIn":true,"planType":"plus"}}`), "account/rateLimits/read": fixture(t, "codex-rate-limits.json")}, fail: map[string]error{}}
}

func TestInspectExposesSafeCLIDiagnostics(t *testing.T) {
	state := New(workingTransport(t), "0.100.0").Inspect(context.Background())
	if state.CLIPath != `/opt/codex/bin/codex` || state.CLIVersion != "0.145.0" {
		t.Fatalf("CLI diagnostics=%+v", state)
	}
}

func TestInitializationSuccessAndFailure(t *testing.T) {
	transport := workingTransport(t)
	provider := New(transport, "0.100.0")
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"initialize", "initialized", "account/read", "account/rateLimits/read"}
	if !reflect.DeepEqual(transport.calls, want) || snapshot.Plan != "PLUS" {
		t.Fatalf("calls=%v snapshot=%+v", transport.calls, snapshot)
	}
	failed := workingTransport(t)
	failed.fail["initialize"] = errors.New("raw server failure")
	_, err = New(failed, "0.100.0").Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrInitialization {
		t.Fatalf("failure = %v", err)
	}
}

func TestNormalUsagePartialFieldsAndMultipleLimitIDs(t *testing.T) {
	provider := New(workingTransport(t), "0.100.0")
	provider.now = func() time.Time { return time.Unix(100, 0) }
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Limits) != 4 || snapshot.Credits == nil || snapshot.Credits.Balance != 12.5 {
		t.Fatalf("normal/multiple limits = %+v", snapshot)
	}
	partial := workingTransport(t)
	partial.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":9}},"rateLimitsByLimitId":{"only":{"usedPercent":2}}}`)
	got, err := New(partial, "0.100.0").Refresh(context.Background())
	if err != nil || len(got.Limits) != 2 || got.Limits[0].WindowMinutes != 0 {
		t.Fatalf("partial response = %+v, %v", got, err)
	}
}

func TestCodexCreditsRemainBalanceAndUnlimitedOnly(t *testing.T) {
	transport := workingTransport(t)
	transport.responses["account/rateLimits/read"] = json.RawMessage(`{"credits":{"balance":"12.5","unlimited":true}}`)
	snapshot, err := New(transport, "0.100.0").Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Credits == nil || snapshot.Credits.Balance != 12.5 || !snapshot.Credits.Unlimited || snapshot.Credits.Spend != nil {
		t.Fatalf("Codex credits changed shape: %+v", snapshot.Credits)
	}
}

func TestSparseRateLimitEventMerge(t *testing.T) {
	provider := New(workingTransport(t), "0.100.0")
	before, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var oldReset time.Time
	for _, limit := range before.Limits {
		if limit.ID == "primary" {
			oldReset = limit.ResetsAt
			break
		}
	}
	if oldReset.IsZero() {
		t.Fatal("primary limit is missing before sparse update")
	}
	if err := provider.ApplyRateLimitsUpdated(json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":88}}}`)); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	after := snapshotFrom(provider.plan, provider.limits, time.Now())
	provider.mu.Unlock()
	var updated *model.UsageLimit
	for i := range after.Limits {
		if after.Limits[i].ID == "primary" {
			updated = &after.Limits[i]
			break
		}
	}
	if updated == nil || updated.UsedPercent != 88 || !updated.ResetsAt.Equal(oldReset) || len(after.Limits) != len(before.Limits) {
		t.Fatalf("sparse merge = before %+v after %+v", before, after)
	}
}
func TestActualAppServerAccountAndMultipleLimitIDs(t *testing.T) {
	transport := workingTransport(t)
	transport.responses["account/read"] = json.RawMessage(`{"account":{"type":"chatgpt","email":"person@example.invalid","planType":"plus"},"requiresOpenaiAuth":true}`)
	transport.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{"planType":"plus","primary":{"usedPercent":10,"windowDurationMins":300}},"rateLimitsByLimitId":{"codex":{"limitId":"codex","planType":"plus","primary":{"usedPercent":20,"windowDurationMins":300},"secondary":{"usedPercent":30,"windowDurationMins":10080},"credits":{"balance":"4.5","unlimited":false}},"review":{"limitId":"review","primary":{"usedPercent":40,"windowDurationMins":1440}}}}`)
	provider := New(transport, "0.100.0")
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan != "PLUS" || len(snapshot.Limits) != 3 || snapshot.Credits == nil || snapshot.Credits.Balance != 4.5 {
		t.Fatalf("actual app-server snapshot = %+v", snapshot)
	}
	if strings.Contains(fmt.Sprintf("%+v", snapshot), "person@example.invalid") {
		t.Fatal("account email reached snapshot")
	}
}

func TestWindowLabelsNeverExposeLimitIDsAndAreSorted(t *testing.T) {
	transport := workingTransport(t)
	transport.responses["account/read"] = json.RawMessage(`{"account":{"type":"chatgpt","planType":"go"},"requiresOpenaiAuth":true}`)
	transport.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{},"rateLimitsByLimitId":{"codex_bengalfo":{"limitId":"codex_bengalfo","primary":{"usedPercent":54,"windowDurationMins":10080},"secondary":{"usedPercent":12,"windowDurationMins":300}}}}`)
	snapshot, err := New(transport, "0.100.0").Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan != "GO" {
		t.Fatalf("normalized plan = %q, want GO", snapshot.Plan)
	}
	if len(snapshot.Limits) != 2 || snapshot.Limits[0].WindowMinutes != 300 || snapshot.Limits[0].Label != "Session" || snapshot.Limits[1].WindowMinutes != 10080 || snapshot.Limits[1].Label != "Weekly" {
		t.Fatalf("window-derived limits = %+v", snapshot.Limits)
	}
	for _, limit := range snapshot.Limits {
		if strings.Contains(limit.Label, "codex_bengalfo") {
			t.Fatalf("raw limit ID leaked into label %q", limit.Label)
		}
		if !strings.Contains(limit.ID, "codex_bengalfo") {
			t.Fatalf("raw limit ID was not retained in model.UsageLimit.ID: %q", limit.ID)
		}
	}
}

func TestAppServerSparseEventCallbackMergesLastFullResponse(t *testing.T) {
	transport := workingTransport(t)
	transport.responses["account/read"] = json.RawMessage(`{"account":{"type":"chatgpt","planType":"plus"}}`)
	transport.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":10,"windowDurationMins":300,"resetsAt":1784912400}},"rateLimitsByLimitId":{"codex":{"limitId":"codex","primary":{"usedPercent":10,"windowDurationMins":300,"resetsAt":1784912400}}}}`)
	provider := New(transport, "0.100.0")
	before, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if transport.handler == nil {
		t.Fatal("rate-limit event handler was not registered")
	}
	transport.handler(json.RawMessage(`{"rateLimits":{"limitId":"codex","primary":{"usedPercent":88}}}`))
	provider.mu.Lock()
	after := snapshotFrom(provider.plan, provider.limits, time.Now())
	provider.mu.Unlock()
	if len(after.Limits) != 1 || after.Limits[0].UsedPercent != 88 || !after.Limits[0].ResetsAt.Equal(before.Limits[0].ResetsAt) {
		t.Fatalf("event merge = before %+v after %+v", before, after)
	}
}

func TestAPIKeyAccountReturnsSafeNoSubscriptionLimitGuidance(t *testing.T) {
	transport := workingTransport(t)
	transport.responses["account/read"] = json.RawMessage(`{"account":{"type":"apiKey"},"requiresOpenaiAuth":true}`)
	_, err := New(transport, "0.100.0").Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrSubscriptionUnavailable || safe.Key != "error.codex_api_key_no_limits" {
		t.Fatalf("API key error = %v", err)
	}
	if transport.discarded != 0 || !transport.active {
		t.Fatalf("API key session discarded=%d active=%v", transport.discarded, transport.active)
	}
}

func TestRateLimitsProcessExitInvalidatesSession(t *testing.T) {
	transport := workingTransport(t)
	transport.fail["account/rateLimits/read"] = process.ErrProcessExited

	_, err := New(transport, "0.100.0").Refresh(context.Background())

	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrProcessExited || transport.discarded != 1 {
		t.Fatalf("process exit error=%v discarded=%d", err, transport.discarded)
	}
}

func TestUnclassifiedTransportErrorAndHandshakeTimeoutInvalidateSession(t *testing.T) {
	transport := workingTransport(t)
	transport.fail["account/rateLimits/read"] = errors.New("broken JSONL transport")

	_, err := New(transport, "0.100.0").Refresh(context.Background())

	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrInitialization || transport.discarded != 1 {
		t.Fatalf("unclassified error=%v discarded=%d", err, transport.discarded)
	}

	handshake := workingTransport(t)
	handshake.fail["initialize"] = context.DeadlineExceeded
	_, err = New(handshake, "0.100.0").Refresh(context.Background())
	if !errors.As(err, &safe) || safe.Code != model.ErrTimeout || handshake.discarded != 1 {
		t.Fatalf("handshake timeout=%v discarded=%d", err, handshake.discarded)
	}
}

func TestPostHandshakeTimeoutKeepsSession(t *testing.T) {
	transport := workingTransport(t)
	transport.fail["account/rateLimits/read"] = context.DeadlineExceeded

	_, err := New(transport, "0.100.0").Refresh(context.Background())

	var safe model.SafeError
	if !errors.As(err, &safe) || safe.Code != model.ErrQuotaExhausted || transport.discarded != 0 || !transport.active {
		t.Fatalf("post-handshake timeout=%v discarded=%d active=%v", err, transport.discarded, transport.active)
	}
}

func TestResponseContentErrorsKeepSession(t *testing.T) {
	tests := []struct {
		name       string
		account    json.RawMessage
		accountErr error
		code       model.ErrorCode
	}{
		{name: "not logged in", account: json.RawMessage(`{"account":null}`), code: model.ErrNotLoggedIn},
		{name: "invalid response", account: json.RawMessage(`not-json`), code: model.ErrInvalidResponse},
		{name: "invalid RPC response", accountErr: process.ErrRPCResponse, code: model.ErrInitialization},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := workingTransport(t)
			transport.responses["account/read"] = test.account
			transport.fail["account/read"] = test.accountErr

			_, err := New(transport, "0.100.0").Refresh(context.Background())

			var safe model.SafeError
			if !errors.As(err, &safe) || safe.Code != test.code || transport.discarded != 0 || !transport.active {
				t.Fatalf("response error=%v discarded=%d active=%v", err, transport.discarded, transport.active)
			}
		})
	}
}

func TestRefreshAfterInvalidationRequestsNewSession(t *testing.T) {
	transport := workingTransport(t)
	transport.fail["account/rateLimits/read"] = process.ErrProcessExited
	provider := New(transport, "0.100.0")

	if _, err := provider.Refresh(context.Background()); err == nil {
		t.Fatal("first refresh unexpectedly succeeded")
	}
	delete(transport.fail, "account/rateLimits/read")
	if _, err := provider.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transport.session != 2 {
		t.Fatalf("session count=%d, want 2", transport.session)
	}
}

func TestAppServerTransportInvalidateIsIdempotent(t *testing.T) {
	transport := NewAppServerTransport(nil)

	transport.Invalidate()
	transport.Invalidate()
}

func fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
