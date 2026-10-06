package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
)

// Every CLI test injects this fake; no test invokes an installed executable.
type fakeAGY struct {
	version      []byte
	raw          []byte
	err          error
	stderr       string
	versionCalls int
	usageCalls   int
	specs        []process.CommandSpec
	budgets      []time.Duration
}

func (f *fakeAGY) run(ctx context.Context, spec process.CommandSpec, log process.LogFunc) ([]byte, error) {
	f.specs = append(f.specs, spec)
	deadline, _ := ctx.Deadline()
	f.budgets = append(f.budgets, time.Until(deadline))
	if reflect.DeepEqual(spec.Args, []string{"--version"}) {
		f.versionCalls++
		return f.version, nil
	}
	f.usageCalls++
	if log != nil {
		log(f.stderr)
	}
	return f.raw, f.err
}

func fakeCLI(t *testing.T) (*cliClient, *fakeAGY, *time.Time) {
	t.Helper()
	f := &fakeAGY{version: []byte("1.2.17\r\n"), raw: fixture(t, "antigravity-agy-usage.json")}
	now := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	c := &cliClient{now: func() time.Time { return now }, run: f.run,
		prepare: func() (process.CommandSpec, error) {
			return process.CommandSpec{Name: "fake-agy.exe", Dir: "fake-empty-probe", Env: []string{"SYSTEMROOT=fake"}}, nil
		}}
	t.Cleanup(c.stopBackground)
	return c, f, &now
}

func waitCLI(t *testing.T, c *cliClient) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		c.asyncMu.Lock()
		running := c.asyncRunning
		c.asyncMu.Unlock()
		if !running {
			return
		}
		select {
		case <-deadline:
			t.Fatal("fake CLI did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestCLIUsageFixtureSharesNormalization(t *testing.T) {
	raw, err := parseCLIQuota(fixture(t, "antigravity-agy-usage.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NormalizeQuota(raw, time.Now())
	if err != nil || snapshot.Plan != model.PlanUnknown || len(snapshot.Limits) != 3 {
		t.Fatal("CLI normalization must produce three rows and an unknown plan")
	}
	for i, want := range []struct {
		id        string
		window    int
		reset     string
		remaining float64
	}{
		{"Gemini Models:weekly", 10080, "2026-10-07T02:24:33Z", 69.39476132392883},
		{"Gemini Models:five_hour", 300, "2026-10-05T22:46:20Z", 98.99874925613403},
		{"Claude and GPT models:weekly", 10080, "2026-10-10T08:12:35Z", 0},
	} {
		got := snapshot.Limits[i]
		if got.ID != want.id || got.WindowMinutes != want.window || got.ResetsAt.Format(time.RFC3339) != want.reset || got.RemainingPercent != want.remaining || got.UsedPercent != 100-want.remaining {
			t.Fatalf("row %d normalization mismatch", i)
		}
	}
}

func TestCLIVersionGate(t *testing.T) {
	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"1.1.10", false}, {"1.1.11", true}, {"1.2.17\r\n", true}, {"2.0.0", true},
		{"0.99.99", false}, {"1.0.100", false}, {"garbage", false}, {"agy 1.2.17", false},
		{"1.1.11-beta", false}, {"1.1.11\nwarning", false}, {"01.1.11", false},
		{"18446744073709551616.1.11", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			c, f, now := fakeCLI(t)
			f.version = []byte(tc.value)
			_, err := c.fetch(context.Background())
			if (err == nil) != tc.ok {
				t.Fatal("unexpected version gate decision")
			}
			*now = now.Add(2 * time.Hour)
			_, _ = c.fetch(context.Background())
			if f.versionCalls != 1 || !tc.ok && f.usageCalls != 0 {
				t.Fatal("version gate was bypassed or not cached")
			}
		})
	}
}

