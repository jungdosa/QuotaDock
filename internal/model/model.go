// Package model defines the provider-neutral data exposed to application layers.
package model

import (
	"context"
	"strings"
	"sync"
	"time"
)

type Provider interface {
	Inspect(ctx context.Context) ConnectionState
	Refresh(ctx context.Context) (UsageSnapshot, error)
	Reconnect(ctx context.Context) (UsageSnapshot, error)
	Close() error
}

// ProviderCollection exposes additional independently rendered accounts that
// share a root provider's process or browser infrastructure. The coordinator
// expands these accounts for refreshes but closes only the root provider, so a
// shared resource is never torn down twice.
type ProviderCollection interface {
	AdditionalProviders() map[ProviderID]Provider
}

type ProviderID string

const (
	ProviderClaude      ProviderID = "claude"
	ProviderClaudeAuth  ProviderID = "claude-auth"
	ProviderClaude3     ProviderID = "claude-3"
	ProviderClaude4     ProviderID = "claude-4"
	ProviderClaude5     ProviderID = "claude-5"
	ProviderCodex       ProviderID = "codex"
	ProviderAntigravity ProviderID = "antigravity"
	ProviderGrok        ProviderID = "grok"
)

// ClaudeAccountIDs lists the Claude accounts in display order. The first two
// keep the names they have always had, because every settings file written
// so far keys the second account's label, colour and sign-in route on them;
// the rest are numbered.
func ClaudeAccountIDs() []ProviderID {
	return []ProviderID{ProviderClaude, ProviderClaudeAuth, ProviderClaude3, ProviderClaude4, ProviderClaude5}
}

// IsClaudeAccount reports whether id names any of the Claude accounts.
func IsClaudeAccount(id ProviderID) bool {
	for _, account := range ClaudeAccountIDs() {
		if id == account {
			return true
		}
	}
	return false
}

// ClaudeAccountIndex is the account's position, starting at one, or zero for
// anything that is not a Claude account.
func ClaudeAccountIndex(id ProviderID) int {
	for index, account := range ClaudeAccountIDs() {
		if id == account {
			return index + 1
		}
	}
	return 0
}

type ConnectionStatus string

const (
	StatusUnavailable  ConnectionStatus = "unavailable"
	StatusLoggedOut    ConnectionStatus = "logged_out"
	StatusConnected    ConnectionStatus = "connected"
	StatusOutdated     ConnectionStatus = "outdated"
	StatusInitializing ConnectionStatus = "initializing"
	StatusError        ConnectionStatus = "error"
	StatusClosed       ConnectionStatus = "closed"
)

type ErrorCode string

const (
	ErrNone                    ErrorCode = ""
	ErrCLINotInstalled         ErrorCode = "cli_not_installed"
	ErrNotLoggedIn             ErrorCode = "not_logged_in"
	ErrCLIOutdated             ErrorCode = "cli_outdated"
	ErrInitialization          ErrorCode = "initialization_failed"
	ErrTimeout                 ErrorCode = "timeout"
	ErrInvalidResponse         ErrorCode = "invalid_response"
	ErrProcessExited           ErrorCode = "process_exited"
	ErrUnavailable             ErrorCode = "unavailable"
	ErrUsageUnavailable        ErrorCode = "usage_unavailable"
	ErrSubscriptionUnavailable ErrorCode = "subscription_unavailable"
	// ErrQuotaExhausted marks the case where the CLI is installed, logged in and
	// answered initialize, but then stops answering usage queries - the pattern
	// seen when the account has spent its quota.
	ErrQuotaExhausted ErrorCode = "quota_exhausted"
)

// SafeError never contains provider stderr.
type SafeError struct {
	Code ErrorCode
	Key  string
}

func (e SafeError) Error() string {
	if e.Key != "" {
		return e.Key
	}
	return string(e.Code)
}

type ConnectionState struct {
	Status     ConnectionStatus
	Error      ErrorCode
	ErrorKey   string
	CLIPath    string
	CLIVersion string
	Source     string
}

// SourceWebSignIn labels a connection served by the embedded browser sign-in
// rather than a CLI. Both the provider that sets it and the UI that reads it
// share this constant so the two never drift apart.
const SourceWebSignIn = "Web sign-in"

type Plan string

const PlanUnknown Plan = "UNKNOWN"

type UsageLimit struct {
	ID               string
	Label            string
	UsedPercent      float64
	RemainingPercent float64
	WindowMinutes    int
	ResetsAt         time.Time
	// UsageUnknown marks a limit whose consumption the provider cannot report,
	// leaving only the reset window. It defaults to false, so every existing
	// provider keeps reporting a known figure; only a provider that explicitly
	// sets it (Grok, when the usage field is absent) shows a dash instead of 0%.
	UsageUnknown bool
}

