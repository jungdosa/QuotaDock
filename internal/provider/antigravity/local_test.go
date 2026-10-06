package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
)

func TestLocalRequestBodies(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint, body string
	}{
		{"quota", RetrieveQuotaEndpoint, `{"forceRefresh":true}`},
		{"status", GetUserStatusEndpoint, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.endpoint {
					t.Error("unexpected request method or endpoint")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != tc.body {
					t.Error("request body does not match the endpoint contract")
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			candidate := endpointCandidate{
				port:  uint16(server.Listener.Addr().(*net.TCPAddr).Port),
				token: "test-token", executable: "language_server_windows_x64.exe", verified: true,
			}
			if _, err := requestLocal(context.Background(), candidate, tc.endpoint); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func fakeCLILocalClient(t *testing.T, cli *cliClient) (*LocalClient, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var discoveries, requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case GetUserStatusEndpoint:
			_, _ = w.Write([]byte(`{"userStatus":{"userTier":{"name":"AI PRO"}}}`))
		case RetrieveQuotaEndpoint:
			_, _ = w.Write([]byte(`{"response":{"groups":[{"displayName":"Gemini Models","buckets":[{"remainingFraction":0.75,"window":"168h"}]}]}}`))
		default:
			t.Error("unexpected IDE endpoint")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	candidate := endpointCandidate{
		port: uint16(server.Listener.Addr().(*net.TCPAddr).Port), token: "test-token",
		executable: "language_server_windows_x64.exe", verified: true,
	}
	return &LocalClient{cli: cli, discover: func() ([]endpointCandidate, error) {
		discoveries.Add(1)
		return []endpointCandidate{candidate}, nil
	}}, &discoveries, &requests
}

func TestCLIPriorityLoginFallbackAndRecovery(t *testing.T) {
	for _, entry := range []string{"status", "quota"} {
		t.Run(entry, func(t *testing.T) {
			cli, runner, now := fakeCLI(t)
			success := runner.raw
			client, discoveries, requests := fakeCLILocalClient(t, cli)
			check := func(source string, rows int) {
				t.Helper()
				if entry == "status" {
					running, loggedIn, err := client.Status(context.Background())
					if err != nil || !running || !loggedIn || client.Source() != source {
						t.Fatal("unexpected status or selected source")
					}
				}
				raw, err := client.RetrieveUserQuotaSummary(context.Background())
				if err != nil || client.Source() != source {
					t.Fatal("unexpected quota source")
				}
				snapshot, err := NormalizeQuota(raw, *now)
				if err != nil || len(snapshot.Limits) != rows {
					t.Fatal("quota came from the wrong source")
				}
				if source == "Local LSP" && snapshot.Limits[0].UsedPercent != 25 {
					t.Fatal("IDE quota was not preserved")
				}
			}
			_, _ = cli.cachedOrStart()
			waitCLI(t, cli)
			check("CLI", 3)
			*now = now.Add(cliInterval - time.Nanosecond)
			check("CLI", 3)
			if discoveries.Load() != 0 || requests.Load() != 0 || runner.usageCalls != 1 {
				t.Fatal("CLI success or its cache contacted the IDE")
			}
			*now = now.Add(time.Nanosecond)
			runner.raw, runner.err, runner.stderr = nil, process.ErrProcessExited, "You are not logged into Antigravity."
			_, _ = cli.cachedOrStart()
			waitCLI(t, cli)
			check("Local LSP", 1)
			if cli.tripped || !errors.Is(cli.lastErr, errCLILoggedOut) || requests.Load() == 0 {
				t.Fatal("login failure did not fall back without tripping")
			}
			runner.raw, runner.err, runner.stderr = success, nil, ""
			*now = now.Add(cliInterval - time.Nanosecond)
			check("Local LSP", 1)
			if runner.usageCalls != 2 {
				t.Fatal("login failure cache retried too early")
			}
			priorDiscoveries, priorRequests := discoveries.Load(), requests.Load()
			*now = now.Add(time.Nanosecond)
			_, _ = cli.cachedOrStart()
			waitCLI(t, cli)
			check("CLI", 3)
			if runner.usageCalls != 3 || runner.versionCalls != 1 || cli.tripped ||
				discoveries.Load() != priorDiscoveries || requests.Load() != priorRequests {
				t.Fatal("CLI did not regain priority without contacting the IDE")
			}
		})
	}
}

func TestCLIFailuresImmediatelyUseLanguageServer(t *testing.T) {
	for _, kind := range []string{"tripped", "missing", "old version", "invalid version", "timeout", "invalid response", "logged out", "disabled"} {
		for _, entry := range []string{"status", "quota"} {
			t.Run(kind+"/"+entry, func(t *testing.T) {
				cli, runner, now := fakeCLI(t)
				switch kind {
				case "tripped":
					cli.trip(errCLIInvalidResponse, "parse", nil)
				case "missing":
					cli.prepare = func() (process.CommandSpec, error) { return process.CommandSpec{}, errCLIUnavailable }
				case "old version":
					runner.version = []byte("1.1.10")
				case "invalid version":
					runner.version = []byte("garbage")
				case "timeout":
					runner.err = process.ErrTimeout
				case "invalid response":
					runner.raw = []byte("{")
				case "logged out":
					runner.raw = []byte(`{"error":"not logged in"}`)
				case "disabled":
					cli = nil
				}
				client, _, requests := fakeCLILocalClient(t, cli)
				check := func() {
					t.Helper()
					if entry == "status" {
						running, loggedIn, err := client.Status(context.Background())
						if err != nil || !running || !loggedIn {
							t.Fatal("CLI failure prevented IDE status")
						}
					} else {
						raw, err := client.RetrieveUserQuotaSummary(context.Background())
						if err != nil {
							t.Fatal("CLI failure prevented IDE quota")
						}
						snapshot, err := NormalizeQuota(raw, *now)
						if err != nil || len(snapshot.Limits) != 1 {
							t.Fatal("unexpected IDE quota")
						}
					}
					if client.Source() != "Local LSP" {
						t.Fatal("fallback did not update source")
					}
				}
				check()
				if cli != nil {
					waitCLI(t, cli)
				}
				calls := runner.usageCalls
				*now = now.Add(time.Minute)
				check()
				if runner.usageCalls != calls || requests.Load() < 2 {
					t.Fatal("cached failure did not immediately use IDE")
				}
				if cli != nil && cli.tripped {
					*now = now.Add(time.Hour)
					check()
					if runner.usageCalls != calls {
						t.Fatal("tripped CLI executed again")
					}
				}
			})
		}
	}
}

func TestCLIFailedRefreshKeepsOnlyFreshSuccess(t *testing.T) {
	cli, runner, now := fakeCLI(t)
	client, _, requests := fakeCLILocalClient(t, cli)
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCLI(t, cli)
	*now = now.Add(cliInterval)
	runner.err = process.ErrTimeout
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil || client.Source() != "CLI" {
		t.Fatal("fresh success was lost during a slow probe")
	}
	waitCLI(t, cli)
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil || client.Source() != "CLI" {
		t.Fatal("transient failure discarded a fresh CLI success")
	}
	*now = now.Add(cliCacheLifetime - cliInterval - time.Nanosecond)
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil || client.Source() != "CLI" {
		t.Fatal("CLI success expired before the 15-minute boundary")
	}
	waitCLI(t, cli)
	*now = now.Add(time.Nanosecond)
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil || client.Source() != "Local LSP" {
		t.Fatal("expired CLI success did not fall back to the IDE")
	}
	waitCLI(t, cli)
	if runner.usageCalls != 3 || requests.Load() == 0 {
		t.Fatal("expired cache failed to retry or use IDE fallback")
	}
}

func TestCLISlowProbeDoesNotConsumeRefreshBudgetOrDuplicate(t *testing.T) {
	cli, runner, _ := fakeCLI(t)
	started := make(chan struct{}, 16)
	finished := make(chan struct{})
	var calls atomic.Int32
	cli.run = func(ctx context.Context, spec process.CommandSpec, _ process.LogFunc) ([]byte, error) {
		if len(spec.Args) == 1 && spec.Args[0] == "--version" {
			return runner.version, nil
		}
		calls.Add(1)
		started <- struct{}{}
		defer close(finished)
		select {
		case <-time.After(30 * time.Second):
			return runner.raw, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	client, _, _ := fakeCLILocalClient(t, cli)
	begin := time.Now()
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil || client.Source() != "Local LSP" {
		t.Fatal("pending CLI probe did not use the IDE")
	}
	if time.Since(begin) > time.Second {
		t.Fatal("slow CLI consumed the refresh budget")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background CLI did not start")
	}
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatal("overlapping refreshes launched duplicate CLI probes")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("app close did not cancel the CLI probe")
	}
	waitCLI(t, cli)
}

func TestCLIAndIDEUnavailablePreservesIDEState(t *testing.T) {
	cli, runner, _ := fakeCLI(t)
	runner.raw = []byte(`{"error":"not logged into Antigravity"}`)
	client := &LocalClient{cli: cli, discover: func() ([]endpointCandidate, error) { return nil, nil }}
	running, loggedIn, err := client.Status(context.Background())
	if running || loggedIn || err != nil {
		t.Fatal("CLI logout replaced the IDE-not-running status")
	}
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); !errors.Is(err, ErrLocalRequest) {
		t.Fatal("CLI logout replaced the IDE quota failure")
	}
	waitCLI(t, cli)
	state := New(client).Inspect(context.Background())
	if state.Status != model.StatusUnavailable || state.Error != model.ErrUnavailable || state.Source != "Local LSP" || cli.tripped {
		t.Fatal("CLI logout leaked to the lane or tripped the breaker")
	}
}

func TestLocalQuotaDisabledBuckets(t *testing.T) {
	for _, tc := range []struct {
		name, buckets string
		wantBuckets   int
	}{
		{"all disabled", `{"disabled":true,"remainingFraction":0,"window":"5h"},{"disabled":true,"remainingFraction":1,"window":"168h"}`, 0},
		{"partly disabled", `{"disabled":true,"remainingFraction":0,"window":"5h"},{"disabled":false,"remainingFraction":0.75,"window":"168h"}`, 1},
		{"disabled flag omitted", `{"remainingFraction":0.75,"window":"168h"}`, 1},
		{"disabled without measurement", `{"disabled":true,"window":"5h"}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(`{"response":{"groups":[{"displayName":"Gemini Models","buckets":[` + tc.buckets + `]}]}}`)
			adapted, err := adaptQuotaResponse(raw, "AI ULTRA")
			if err != nil {
				t.Fatal(err)
			}
			var envelope quotaEnvelope
			if err := json.Unmarshal(adapted, &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.QuotaGroups) != tc.wantBuckets {
				t.Fatalf("group count = %d, want %d", len(envelope.QuotaGroups), tc.wantBuckets)
			}
			if tc.wantBuckets > 0 && len(envelope.QuotaGroups[0].Buckets) != tc.wantBuckets {
				t.Fatalf("bucket count = %d, want %d", len(envelope.QuotaGroups[0].Buckets), tc.wantBuckets)
			}
			snapshot, err := NormalizeQuota(adapted, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Limits) != tc.wantBuckets {
				t.Fatalf("row count = %d, want %d", len(snapshot.Limits), tc.wantBuckets)
			}
			if tc.wantBuckets > 0 && (snapshot.Limits[0].WindowMinutes != 10080 || snapshot.Limits[0].UsedPercent != 25) {
				t.Fatalf("active bucket changed: %+v", snapshot.Limits[0])
			}
		})
	}
}