func TestCLIArgumentsBudgetsAndInterval(t *testing.T) {
	c, f, now := fakeCLI(t)
	first, err := c.fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.specs) != 2 || !reflect.DeepEqual(f.specs[1].Args, []string{"-p", "/usage", "--output-format", "json"}) || f.specs[1].Args[1][0] != byte('/') {
		t.Fatal("usage must remain a literal slash command passed as separate arguments")
	}
	for i, budget := range []time.Duration{cliVersionTimeout, cliUsageTimeout} {
		if f.budgets[i] <= 0 || f.budgets[i] > budget {
			t.Fatal("command deadline missing or too long")
		}
		if f.specs[i].Name != "fake-agy.exe" || f.specs[i].Dir != "fake-empty-probe" || f.specs[i].Env == nil {
			t.Fatal("execution crossed the injected shell-free boundary")
		}
	}
	*now = now.Add(cliInterval - time.Nanosecond)
	cached, err := c.fetch(context.Background())
	if err != nil || !bytes.Equal(first, cached) || f.usageCalls != 1 {
		t.Fatal("cache did not enforce the minimum interval")
	}
	cached[0] = '!'
	again, _ := c.fetch(context.Background())
	if !bytes.Equal(first, again) {
		t.Fatal("caller modified the shared cache")
	}
	*now = now.Add(time.Nanosecond)
	_, _ = c.fetch(context.Background())
	if f.usageCalls != 2 || f.versionCalls != 1 {
		t.Fatal("five-minute boundary or version cache mismatch")
	}
}

func TestCLIEnvironmentAllowlist(t *testing.T) {
	want := []string{"PATH", "SYSTEMROOT", "WINDIR", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "PROGRAMDATA"}
	var requested []string
	env := cliEnvironment(func(key string) string { requested = append(requested, key); return "fake" })
	if !reflect.DeepEqual(requested, want) || len(env) != len(want) {
		t.Fatal("child environment allowlist changed")
	}
	for _, entry := range env {
		for _, prefix := range []string{"ANTIGRAVITY_", "GEMINI_", "GOOGLE_"} {
			if strings.HasPrefix(entry, prefix) {
				t.Fatal("credential environment leaked")
			}
		}
	}
	if empty := cliEnvironment(func(string) string { return "" }); empty == nil || len(empty) != 0 {
		t.Fatal("nil environment would inherit credentials")
	}
}

func TestCLIEnvelopeGuardsTripPermanently(t *testing.T) {
	for _, name := range []string{"agent turn", "status", "command", "turns", "tokens", "missing turns", "null turns", "missing tokens", "null tokens", "missing groups", "malformed", "oversized"} {
		t.Run(name, func(t *testing.T) {
			c, f, now := fakeCLI(t)
			var payload map[string]any
			if err := json.Unmarshal(f.raw, &payload); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "status":
				payload["status"] = "ERROR"
			case "command":
				payload["command"].(map[string]any)["name"] = "chat"
			case "turns":
				payload["num_turns"] = 1
			case "tokens":
				payload["usage"].(map[string]any)["total_tokens"] = 1
			case "missing turns":
				delete(payload, "num_turns")
			case "null turns":
				payload["num_turns"] = nil
			case "missing tokens":
				delete(payload["usage"].(map[string]any), "total_tokens")
			case "null tokens":
				payload["usage"].(map[string]any)["total_tokens"] = nil
			case "missing groups":
				delete(payload["command"].(map[string]any)["data"].(map[string]any), "groups")
			}
			f.raw, _ = json.Marshal(payload)
			switch name {
			case "agent turn":
				f.raw = fixture(t, "antigravity-agy-agent-turn.json")
			case "malformed":
				f.raw = []byte("{")
			case "oversized":
				f.raw = bytes.Repeat([]byte(" "), cliOutputLimit+1)
			}
			_, err := c.fetch(context.Background())
			if !errors.Is(err, errCLIInvalidResponse) || !c.tripped {
				t.Fatal("unsafe response did not trip the breaker")
			}
			*now = now.Add(24 * time.Hour)
			client := &LocalClient{cli: c, discover: func() ([]endpointCandidate, error) { return nil, nil }}
			_ = client.Close()
			provider := New(client)
			if state := provider.Inspect(context.Background()); state.Status != model.StatusUnavailable || state.Source != "Local LSP" {
				t.Fatal("tripped CLI must fall back to the unavailable IDE state")
			}
			_, _ = provider.Reconnect(context.Background())
			if f.versionCalls != 1 || f.usageCalls != 1 {
				t.Fatal("reconnect or elapsed time reset the circuit breaker")
			}
		})
	}
}

type fakeExitCodeError struct{}

func (fakeExitCodeError) Error() string { return "private-process-error" }
func (fakeExitCodeError) Unwrap() error { return process.ErrProcessExited }
func (fakeExitCodeError) ExitCode() int { return 17 }

