package claude

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/model"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

const webUsageJSON = `{"five_hour":{"utilization":12,"resets_at":"2030-01-02T03:04:05Z"},
	"seven_day":{"utilization":34,"resets_at":"2030-01-08T00:00:00Z"}}`

// A working CLI must keep serving the lane; the browser session is a
// fallback, never a replacement.
func TestWebAuthDoesNotDisplaceAWorkingCLI(t *testing.T) {
	client := &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`),
		limits: json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":5,"resets_at":"2030-01-02T00:00:00Z"}}}`)}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// The CLI figure (5%) must win over the web figure (12%).
	if len(snapshot.Limits) == 0 || snapshot.Limits[0].UsedPercent != 5 {
		t.Fatalf("web auth displaced the CLI: %+v", snapshot.Limits)
	}
}

func TestWebAuthAlsoExposesAnIndependentAccount(t *testing.T) {
	client := &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`),
		limits: json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":5,"resets_at":"2030-01-02T00:00:00Z"}}}`)}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	additional := provider.AdditionalProviders()
	web := additional[model.ProviderClaudeAuth]
	if web == nil {
		t.Fatal("embedded sign-in account was not exposed")
	}
	snapshot, err := web.Refresh(context.Background())
	if err != nil {
		t.Fatalf("web account refresh: %v", err)
	}
	if snapshot.Provider != model.ProviderClaudeAuth || len(snapshot.Limits) != 2 || snapshot.Limits[0].UsedPercent != 12 {
		t.Fatalf("web account snapshot = %+v", snapshot)
	}
	state := web.Inspect(context.Background())
	if state.Status != model.StatusConnected || state.Source != model.SourceWebSignIn {
		t.Fatalf("web account state = %+v", state)
	}
}

func TestWebAuthAccountKeepsLoggedOutStateAfterRejectedSession(t *testing.T) {
	provider := newProvider(&fakeClient{versionErr: shared.ErrNotInstalled}, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, err: errOAuthReauthentication})
	web := provider.AdditionalProviders()[model.ProviderClaudeAuth]
	if _, err := web.Refresh(context.Background()); err == nil {
		t.Fatal("rejected browser session did not fail")
	}
	if state := web.Inspect(context.Background()); state.Status != model.StatusLoggedOut || state.Error != model.ErrNotLoggedIn {
		t.Fatalf("rejected browser state = %+v", state)
	}
}

// With no CLI at all, a signed-in browser session serves the lane.
func TestWebAuthServesTheLaneWhenTheCLIIsMissing(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(snapshot.Limits) != 2 || snapshot.Limits[0].UsedPercent != 12 {
		t.Fatalf("web usage was not used: %+v", snapshot.Limits)
	}
	if state := provider.Inspect(context.Background()); state.Status == model.StatusConnected && state.Source == model.SourceWebSignIn {
		t.Log("inspect reports the web source")
	}
}

// A failing or unavailable web session must leave the CLI error standing so
// the lane keeps telling the user to install or sign in.
func TestWebAuthFailureKeepsTheCLIError(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	for name, fetcher := range map[string]oauthUsageFetcher{
		"unavailable": fakeOAuthFetcher{available: false},
		"fetch fails": fakeOAuthFetcher{available: true, err: errors.New("no session")},
		"bad payload": fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage("not json")}},
	} {
		t.Run(name, func(t *testing.T) {
			provider := newProvider(client, nil, "2.0.0")
			provider.SetWebAuth(fetcher)
			if _, err := provider.Refresh(context.Background()); err == nil {
				t.Fatal("a failing web session hid the CLI error")
			}
		})
	}
}

func TestChatOrganizationPicksTheAppOrgAndRejectsJunk(t *testing.T) {
	raw := `[{"uuid":"api-only","capabilities":["api"]},{"uuid":"app","capabilities":["chat","claude_max"],"rate_limit_tier":"tier_x","billing_type":"max"}]`
	org, ok := chatOrganization(raw)
	if !ok || org.UUID != "app" || org.RateLimitTier != "tier_x" || org.BillingType != "max" {
		t.Fatalf("organization = %+v ok=%t", org, ok)
	}
	for _, bad := range []string{"", "<html>signed out</html>", "[]", `[{"capabilities":["chat"]}]`, `[{"uuid":"x","capabilities":["api"]}]`} {
		if _, ok := chatOrganization(bad); ok {
			t.Fatalf("junk accepted: %q", bad)
		}
	}
}

// Refresh succeeding while Inspect still reported an error left the row
// showing a failure the user had already worked around by signing in.
func TestInspectReportsTheWebSourceWhenTheCLICannotServe(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	state := provider.Inspect(context.Background())
	if state.Status != model.StatusConnected || state.Source != model.SourceWebSignIn {
		t.Fatalf("inspect = %+v, want connected via the web sign-in", state)
	}
}

