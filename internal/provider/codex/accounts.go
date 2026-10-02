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

// Accounts retains the existing CLI provider and owns isolated added accounts.
// AdditionalProviders returns a copy; changing the count cannot race iteration.
type Accounts struct {
	*Provider
	extra map[model.ProviderID]*Account
	count atomic.Int32
}

type Account struct {
	provider *Provider
	id       model.ProviderID
	mu       sync.Mutex // One lifecycle operation per account; Group alone is not a lock.
	active   atomic.Bool
	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

func NewAccounts(root string, log process.LogFunc) *Accounts {
	a := &Accounts{Provider: New(NewAppServerTransport(log), MinimumCLIVersion), extra: make(map[model.ProviderID]*Account)}
	if root != "" {
		for _, id := range model.CodexAccountIDs()[1:] {
			t := NewAccountTransport(filepath.Join(root, string(id)), log)
			a.extra[id] = &Account{id: id, provider: New(t, MinimumCLIVersion)}
		}
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
				}
			})
		}
	}
}

func (a *Accounts) AdditionalProviders() map[model.ProviderID]model.Provider {
	result := make(map[model.ProviderID]model.Provider)
	for id, account := range a.extra {
		if account.active.Load() {
			result[id] = account
		}
	}
	return result
}

func (a *Accounts) SignIn(ctx context.Context, id model.ProviderID, open func(string) error) error {
	account := a.extra[id]
	if account == nil || !account.active.Load() {
		return model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	return account.signIn(ctx, open)
}

func (a *Accounts) CancelSignIn(id model.ProviderID) {
	if account := a.extra[id]; account != nil {
		account.cancelOperation()
	}
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
	if !a.mu.TryLock() {
		return model.ConnectionState{Status: model.StatusInitializing}
	}
	defer a.mu.Unlock()
	if !a.active.Load() {
		return model.ConnectionState{Status: model.StatusClosed}
	}
	return a.provider.Inspect(ctx)
}
func (a *Account) read(ctx context.Context, reconnect bool) (model.UsageSnapshot, error) {
	if !a.mu.TryLock() {
		return model.UsageSnapshot{}, accountBusy()
	}
	defer a.mu.Unlock()
	if !a.active.Load() {
		return model.UsageSnapshot{}, accountBusy()
	}
	ctx, done := a.operationContext(ctx)
	defer done()
	var snapshot model.UsageSnapshot
	var err error
	if reconnect {
		snapshot, err = a.provider.Reconnect(ctx)
	} else {
		snapshot, err = a.provider.Refresh(ctx)
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
	if !a.active.Load() {
		return accountBusy()
	}
	ctx, done := a.operationContext(ctx)
	defer done()
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