func TestCLITripLogsReasonWithoutPayload(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, tc := range []struct {
		name, reason string
		prepare      func(*fakeAGY)
		exitCode     bool
	}{
		{"nonzero", "exit_nonzero", func(f *fakeAGY) { f.err = fakeExitCodeError{} }, true},
		{"output limit", "output_limit", func(f *fakeAGY) { f.err = process.ErrOutputLimit }, false},
		{"parse", "parse", func(f *fakeAGY) { f.raw = []byte(`{"private-payload-must-not-leak":true}`) }, false},
		{"turns", "turns_or_tokens", func(f *fakeAGY) { f.raw = fixture(t, "antigravity-agy-agent-turn.json") }, false},
		{"login", "login_unverified", func(f *fakeAGY) {
			f.err, f.stderr = process.ErrProcessExited, "Please sign in private-payload-must-not-leak"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			c, f, now := fakeCLI(t)
			tc.prepare(f)
			_, _ = c.fetch(context.Background())
			*now = now.Add(time.Hour)
			_, _ = c.fetch(context.Background())
			var event map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
				t.Fatal(err)
			}
			if event["msg"] != "antigravity.cli.tripped" || event["reason"] != tc.reason ||
				strings.Contains(output.String(), "private-payload") || strings.Count(output.String(), "antigravity.cli.tripped") != 1 {
				t.Fatal("trip log reason, count, or payload is incorrect")
			}
			_, hasCode := event["exit_code"]
			if hasCode != tc.exitCode || tc.exitCode && event["exit_code"] != float64(17) {
				t.Fatal("exit code presence or value is incorrect")
			}
		})
	}
}

func TestCLILoginFailuresAndTransientBudget(t *testing.T) {
	for _, kind := range []string{"exit", "exit with sign-in stderr", "stderr", "envelope", "timeout", "output limit", "transport"} {
		t.Run(kind, func(t *testing.T) {
			c, f, now := fakeCLI(t)
			want := errCLILoggedOut
			wantTripped := false
			switch kind {
			case "exit":
				// A bare nonzero exit (network or service failure) must not
				// claim the user is signed out.
				f.err = process.ErrProcessExited
				want = errCLIUnavailable
				wantTripped = true
			case "exit with sign-in stderr":
				f.err = process.ErrProcessExited
				f.stderr = "You are not logged into Antigravity."
			case "stderr":
				f.stderr = "Please sign in"
				wantTripped = true
			case "envelope":
				f.raw = []byte(`{"status":"ERROR","error":"not logged in"}`)
			case "timeout":
				f.err = process.ErrTimeout
				want = context.DeadlineExceeded
			case "output limit":
				f.err = process.ErrOutputLimit
				want = errCLIInvalidResponse
				wantTripped = true
			case "transport":
				f.err = errors.New("private failure")
				want = ErrLocalRequest
			}
			if _, err := c.fetch(context.Background()); !errors.Is(err, want) || c.tripped != wantTripped {
				t.Fatal("unexpected CLI failure classification or breaker state")
			}
			client := &LocalClient{cli: c, discover: func() ([]endpointCandidate, error) { return nil, nil }}
			state := New(client).Inspect(context.Background())
			if state.Status != model.StatusUnavailable || state.Error != model.ErrUnavailable || state.Source != "Local LSP" {
				t.Fatal("CLI failure leaked instead of the unavailable IDE state")
			}
			*now = now.Add(time.Minute)
			_, _ = c.fetch(context.Background())
			if f.usageCalls != 1 {
				t.Fatal("failed attempt bypassed rate limit")
			}
		})
	}
}

func TestCLILanguageServerPriorityAndSwitching(t *testing.T) {
	c, f, _ := fakeCLI(t)
	discoveries := 0
	client := &LocalClient{cli: c, discover: func() ([]endpointCandidate, error) {
		discoveries++
		return []endpointCandidate{{}}, errors.New("discovery failed")
	}}
	provider := New(client)
	// The first call starts the probe and still uses the IDE fallback.
	_ = provider.Inspect(context.Background())
	waitCLI(t, c)
	if state := provider.Inspect(context.Background()); state.Status != model.StatusConnected || state.Source != "CLI" {
		t.Fatal("CLI success must take priority over IDE discovery")
	}
	if _, err := provider.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _ = client.RetrieveUserQuotaSummary(context.Background())
	if f.versionCalls != 1 || f.usageCalls != 1 || discoveries != 1 || client.Source() != "CLI" {
		t.Fatal("cached CLI success must avoid IDE discovery and repeated commands")
	}
}

