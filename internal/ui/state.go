package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"sync"
	"time"
)

type UsageRowState struct {
	Label                   string
	DisplayLabel            string
	Percent                 float64
	ResetsAt                time.Time
	WindowMinutes           int
	DisplayOverride         bool
	DisplayRemaining        string
	DisplayReset            string
	DisplayRemainingPercent float64
}
type LaneState struct {
	Provider   model.ProviderID
	Name       string
	Plan       model.Plan
	Status     model.ConnectionStatus
	Error      model.ErrorCode
	ErrorKey   string
	CLIPath    string
	CLIVersion string
	Source     string
	// Credits is the provider's paid extra-usage balance when it reports one
	// (currently Codex only).
	Credits *model.Credits
	Rows    []UsageRowState
}
type ViewState struct {
	Lanes       []LaneState
	LastRefresh time.Time
}

type Controller struct {
	mu          sync.RWMutex
	coordinator provider.Coordinator
	config      settings.Config
	state       ViewState
	listeners   []func(ViewState)
}

func NewController(coordinator provider.Coordinator, config settings.Config) *Controller {
	c := &Controller{coordinator: coordinator, config: config.Validated()}
	c.state = defaultViewState()
	return c
}
func defaultViewState() ViewState {
	return ViewState{Lanes: []LaneState{{Provider: model.ProviderClaude, Name: "Claude", Status: model.StatusUnavailable}, {Provider: model.ProviderCodex, Name: "Codex", Status: model.StatusUnavailable}, {Provider: model.ProviderAntigravity, Name: "Antigravity", Status: model.StatusUnavailable}}}
}
func (c *Controller) Config() settings.Config { c.mu.RLock(); defer c.mu.RUnlock(); return c.config }
func (c *Controller) SetConfig(cfg settings.Config) {
	c.mu.Lock()
	c.config = cfg.Validated()
	state := cloneState(c.state)
	listeners := append([]func(ViewState){}, c.listeners...)
	c.mu.Unlock()
	for _, fn := range listeners {
		fn(state)
	}
}
func (c *Controller) State() ViewState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneState(c.state)
}
func (c *Controller) Subscribe(fn func(ViewState)) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	c.listeners = append(c.listeners, fn)
	state := cloneState(c.state)
	c.mu.Unlock()
	fn(state)
}
func (c *Controller) Refresh(ctx context.Context) ViewState {
	outcomes := c.coordinator.RefreshAll(ctx)
	next := defaultViewState()
	next.LastRefresh = time.Now().UTC()
	for i := range next.Lanes {
		lane := &next.Lanes[i]
		implementation := c.coordinator.Providers[lane.Provider]
		if implementation == nil {
			continue
		}
		inspection := implementation.Inspect(ctx)
		applyConnectionState(lane, inspection)
		outcome, ok := outcomes[lane.Provider]
		if !ok {
			if lane.Status == model.StatusConnected {
				lane.Status = model.StatusError
			}
			continue
		}
		lane.Plan = outcome.Snapshot.Plan
		if outcome.Err != nil {
			var safe model.SafeError
			if errors.As(outcome.Err, &safe) {
				lane.Error = safe.Code
				lane.ErrorKey = safe.Key
			}
			if lane.Error != model.ErrUsageUnavailable && lane.Status == model.StatusConnected {
				lane.Status = model.StatusError
			}
			continue
		}
		lane.Status = model.StatusConnected
		lane.Credits = outcome.Snapshot.Credits
		for _, limit := range outcome.Snapshot.Limits {
			lane.Rows = append(lane.Rows, UsageRowState{Label: limit.Label, Percent: limit.UsedPercent, ResetsAt: limit.ResetsAt, WindowMinutes: limit.WindowMinutes})
		}
		sortLaneRows(lane.Provider, lane.Rows)
		assignUniqueDisplayLabels(lane.Rows)
	}
	c.mu.Lock()
	c.state = next
	listeners := append([]func(ViewState){}, c.listeners...)
	snapshot := cloneState(next)
	c.mu.Unlock()
	for _, fn := range listeners {
		fn(cloneState(snapshot))
	}
	return snapshot
}

func applyConnectionState(lane *LaneState, state model.ConnectionState) {
	lane.Status = state.Status
	lane.Error = state.Error
	lane.ErrorKey = state.ErrorKey
	lane.CLIPath = state.CLIPath
	lane.CLIVersion = state.CLIVersion
	lane.Source = state.Source
}

