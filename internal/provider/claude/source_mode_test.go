package claude

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

type modeOAuthFetcher struct {
	mu               sync.Mutex
	available        bool
	result           oauthResult
	err              error
	availableSources []OAuthCredentialSources
	fetchSources     []OAuthCredentialSources
	record           func(string)
}

func (f *modeOAuthFetcher) Available() bool {
	return f.AvailableFrom(OAuthCredentialDefault)
}

func (f *modeOAuthFetcher) AvailableFrom(sources OAuthCredentialSources) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.availableSources = append(f.availableSources, sources)
	return f.available
}

func (f *modeOAuthFetcher) Fetch(ctx context.Context) (oauthResult, error) {
	return f.FetchFrom(ctx, OAuthCredentialDefault)
}

func (f *modeOAuthFetcher) FetchFrom(_ context.Context, sources OAuthCredentialSources) (oauthResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetchSources = append(f.fetchSources, sources)
	if f.record != nil {
		f.record("oauth.fetch")
	}
	return f.result, f.err
}

func (f *modeOAuthFetcher) calls() ([]OAuthCredentialSources, []OAuthCredentialSources) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.availableSources), slices.Clone(f.fetchSources)
}

type modeClient struct {
	mu         sync.Mutex
	version    string
	versionErr error
	auth       json.RawMessage
	authErr    error
	limits     json.RawMessage
	limitsErr  error
	calls      []string
	record     func(string)
}

func (c *modeClient) note(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
	if c.record != nil {
		c.record(call)
	}
}

func (c *modeClient) Version(context.Context) (string, error) {
	c.note("cli.version")
	return c.version, c.versionErr
}

func (c *modeClient) AuthStatus(context.Context) (json.RawMessage, error) {
	c.note("cli.auth-status")
	return c.auth, c.authErr
}

func (c *modeClient) RateLimits(context.Context) (json.RawMessage, error) {
	c.note("cli.rate-limits")
	return c.limits, c.limitsErr
}

func (c *modeClient) Close() error { return nil }

func (c *modeClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

type modeWebFetcher struct {
	available      bool
	result         oauthResult
	err            error
	record         func(string)
	started        chan struct{}
	release        chan struct{}
	startOnce      sync.Once
	availableCalls atomic.Int32
	fetchCalls     atomic.Int32
}

func (f *modeWebFetcher) Available() bool {
	f.availableCalls.Add(1)
	if f.record != nil {
		f.record("web.available")
	}
	return f.available
}

func (f *modeWebFetcher) Fetch(ctx context.Context) (oauthResult, error) {
	f.fetchCalls.Add(1)
	if f.record != nil {
		f.record("web.fetch")
	}
	if f.started != nil {
		f.startOnce.Do(func() { close(f.started) })
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return oauthResult{}, ctx.Err()
		}
	}
	return f.result, f.err
}

func TestSourceModesMakeInspectAndRefreshUseTheSameFetcher(t *testing.T) {
	tests := []struct {
		name        string
		mode        SourceMode
		oauthSource OAuthCredentialSources
		web         bool
	}{
		{name: "Auto", mode: SourceModeAuto, oauthSource: OAuthCredentialDefault},
		{name: "CLI", mode: SourceModeCLI, oauthSource: OAuthCredentialFile},
		{name: "Auth", mode: SourceModeAuth, web: true},
		{name: "Other", mode: SourceModeOther, oauthSource: OAuthCredentialEnvironment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oauth := &modeOAuthFetcher{
				available: true,
				result: oauthResult{
					raw:           json.RawMessage(syntheticUsage),
					rateLimitTier: "default_claude_max_5x",
				},
			}
			client := &modeClient{
				version: "2.1.0",
				auth:    json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`),
				limits:  json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":5}}}`),
			}
			web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}}
			provider := newProvider(client, oauth, "2.0.0")
			provider.SetWebAuth(web)
			provider.SetSourceMode(string(tc.mode))

			before := provider.Inspect(context.Background())
			if before.Status != model.StatusConnected {
				t.Fatalf("inspect before refresh = %+v", before)
			}
			if _, err := provider.Refresh(context.Background()); err != nil {
				t.Fatalf("refresh: %v", err)
			}
			after := provider.Inspect(context.Background())
			if after.Status != model.StatusConnected || after.Source != before.Source {
				t.Fatalf("inspect/refresh source drifted: before=%+v after=%+v", before, after)
			}

			availableCalls, fetchCalls := oauth.calls()
			if tc.web {
				if len(availableCalls) != 0 || len(fetchCalls) != 0 || web.fetchCalls.Load() != 1 {
					t.Fatalf("auth calls: oauth available=%v fetch=%v, web fetch=%d", availableCalls, fetchCalls, web.fetchCalls.Load())
				}
				if before.Source != model.SourceWebSignIn {
					t.Fatalf("auth inspect source = %q", before.Source)
				}
			} else {
				if want := []OAuthCredentialSources{tc.oauthSource, tc.oauthSource}; !slices.Equal(availableCalls, want) {
					t.Fatalf("inspect sources = %v, want %v", availableCalls, want)
				}
				if want := []OAuthCredentialSources{tc.oauthSource}; !slices.Equal(fetchCalls, want) {
					t.Fatalf("refresh sources = %v, want %v", fetchCalls, want)
				}
				if web.fetchCalls.Load() != 0 {
					t.Fatalf("web fetches = %d", web.fetchCalls.Load())
				}
			}
			if client.callCount() != 0 {
				t.Fatalf("mode unexpectedly inspected the CLI %d times", client.callCount())
			}
		})
	}
}

