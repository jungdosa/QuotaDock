// Package claude reads Claude OAuth usage and retains the official CLI status fallback.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
	"time"
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

type Provider struct {
	client         Client
	oauth          oauthUsageFetcher
	minimumVersion string
	state          *model.StateMachine
	group          process.Group[model.UsageSnapshot]
	now            func() time.Time
}

func New(client Client, minimumVersion string) *Provider {
	return newProvider(client, NewOAuthClient(), minimumVersion)
}

func newProvider(client Client, oauth oauthUsageFetcher, minimumVersion string) *Provider {
	return &Provider{client: client, oauth: oauth, minimumVersion: minimumVersion, state: model.NewStateMachine(), now: time.Now}
}

type authStatus struct {
	LoggedIn         bool   `json:"loggedIn"`
	Authenticated    bool   `json:"authenticated"`
	SubscriptionType string `json:"subscriptionType"`
}

func (p *Provider) Inspect(ctx context.Context) model.ConnectionState {
	if p.oauth != nil && p.oauth.Available() {
		state := model.ConnectionState{Status: model.StatusConnected}
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
		return p.setState(state)
	}
	return p.inspectCLI(ctx)
}

func (p *Provider) inspectCLI(ctx context.Context) model.ConnectionState {
	version, err := p.client.Version(ctx)
	if errors.Is(err, shared.ErrNotInstalled) {
		return p.set(model.StatusUnavailable, model.ErrCLINotInstalled, "error.cli_not_installed")
	}
	if err != nil {
		return p.set(model.StatusError, model.ErrUnavailable, "error.unavailable")
	}
	path := ""
	if source, ok := p.client.(executableSource); ok {
		path, _ = source.ExecutablePath()
	}
	if p.minimumVersion != "" && !shared.VersionAtLeast(version, p.minimumVersion) {
		return p.setCLI(model.StatusOutdated, model.ErrCLIOutdated, "error.cli_outdated", path, version)
	}
	raw, err := p.client.AuthStatus(ctx)
	if err != nil {
		return p.setCLI(model.StatusError, model.ErrInvalidResponse, "error.invalid_response", path, version)
	}
	var status authStatus
	if json.Unmarshal(raw, &status) != nil {
		return p.setCLI(model.StatusError, model.ErrInvalidResponse, "error.invalid_response", path, version)
	}
	if !status.LoggedIn && !status.Authenticated {
		return p.setCLI(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in", path, version)
	}
	return p.setCLI(model.StatusConnected, model.ErrNone, "", path, version)
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
	return p.group.Do(ctx, func() (model.UsageSnapshot, error) {
		if p.oauth != nil {
			result, err := p.oauth.Fetch(ctx)
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
		return p.refreshCLI(ctx)
	})
}

func (p *Provider) refreshCLI(ctx context.Context) (model.UsageSnapshot, error) {
	state := p.inspectCLI(ctx)
	if state.Status != model.StatusConnected {
		return model.UsageSnapshot{}, model.SafeError{Code: state.Error, Key: state.ErrorKey}
	}
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