type Credits struct {
	Balance   float64
	Unlimited bool
	// Spend is populated only when a provider reports a paid extra-usage meter.
	Spend *CreditSpend
	// ResetCredits is the number of rate-limit reset passes the account holds.
	// Zero means none, which is also what an account without the feature
	// reports, so the two are indistinguishable and both hide the surface.
	ResetCredits int
	// HasCredits mirrors the server's own flag rather than being derived from
	// Balance: a zero balance with the feature enabled is not the same thing as
	// the feature being absent.
	HasCredits bool
}

type CreditSpend struct {
	Used     float64
	Limit    float64
	Currency string
	Percent  float64
}

// UsageSnapshot deliberately has no representation for authentication data,
// account identifiers, command lines, or local endpoints.
type UsageSnapshot struct {
	Provider  ProviderID
	Plan      Plan
	Limits    []UsageLimit
	Credits   *Credits
	FetchedAt time.Time
}

func PercentPair(value float64, valueIsRemaining bool) (used, remaining float64) {
	value = ClampPercent(value)
	if valueIsRemaining {
		return 100 - value, value
	}
	return value, 100 - value
}

func ClampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

type AlertLevel string

const (
	AlertNormal  AlertLevel = "normal"
	AlertWarning AlertLevel = "warning"
	AlertDanger  AlertLevel = "danger"
)

func ClassifyUsage(usedPercent, warningThreshold, dangerThreshold float64) AlertLevel {
	usedPercent = ClampPercent(usedPercent)
	warningThreshold = ClampPercent(warningThreshold)
	dangerThreshold = ClampPercent(dangerThreshold)
	if dangerThreshold < warningThreshold {
		dangerThreshold = warningThreshold
	}
	if usedPercent >= dangerThreshold {
		return AlertDanger
	}
	if usedPercent >= warningThreshold {
		return AlertWarning
	}
	return AlertNormal
}

var planAllowlists = map[ProviderID]map[string]Plan{
	ProviderClaude:      {"PRO": "PRO", "MAX": "MAX", "MAX 5X": "MAX 5X", "MAX 20X": "MAX 20X", "TEAM": "TEAM", "ENTERPRISE": "ENTERPRISE", "FREE": "FREE"},
	ProviderClaudeAuth:  {"PRO": "PRO", "MAX": "MAX", "MAX 5X": "MAX 5X", "MAX 20X": "MAX 20X", "TEAM": "TEAM", "ENTERPRISE": "ENTERPRISE", "FREE": "FREE"},
	ProviderAntigravity: {"AI PRO": "AI PRO", "AI ULTRA": "AI ULTRA", "AI ULTRA 5X": "AI ULTRA 5X", "AI ULTRA 20X": "AI ULTRA 20X", "ENTERPRISE": "ENTERPRISE", "STANDARD": "STANDARD", "FREE": "FREE"},
	ProviderCodex: {
		"FREE":                            "FREE",
		"GO":                              "GO",
		"PLUS":                            "PLUS",
		"PRO":                             "PRO",
		"PROLITE":                         "PRO LITE",
		"TEAM":                            "TEAM",
		"SELF SERVE BUSINESS USAGE BASED": "BUSINESS",
		"BUSINESS":                        "BUSINESS",
		"ENTERPRISE CBP USAGE BASED":      "ENTERPRISE",
		"ENTERPRISE":                      "ENTERPRISE",
		"EDU":                             "EDU",
	},
}

func NormalizePlan(provider ProviderID, raw string) Plan {
	key := strings.ToUpper(strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(raw)), " "))
	// Every Claude account reads the same plans; the table is keyed on the
	// first account and the rest are folded onto it.
	if IsClaudeAccount(provider) {
		provider = ProviderClaude
	}
	if plan, ok := planAllowlists[provider][key]; ok {
		return plan
	}
	return PlanUnknown
}

func UsageWindowLabel(windowMinutes int) string {
	switch {
	case windowMinutes <= 0:
		return "Other"
	case windowMinutes <= 360:
		return "Session"
	case windowMinutes <= 10080:
		return "Weekly"
	default:
		return "Monthly"
	}
}

type StateMachine struct {
	mu    sync.RWMutex
	state ConnectionState
}

func NewStateMachine() *StateMachine {
	return &StateMachine{state: ConnectionState{Status: StatusUnavailable}}
}
func (s *StateMachine) Current() ConnectionState { s.mu.RLock(); defer s.mu.RUnlock(); return s.state }
func (s *StateMachine) Set(next ConnectionState) { s.mu.Lock(); defer s.mu.Unlock(); s.state = next }
