// Package codex implements the official app-server initialization contract and normalization.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

type Transport interface {
	Version(context.Context) (string, error)
	Request(context.Context, string, any) (json.RawMessage, error)
	Notify(context.Context, string, any) error
	Invalidate()
	Close() error
}

type updateSource interface {
	SetRateLimitsUpdatedHandler(func(json.RawMessage))
}

type executableSource interface {
	ExecutablePath() (string, error)
}

const maxAutomaticReconnects = 3

type Provider struct {
	transport           Transport
	minimumVersion      string
	state               *model.StateMachine
	group               process.Group[model.UsageSnapshot]
	mu                  sync.Mutex
	initialized         bool
	handshakeOK         bool
	accountType         string
	plan                model.Plan
	limits              rateEnvelope
	consecutiveFailures int
	reconnectAttempts   int
	reconnectEligible   bool
	now                 func() time.Time
}

func New(transport Transport, minimumVersion string) *Provider {
	provider := &Provider{transport: transport, minimumVersion: minimumVersion, state: model.NewStateMachine(), now: time.Now}
	if source, ok := transport.(updateSource); ok {
		source.SetRateLimitsUpdatedHandler(func(raw json.RawMessage) {
			_ = provider.ApplyRateLimitsUpdated(raw)
		})
	}
	return provider
}

func (p *Provider) Inspect(ctx context.Context) model.ConnectionState {
	version, err := p.transport.Version(ctx)
	if errors.Is(err, shared.ErrNotInstalled) {
		return p.set(model.StatusUnavailable, model.ErrCLINotInstalled, "error.cli_not_installed")
	}
	if err != nil {
		return p.set(model.StatusError, model.ErrUnavailable, "error.unavailable")
	}
	if p.minimumVersion != "" && !shared.VersionAtLeast(version, p.minimumVersion) {
		return p.set(model.StatusOutdated, model.ErrCLIOutdated, "error.cli_outdated")
	}
	p.mu.Lock()
	initialized := p.initialized
	p.mu.Unlock()
	path := ""
	if source, ok := p.transport.(executableSource); ok {
		path, _ = source.ExecutablePath()
	}
	if initialized {
		return p.setCLI(model.StatusConnected, model.ErrNone, "", path, version)
	}
	return p.setCLI(model.StatusInitializing, model.ErrNone, "", path, version)
}

func (p *Provider) set(status model.ConnectionStatus, code model.ErrorCode, key string) model.ConnectionState {
	state := model.ConnectionState{Status: status, Error: code, ErrorKey: key}
	p.state.Set(state)
	return state
}

func (p *Provider) setCLI(status model.ConnectionStatus, code model.ErrorCode, key, path, version string) model.ConnectionState {
	state := model.ConnectionState{Status: status, Error: code, ErrorKey: key, CLIPath: path, CLIVersion: version}
	p.state.Set(state)
	return state
}

func (p *Provider) initialize(ctx context.Context) error {
	state := p.Inspect(ctx)
	if state.Status == model.StatusUnavailable || state.Status == model.StatusOutdated || state.Status == model.StatusError {
		return model.SafeError{Code: state.Error, Key: state.ErrorKey}
	}
	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()
	if _, err := p.transport.Request(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "QuotaDock", "version": "0.4.0"}}); err != nil {
		return p.recoverableTransportError(err, true)
	}
	// The handshake succeeded, so the CLI is installed, logged in and talking to
	// us. From here on a silent timeout is not a setup failure: it is the pattern
	// the app-server shows once the account has spent its quota.
	p.mu.Lock()
	p.handshakeOK = true
	p.mu.Unlock()
	if err := p.transport.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return p.recoverableTransportError(err, false)
	}
	accountRaw, err := p.transport.Request(ctx, "account/read", map[string]any{})
	if err != nil {
		return p.postHandshakeError(err)
	}
	var account struct {
		Account *struct {
			Type     string `json:"type"`
			LoggedIn *bool  `json:"loggedIn"`
			PlanType string `json:"planType"`
		} `json:"account"`
		PlanType string `json:"planType"`
	}
	if json.Unmarshal(accountRaw, &account) != nil {
		return model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	loggedIn := account.Account != nil
	if loggedIn && account.Account.LoggedIn != nil {
		loggedIn = *account.Account.LoggedIn
	}
	if !loggedIn {
		return model.SafeError{Code: model.ErrNotLoggedIn, Key: "error.not_logged_in"}
	}
	planType := account.Account.PlanType
	if planType == "" {
		planType = account.PlanType
	}
	p.mu.Lock()
	p.accountType = account.Account.Type
	p.plan = model.NormalizePlan(model.ProviderCodex, planType)
	p.initialized = true
	p.mu.Unlock()
	p.set(model.StatusConnected, model.ErrNone, "")
	return nil
}

