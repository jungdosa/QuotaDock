// Package claude reads Claude OAuth usage and retains the official CLI status fallback.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

type Client interface {
	Version(context.Context) (string, error)
	AuthStatus(context.Context) (json.RawMessage, error)
	RateLimits(context.Context) (json.RawMessage, error)
	Close() error
}

type executableSource interface {
	ExecutablePath() (string, error)
}

// SourceMode selects the source that serves the root Claude lane. The empty
// value is deliberately Auto so configurations written before this selector
// retain the historical credential -> CLI auth-status -> web fallback.
type SourceMode string

const (
	SourceModeAuto  SourceMode = ""
	SourceModeCLI   SourceMode = "cli"
	SourceModeAuth  SourceMode = "auth"
	SourceModeOther SourceMode = "other"
)

type sourceSelection uint32

const (
	selectAuto sourceSelection = iota
	selectCLI
	selectAuth
	selectOther
	sourceSelectionCount
)

type Provider struct {
	client         Client
	oauth          oauthUsageFetcher
	webAuth        oauthUsageFetcher
	minimumVersion string
	state          *model.StateMachine
	groups         [sourceSelectionCount]process.Group[model.UsageSnapshot]
	webGroup       process.Group[model.UsageSnapshot]
	webProvider    *WebProvider
	// accounts are the third and later Claude accounts, each with a browser
	// fetcher of its own. The second account stays on webProvider, which shares
	// the root provider's browser profile as it always has.
	accounts map[model.ProviderID]*WebProvider
	source   atomic.Uint32
	now      func() time.Time
}

func New(client Client, minimumVersion string) *Provider {
	return newProvider(client, NewOAuthClient(), minimumVersion)
}

func newProvider(client Client, oauth oauthUsageFetcher, minimumVersion string) *Provider {
	return &Provider{client: client, oauth: oauth, minimumVersion: minimumVersion, state: model.NewStateMachine(), now: time.Now}
}

// SetSourceMode applies a stored connection-method value. Unknown values are
// treated as Auto, matching settings validation's safe fallback.
func (p *Provider) SetSourceMode(value string) {
	selection := selectAuto
	switch SourceMode(value) {
	case SourceModeCLI:
		selection = selectCLI
	case SourceModeAuth:
		selection = selectAuth
	case SourceModeOther:
		selection = selectOther
	}
	p.source.Store(uint32(selection))
}

func (p *Provider) sourceSelection() sourceSelection {
	selection := sourceSelection(p.source.Load())
	if selection >= sourceSelectionCount {
		return selectAuto
	}
	return selection
}

// SetWebAuth attaches the embedded-browser fetcher used by Auth mode, the
// independent Auth lane, and Auto's final fallback. In Auto it never displaces
// a working CLI: the order stays credential, CLI auth-status, then web.
func (p *Provider) SetWebAuth(webAuth oauthUsageFetcher) {
	p.webAuth = webAuth
	if webAuth == nil {
		p.webProvider = nil
		return
	}
	p.webProvider = &WebProvider{parent: p, id: model.ProviderClaudeAuth, state: model.NewStateMachine()}
	p.webProvider.SetSourceMode(string(SourceModeAuth))
}

// SetAccountWebAuth attaches a browser fetcher for one of the further Claude
// accounts, the third onward. Each of those has a profile folder and a fetch
// group of its own: a profile takes one writer, and two accounts sharing one
// would race each other the way the two requests of a single read once did.
// The first two accounts are not accepted here; they keep sharing the root
// provider's profile through SetWebAuth.
func (p *Provider) SetAccountWebAuth(id model.ProviderID, webAuth oauthUsageFetcher) {
	if model.ClaudeAccountIndex(id) < 3 {
		return
	}
	if webAuth == nil {
		delete(p.accounts, id)
		return
	}
	if p.accounts == nil {
		p.accounts = make(map[model.ProviderID]*WebProvider)
	}
	account := &WebProvider{parent: p, id: id, fetcher: webAuth, state: model.NewStateMachine()}
	account.SetSourceMode(string(SourceModeAuth))
	p.accounts[id] = account
}

