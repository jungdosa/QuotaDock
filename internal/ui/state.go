package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"sync"
	"time"
)

type UsageRowState struct {
	Label        string
	DisplayLabel string
	Percent      float64
	// UsageUnknown marks a row whose consumption the provider cannot report
	// yet (Grok: the usage field is unconfirmed). The row then shows only its
	// reset window — an empty meter with a number would claim "0% used".
	UsageUnknown            bool
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
	// StaleFor counts the refreshes in a row for which these rows are the last
	// reading that succeeded rather than a fresh one. Zero means the rows are
	// current. Error carries the code of the failure being ridden out.
	StaleFor int
}

// StaleRefreshLimit is how many consecutive failed refreshes a lane keeps
// showing its last good reading for before it shows the failure instead. A
// single missed minute is almost always the network or the service taking
// its time, and blanking the lane for it turned every blip into a visible
// outage; a provider that stays down for this long is reported as down.
const StaleRefreshLimit = 3

// transientRefreshError reports whether a failure is the kind that passes on
// its own. A sign-in that expired or a CLI that is missing needs the user, so
// those show at once; a request that timed out, a service that did not answer
// or answered with something unreadable, or a helper process that exited is
// tried again next refresh before anyone is told.
func transientRefreshError(code model.ErrorCode) bool {
	switch code {
	case model.ErrTimeout, model.ErrUnavailable, model.ErrInvalidResponse, model.ErrProcessExited:
		return true
	}
	return false
}

// keepLastReading decides whether a lane whose refresh just failed should go
// on showing the reading it had. It does so only while the failure is
// transient, the lane really had a reading, and the limit is not yet reached.
func keepLastReading(prior LaneState, code model.ErrorCode) (LaneState, bool) {
	if !transientRefreshError(code) || prior.Status != model.StatusConnected || len(prior.Rows) == 0 {
		return LaneState{}, false
	}
	if prior.StaleFor >= StaleRefreshLimit {
		return LaneState{}, false
	}
	return prior, true
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
	c.applyProviderSourceModes(c.config)
	c.state = defaultViewState()
	return c
}
func defaultViewState() ViewState {
	// Keep the established four provider indexes stable for callers and tests;
	// visibleLanes places the optional Auth account beside Claude when enabled.
	// The further Claude accounts follow the second so every index that
	// existed before them stays where it was.
	return ViewState{Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Status: model.StatusUnavailable},
		{Provider: model.ProviderCodex, Name: "Codex", Status: model.StatusUnavailable},
		{Provider: model.ProviderAntigravity, Name: "Antigravity", Status: model.StatusUnavailable},
		{Provider: model.ProviderGrok, Name: "Grok", Status: model.StatusUnavailable},
		{Provider: model.ProviderClaudeAuth, Name: "Claude Auth", Status: model.StatusUnavailable},
		{Provider: model.ProviderClaude3, Name: "Claude 3", Status: model.StatusUnavailable},
		{Provider: model.ProviderClaude4, Name: "Claude 4", Status: model.StatusUnavailable},
		{Provider: model.ProviderClaude5, Name: "Claude 5", Status: model.StatusUnavailable},
	}}
}
func (c *Controller) Config() settings.Config { c.mu.RLock(); defer c.mu.RUnlock(); return c.config }
func (c *Controller) SetConfig(cfg settings.Config) {
	c.mu.Lock()
	c.config = cfg.Validated()
	c.applyProviderSourceModes(c.config)
	state := cloneState(c.state)
	listeners := append([]func(ViewState){}, c.listeners...)
	c.mu.Unlock()
	for _, fn := range listeners {
		fn(state)
	}
}

type sourceModeSetter interface {
	SetSourceMode(string)
}

