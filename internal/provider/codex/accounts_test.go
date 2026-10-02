package codex

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

func TestAccountTransportDoesNotChangeParentOrPrimaryEnvironment(t *testing.T) {
	t.Setenv("CODEX_HOME", "existing-cli-home")
	t.Setenv("OPENAI_API_KEY", "synthetic-test-key")
	root := NewAppServerTransport(nil).command("codex", "app-server")
	if root.Env != nil || root.Dir != "" || len(root.Args) != 1 {
		t.Fatal("primary account changed")
	}
	a := NewAccountTransport(filepath.Join(t.TempDir(), "two"), nil)
	b := NewAccountTransport(filepath.Join(t.TempDir(), "three"), nil)
	for _, transport := range []*AppServerTransport{a, b} {
		spec := transport.command("codex", "app-server")
		found := false
		for _, env := range spec.Env {
			if env == "CODEX_HOME="+transport.home {
				found = true
			}
			if strings.HasPrefix(env, "OPENAI_API_KEY=") || env == "CODEX_HOME=existing-cli-home" {
				t.Fatal("inherited authentication leaked")
			}
		}
		if !found || spec.Dir != transport.home || !strings.Contains(strings.Join(spec.Args, " "), `cli_auth_credentials_store="file"`) {
			t.Fatal("account not isolated")
		}
	}
}

func TestCodexAccountsKeepSnapshotsAndFailuresIndependent(t *testing.T) {
	root := workingTransport(t)
	two := workingTransport(t)
	three := workingTransport(t)
	two.responses["account/rateLimits/read"] = fixture(t, "codex-rate-limits-spark.json")
	three.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":17,"windowDurationMins":300}}}`)
	set := &Accounts{Provider: New(root, ""), extra: map[model.ProviderID]*Account{
		model.ProviderCodex2: {id: model.ProviderCodex2, provider: New(two, "")},
		model.ProviderCodex3: {id: model.ProviderCodex3, provider: New(three, "")},
	}}
	set.SetAccountCount(3)
	ctx := context.Background()
	a, err := set.extra[model.ProviderCodex2].Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := set.extra[model.ProviderCodex3].Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Provider != model.ProviderCodex2 || b.Provider != model.ProviderCodex3 || len(a.Limits) < 3 || len(b.Limits) != 1 || b.Limits[0].UsedPercent != 17 {
		t.Fatalf("account data mixed: %+v / %+v", a, b)
	}
	two.fail["account/rateLimits/read"] = errors.New("offline")
	_, _ = set.extra[model.ProviderCodex2].Reconnect(ctx)
	if three.closed != 0 || root.closed != 0 {
		t.Fatal("reconnect closed another account")
	}
	b, err = set.extra[model.ProviderCodex3].Refresh(ctx)
	if err != nil || b.Limits[0].UsedPercent != 17 {
		t.Fatal("one account blocked another")
	}
	if err := set.Close(); err != nil {
		t.Fatal(err)
	}
	if root.closed == 0 || two.closed == 0 || three.closed == 0 {
		t.Fatal("root close leaked an account")
	}
}

type waitingLoginTransport struct {
	*fakeTransport
	entered chan struct{}
}

func (t *waitingLoginTransport) SignIn(ctx context.Context, _ func(string) error) error {
	close(t.entered)
	<-ctx.Done()
	return ctx.Err()
}

func TestCodexLoginIsExclusiveAndCancellable(t *testing.T) {
	transport := &waitingLoginTransport{fakeTransport: workingTransport(t), entered: make(chan struct{})}
	a := &Account{id: model.ProviderCodex2, provider: New(transport, "")}
	a.active.Store(true)
	a.provider.plan = "PLUS"
	a.provider.limits.RateLimits.PlanType = "plus"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.signIn(ctx, func(string) error { return nil }) }()
	select {
	case <-transport.entered:
	case <-ctx.Done():
		t.Fatal("login did not start")
	}
	if _, err := a.Refresh(ctx); err == nil {
		t.Fatal("refresh entered during sign-in")
	}
	if _, err := a.Reconnect(ctx); err == nil {
		t.Fatal("reconnect entered during sign-in")
	}
	if err := a.signIn(ctx, nil); err == nil {
		t.Fatal("duplicate login entered")
	}
	a.cancelOperation()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("cancel did not complete")
	}
	if a.provider.plan != model.PlanUnknown || a.provider.limits.RateLimits.PlanType != "" || a.provider.initialized {
		t.Fatal("old identity cache survived login")
	}
}

func TestLoggedOutCodexDoesNotRepeatHandshake(t *testing.T) {
	f := workingTransport(t)
	f.responses["account/read"] = json.RawMessage(`{"account":null}`)
	p := New(f, "")
	for range 2 {
		_, _ = p.Refresh(context.Background())
	}
	count := 0
	for _, call := range f.calls {
		if call == "initialize" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("handshakes=%d, want one", count)
	}
}

func TestCodexLoginCancelledBeforeWorkerDoesNotStart(t *testing.T) {
	transport := &waitingLoginTransport{fakeTransport: workingTransport(t), entered: make(chan struct{})}
	a := &Account{id: model.ProviderCodex2, provider: New(transport, "")}
	a.active.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.signIn(ctx, func(string) error { t.Fatal("cancelled login opened browser"); return nil }); err == nil {
		t.Fatal("cancel ignored")
	}
	select {
	case <-transport.entered:
		t.Fatal("cancelled worker entered login")
	default:
	}
	if transport.closed != 0 {
		t.Fatal("cancelled worker touched session")
	}
}

func TestRemovingAnAccountCancelsLoginAndAllowsReadd(t *testing.T) {
	transport := &waitingLoginTransport{fakeTransport: workingTransport(t), entered: make(chan struct{})}
	a := &Account{id: model.ProviderCodex2, provider: New(transport, "")}
	set := &Accounts{Provider: New(workingTransport(t), ""), extra: map[model.ProviderID]*Account{model.ProviderCodex2: a}}
	set.SetAccountCount(2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- set.SignIn(ctx, model.ProviderCodex2, func(string) error { return nil }) }()
	select {
	case <-transport.entered:
	case <-ctx.Done():
		t.Fatal("login not started")
	}
	set.SetAccountCount(1)
	if len(set.AdditionalProviders()) != 0 {
		t.Fatal("removed provider is still polled")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("removal did not cancel login")
	}
	set.SetAccountCount(2)
	if len(set.AdditionalProviders()) != 1 {
		t.Fatal("re-add failed")
	}
	// Removal closes asynchronously. A poll may briefly find its lifecycle lock
	// busy even after re-add; require recovery before the deadline, not on poll one.
	for {
		if _, err := a.Refresh(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("re-added account did not resume refreshing")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := set.Close(); err != nil {
		t.Fatal(err)
	}
}