// postHandshakeError classifies a failure that happened *after* a successful
// initialize handshake. A timeout there means the CLI is installed, logged in
// and responsive, yet refuses to answer usage queries — what we observe when the
// account has spent its quota. Other failures keep their original meaning.
func (p *Provider) postHandshakeError(err error) error {
	p.mu.Lock()
	handshake := p.handshakeOK
	p.mu.Unlock()
	if !handshake {
		return p.recoverableTransportError(err, true)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, process.ErrTimeout) {
		return model.SafeError{Code: model.ErrQuotaExhausted, Key: "error.quota_exhausted"}
	}
	return p.recoverableTransportError(err, false)
}

func (p *Provider) recoverableTransportError(err error, invalidateTimeout bool) error {
	safeErr := safeTransportError(err)
	var classified model.SafeError
	if errors.Is(err, process.ErrProcessExited) ||
		(!errors.Is(err, process.ErrRPCResponse) && errors.As(safeErr, &classified) &&
			(classified.Code == model.ErrInitialization ||
				(invalidateTimeout && classified.Code == model.ErrTimeout))) {
		p.transport.Invalidate()
		p.mu.Lock()
		p.initialized = false
		p.handshakeOK = false
		p.mu.Unlock()
	}
	return safeErr
}

func safeTransportError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, process.ErrTimeout):
		return model.SafeError{Code: model.ErrTimeout, Key: "error.timeout"}
	case errors.Is(err, process.ErrProcessExited):
		return model.SafeError{Code: model.ErrProcessExited, Key: "error.process_exited"}
	default:
		return model.SafeError{Code: model.ErrInitialization, Key: "error.initialization_failed"}
	}
}

func (p *Provider) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	return p.group.Do(ctx, func() (model.UsageSnapshot, error) {
		// The reset error is deliberately not fatal here. Close reports
		// process_exited whenever the app-server already died - the very case a
		// reconnect exists to repair - and resetConnection clears the handshake
		// state regardless. Aborting on it would spend the whole reconnect
		// budget on refreshes that never actually retried.
		_ = p.autoReconnectIfNeeded()
		return p.refreshAndRecord(ctx)
	})
}

func (p *Provider) refreshAndRecord(ctx context.Context) (model.UsageSnapshot, error) {
	snapshot, err := p.refresh(ctx)
	p.recordRefreshResult(err)
	return snapshot, err
}

func (p *Provider) refresh(ctx context.Context) (model.UsageSnapshot, error) {
	if err := p.initialize(ctx); err != nil {
		return model.UsageSnapshot{}, err
	}
	p.mu.Lock()
	accountType := p.accountType
	p.mu.Unlock()
	if accountType == "apiKey" {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrSubscriptionUnavailable, Key: "error.codex_api_key_no_limits"}
	}
	rateRaw, err := p.transport.Request(ctx, "account/rateLimits/read", map[string]any{})
	if err != nil {
		p.mu.Lock()
		p.initialized = false
		p.mu.Unlock()
		return model.UsageSnapshot{}, p.postHandshakeError(err)
	}
	var limits rateEnvelope
	if json.Unmarshal(rateRaw, &limits) != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	p.mu.Lock()
	p.limits = limits
	if normalized := model.NormalizePlan(model.ProviderCodex, limits.RateLimits.PlanType); normalized != model.PlanUnknown {
		p.plan = normalized
	}
	snapshot := snapshotFrom(p.plan, p.limits, p.now())
	p.mu.Unlock()
	return snapshot, nil
}

func (p *Provider) autoReconnectIfNeeded() error {
	p.mu.Lock()
	if p.consecutiveFailures == 0 || !p.reconnectEligible || p.reconnectAttempts >= maxAutomaticReconnects {
		p.mu.Unlock()
		return nil
	}
	p.reconnectAttempts++
	attempt := p.reconnectAttempts
	failures := p.consecutiveFailures
	p.mu.Unlock()

	err := p.resetConnection()
	slog.Debug("session.reconnect", "provider", "codex", "attempt", attempt, "failures", failures, "ok", err == nil)
	return err
}

func (p *Provider) recordRefreshResult(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.consecutiveFailures = 0
		p.reconnectAttempts = 0
		p.reconnectEligible = false
		return
	}
	p.consecutiveFailures++
	p.reconnectEligible = reconnectableRefreshError(err, p.accountType)
}

func reconnectableRefreshError(err error, accountType string) bool {
	var safe model.SafeError
	if !errors.As(err, &safe) {
		return true
	}
	switch safe.Code {
	case model.ErrCLINotInstalled, model.ErrCLIOutdated, model.ErrNotLoggedIn:
		return false
	case model.ErrSubscriptionUnavailable:
		return accountType != "apiKey"
	default:
		return true
	}
}