// A working CLI keeps owning the row, so the source stays empty.
func TestInspectPrefersTheCLIOverTheWebSession(t *testing.T) {
	client := &fakeClient{version: "2.1.0", auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`)}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	state := provider.Inspect(context.Background())
	if state.Status != model.StatusConnected || state.Source == model.SourceWebSignIn {
		t.Fatalf("inspect = %+v, want the CLI to own the row", state)
	}
}

// Without a usable browser session the CLI error must survive untouched.
func TestInspectKeepsTheCLIErrorWithoutASession(t *testing.T) {
	client := &fakeClient{versionErr: shared.ErrNotInstalled}
	provider := newProvider(client, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: false})
	if state := provider.Inspect(context.Background()); state.Status == model.StatusConnected {
		t.Fatalf("inspect = %+v, want the CLI error preserved", state)
	}
}

// stubWebSession answers each round trip from a queue so a test can make the
// second one fail while the first succeeds.
type stubWebSession struct {
	urls    []string
	replies []string
	errs    []error
}

func (s *stubWebSession) Fetch(_ context.Context, url string) (string, error) {
	index := len(s.urls)
	s.urls = append(s.urls, url)
	var reply string
	if index < len(s.replies) {
		reply = s.replies[index]
	}
	var err error
	if index < len(s.errs) {
		err = s.errs[index]
	}
	return reply, err
}

func (s *stubWebSession) Close() error { return nil }

// recordingHandler keeps the events a test triggers so their attributes can be
// asserted without a real log file.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) find(message string) (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, record := range h.records {
		if record.Message == message {
			return record, true
		}
	}
	return slog.Record{}, false
}

func captureEvents(t *testing.T) *recordingHandler {
	t.Helper()
	handler := &recordingHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return handler
}

func attrValue(record slog.Record, key string) (slog.Value, bool) {
	var found slog.Value
	var ok bool
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			found, ok = attr.Value, true
			return false
		}
		return true
	})
	return found, ok
}

// A read that runs out of time must say which of its two round trips was still
// running. Without that the log only shows the shared refresh budget expiring,
// which is the same line for every possible cause.
func TestWebAuthTraceNamesTheRoundTripThatRanOut(t *testing.T) {
	events := captureEvents(t)
	session := &stubWebSession{
		replies: []string{`[{"uuid":"org-1","capabilities":["chat"],"rate_limit_tier":"pro","billing_type":"pro"}]`},
		errs:    []error{nil, context.DeadlineExceeded},
	}
	fetcher := &WebAuthFetcher{userDataDir: t.TempDir(), newSession: func(string) webSession { return session }}
	if _, err := fetcher.Fetch(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the deadline was not reported to the caller: %v", err)
	}
	record, ok := events.find("webauth.fetch")
	if !ok {
		t.Fatal("no webauth.fetch event was recorded")
	}
	stage, _ := attrValue(record, "stage")
	if stage.String() != "usage" {
		t.Fatalf("stage = %q, want the usage round trip", stage.String())
	}
	reason, _ := attrValue(record, "err")
	if reason.String() != "deadline" {
		t.Fatalf("err = %q, want deadline", reason.String())
	}
	if value, ok := attrValue(record, "ok"); !ok || value.Bool() {
		t.Fatalf("a timed-out read reported ok = %v", value)
	}
}

// The usage address carries the organization identifier, so no attribute of
// this event may quote an address. A regression here would put an account
// identifier into a log file users are asked to attach to reports.
func TestWebAuthTraceNeverCarriesTheAddress(t *testing.T) {
	events := captureEvents(t)
	session := &stubWebSession{
		replies: []string{
			`[{"uuid":"secret-org-uuid","capabilities":["chat"],"rate_limit_tier":"pro","billing_type":"pro"}]`,
			webUsageJSON,
		},
	}
	fetcher := &WebAuthFetcher{userDataDir: t.TempDir(), newSession: func(string) webSession { return session }}
	if _, err := fetcher.Fetch(context.Background()); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(session.urls) != 2 || !strings.Contains(session.urls[1], "secret-org-uuid") {
		t.Fatalf("the test did not exercise the identified address: %v", session.urls)
	}
	record, ok := events.find("webauth.fetch")
	if !ok {
		t.Fatal("no webauth.fetch event was recorded")
	}
	record.Attrs(func(attr slog.Attr) bool {
		rendered := attr.Value.String()
		if strings.Contains(rendered, "secret-org-uuid") || strings.Contains(rendered, "claude.ai") {
			t.Fatalf("attribute %q leaked the address: %q", attr.Key, rendered)
		}
		return true
	})
	if value, ok := attrValue(record, "ok"); !ok || !value.Bool() {
		t.Fatalf("a completed read reported ok = %v", value)
	}
}
