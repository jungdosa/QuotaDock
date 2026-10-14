package codex

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/jungdosa/QuotaDock/internal/diagnostics"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
)

// Accounts retains the shared CLI provider and owns five isolated account slots.
// AdditionalProviders returns a copy; changing the count cannot race iteration.
type Accounts struct {
	*Provider
	extra    map[model.ProviderID]*Account
	count    atomic.Int32
	sharedMu sync.Mutex
}

type Account struct {
	provider             *Provider
	id                   model.ProviderID
	mu                   sync.Mutex // One lifecycle operation per account; Group alone is not a lock.
	active               atomic.Bool
	resetPending         atomic.Bool
	resetGeneration      atomic.Uint64
	mode                 atomic.Uint32
	parent               *Accounts
	isolationUnavailable bool
	cancelMu             sync.Mutex
	cancel               context.CancelFunc
}

func NewAccounts(root string, log process.LogFunc) *Accounts {
	a := &Accounts{Provider: New(NewAppServerTransport(log), MinimumCLIVersion), extra: make(map[model.ProviderID]*Account)}
	for _, id := range model.CodexAccountIDs() {
		home := ""
		if root != "" {
			home = filepath.Join(root, string(id))
		}
		t := NewAccountTransport(home, log)
		a.extra[id] = &Account{id: id, provider: New(t, MinimumCLIVersion), parent: a, isolationUnavailable: home == ""}
	}
	a.SetAccountCount(1)
	return a
}

func (a *Accounts) SetAccountCount(count int) {
	count = max(1, min(len(model.CodexAccountIDs()), count))
	a.count.Store(int32(count))
	for id, account := range a.extra {
		active := model.CodexAccountIndex(id) <= count
		wasActive := account.active.Swap(active)
		if wasActive && !active {
			account.cancelOperation()
			diagnostics.Go("codex_account_close", func() {
				account.mu.Lock()
				defer account.mu.Unlock()
				if !account.active.Load() {
					_ = account.provider.resetConnection()
					clearAccountUsage(account.provider)
					if account.sourceMode() == accountModeCLI {
						a.sharedMu.Lock()
						if !a.sharedCLIInUse() {
							_ = a.Provider.resetConnection()
							clearAccountUsage(a.Provider)
						}
						a.sharedMu.Unlock()
					}
				}
			})
		}
	}
}

func (a *Accounts) AdditionalProviders() map[model.ProviderID]model.Provider {
	result := make(map[model.ProviderID]model.Provider)
	for id, account := range a.extra {
		if id != model.ProviderCodex && account.active.Load() {
			result[id] = account
		}
	}
	return result
}

func (a *Accounts) SignIn(ctx context.Context, id model.ProviderID, open func(string) error) error {
	account := a.extra[id]
	if account == nil || !account.active.Load() || account.sourceMode() != accountModeAuth || account.isolationUnavailable {
		return model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	return account.signIn(ctx, open)
}

func (a *Accounts) CancelSignIn(id model.ProviderID) {
	if account := a.extra[id]; account != nil && account.sourceMode() == accountModeAuth {
		account.cancelOperation()
	}
}

const (
	accountModeDefault uint32 = iota
	accountModeCLI
	accountModeAuth
)

func (a *Account) sourceMode() uint32 {
	if mode := a.mode.Load(); mode != accountModeDefault {
		return mode
	}
	if a.id == model.ProviderCodex {
		return accountModeCLI
	}
	return accountModeAuth
}

// SetSourceMode applies a persisted route. An absent or unknown value keeps
// the original first-account CLI and additional-account Auth defaults.
func (a *Accounts) SetSourceMode(id model.ProviderID, value string) bool {
	account := a.extra[id]
	if account == nil {
		return false
	}
	mode := accountModeDefault
	switch value {
	case "cli":
		mode = accountModeCLI
	case "auth":
		mode = accountModeAuth
	}
	previous := account.sourceMode()
	if previous == mode || (mode == accountModeDefault && previous == account.defaultMode()) {
		account.mode.Store(mode)
		return false
	}
	account.mode.Store(mode)
	account.cancelOperation()
	account.resetPending.Store(true)
	generation := account.resetGeneration.Add(1)
	// The old operation releases the lifecycle lock before its connection is
	// reset. Other lanes may still own the shared CLI connection.
	diagnostics.Go("codex_source_reset", func() {
		account.mu.Lock()
		defer account.mu.Unlock()
		defer func() {
			if account.resetGeneration.Load() == generation {
				account.resetPending.Store(false)
			}
		}()
		_ = account.provider.resetConnection()
		clearAccountUsage(account.provider)
		if previous == accountModeCLI && !a.sharedCLIInUse() {
			a.sharedMu.Lock()
			if !a.sharedCLIInUse() {
				_ = a.Provider.resetConnection()
				clearAccountUsage(a.Provider)
			}
			a.sharedMu.Unlock()
		}
	})
	return true
}

func (a *Account) defaultMode() uint32 {
	if a.id == model.ProviderCodex {
		return accountModeCLI
	}
	return accountModeAuth
}

func (a *Accounts) sharedCLIInUse() bool {
	for _, account := range a.extra {
		if account.active.Load() && account.sourceMode() == accountModeCLI {
			return true
		}
	}
	return false
}

func clearAccountUsage(p *Provider) {
	p.mu.Lock()
	p.plan = model.PlanUnknown
	p.limits = rateEnvelope{}
	p.accountType = ""
	p.consecutiveFailures = 0
	p.reconnectAttempts = 0
	p.reconnectEligible = false
	p.mu.Unlock()
	p.state.Set(model.ConnectionState{Status: model.StatusInitializing})
}

func (a *Accounts) Inspect(ctx context.Context) model.ConnectionState {
	if first := a.extra[model.ProviderCodex]; first != nil {
		return first.Inspect(ctx)
	}
	return a.Provider.Inspect(ctx)
}
func (a *Accounts) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	if first := a.extra[model.ProviderCodex]; first != nil {
		return first.Refresh(ctx)
	}
	return a.Provider.Refresh(ctx)
}
func (a *Accounts) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	if first := a.extra[model.ProviderCodex]; first != nil {
		return first.Reconnect(ctx)
	}
	return a.Provider.Reconnect(ctx)
}