func sortLaneRows(providerID model.ProviderID, rows []UsageRowState) {
	sort.SliceStable(rows, func(i, j int) bool {
		leftGroup := rowGroupRank(providerID, rows[i])
		rightGroup := rowGroupRank(providerID, rows[j])
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		leftWindow, rightWindow := rows[i].WindowMinutes, rows[j].WindowMinutes
		if leftWindow != rightWindow {
			return windowComesBefore(leftWindow, rightWindow)
		}
		leftScope := modelScopeRank(providerID, rows[i])
		rightScope := modelScopeRank(providerID, rows[j])
		if leftScope != rightScope {
			return leftScope < rightScope
		}
		leftReset, rightReset := rows[i].ResetsAt, rows[j].ResetsAt
		if !leftReset.IsZero() && !rightReset.IsZero() && !leftReset.Equal(rightReset) {
			return leftReset.Before(rightReset)
		}
		return false
	})
}

func rowGroupRank(providerID model.ProviderID, row UsageRowState) int {
	if providerID != model.ProviderAntigravity {
		return 0
	}
	label := strings.ToLower(row.Label)
	switch {
	case strings.Contains(label, "gemini"):
		return 0
	case strings.Contains(label, "claude"), strings.Contains(label, "gpt"):
		return 1
	default:
		return 2
	}
}

func modelScopeRank(providerID model.ProviderID, row UsageRowState) int {
	if providerID == model.ProviderClaude && strings.Contains(strings.ToLower(row.Label), "fable") {
		return 1
	}
	return 0
}

func windowComesBefore(left, right int) bool {
	if left <= 0 {
		return false
	}
	if right <= 0 {
		return true
	}
	return left < right
}

func assignUniqueDisplayLabels(rows []UsageRowState) {
	counts := make(map[string]int, len(rows))
	baseLabels := make([]string, len(rows))
	durationCounts := make(map[string]map[string]int, len(rows))
	for i := range rows {
		rows[i].DisplayLabel = ""
		row := rows[i]
		base := usageLabel(row, false)
		baseKey := strings.ToLower(base)
		baseLabels[i] = base
		counts[baseKey]++
		if durationCounts[baseKey] == nil {
			durationCounts[baseKey] = make(map[string]int)
		}
		durationCounts[baseKey][compactWindowDuration(row.WindowMinutes)]++
	}

	used := make(map[string]bool, len(rows))
	for i, base := range baseLabels {
		if counts[strings.ToLower(base)] == 1 {
			rows[i].DisplayLabel = base
			used[strings.ToLower(base)] = true
		}
	}
	for i, base := range baseLabels {
		baseKey := strings.ToLower(base)
		if counts[baseKey] == 1 {
			continue
		}
		suffix := compactWindowDuration(rows[i].WindowMinutes)
		if suffix != "" && durationCounts[baseKey][suffix] == 1 {
			rows[i].DisplayLabel = nextUniqueLabel(base+" · "+suffix, used)
			continue
		}
		rows[i].DisplayLabel = nextOrdinalLabel(base, used)
	}
}

func nextUniqueLabel(candidate string, used map[string]bool) string {
	if key := strings.ToLower(candidate); !used[key] {
		used[key] = true
		return candidate
	}
	for ordinal := 2; ; ordinal++ {
		label := fmt.Sprintf("%s %d", candidate, ordinal)
		key := strings.ToLower(label)
		if !used[key] {
			used[key] = true
			return label
		}
	}
}

func nextOrdinalLabel(base string, used map[string]bool) string {
	for ordinal := 1; ; ordinal++ {
		label := fmt.Sprintf("%s (%d)", base, ordinal)
		key := strings.ToLower(label)
		if !used[key] {
			used[key] = true
			return label
		}
	}
}

func compactWindowDuration(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	if minutes%(24*60) == 0 {
		return fmt.Sprintf("%dd", minutes/(24*60))
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}

func (c *Controller) Close() error { return c.coordinator.Close() }
func cloneState(in ViewState) ViewState {
	out := in
	out.Lanes = append([]LaneState(nil), in.Lanes...)
	for i := range out.Lanes {
		out.Lanes[i].Rows = append([]UsageRowState(nil), in.Lanes[i].Rows...)
	}
	return out
}