// AdditionalProviders exposes every further Claude account as its own lane.
// The second shares the fetch group with the root provider so a CLI-less
// fallback and the explicit Auth lane never open duplicate WebView2 sessions
// against the same profile; the third onward each bring their own.
func (p *Provider) AdditionalProviders() map[model.ProviderID]model.Provider {
	if p.webProvider == nil && len(p.accounts) == 0 {
		return nil
	}
	additional := make(map[model.ProviderID]model.Provider, 1+len(p.accounts))
	if p.webProvider != nil {
		additional[model.ProviderClaudeAuth] = p.webProvider
	}
	for id, account := range p.accounts {
		additional[id] = account
	}
	return additional
}

type authStatus struct {
	LoggedIn         bool   `json:"loggedIn"`
	Authenticated    bool   `json:"authenticated"`
	SubscriptionType string `json:"subscriptionType"`
}

func (p *Provider) Inspect(ctx context.Context) model.ConnectionState {
	switch p.sourceSelection() {
	case selectCLI:
		if state, ok := p.inspectOAuth(ctx, OAuthCredentialFile, true); ok {
			return state
		}
		return p.inspectCLI(ctx)
	case selectAuth:
		return p.inspectWebOnly()
	case selectOther:
		if state, ok := p.inspectOAuth(ctx, OAuthCredentialEnvironment, false); ok {
			return state
		}
		return p.set(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in")
	default:
		return p.inspectAuto(ctx)
	}
}

func (p *Provider) inspectAuto(ctx context.Context) model.ConnectionState {
	if state, ok := p.inspectOAuth(ctx, OAuthCredentialDefault, true); ok {
		return state
	}
	state := p.inspectCLI(ctx)
	if state.Status == model.StatusConnected {
		return state
	}
	// The CLI cannot serve the lane. A signed-in browser session still can, so
	// the row must not report a failure the user has already worked around.
	if p.webAuth != nil && p.webAuth.Available() {
		return p.setState(model.ConnectionState{Status: model.StatusConnected, Source: model.SourceWebSignIn})
	}
	return state
}

func (p *Provider) inspectOAuth(ctx context.Context, sources OAuthCredentialSources, includeCLIDiagnostics bool) (model.ConnectionState, bool) {
	state, ok := p.inspectOAuthState(ctx, sources, includeCLIDiagnostics)
	if !ok {
		return model.ConnectionState{}, false
	}
	return p.setState(state), true
}

func (p *Provider) inspectOAuthState(ctx context.Context, sources OAuthCredentialSources, includeCLIDiagnostics bool) (model.ConnectionState, bool) {
	if !p.oauthAvailable(sources) {
		return model.ConnectionState{}, false
	}
	state := model.ConnectionState{Status: model.StatusConnected}
	if includeCLIDiagnostics {
		if source, ok := p.client.(executableSource); ok {
			path, err := source.ExecutablePath()
			switch {
			case errors.Is(err, shared.ErrNotInstalled):
				state.Error = model.ErrCLINotInstalled
				state.ErrorKey = "error.cli_not_installed"
			case err == nil:
				state.CLIPath = path
				state.CLIVersion, _ = p.client.Version(ctx)
			}
		}
	}
	return state, true
}

func (p *Provider) inspectWebOnly() model.ConnectionState {
	state := model.ConnectionState{Status: model.StatusLoggedOut, Error: model.ErrNotLoggedIn, ErrorKey: "error.not_logged_in", Source: model.SourceWebSignIn}
	if p.webAuth != nil && p.webAuth.Available() {
		state = model.ConnectionState{Status: model.StatusConnected, Source: model.SourceWebSignIn}
	}
	return p.setState(state)
}

func (p *Provider) oauthAvailable(sources OAuthCredentialSources) bool {
	if p.oauth == nil {
		return false
	}
	if selectable, ok := p.oauth.(sourceSelectableOAuthUsageFetcher); ok {
		return selectable.AvailableFrom(sources)
	}
	return p.oauth.Available()
}

func (p *Provider) fetchOAuth(ctx context.Context, sources OAuthCredentialSources) (oauthResult, error) {
	if p.oauth == nil {
		return oauthResult{}, errOAuthCredentialsUnavailable
	}
	if selectable, ok := p.oauth.(sourceSelectableOAuthUsageFetcher); ok {
		return selectable.FetchFrom(ctx, sources)
	}
	return p.oauth.Fetch(ctx)
}

func (p *Provider) inspectCLI(ctx context.Context) model.ConnectionState {
	return p.setState(p.inspectCLIState(ctx))
}

func (p *Provider) inspectCLIState(ctx context.Context) model.ConnectionState {
	version, err := p.client.Version(ctx)
	if errors.Is(err, shared.ErrNotInstalled) {
		return model.ConnectionState{Status: model.StatusUnavailable, Error: model.ErrCLINotInstalled, ErrorKey: "error.cli_not_installed"}
	}
	if err != nil {
		return model.ConnectionState{Status: model.StatusError, Error: model.ErrUnavailable, ErrorKey: "error.unavailable"}
	}
	path := ""
	if source, ok := p.client.(executableSource); ok {
		path, _ = source.ExecutablePath()
	}
	if p.minimumVersion != "" && !shared.VersionAtLeast(version, p.minimumVersion) {
		return model.ConnectionState{Status: model.StatusOutdated, Error: model.ErrCLIOutdated, ErrorKey: "error.cli_outdated", CLIPath: path, CLIVersion: version}
	}
	raw, err := p.client.AuthStatus(ctx)
	if err != nil {
		return model.ConnectionState{Status: model.StatusError, Error: model.ErrInvalidResponse, ErrorKey: "error.invalid_response", CLIPath: path, CLIVersion: version}
	}
	var status authStatus
	if json.Unmarshal(raw, &status) != nil {
		return model.ConnectionState{Status: model.StatusError, Error: model.ErrInvalidResponse, ErrorKey: "error.invalid_response", CLIPath: path, CLIVersion: version}
	}
	if !status.LoggedIn && !status.Authenticated {
		return model.ConnectionState{Status: model.StatusLoggedOut, Error: model.ErrNotLoggedIn, ErrorKey: "error.not_logged_in", CLIPath: path, CLIVersion: version}
	}
	return model.ConnectionState{Status: model.StatusConnected, CLIPath: path, CLIVersion: version}
}
func (p *Provider) set(status model.ConnectionStatus, code model.ErrorCode, key string) model.ConnectionState {
	state := model.ConnectionState{Status: status, Error: code, ErrorKey: key}
	return p.setState(state)
}
func (p *Provider) setCLI(status model.ConnectionStatus, code model.ErrorCode, key, path, version string) model.ConnectionState {
	state := model.ConnectionState{Status: status, Error: code, ErrorKey: key, CLIPath: path, CLIVersion: version}
	return p.setState(state)
}
func (p *Provider) setState(state model.ConnectionState) model.ConnectionState {
	p.state.Set(state)
	return state
}

func (p *Provider) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	selection := p.sourceSelection()
	return p.groups[selection].Do(ctx, func() (model.UsageSnapshot, error) {
		switch selection {
		case selectCLI:
			return p.refreshCLIOnly(ctx)
		case selectAuth:
			return p.refreshWebOnly(ctx)
		case selectOther:
			return p.refreshOtherOnly(ctx)
		default:
			return p.refreshAuto(ctx)
		}
	})
}

