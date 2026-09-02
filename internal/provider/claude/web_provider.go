package claude

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
)

// WebProvider is the independently rendered Claude account held by
// QuotaDock's isolated WebView2 profile. It deliberately exposes no account
// identity and never reads browser cookies directly.
type WebProvider struct {
	parent *Provider
	// id is the lane this account is drawn as. The second account keeps the
	// id it has always had; the third onward are numbered.
	id    model.ProviderID
	state *model.StateMachine
	// fetcher and group belong to an account with a browser profile of its
	// own. When fetcher is nil the account is the second one, which reads
	// through the parent's shared profile and shared fetch group instead.
	fetcher oauthUsageFetcher
	group   process.Group[model.UsageSnapshot]
	source  atomic.Uint32
}

// accountID is the lane this provider reports as.
func (p *WebProvider) accountID() model.ProviderID {
	if p.id == "" {
		return model.ProviderClaudeAuth
	}
	return p.id
}

// webFetcher is the browser fetcher this account reads through: its own when
// it has one, otherwise the parent's shared one.
func (p *WebProvider) webFetcher() oauthUsageFetcher {
	if p.fetcher != nil {
		return p.fetcher
	}
	if p.parent == nil {
		return nil
	}
	return p.parent.webAuth
}

// fetchWeb reads usage through the browser. An account with its own profile
// runs the read in its own group; the second account shares the parent's, so
// it and a CLI-less root fallback never open two browsers on one profile.
func (p *WebProvider) fetchWeb(ctx context.Context) (model.UsageSnapshot, error) {
	if p.fetcher == nil {
		return p.parent.fetchWebAuth(ctx)
	}
	return p.group.Do(ctx, func() (model.UsageSnapshot, error) {
		if !p.fetcher.Available() {
			return model.UsageSnapshot{}, errOAuthCredentialsUnavailable
		}
		result, err := p.fetcher.Fetch(ctx)
		if err != nil {
			return model.UsageSnapshot{}, err
		}
		return NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, p.parent.now())
	})
}

// SetSourceMode makes the additional account independently selectable. Its
// absent/unknown value remains Auth, preserving the original second-lane
// behaviour for every configuration written before account selection existed.
func (p *WebProvider) SetSourceMode(value string) {
	selection := selectAuth
	switch SourceMode(value) {
	case SourceModeCLI:
		selection = selectCLI
	case SourceModeOther:
		selection = selectOther
	case SourceModeAuth:
		selection = selectAuth
	}
	previous := sourceSelection(p.source.Swap(uint32(selection)))
	if previous != selection && p.state != nil {
		p.state.Set(model.ConnectionState{Status: model.StatusUnavailable, Source: p.connectionSource()})
	}
}

func (p *WebProvider) sourceSelection() sourceSelection {
	selection := sourceSelection(p.source.Load())
	if selection != selectCLI && selection != selectAuth && selection != selectOther {
		return selectAuth
	}
	return selection
}

