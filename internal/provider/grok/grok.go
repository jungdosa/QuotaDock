// Package grok reads the official Grok CLI credential and queries the
// first-party Grok billing endpoint without rotating or exposing that token.
package grok

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/security"
)

const (
	billingEndpoint  = "https://grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig"
	requestTimeout   = 10 * time.Second
	maxResponseBytes = 1 << 20
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Provider struct {
	client         HTTPClient
	credentialPath string
	endpoint       string
	allowURL       func(string) bool
	state          *model.StateMachine
	group          process.Group[model.UsageSnapshot]
	now            func() time.Time
}

func New(client HTTPClient, credentialPath string) *Provider {
	if client == nil {
		client = &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	if credentialPath == "" {
		credentialPath = DefaultCredentialPath()
	}
	return &Provider{
		client:         client,
		credentialPath: credentialPath,
		endpoint:       billingEndpoint,
		allowURL:       security.IsAllowedProviderRequestURL,
		state:          model.NewStateMachine(),
		now:            time.Now,
	}
}

func (p *Provider) Inspect(context.Context) model.ConnectionState {
	_, err := LoadCredential(p.credentialPath)
	switch {
	case errors.Is(err, shared.ErrNotLoggedIn):
		return p.set(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in")
	case errors.Is(err, errCredentialInvalid):
		return p.set(model.StatusError, model.ErrInvalidResponse, "error.invalid_response")
	case err != nil:
		return p.set(model.StatusError, model.ErrUnavailable, "error.unavailable")
	default:
		return p.set(model.StatusConnected, model.ErrNone, "")
	}
}

func (p *Provider) set(status model.ConnectionStatus, code model.ErrorCode, key string) model.ConnectionState {
	state := model.ConnectionState{
		Status:   status,
		Error:    code,
		ErrorKey: key,
		Source:   "Grok CLI",
	}
	p.state.Set(state)
	return state
}

func (p *Provider) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	return p.group.Do(ctx, func() (model.UsageSnapshot, error) {
		credential, err := LoadCredential(p.credentialPath)
		if err != nil {
			return model.UsageSnapshot{}, credentialSafeError(err)
		}
		return p.fetch(ctx, credential)
	})
}

func credentialSafeError(err error) error {
	switch {
	case errors.Is(err, shared.ErrNotLoggedIn):
		return model.SafeError{Code: model.ErrNotLoggedIn, Key: "error.not_logged_in"}
	case errors.Is(err, errCredentialInvalid):
		return model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	default:
		return model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
}

func (p *Provider) fetch(ctx context.Context, credential Credential) (model.UsageSnapshot, error) {
	if !p.allowURL(p.endpoint) {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(emptyRequestFrame()))
	if err != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+credential.Key)
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Content-Type", "application/grpc-web+proto")
	request.Header.Set("x-grpc-web", "1")
	request.Header.Set("x-user-agent", "connect-es/2.1.1")
	request.Header.Set("Origin", "https://grok.com")
	request.Header.Set("Referer", "https://grok.com/?_s=usage")
	request.Header.Set("User-Agent", "QuotaDock")

	response, err := p.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrTimeout, Key: "error.timeout"}
		}
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	if response == nil || response.Body == nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	if len(body) > maxResponseBytes {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	frames, err := decodeGRPCWebFrames(body)
	if errors.Is(err, errGRPCWebEmptyBody) {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrNotLoggedIn, Key: "error.not_logged_in"}
	}
	if err != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	for _, frame := range frames {
		if frame.Trailer {
			continue
		}
		snapshot, normalizeErr := NormalizeBilling(frame.Payload, p.now())
		if normalizeErr != nil {
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
		}
		return snapshot, nil
	}
	return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
}

func (p *Provider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return p.Refresh(ctx)
}

func (p *Provider) Close() error {
	p.state.Set(model.ConnectionState{Status: model.StatusClosed})
	return nil
}

var _ model.Provider = (*Provider)(nil)