// refreshAuto is the pre-selector behavior kept intact for configurations
// without a stored connection method.
func (p *Provider) refreshAuto(ctx context.Context) (model.UsageSnapshot, error) {
	if p.oauth != nil {
		result, err := p.fetchOAuth(ctx, OAuthCredentialDefault)
		if err == nil {
			snapshot, normalizeErr := NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, p.now())
			if normalizeErr != nil {
				return p.refreshCLI(ctx)
			}
			if snapshot.Plan == model.PlanUnknown {
				if authRaw, authErr := p.client.AuthStatus(ctx); authErr == nil {
					var auth authStatus
					if json.Unmarshal(authRaw, &auth) == nil {
						snapshot.Plan = NormalizeClaudeOAuthPlan("", auth.SubscriptionType)
					}
				}
			}
			p.set(model.StatusConnected, model.ErrNone, "")
			return snapshot, nil
		}
		switch {
		case errors.Is(err, errOAuthCredentialsUnavailable):
			// Fall through to the existing CLI auth-status path.
		case errors.Is(err, errOAuthReauthentication):
			p.set(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in")
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrNotLoggedIn, Key: "error.not_logged_in"}
		case errors.Is(err, context.DeadlineExceeded):
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrTimeout, Key: "error.timeout"}
		case errors.Is(err, errOAuthRateLimited):
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUsageUnavailable, Key: "error.usage_unavailable"}
		default:
			// Endpoint and refresh failures safely fall back to the CLI auth-status path.
		}
	}
	cliSnapshot, cliErr := p.refreshCLI(ctx)
	if cliErr == nil {
		return cliSnapshot, nil
	}
	// The CLI is unavailable. If the user signed in through the embedded
	// browser, read usage from that session instead. It only produces a
	// snapshot on success; any failure leaves the CLI error standing so
	// the lane keeps guiding the user to install or sign in.
	if snapshot, ok := p.refreshWebAuth(ctx); ok {
		return snapshot, nil
	}
	return cliSnapshot, cliErr
}

