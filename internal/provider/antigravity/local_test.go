package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
			check("CLI", 3)
			*now = now.Add(cliInterval - time.Nanosecond)
			check("CLI", 3)
			if discoveries.Load() != 0 || requests.Load() != 0 || runner.usageCalls != 1 {
				t.Fatal("CLI success or its cache contacted the IDE")
			}
			*now = now.Add(time.Nanosecond)
			runner.raw, runner.err, runner.stderr = nil, process.ErrProcessExited, "You are not logged into Antigravity."
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
					cli.trip(errCLIInvalidResponse)
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

func TestCLIFailedRefreshDoesNotServeOldSuccess(t *testing.T) {
	cli, runner, now := fakeCLI(t)
	client, _, requests := fakeCLILocalClient(t, cli)
	if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(cliInterval)
	runner.err = process.ErrTimeout
	for range 2 {
		if _, err := client.RetrieveUserQuotaSummary(context.Background()); err != nil || client.Source() != "Local LSP" {
			t.Fatal("failed refresh or its cache returned stale CLI success")
		}
	}
	if runner.usageCalls != 2 || requests.Load() == 0 {
		t.Fatal("failed refresh bypassed interval or IDE fallback")
	}
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
