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
	before := accountCount(*cfg, id)
	if model.IsCodexAccount(id) {
		cfg.CodexAccounts = before + delta
	} else {
		cfg.ClaudeAccounts = before + delta
		cfg.ShowClaudeAuth = cfg.ClaudeAccounts >= 2
	}
	if delta > 0 {
		placeAddedAccount(cfg, id, before)
	}
}

// placeAddedAccount moves the account that just became visible to directly
// below the last one that already was. The saved order names every account,
// shown or not, so a hidden one can sit anywhere in it; left there, the new
// card and the add and remove buttons it carries would turn up far from the
// card whose button was pressed.
func placeAddedAccount(cfg *settings.Config, id model.ProviderID, shown int) {
	family := model.ClaudeAccountIDs()
	if model.IsCodexAccount(id) {
		family = model.CodexAccountIDs()
	}
	if shown < 1 || shown >= len(family) {
		return
	}
	anchor, added := string(family[shown-1]), string(family[shown])
	order := make([]string, 0, len(cfg.LaneOrder))
	for _, entry := range settings.NormalizeLaneOrder(cfg.LaneOrder) {
		if entry != added {
			order = append(order, entry)
		}
	}
	for index, entry := range order {
		if entry == anchor {
			order = append(order[:index+1], append([]string{added}, order[index+1:]...)...)
			break
		}
	}
	cfg.LaneOrder = order
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
	c.forgetAccount(id, model.StatusLoggedOut)
}

// ResetAccount invalidates old readings while a changed connection method is
// still waiting for its first inspection.
func (c *Controller) ResetAccount(id model.ProviderID) {
	c.forgetAccount(id, model.StatusInitializing)
}

func (c *Controller) forgetAccount(id model.ProviderID, status model.ConnectionStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accountEpoch == nil {
		c.accountEpoch = make(map[model.ProviderID]uint64)
	}
	c.accountEpoch[id]++
	for i := range c.state.Lanes {
		if c.state.Lanes[i].Provider == id {
			c.state.Lanes[i] = LaneState{Provider: id, Name: string(id), Status: status}
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