func (p *Provider) refreshCLIOnly(ctx context.Context) (model.UsageSnapshot, error) {
	if p.oauth != nil {
		result, err := p.fetchOAuth(ctx, OAuthCredentialFile)
		if err == nil {
			snapshot, normalizeErr := NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, p.now())
			if normalizeErr != nil {
				return p.refreshCLI(ctx)
			}
			if snapshot.Plan == model.PlanUnknown {
				if authRaw, authErr := p.client.AuthStatus(ctx); authErr == nil {
					var auth authStatus
					if json.Unmarshal(authRaw, &auth) == nil {
						snapshot.Plan = NormalizeClaudeOAuthPlan("", auth.SubscriptionType)
					}
				}
			}
			p.set(model.StatusConnected, model.ErrNone, "")
			return snapshot, nil
		}
		// CLI mode gives the official auth-status path the final decision for
		// every credential-file failure, and never consults the web session.
	}
	return p.refreshCLI(ctx)
}

func (p *Provider) refreshOtherOnly(ctx context.Context) (model.UsageSnapshot, error) {
	result, err := p.fetchOAuth(ctx, OAuthCredentialEnvironment)
	if err != nil {
		return p.failSelectedSource(err, "")
	}
	snapshot, err := NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, p.now())
	if err != nil {
		return p.failSelectedSource(errOAuthInvalidResponse, "")
	}
	p.set(model.StatusConnected, model.ErrNone, "")
	return snapshot, nil
}

func (p *Provider) refreshWebOnly(ctx context.Context) (model.UsageSnapshot, error) {
	snapshot, err := p.fetchWebAuth(ctx)
	if err != nil {
		return p.failSelectedSource(err, model.SourceWebSignIn)
	}
	p.setState(model.ConnectionState{Status: model.StatusConnected, Source: model.SourceWebSignIn})
	return snapshot, nil
}

func (p *Provider) failSelectedSource(err error, source string) (model.UsageSnapshot, error) {
	status := model.StatusError
	code := model.ErrUnavailable
	key := "error.unavailable"
	switch {
	case errors.Is(err, errOAuthCredentialsUnavailable), errors.Is(err, errOAuthReauthentication):
		status, code, key = model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		code, key = model.ErrTimeout, "error.timeout"
	case errors.Is(err, errOAuthRateLimited):
		status, code, key = model.StatusConnected, model.ErrUsageUnavailable, "error.usage_unavailable"
	case errors.Is(err, errOAuthInvalidResponse):
		code, key = model.ErrInvalidResponse, "error.invalid_response"
	}
	p.setState(model.ConnectionState{Status: status, Error: code, ErrorKey: key, Source: source})
	return model.UsageSnapshot{}, model.SafeError{Code: code, Key: key}
}

func (p *Provider) refreshWebAuth(ctx context.Context) (model.UsageSnapshot, bool) {
	snapshot, err := p.fetchWebAuth(ctx)
	if err != nil {
		return model.UsageSnapshot{}, false
	}
	p.setState(model.ConnectionState{Status: model.StatusConnected, Source: model.SourceWebSignIn})
	return snapshot, true
}

func (p *Provider) fetchWebAuth(ctx context.Context) (model.UsageSnapshot, error) {
	return p.webGroup.Do(ctx, func() (model.UsageSnapshot, error) {
		if p.webAuth == nil || !p.webAuth.Available() {
			return model.UsageSnapshot{}, errOAuthCredentialsUnavailable
		}
		result, err := p.webAuth.Fetch(ctx)
		if err != nil {
			return model.UsageSnapshot{}, err
		}
		return NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, p.now())
	})
}