func (p *Provider) resetConnection() error {
	// Closing a transport whose process already exited reports an error, yet the
	// goal - leaving no live session behind - is met either way. Returning early
	// on that error used to leave initialized/handshakeOK set, so the next
	// refresh skipped the handshake and talked to a session that no longer
	// existed. Clear the flags unconditionally; the error still reaches the
	// caller so Reconnect and the reconnect log keep reporting it.
	err := p.transport.Close()
	p.mu.Lock()
	p.initialized = false
	p.handshakeOK = false
	p.mu.Unlock()
	return err
}

func (p *Provider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	if err := p.resetConnection(); err != nil {
		return model.UsageSnapshot{}, model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	return p.group.Do(ctx, func() (model.UsageSnapshot, error) {
		return p.refreshAndRecord(ctx)
	})
}

func (p *Provider) Close() error {
	p.state.Set(model.ConnectionState{Status: model.StatusClosed})
	return p.transport.Close()
}

type rateWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	WindowDurationMins *int     `json:"windowDurationMins"`
	ResetsAt           *int64   `json:"resetsAt"`
}

type creditState struct {
	Balance    json.RawMessage `json:"balance"`
	Unlimited  *bool           `json:"unlimited"`
	HasCredits *bool           `json:"hasCredits"`
}

type resetCreditState struct {
	AvailableCount int `json:"availableCount"`
}

type rateSnapshot struct {
	Primary   *rateWindow  `json:"primary"`
	Secondary *rateWindow  `json:"secondary"`
	Credits   *creditState `json:"credits"`
	LimitID   string       `json:"limitId"`
	PlanType  string       `json:"planType"`
}

type rateBucket struct {
	Snapshot rateSnapshot
	Legacy   *rateWindow
}