func TestAuthModeUsesWebEvenWithLiveCLICredentials(t *testing.T) {
	oauth := &modeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(syntheticUsage)}}
	client := &modeClient{
		version: "2.1.0",
		auth:    json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`),
		limits:  json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":5}}}`),
	}
	web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}}
	provider := newProvider(client, oauth, "2.0.0")
	provider.SetWebAuth(web)
	provider.SetSourceMode(string(SourceModeAuth))

	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Limits) == 0 || snapshot.Limits[0].UsedPercent != 12 {
		t.Fatalf("auth mode snapshot = %+v", snapshot)
	}
	_, oauthFetches := oauth.calls()
	if len(oauthFetches) != 0 || client.callCount() != 0 || web.fetchCalls.Load() != 1 {
		t.Fatalf("auth mode used oauth=%v cli=%d web=%d", oauthFetches, client.callCount(), web.fetchCalls.Load())
	}
}

func TestCLIModeFailureNeverFallsBackToWeb(t *testing.T) {
	oauth := &modeOAuthFetcher{err: errors.New("synthetic OAuth failure")}
	client := &modeClient{versionErr: shared.ErrNotInstalled}
	web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}}
	provider := newProvider(client, oauth, "2.0.0")
	provider.SetWebAuth(web)
	provider.SetSourceMode(string(SourceModeCLI))

	if _, err := provider.Refresh(context.Background()); err == nil {
		t.Fatal("CLI mode failure unexpectedly succeeded")
	}
	if web.availableCalls.Load() != 0 || web.fetchCalls.Load() != 0 {
		t.Fatalf("CLI mode fell back to web: available=%d fetch=%d", web.availableCalls.Load(), web.fetchCalls.Load())
	}
}