func (p *Provider) refreshCLI(ctx context.Context) (model.UsageSnapshot, error) {
	state := p.inspectCLIState(ctx)
	p.setState(state)
	if state.Status != model.StatusConnected {
		return model.UsageSnapshot{}, model.SafeError{Code: state.Error, Key: state.ErrorKey}
	}
	return p.refreshCLIAfterInspection(ctx)
}

// refreshCLISnapshot is the state-neutral CLI path used by an additional
// account. It must not overwrite the root account's connection badge.
func (p *Provider) refreshCLISnapshot(ctx context.Context) (model.UsageSnapshot, error) {
	state := p.inspectCLIState(ctx)
	if state.Status != model.StatusConnected {
		return model.UsageSnapshot{}, model.SafeError{Code: state.Error, Key: state.ErrorKey}
	}
	return p.refreshCLIAfterInspection(ctx)
}

func (p *Provider) refreshCLIAfterInspection(ctx context.Context) (model.UsageSnapshot, error) {
	authRaw, err := p.client.AuthStatus(ctx)
	if err != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	var auth authStatus
	if json.Unmarshal(authRaw, &auth) != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	raw, err := p.client.RateLimits(ctx)
	if err != nil {
		if errors.Is(err, ErrRateLimitsUnavailable) {
			return model.UsageSnapshot{
				Provider:  model.ProviderClaude,
				Plan:      NormalizeClaudeOAuthPlan("", auth.SubscriptionType),
				FetchedAt: p.now().UTC(),
			}, model.SafeError{Code: model.ErrUsageUnavailable, Key: "error.usage_unavailable"}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrTimeout, Key: "error.timeout"}
		}
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	snapshot, err := NormalizeRateLimits(raw, auth.SubscriptionType, p.now())
	if err != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	return snapshot, nil
}
func (p *Provider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) { return p.Refresh(ctx) }
func (p *Provider) Close() error {
	p.state.Set(model.ConnectionState{Status: model.StatusClosed})
	if p.webProvider != nil {
		_ = p.webProvider.Close()
	}
	for _, account := range p.accounts {
		_ = account.Close()
	}
	return p.client.Close()
}

type rateLimit struct {
	UsedPercentage *float64        `json:"used_percentage"`
	ResetsAt       json.RawMessage `json:"resets_at"`
}
type rateEnvelope struct {
	RateLimits struct {
		FiveHour      *rateLimit `json:"five_hour"`
		SevenDay      *rateLimit `json:"seven_day"`
		SevenDayFable *rateLimit `json:"seven_day_fable"`
	} `json:"rate_limits"`
}

func NormalizeRateLimits(raw json.RawMessage, plan string, fetchedAt time.Time) (model.UsageSnapshot, error) {
	var envelope rateEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return model.UsageSnapshot{}, err
	}
	snapshot := model.UsageSnapshot{Provider: model.ProviderClaude, Plan: model.NormalizePlan(model.ProviderClaude, plan), FetchedAt: fetchedAt.UTC()}
	appendLimit := func(id, label string, window int, limit *rateLimit) {
		if limit == nil || limit.UsedPercentage == nil {
			return
		}
		used, remaining := model.PercentPair(*limit.UsedPercentage, false)
		snapshot.Limits = append(snapshot.Limits, model.UsageLimit{ID: id, Label: label, UsedPercent: used, RemainingPercent: remaining, WindowMinutes: window, ResetsAt: parseTime(limit.ResetsAt)})
	}
	appendLimit("five_hour", "5 hour", 300, envelope.RateLimits.FiveHour)
	appendLimit("seven_day", "7 day", 10080, envelope.RateLimits.SevenDay)
	appendLimit("seven_day_fable", "Fable 7 day", 10080, envelope.RateLimits.SevenDayFable)
	return snapshot, nil
}
func parseTime(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	var unix int64
	if json.Unmarshal(raw, &unix) == nil {
		return time.Unix(unix, 0).UTC()
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		parsed, _ := time.Parse(time.RFC3339, value)
		return parsed.UTC()
	}
	return time.Time{}
}

var _ model.Provider = (*Provider)(nil)
var _ model.ProviderCollection = (*Provider)(nil)