func (b *rateBucket) UnmarshalJSON(raw []byte) error {
	var probe struct {
		UsedPercent json.RawMessage `json:"usedPercent"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return err
	}
	if len(probe.UsedPercent) > 0 {
		var window rateWindow
		if err := json.Unmarshal(raw, &window); err != nil {
			return err
		}
		b.Legacy = &window
		return nil
	}
	return json.Unmarshal(raw, &b.Snapshot)
}

type rateEnvelope struct {
	RateLimitResetCredits resetCreditState       `json:"rateLimitResetCredits"`
	RateLimits            rateSnapshot           `json:"rateLimits"`
	RateLimitsByLimitID   map[string]*rateBucket `json:"rateLimitsByLimitId"`
	Credits               *creditState           `json:"credits"`
}

func (p *Provider) ApplyRateLimitsUpdated(raw json.RawMessage) error {
	var update rateEnvelope
	if err := json.Unmarshal(raw, &update); err != nil {
		return model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limits.RateLimits = mergeSnapshot(p.limits.RateLimits, update.RateLimits)
	if normalized := model.NormalizePlan(model.ProviderCodex, update.RateLimits.PlanType); normalized != model.PlanUnknown {
		p.plan = normalized
	}
	if p.limits.RateLimitsByLimitID == nil {
		p.limits.RateLimitsByLimitID = make(map[string]*rateBucket)
	}
	if update.RateLimits.LimitID != "" {
		current := p.limits.RateLimitsByLimitID[update.RateLimits.LimitID]
		if current == nil {
			current = &rateBucket{}
		}
		current.Snapshot = mergeSnapshot(current.Snapshot, update.RateLimits)
		p.limits.RateLimitsByLimitID[update.RateLimits.LimitID] = current
	}
	for id, bucket := range update.RateLimitsByLimitID {
		if bucket == nil {
			continue
		}
		current := p.limits.RateLimitsByLimitID[id]
		if current == nil {
			current = &rateBucket{}
		}
		if bucket.Legacy != nil {
			current.Legacy = mergeWindow(current.Legacy, bucket.Legacy)
		} else {
			current.Snapshot = mergeSnapshot(current.Snapshot, bucket.Snapshot)
		}
		p.limits.RateLimitsByLimitID[id] = current
	}
	if update.Credits != nil {
		p.limits.Credits = mergeCredits(p.limits.Credits, update.Credits)
	}
	return nil
}

func mergeSnapshot(current, update rateSnapshot) rateSnapshot {
	current.Primary = mergeWindow(current.Primary, update.Primary)
	current.Secondary = mergeWindow(current.Secondary, update.Secondary)
	if update.Credits != nil {
		current.Credits = mergeCredits(current.Credits, update.Credits)
	}
	if update.LimitID != "" {
		current.LimitID = update.LimitID
	}
	if update.PlanType != "" {
		current.PlanType = update.PlanType
	}
	return current
}

func mergeWindow(current, update *rateWindow) *rateWindow {
	if update == nil {
		return current
	}
	if current == nil {
		current = &rateWindow{}
	}
	copy := *current
	if update.UsedPercent != nil {
		copy.UsedPercent = update.UsedPercent
	}
	if update.WindowDurationMins != nil {
		copy.WindowDurationMins = update.WindowDurationMins
	}
	if update.ResetsAt != nil {
		copy.ResetsAt = update.ResetsAt
	}
	return &copy
}

func mergeCredits(current, update *creditState) *creditState {
	if update == nil {
		return current
	}
	if current == nil {
		current = &creditState{}
	}
	copy := *current
	if len(update.Balance) > 0 && string(update.Balance) != "null" {
		copy.Balance = append(json.RawMessage(nil), update.Balance...)
	}
	if update.Unlimited != nil {
		copy.Unlimited = update.Unlimited
	}
	if update.HasCredits != nil {
		copy.HasCredits = update.HasCredits
	}
	return &copy
}

func snapshotFrom(plan model.Plan, envelope rateEnvelope, fetchedAt time.Time) model.UsageSnapshot {
	snapshot := model.UsageSnapshot{Provider: model.ProviderCodex, Plan: plan, FetchedAt: fetchedAt.UTC()}
	appendWindow := func(id string, window *rateWindow) {
		if window == nil || window.UsedPercent == nil {
			return
		}
		used, remaining := model.PercentPair(*window.UsedPercent, false)
		limit := model.UsageLimit{ID: id, UsedPercent: used, RemainingPercent: remaining}
		if window.WindowDurationMins != nil {
			limit.WindowMinutes = *window.WindowDurationMins
		}
		limit.Label = model.UsageWindowLabel(limit.WindowMinutes)
		if window.ResetsAt != nil {
			limit.ResetsAt = time.Unix(*window.ResetsAt, 0).UTC()
		}
		snapshot.Limits = append(snapshot.Limits, limit)
	}

	ids := make([]string, 0, len(envelope.RateLimitsByLimitID))
	hasNestedBuckets := false
	for id, bucket := range envelope.RateLimitsByLimitID {
		ids = append(ids, id)
		if bucket != nil && bucket.Legacy == nil {
			hasNestedBuckets = true
		}
	}
	sort.Strings(ids)
	if !hasNestedBuckets {
		appendWindow("primary", envelope.RateLimits.Primary)
		appendWindow("secondary", envelope.RateLimits.Secondary)
	}
	for _, id := range ids {
		bucket := envelope.RateLimitsByLimitID[id]
		if bucket == nil {
			continue
		}
		if bucket.Legacy != nil {
			appendWindow(id, bucket.Legacy)
			continue
		}
		appendWindow(id+":primary", bucket.Snapshot.Primary)
		appendWindow(id+":secondary", bucket.Snapshot.Secondary)
	}
	// Collapse windows that share the same duration (e.g. two 7-day weekly
	// buckets) into a single row, keeping the one closest to its limit. This
	// matches the reference widget, which shows one weekly bar per provider.
	if len(snapshot.Limits) > 1 {
		seen := make(map[int]int, len(snapshot.Limits))
		deduped := make([]model.UsageLimit, 0, len(snapshot.Limits))
		for _, limit := range snapshot.Limits {
			if limit.WindowMinutes > 0 {
				if idx, ok := seen[limit.WindowMinutes]; ok {
					if limit.UsedPercent > deduped[idx].UsedPercent {
						deduped[idx] = limit
					}
					continue
				}
				seen[limit.WindowMinutes] = len(deduped)
			}
			deduped = append(deduped, limit)
		}
		snapshot.Limits = deduped
	}
	sort.SliceStable(snapshot.Limits, func(i, j int) bool {
		left, right := snapshot.Limits[i].WindowMinutes, snapshot.Limits[j].WindowMinutes
		if left <= 0 {
			return false
		}
		if right <= 0 {
			return true
		}
		return left < right
	})
	credits := envelope.Credits
	if envelope.RateLimits.Credits != nil {
		credits = envelope.RateLimits.Credits
	}
	if credits == nil {
		for _, id := range ids {
			bucket := envelope.RateLimitsByLimitID[id]
			if bucket != nil && bucket.Snapshot.Credits != nil {
				credits = bucket.Snapshot.Credits
				break
			}
		}
	}
	resetCredits := envelope.RateLimitResetCredits.AvailableCount
	if resetCredits < 0 {
		resetCredits = 0
	}
	if credits != nil || resetCredits > 0 {
		value := &model.Credits{ResetCredits: resetCredits}
		if credits != nil {
			if balance, ok := parseBalance(credits.Balance); ok {
				value.Balance = balance
			}
			if credits.Unlimited != nil {
				value.Unlimited = *credits.Unlimited
			}
			if credits.HasCredits != nil {
				value.HasCredits = *credits.HasCredits
			}
		}
		snapshot.Credits = value
	}
	return snapshot
}

func parseBalance(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return number, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	return number, err == nil
}

var _ model.Provider = (*Provider)(nil)