func (c *Controller) applyProviderSourceModes(config settings.Config) {
	for _, id := range model.ClaudeAccountIDs() {
		implementation := c.coordinator.Provider(id)
		setter, ok := implementation.(sourceModeSetter)
		if !ok {
			continue
		}
		value := config.ConnectionMethods[string(id)]
		// Every account past the first defaults to the browser sign-in; only
		// the first can fall back to the CLI on its own.
		if id != model.ProviderClaude && value == "" {
			value = settings.ConnectionMethodAuth
		}
		setter.SetSourceMode(value)
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
	// The reading each lane showed before this refresh. A failure is judged
	// against it rather than against the inspection that follows the failure,
	// because a provider that just failed already reports itself as failed.
	c.mu.RLock()
	prior := make(map[model.ProviderID]LaneState, len(c.state.Lanes))
	for _, lane := range c.state.Lanes {
		prior[lane.Provider] = lane
	}
	c.mu.RUnlock()
	outcomes := c.coordinator.RefreshAll(ctx)
	next := defaultViewState()
	next.LastRefresh = time.Now().UTC()
	for i := range next.Lanes {
		lane := &next.Lanes[i]
		implementation := c.coordinator.Provider(lane.Provider)
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
			if kept, keep := keepLastReading(prior[lane.Provider], lane.Error); keep {
				// The lane stays connected and keeps its rows; the error code
				// rides along so the state log still records the failure and
				// the connection card can say the reading is being held.
				lane.Status = model.StatusConnected
				lane.Plan = kept.Plan
				lane.Credits = kept.Credits
				lane.Rows = append([]UsageRowState(nil), kept.Rows...)
				lane.StaleFor = kept.StaleFor + 1
				continue
			}
			if lane.Error != model.ErrUsageUnavailable && lane.Status == model.StatusConnected {
				lane.Status = model.StatusError
			}
			continue
		}
		lane.Status = model.StatusConnected
		lane.Credits = outcome.Snapshot.Credits
		for _, limit := range outcome.Snapshot.Limits {
			// A limit only reads as "unknown" when the provider says so; Grok
			// now reports a real weekly percent, and any provider that cannot
			// report consumption sets the flag on the limit itself.
			lane.Rows = append(lane.Rows, UsageRowState{Label: limit.Label, Percent: limit.UsedPercent, UsageUnknown: limit.UsageUnknown, ResetsAt: limit.ResetsAt, WindowMinutes: limit.WindowMinutes})
		}
		sortLaneRows(lane.Provider, lane.Rows)
		assignUniqueDisplayLabels(lane.Rows)
	}
	c.mu.Lock()
	previous := cloneState(c.state)
	c.state = next
	listeners := append([]func(ViewState){}, c.listeners...)
	snapshot := cloneState(next)
	c.mu.Unlock()
	logProviderStateTransitions(previous, snapshot)
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

func logProviderStateTransitions(previous, next ViewState) {
	before := make(map[model.ProviderID]LaneState, len(previous.Lanes))
	for _, lane := range previous.Lanes {
		before[lane.Provider] = lane
	}
	for _, lane := range next.Lanes {
		prior, ok := before[lane.Provider]
		if ok && prior.Status == lane.Status && prior.Error == lane.Error {
			continue
		}
		from := model.StatusUnavailable
		if ok {
			from = prior.Status
		}
		slog.Info(
			"provider.state",
			"provider", string(lane.Provider),
			"from", string(from),
			"to", string(lane.Status),
			"err", string(lane.Error),
		)
	}
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

// codexRowGroup returns the model group a Codex row belongs to, or "" for the
// account-wide limit. The provider follows the Antigravity convention: a row
// scoped to one model (GPT-5.3-Codex-Spark today) carries its group name in
// Label, while an account-wide row carries the plain period word. Comparing
// against the period word for the row's own window keeps the test exact —
// any other label is a group name, whatever the model is called next.
func codexRowGroup(row UsageRowState) string {
	if row.Label == "" || row.Label == model.UsageWindowLabel(row.WindowMinutes) {
		return ""
	}
	// Rows built by the UI's own fixtures and older paths carry the localized
	// period word instead of the provider's canonical one. Those are still
	// account-wide rows, not a model called "세션".
	switch strings.ToLower(row.Label) {
	case "session", "weekly", "세션", "주간":
		return ""
	}
	return row.Label
}

func rowGroupRank(providerID model.ProviderID, row UsageRowState) int {
	if providerID == model.ProviderCodex {
		// The account-wide limit is the one the user actually budgets
		// against, so it stays on top; model-scoped limits follow beneath it
		// rather than interleaving by window length.
		if codexRowGroup(row) == "" {
			return 0
		}
		return 1
	}
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
	if (providerID == model.ProviderClaude || providerID == model.ProviderClaudeAuth) && strings.Contains(strings.ToLower(row.Label), "fable") {
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
