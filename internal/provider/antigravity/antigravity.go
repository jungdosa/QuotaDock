// Package antigravity connects to and normalizes verified local language-server quota responses.
package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
	"strings"
	"time"
)

var allowedExecutables = map[string]struct{}{"language_server.exe": {}, "language_server_windows_x64.exe": {}, "language_server_windows_arm64.exe": {}}

func IsAllowedExecutable(name string) bool {
	_, ok := allowedExecutables[strings.ToLower(name)]
	return ok
}

type Client interface {
	Status(context.Context) (running bool, loggedIn bool, err error)
	RetrieveUserQuotaSummary(context.Context) (json.RawMessage, error)
	Close() error
}
type Provider struct {
	client Client
	state  *model.StateMachine
	group  process.Group[model.UsageSnapshot]
	now    func() time.Time
}

func New(client Client) *Provider {
	return &Provider{client: client, state: model.NewStateMachine(), now: time.Now}
}
func (p *Provider) Inspect(ctx context.Context) model.ConnectionState {
	running, loggedIn, err := p.client.Status(ctx)
	if err != nil {
		return p.set(model.StatusError, model.ErrUnavailable, "error.unavailable")
	}
	if !running {
		return p.set(model.StatusUnavailable, model.ErrUnavailable, "error.unavailable")
	}
	if !loggedIn {
		return p.set(model.StatusLoggedOut, model.ErrNotLoggedIn, "error.not_logged_in")
	}
	return p.set(model.StatusConnected, model.ErrNone, "")
}
func (p *Provider) set(status model.ConnectionStatus, code model.ErrorCode, key string) model.ConnectionState {
	state := model.ConnectionState{Status: status, Error: code, ErrorKey: key, Source: "Local LSP"}
	p.state.Set(state)
	return state
}
func (p *Provider) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	return p.group.Do(ctx, func() (model.UsageSnapshot, error) {
		state := p.Inspect(ctx)
		if state.Status != model.StatusConnected {
			return model.UsageSnapshot{}, model.SafeError{Code: state.Error, Key: state.ErrorKey}
		}
		raw, err := p.client.RetrieveUserQuotaSummary(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return model.UsageSnapshot{}, model.SafeError{Code: model.ErrTimeout, Key: "error.timeout"}
			}
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
		}
		snapshot, err := NormalizeQuota(raw, p.now())
		if err != nil {
			return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
		}
		return snapshot, nil
	})
}
func (p *Provider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) { return p.Refresh(ctx) }
func (p *Provider) Close() error {
	p.state.Set(model.ConnectionState{Status: model.StatusClosed})
	return p.client.Close()
}

type tier struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}
type bucket struct {
	ID                string   `json:"id"`
	RemainingFraction *float64 `json:"remainingFraction"`
	WindowMinutes     int      `json:"windowMinutes"`
	ResetTime         int64    `json:"resetTime"`
}
type quotaGroup struct {
	Name    string   `json:"name"`
	Buckets []bucket `json:"buckets"`
}
type quotaEnvelope struct {
	UserStatus struct {
		UserTier tier `json:"userTier"`
	} `json:"userStatus"`
	PlanStatus struct {
		PlanInfo struct {
			PlanName string `json:"planName"`
		} `json:"planInfo"`
	} `json:"planStatus"`
	QuotaGroups []quotaGroup `json:"quotaGroups"`
}

func NormalizeQuota(raw json.RawMessage, fetchedAt time.Time) (model.UsageSnapshot, error) {
	var envelope quotaEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return model.UsageSnapshot{}, err
	}
	tierValue := envelope.UserStatus.UserTier.Name
	if tierValue == "" {
		tierValue = envelope.UserStatus.UserTier.ID
	}
	if tierValue == "" {
		tierValue = envelope.PlanStatus.PlanInfo.PlanName
	}
	snapshot := model.UsageSnapshot{Provider: model.ProviderAntigravity, Plan: model.NormalizePlan(model.ProviderAntigravity, canonicalTier(tierValue)), FetchedAt: fetchedAt.UTC()}
	for _, group := range envelope.QuotaGroups {
		for _, entry := range group.Buckets {
			if entry.RemainingFraction == nil {
				continue
			}
			used, remaining := model.PercentPair(*entry.RemainingFraction*100, true)
			limit := model.UsageLimit{ID: group.Name + ":" + entry.ID, Label: group.Name, UsedPercent: used, RemainingPercent: remaining, WindowMinutes: entry.WindowMinutes}
			if entry.ResetTime != 0 {
				limit.ResetsAt = time.Unix(entry.ResetTime, 0).UTC()
			}
			snapshot.Limits = append(snapshot.Limits, limit)
		}
	}
	return snapshot, nil
}
func canonicalTier(value string) string {
	value = strings.ToUpper(strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(value)), " "))
	value = strings.TrimPrefix(value, "GOOGLE ")
	switch value {
	case "PRO":
		return "AI PRO"
	case "ULTRA":
		return "AI ULTRA"
	}
	return value
}

var _ model.Provider = (*Provider)(nil)