func TestCLIModeCredentialFailureUsesCLIAuthStatus(t *testing.T) {
	oauth := &modeOAuthFetcher{err: errOAuthReauthentication}
	client := &modeClient{
		version: "2.1.0",
		auth:    json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`),
		limits:  json.RawMessage(`{"rate_limits":{"five_hour":{"used_percentage":5}}}`),
	}
	web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}}
	provider := newProvider(client, oauth, "2.0.0")
	provider.SetWebAuth(web)
	provider.SetSourceMode(string(SourceModeCLI))

	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Limits) == 0 || snapshot.Limits[0].UsedPercent != 5 {
		t.Fatalf("CLI fallback snapshot = %+v", snapshot)
	}
	if web.availableCalls.Load() != 0 || web.fetchCalls.Load() != 0 {
		t.Fatal("CLI auth-status fallback consulted web")
	}
}

func TestAbsentModePreservesCredentialCLIWebFallbackOrder(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	record := func(call string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, call)
	}
	oauth := &modeOAuthFetcher{err: errOAuthCredentialsUnavailable, record: record}
	client := &modeClient{versionErr: shared.ErrNotInstalled, record: record}
	web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}, record: record}
	provider := newProvider(client, oauth, "2.0.0")
	provider.SetWebAuth(web)

	if _, err := provider.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"oauth.fetch", "cli.version", "web.available", "web.fetch"}
	if !slices.Equal(calls, want) {
		t.Fatalf("fallback order = %v, want %v", calls, want)
	}
}

func TestChangingModeAffectsTheNextRefresh(t *testing.T) {
	oauth := &modeOAuthFetcher{
		available: true,
		result:    oauthResult{raw: json.RawMessage(syntheticUsage), rateLimitTier: "default_claude_max_5x"},
	}
	web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}}
	provider := newProvider(&modeClient{}, oauth, "2.0.0")
	provider.SetWebAuth(web)

	if _, err := provider.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.SetSourceMode(string(SourceModeAuth))
	snapshot, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Limits) == 0 || snapshot.Limits[0].UsedPercent != 12 || web.fetchCalls.Load() != 1 {
		t.Fatalf("changed mode did not serve web usage: snapshot=%+v web fetch=%d", snapshot, web.fetchCalls.Load())
	}
}

func TestRootAndAdditionalAuthLanesShareOneWebFetch(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	web := &modeWebFetcher{
		available: true,
		result:    oauthResult{raw: json.RawMessage(webUsageJSON)},
		started:   started,
		release:   release,
	}
	provider := newProvider(&modeClient{}, nil, "2.0.0")
	provider.SetWebAuth(web)
	provider.SetSourceMode(string(SourceModeAuth))
	additional := provider.AdditionalProviders()[model.ProviderClaudeAuth]
	if additional == nil {
		t.Fatal("additional Auth lane is missing")
	}

	results := make(chan error, 2)
	go func() {
		_, err := provider.Refresh(context.Background())
		results <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("root Auth fetch did not start")
	}
	go func() {
		_, err := additional.Refresh(context.Background())
		results <- err
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("shared Auth refresh did not finish")
		}
	}
	if web.fetchCalls.Load() != 1 {
		t.Fatalf("shared profile fetched %d times", web.fetchCalls.Load())
	}
}

func TestAdditionalAccountSourceModesKeepInspectAndRefreshAligned(t *testing.T) {
	tests := []struct {
		name        string
		mode        SourceMode
		oauthSource OAuthCredentialSources
		web         bool
		source      string
	}{
		{name: "CLI", mode: SourceModeCLI, oauthSource: OAuthCredentialFile},
		{name: "Auth", mode: SourceModeAuth, web: true, source: model.SourceWebSignIn},
		{name: "Other", mode: SourceModeOther, oauthSource: OAuthCredentialEnvironment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oauth := &modeOAuthFetcher{
				available: true,
				result: oauthResult{
					raw:           json.RawMessage(syntheticUsage),
					rateLimitTier: "default_claude_max_5x",
				},
			}
			web := &modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}}
			root := newProvider(&modeClient{}, oauth, "2.0.0")
			root.SetWebAuth(web)
			additional := root.AdditionalProviders()[model.ProviderClaudeAuth]
			setter, ok := additional.(interface{ SetSourceMode(string) })
			if !ok {
				t.Fatal("additional account does not accept a source mode")
			}
			setter.SetSourceMode(string(tc.mode))

			before := additional.Inspect(context.Background())
			if before.Status != model.StatusConnected || before.Source != tc.source {
				t.Fatalf("inspect before refresh = %+v", before)
			}
			snapshot, err := additional.Refresh(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			after := additional.Inspect(context.Background())
			if snapshot.Provider != model.ProviderClaudeAuth || after.Status != before.Status || after.Source != before.Source {
				t.Fatalf("additional source drifted: snapshot=%+v before=%+v after=%+v", snapshot, before, after)
			}

			availableCalls, fetchCalls := oauth.calls()
			if tc.web {
				if len(availableCalls) != 0 || len(fetchCalls) != 0 || web.fetchCalls.Load() != 1 {
					t.Fatalf("Auth source calls oauth=%v/%v web=%d", availableCalls, fetchCalls, web.fetchCalls.Load())
				}
			} else {
				if want := []OAuthCredentialSources{tc.oauthSource, tc.oauthSource}; !slices.Equal(availableCalls, want) {
					t.Fatalf("inspect sources=%v, want %v", availableCalls, want)
				}
				if want := []OAuthCredentialSources{tc.oauthSource}; !slices.Equal(fetchCalls, want) {
					t.Fatalf("refresh sources=%v, want %v", fetchCalls, want)
				}
				if web.fetchCalls.Load() != 0 {
					t.Fatalf("non-Auth source fetched web %d times", web.fetchCalls.Load())
				}
			}
		})
	}
}

func TestAdditionalCLIMissingStateMatchesRefreshError(t *testing.T) {
	oauth := &modeOAuthFetcher{err: errOAuthCredentialsUnavailable}
	root := newProvider(&modeClient{versionErr: shared.ErrNotInstalled}, oauth, "2.0.0")
	root.SetWebAuth(&modeWebFetcher{available: true, result: oauthResult{raw: json.RawMessage(webUsageJSON)}})
	additional := root.AdditionalProviders()[model.ProviderClaudeAuth]
	additional.(interface{ SetSourceMode(string) }).SetSourceMode(string(SourceModeCLI))

	before := additional.Inspect(context.Background())
	_, err := additional.Refresh(context.Background())
	var safe model.SafeError
	if !errors.As(err, &safe) {
		t.Fatalf("refresh error = %v, want SafeError", err)
	}
	after := additional.Inspect(context.Background())
	if before.Status != model.StatusUnavailable || before.Error != model.ErrCLINotInstalled ||
		after.Status != before.Status || after.Error != before.Error || safe.Code != before.Error {
		t.Fatalf("Inspect/Refresh mismatch: before=%+v error=%+v after=%+v", before, safe, after)
	}
}