func (a *Accounts) Close() error {
	for _, account := range a.extra {
		account.active.Store(false)
		account.cancelOperation()
	}
	err := a.Provider.Close()
	for _, account := range a.extra {
		err = errors.Join(err, account.Close())
	}
	return err
}

func (a *Account) operationContext(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	a.cancelMu.Lock()
	a.cancel = cancel
	a.cancelMu.Unlock()
	if !a.active.Load() {
		cancel()
	}
	return ctx, func() { cancel(); a.cancelMu.Lock(); a.cancel = nil; a.cancelMu.Unlock() }
}
func (a *Account) cancelOperation() {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}
func accountBusy() error {
	return model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
}

func (a *Account) Inspect(ctx context.Context) model.ConnectionState {
	if a.resetPending.Load() {
		return model.ConnectionState{Status: model.StatusInitializing}
	}
	if !a.mu.TryLock() {
		return model.ConnectionState{Status: model.StatusInitializing}
	}
	defer a.mu.Unlock()
	if a.resetPending.Load() {
		return model.ConnectionState{Status: model.StatusInitializing}
	}
	if !a.active.Load() {
		return model.ConnectionState{Status: model.StatusClosed}
	}
	if a.sourceMode() == accountModeAuth && a.isolationUnavailable {
		return model.ConnectionState{Status: model.StatusUnavailable, Error: model.ErrUnavailable, ErrorKey: "error.unavailable"}
	}
	provider, unlock := a.selectedProvider()
	defer unlock()
	return provider.Inspect(ctx)
}
func (a *Account) selectedProvider() (*Provider, func()) {
	if a.parent != nil && a.sourceMode() == accountModeCLI {
		a.parent.sharedMu.Lock()
		return a.parent.Provider, a.parent.sharedMu.Unlock
	}
	return a.provider, func() {}
}
func (a *Account) read(ctx context.Context, reconnect bool) (model.UsageSnapshot, error) {
	if a.resetPending.Load() {
		return model.UsageSnapshot{}, accountBusy()
	}
	if !a.mu.TryLock() {
		return model.UsageSnapshot{}, accountBusy()
	}
	defer a.mu.Unlock()
	if a.resetPending.Load() {
		return model.UsageSnapshot{}, accountBusy()
	}
	if !a.active.Load() {
		return model.UsageSnapshot{}, accountBusy()
	}
	if a.sourceMode() == accountModeAuth && a.isolationUnavailable {
		return model.UsageSnapshot{}, accountBusy()
	}
	ctx, done := a.operationContext(ctx)
	defer done()
	mode := a.sourceMode()
	if a.resetPending.Load() {
		return model.UsageSnapshot{}, accountBusy()
	}
	provider, unlock := a.selectedProvider()
	defer unlock()
	var snapshot model.UsageSnapshot
	var err error
	if reconnect {
		snapshot, err = provider.Reconnect(ctx)
	} else {
		snapshot, err = provider.Refresh(ctx)
	}
	if mode != a.sourceMode() {
		return model.UsageSnapshot{}, accountBusy()
	}
	snapshot.Provider = a.id
	return snapshot, err
}
func (a *Account) Refresh(ctx context.Context) (model.UsageSnapshot, error) {
	return a.read(ctx, false)
}
func (a *Account) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return a.read(ctx, true)
}
func (a *Account) Close() error {
	a.cancelOperation()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provider.Close()
}
func (a *Account) signIn(ctx context.Context, open func(string) error) error {
	if err := ctx.Err(); err != nil {
		return safeTransportError(err)
	}
	// The authoritative duplicate guard lives here, independent of UI buttons.
	if !a.mu.TryLock() {
		return accountBusy()
	}
	defer a.mu.Unlock()
	if !a.active.Load() || a.sourceMode() != accountModeAuth || a.resetPending.Load() {
		return accountBusy()
	}
	ctx, done := a.operationContext(ctx)
	defer done()
	if a.sourceMode() != accountModeAuth || a.resetPending.Load() {
		return accountBusy()
	}
	t, ok := a.provider.transport.(interface {
		SignIn(context.Context, func(string) error) error
	})
	if !ok {
		return accountBusy()
	}
	_ = a.provider.resetConnection()
	// A slot can be signed into a different identity. Never reuse its old data.
	a.provider.mu.Lock()
	a.provider.plan = model.PlanUnknown
	a.provider.limits = rateEnvelope{}
	a.provider.accountType = ""
	a.provider.consecutiveFailures = 0
	a.provider.reconnectAttempts = 0
	a.provider.reconnectEligible = false
	a.provider.mu.Unlock()
	err := t.SignIn(ctx, open)
	_ = a.provider.resetConnection() // Next read starts a fresh handshake after login.
	return err
}
