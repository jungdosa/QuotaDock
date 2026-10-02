package ui

import (
	"errors"
	"fmt"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func accountCount(cfg settings.Config, id model.ProviderID) int {
	if model.IsCodexAccount(id) {
		return max(1, min(settings.MaxCodexAccounts, cfg.CodexAccounts))
	}
	if model.IsClaudeAccount(id) {
		return claudeAccountCount(cfg)
	}
	return 1
}
func adjustAccountCount(cfg *settings.Config, id model.ProviderID, delta int) {
	if model.IsCodexAccount(id) {
		cfg.CodexAccounts = accountCount(*cfg, id) + delta
		return
	}
	cfg.ClaudeAccounts = accountCount(*cfg, id) + delta
	cfg.ShowClaudeAuth = cfg.ClaudeAccounts >= 2
}
func accountLabelsVisible(cfg settings.Config, id model.ProviderID) bool {
	return accountCount(cfg, id) >= 2 && ((model.IsClaudeAccount(id) && cfg.ShowClaude) || (model.IsCodexAccount(id) && cfg.ShowCodex))
}
func accountDisplayName(cfg settings.Config, id model.ProviderID) string {
	if model.IsClaudeAccount(id) {
		return claudeAccountDisplayName(cfg, id, accountLabelsVisible(cfg, id))
	}
	if !model.IsCodexAccount(id) {
		return string(id)
	}
	if accountCount(cfg, id) == 1 {
		return "Codex"
	}
	if label := cfg.AccountLabels[string(id)]; label != "" {
		return label
	}
	if id == model.ProviderCodex {
		return "Codex CLI"
	}
	return fmt.Sprintf("Codex %d", model.CodexAccountIndex(id))
}
func compactAccountHeader(lanes []LaneState, id model.ProviderID) bool {
	family := model.AccountFamily(id)
	if family == "" {
		return false
	}
	count := 0
	for _, lane := range lanes {
		if model.AccountFamily(lane.Provider) == family {
			count++
		}
	}
	return count >= 2
}

// ForgetAccount drops readings before a new identity signs into the same slot.
// It also prevents an in-flight refresh from republishing the prior identity.
func (c *Controller) ForgetAccount(id model.ProviderID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accountEpoch == nil {
		c.accountEpoch = make(map[model.ProviderID]uint64)
	}
	c.accountEpoch[id]++
	for i := range c.state.Lanes {
		if c.state.Lanes[i].Provider == id {
			c.state.Lanes[i] = LaneState{Provider: id, Name: string(id), Status: model.StatusLoggedOut}
		}
	}
}

func (v *View) SetSigningIn(id model.ProviderID, busy bool) {
	if v.signingIn == nil {
		v.signingIn = make(map[model.ProviderID]bool)
	}
	v.signingIn[id] = busy
	// Rebuild the cards so the open panel exposes cancel while login is pending.
	v.connectionCache = nil
	v.syncConnections()
	v.resizeCurrentWidget()
}

func (v *View) ShowSignInError(err error) {
	key := "error.unavailable"
	var safe model.SafeError
	if errors.As(err, &safe) {
		key = safe.Key
	}
	popup := widget.NewModalPopUp(container.NewVBox(widget.NewLabel(v.text(key))), v.Canvas)
	close := NewSmallButton(v.text("connection.panel_close"), "", popup.Hide, v.colors)
	popup.Content = container.NewVBox(widget.NewLabel(v.text(key)), close)
	popup.Show()
}
