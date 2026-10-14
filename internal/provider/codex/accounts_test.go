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

func TestAccountRoutesShareCLIAndKeepSnapshotIDs(t *testing.T) {
	cli := workingTransport(t)
	firstAuth := workingTransport(t)
	secondAuth := workingTransport(t)
	firstAuth.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":31,"windowDurationMins":300}}}`)
	secondAuth.responses["account/rateLimits/read"] = json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":42,"windowDurationMins":300}}}`)
	set := &Accounts{Provider: New(cli, ""), extra: make(map[model.ProviderID]*Account)}
	set.extra[model.ProviderCodex] = &Account{id: model.ProviderCodex, provider: New(firstAuth, ""), parent: set}
	set.extra[model.ProviderCodex2] = &Account{id: model.ProviderCodex2, provider: New(secondAuth, ""), parent: set}
	set.SetAccountCount(2)
	ctx := context.Background()

	first, err := set.Refresh(ctx)
	if err != nil || first.Provider != model.ProviderCodex || len(cli.calls) == 0 || len(firstAuth.calls) != 0 {
		t.Fatalf("default first route: snapshot=%+v err=%v", first, err)
	}
	second, err := set.extra[model.ProviderCodex2].Refresh(ctx)
	if err != nil || second.Provider != model.ProviderCodex2 || second.Limits[0].UsedPercent != 42 || len(secondAuth.calls) == 0 {
		t.Fatalf("default second route: snapshot=%+v err=%v", second, err)
	}
	if !set.SetSourceMode(model.ProviderCodex, "auth") {
		t.Fatal("first route did not change")
	}
	waitAccountReset(t, set.extra[model.ProviderCodex])
	first, err = set.Refresh(ctx)
	if err != nil || first.Provider != model.ProviderCodex || first.Limits[0].UsedPercent != 31 {
		t.Fatalf("first Auth route: snapshot=%+v err=%v", first, err)
	}
	if !set.SetSourceMode(model.ProviderCodex2, "cli") {
		t.Fatal("second route did not change")
	}
	waitAccountReset(t, set.extra[model.ProviderCodex2])
	second, err = set.extra[model.ProviderCodex2].Refresh(ctx)
	if err != nil || second.Provider != model.ProviderCodex2 || second.Limits[0].UsedPercent == 42 {
		t.Fatalf("second CLI route: snapshot=%+v err=%v", second, err)
	}
	if got := set.Inspect(ctx); got.Status != model.StatusConnected {
		t.Fatalf("first Auth state=%+v", got)
	}
	cliCloses := cli.closed
	if !set.SetSourceMode(model.ProviderCodex, "cli") {
		t.Fatal("first route did not return to CLI")
	}
	waitAccountReset(t, set.extra[model.ProviderCodex])
	if cli.closed != cliCloses {
		t.Fatal("switching first account closed CLI used by second")
	}
	if _, err := set.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !set.SetSourceMode(model.ProviderCodex2, "invalid") {
		t.Fatal("invalid second route did not restore Auth default")
	}
	waitAccountReset(t, set.extra[model.ProviderCodex2])
	if cli.closed != cliCloses {
		t.Fatal("switching second account closed CLI used by first")
	}
	if set.SetSourceMode(model.ProviderCodex, "") || set.extra[model.ProviderCodex].sourceMode() != accountModeCLI {
		t.Fatal("empty first route did not keep CLI default")
	}
}

func waitAccountReset(t *testing.T, account *Account) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for account.resetPending.Load() {
		select {
		case <-deadline:
			t.Fatal("source reset did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestFirstAccountAuthSignInAndCLIRejection(t *testing.T) {
	transport := &waitingLoginTransport{fakeTransport: workingTransport(t), entered: make(chan struct{})}
	set := &Accounts{Provider: New(workingTransport(t), ""), extra: make(map[model.ProviderID]*Account)}
	set.extra[model.ProviderCodex] = &Account{id: model.ProviderCodex, provider: New(transport, ""), parent: set}
	set.SetAccountCount(1)
	if err := set.SignIn(context.Background(), model.ProviderCodex, nil); err == nil {
		t.Fatal("CLI route accepted app sign-in")
	}
	set.SetSourceMode(model.ProviderCodex, "auth")
	waitAccountReset(t, set.extra[model.ProviderCodex])
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- set.SignIn(ctx, model.ProviderCodex, nil) }()
	select {
	case <-transport.entered:
	case <-ctx.Done():
		t.Fatal("first Auth sign-in did not start")
	}
	set.CancelSignIn(model.ProviderCodex)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("first Auth sign-in did not cancel")
	}
}

func TestAllCodexAccountsHaveIsolatedHomes(t *testing.T) {
	root := t.TempDir()
	set := NewAccounts(root, nil)
	defer set.Close()
	for _, id := range model.CodexAccountIDs() {
		account := set.extra[id]
		transport, ok := account.provider.transport.(*AppServerTransport)
		if !ok || transport.home != filepath.Join(root, string(id)) {
			t.Fatalf("%s has no isolated home", id)
		}
	}
	if _, included := set.AdditionalProviders()[model.ProviderCodex]; included {
		t.Fatal("first account exposed as an additional lane")
	}
}

func TestMissingIsolationRootCannotReadDefaultProfileAsAuth(t *testing.T) {
	set := NewAccounts("", nil)
	defer set.Close()
	set.SetSourceMode(model.ProviderCodex, "auth")
	waitAccountReset(t, set.extra[model.ProviderCodex])
	if _, err := set.Refresh(context.Background()); err == nil {
		t.Fatal("Auth read fell back to default CLI profile")
	}
	if err := set.SignIn(context.Background(), model.ProviderCodex, func(string) error { return nil }); err == nil {
		t.Fatal("Auth login without an isolated home was accepted")
	}
}

func TestTwoCodexAccountsCanReadSharedCLIConcurrently(t *testing.T) {
	set := &Accounts{Provider: New(workingTransport(t), ""), extra: make(map[model.ProviderID]*Account)}
	set.extra[model.ProviderCodex] = &Account{id: model.ProviderCodex, provider: New(workingTransport(t), ""), parent: set}
	set.extra[model.ProviderCodex2] = &Account{id: model.ProviderCodex2, provider: New(workingTransport(t), ""), parent: set}
	set.SetAccountCount(2)
	set.SetSourceMode(model.ProviderCodex2, "cli")
	waitAccountReset(t, set.extra[model.ProviderCodex2])
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type result struct {
		id  model.ProviderID
		err error
	}
	results := make(chan result, 2)
	for _, account := range []*Account{set.extra[model.ProviderCodex], set.extra[model.ProviderCodex2]} {
		go func(account *Account) {
			snapshot, err := account.Refresh(ctx)
			results <- result{id: snapshot.Provider, err: err}
		}(account)
	}
	seen := make(map[model.ProviderID]bool)
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatal(got.err)
			}
			seen[got.id] = true
		case <-ctx.Done():
			t.Fatal("shared CLI read deadlocked")
		}
	}
	if !seen[model.ProviderCodex] || !seen[model.ProviderCodex2] {
		t.Fatalf("shared CLI snapshots have wrong IDs: %v", seen)
	}
}