func TestCLINarrowLoginFailuresRetry(t *testing.T) {
	for _, phrase := range []string{"NOT LOGGED IN", "You are not logged into Antigravity.", "Not Signed In"} {
		for _, channel := range []string{"stderr", "nonzero exit", "envelope"} {
			t.Run(phrase+"/"+channel, func(t *testing.T) {
				c, f, now := fakeCLI(t)
				success := f.raw
				f.raw = nil
				if channel == "envelope" {
					f.raw, _ = json.Marshal(map[string]string{"status": "ERROR", "error": phrase})
				} else {
					f.stderr = phrase
					if channel == "nonzero exit" {
						f.err = process.ErrProcessExited
					}
				}
				if _, err := c.fetch(context.Background()); !errors.Is(err, errCLILoggedOut) || c.tripped {
					t.Fatal("narrow login failure must remain retryable")
				}
				f.raw, f.stderr, f.err = success, "", nil
				*now = now.Add(cliInterval - time.Nanosecond)
				if _, err := c.fetch(context.Background()); !errors.Is(err, errCLILoggedOut) || f.usageCalls != 1 {
					t.Fatal("login failure cache did not enforce the interval")
				}
				*now = now.Add(time.Nanosecond)
				if _, err := c.fetch(context.Background()); err != nil || f.usageCalls != 2 || f.versionCalls != 1 || c.tripped {
					t.Fatal("CLI did not recover after login at the five-minute boundary")
				}
			})
		}
	}
}

func TestCLILoginMessageCannotOverrideSafetyEvidence(t *testing.T) {
	for _, evidence := range []string{"turns", "tokens", "output limit"} {
		for _, channel := range []string{"stderr", "envelope", "nonzero exit"} {
			t.Run(evidence+"/"+channel, func(t *testing.T) {
				c, f, now := fakeCLI(t)
				payload := map[string]any{"num_turns": 0, "usage": map[string]any{"total_tokens": 0}}
				if channel == "envelope" {
					payload["error"] = "not logged into Antigravity"
				} else {
					f.stderr = "not signed in"
					if channel == "nonzero exit" {
						f.err = process.ErrProcessExited
					}
				}
				switch evidence {
				case "turns":
					payload["num_turns"] = 1
				case "tokens":
					payload["usage"].(map[string]any)["total_tokens"] = 1
				case "output limit":
					f.err = process.ErrOutputLimit
				}
				f.raw, _ = json.Marshal(payload)
				if _, err := c.fetch(context.Background()); !errors.Is(err, errCLIInvalidResponse) || !c.tripped {
					t.Fatal("login message bypassed safety evidence")
				}
				*now = now.Add(time.Hour)
				_, _ = c.fetch(context.Background())
				if f.usageCalls != 1 {
					t.Fatal("tripped CLI was retried")
				}
			})
		}
	}
}

func TestCLIBroadLoginMessagesStillTrip(t *testing.T) {
	for _, phrase := range []string{"Please sign in", "login required", "unauthenticated", "not authenticated", "authentication required"} {
		t.Run(phrase, func(t *testing.T) {
			c, f, _ := fakeCLI(t)
			f.raw = nil
			f.stderr, f.err = phrase, process.ErrProcessExited
			if _, err := c.fetch(context.Background()); !errors.Is(err, errCLILoggedOut) || !c.tripped {
				t.Fatal("broad login text authorized a retry")
			}
		})
	}
}

func TestCLIConcurrentPollingAndRecreatedClientsShareState(t *testing.T) {
	c, f, _ := fakeCLI(t)
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			client := &LocalClient{cli: c, discover: func() ([]endpointCandidate, error) { return nil, nil }}
			_, _ = New(client).Refresh(context.Background())
		}()
	}
	group.Wait()
	waitCLI(t, c)
	if f.usageCalls != 1 || f.versionCalls != 1 {
		t.Fatal("concurrent polling created duplicate commands")
	}
}

func TestCLIVersionFailureAndPathChangeFailClosed(t *testing.T) {
	c, f, now := fakeCLI(t)
	c.run = func(context.Context, process.CommandSpec, process.LogFunc) ([]byte, error) {
		return nil, process.ErrTimeout
	}
	if _, err := c.fetch(context.Background()); !errors.Is(err, errCLIUnavailable) || c.versionOK {
		t.Fatal("version timeout authorized usage")
	}
	c, f, now = fakeCLI(t)
	_, _ = c.fetch(context.Background())
	*now = now.Add(time.Hour)
	c.prepare = func() (process.CommandSpec, error) { return process.CommandSpec{Name: "different-fake.exe"}, nil }
	if _, err := c.fetch(context.Background()); !errors.Is(err, errCLIUnavailable) || f.usageCalls != 1 {
		t.Fatal("cached gate authorized a different executable")
	}
}