func (p *WebProvider) Inspect(ctx context.Context) model.ConnectionState {
	if p.parent == nil {
		return p.set(model.StatusUnavailable, model.ErrUnavailable, "error.unavailable", "")
	}
	switch p.sourceSelection() {
	case selectCLI:
		if state, ok := p.parent.inspectOAuthState(ctx, OAuthCredentialFile, true); ok {
			return p.setState(state)
		}
		return p.setState(p.parent.inspectCLIState(ctx))
	case selectOther:
		if state, ok := p.parent.inspectOAuthState(ctx, OAuthCredentialEnvironment, false); ok {
			return p.setState(state)
		}
		return p.set(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in", "")
	}
	if fetcher := p.webFetcher(); fetcher == nil || !fetcher.Available() {
		return p.set(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in", model.SourceWebSignIn)
	}
	current := p.state.Current()
	if current.Status == model.StatusLoggedOut || current.Status == model.StatusError {
		return current
	}
	return p.set(model.StatusConnected, model.ErrNone, "", model.SourceWebSignIn)
}

func (p *WebProvider) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	if p.parent == nil {
		return p.fail(model.StatusUnavailable, model.ErrUnavailable, "error.unavailable")
	}
	var snapshot model.UsageSnapshot
	var err error
	switch p.sourceSelection() {
	case selectCLI:
		snapshot, err = p.refreshCLI(ctx)
	case selectOther:
		snapshot, err = p.refreshOAuth(ctx, OAuthCredentialEnvironment)
	default:
		snapshot, err = p.fetchWeb(ctx)
	}
	if err != nil {
		var safe model.SafeError
		if errors.As(err, &safe) {
			status := model.StatusError
			switch safe.Code {
			case model.ErrCLINotInstalled:
				status = model.StatusUnavailable
			case model.ErrCLIOutdated:
				status = model.StatusOutdated
			case model.ErrNotLoggedIn:
				status = model.StatusLoggedOut
			case model.ErrUsageUnavailable:
				status = model.StatusConnected
			}
			return p.fail(status, safe.Code, safe.Key)
		}
		switch {
		case errors.Is(err, errOAuthCredentialsUnavailable), errors.Is(err, errOAuthReauthentication):
			return p.fail(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return p.fail(model.StatusError, model.ErrTimeout, "error.timeout")
		case errors.Is(err, errOAuthRateLimited):
			return p.fail(model.StatusConnected, model.ErrUsageUnavailable, "error.usage_unavailable")
		case errors.Is(err, errOAuthInvalidResponse):
			return p.fail(model.StatusError, model.ErrInvalidResponse, "error.invalid_response")
		default:
			return p.fail(model.StatusError, model.ErrUnavailable, "error.unavailable")
		}
	}
	snapshot.Provider = p.accountID()
	p.set(model.StatusConnected, model.ErrNone, "", p.connectionSource())
	return snapshot, nil
}

func (p *WebProvider) refreshOAuth(ctx context.Context, sources OAuthCredentialSources) (model.UsageSnapshot, error) {
	result, err := p.parent.fetchOAuth(ctx, sources)
	if err != nil {
		return model.UsageSnapshot{}, err
	}
	return NormalizeOAuthUsage(result.raw, result.rateLimitTier, result.subscriptionType, p.parent.now())
}

func (p *WebProvider) refreshCLI(ctx context.Context) (model.UsageSnapshot, error) {
	if p.parent.oauth != nil {
		if snapshot, err := p.refreshOAuth(ctx, OAuthCredentialFile); err == nil {
			if snapshot.Plan == model.PlanUnknown {
				if authRaw, authErr := p.parent.client.AuthStatus(ctx); authErr == nil {
					var auth authStatus
					if json.Unmarshal(authRaw, &auth) == nil {
						snapshot.Plan = NormalizeClaudeOAuthPlan("", auth.SubscriptionType)
					}
				}
			}
			return snapshot, nil
		}
	}
	return p.parent.refreshCLISnapshot(ctx)
}

func (p *WebProvider) connectionSource() string {
	if p.sourceSelection() == selectAuth {
		return model.SourceWebSignIn
	}
	return ""
}

func (p *WebProvider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return p.Refresh(ctx)
}

func (p *WebProvider) Close() error {
	p.state.Set(model.ConnectionState{Status: model.StatusClosed, Source: p.connectionSource()})
	return nil
}

func (p *WebProvider) fail(status model.ConnectionStatus, code model.ErrorCode, key string) (model.UsageSnapshot, error) {
	p.set(status, code, key, p.connectionSource())
	return model.UsageSnapshot{}, model.SafeError{Code: code, Key: key}
}

func (p *WebProvider) set(status model.ConnectionStatus, code model.ErrorCode, key, source string) model.ConnectionState {
	return p.setState(model.ConnectionState{Status: status, Error: code, ErrorKey: key, Source: source})
}

func (p *WebProvider) setState(state model.ConnectionState) model.ConnectionState {
	p.state.Set(state)
	return state
}

var _ model.Provider = (*WebProvider)(nil)
