package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// accountsState connects every Claude account with one usage row each, so a
// test can tell an account that was hidden from one that was never there.
func accountsState() ViewState {
	state := orderedState()
	row := UsageRowState{Label: "Session", Percent: 33, WindowMinutes: 300, ResetsAt: time.Now().Add(time.Hour)}
	for _, id := range []model.ProviderID{model.ProviderClaude3, model.ProviderClaude4, model.ProviderClaude5} {
		state.Lanes = append(state.Lanes, LaneState{Provider: id, Name: string(id), Status: model.StatusConnected, Rows: []UsageRowState{row}})
	}
	return state
}

func accountsView(t *testing.T, count int) *View {
	t.Helper()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	config := settings.Default()
	config.ClaudeAccounts = count
	config.ShowGrok = true
	config = config.Validated()
	view := NewView(nil, catalog, i18n.English, config, Actions{AppVersion: testAppVersion})
	view.state = accountsState()
	view.config = config
	return view
}

func claudeLaneNames(lanes []LaneState) []string {
	names := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		if model.IsClaudeAccount(lane.Provider) {
			names = append(names, lane.Name)
		}
	}
	return names
}

// Three accounts are three lanes, each with a name of its own, and the third
// is numbered until the user names it.
func TestThreeClaudeAccountsAreDrawnWithTheirOwnNames(t *testing.T) {
	view := accountsView(t, 3)
	got := strings.Join(claudeLaneNames(view.visibleLanes()), ",")
	if got != "Claude CLI,Claude Auth,Claude 3" {
		t.Fatalf("claude lanes = %s", got)
	}
	cells := 0
	for _, cell := range view.nanoCellStates() {
		if cell.kind == ProviderIconClaude {
			cells++
		}
	}
	if cells != 3 {
		t.Fatalf("nano drew %d Claude cards, want 3", cells)
	}
	if !compactShowsClaudeAccountHeaders(view.visibleLanes()) {
		t.Fatal("compact does not label three accounts")
	}
}

// The count is the boundary: accounts past it are neither drawn nor listed,
// and a fifth account is drawn only when the count says five.
func TestAccountsPastTheCountAreNotDrawn(t *testing.T) {
	for count := 1; count <= settings.MaxClaudeAccounts; count++ {
		view := accountsView(t, count)
		if got := len(claudeLaneNames(view.visibleLanes())); got != count {
			t.Fatalf("count %d drew %d Claude lanes", count, got)
		}
		view.buildConnectionRows()
		cards := 0
		for _, card := range view.connectionCache {
			if model.IsClaudeAccount(card.id) {
				cards++
			}
		}
		if cards != count {
			t.Fatalf("count %d listed %d Claude cards", count, cards)
		}
	}
}

// A user names a later account the same way as the first two, and the name
// reaches every screen.
func TestALaterAccountTakesItsLabel(t *testing.T) {
	view := accountsView(t, 4)
	config := view.config
	config.AccountLabels = map[string]string{"claude-4": "Lab"}
	view.config = config.Validated()
	got := strings.Join(claudeLaneNames(view.visibleLanes()), ",")
	if !strings.HasSuffix(got, ",Lab") {
		t.Fatalf("the fourth account did not take its label: %s", got)
	}
	tooltip := BuildTrayTooltip(view.state, view.config, i18n.English, time.Now())
	if !strings.Contains(tooltip, "Lab") {
		t.Fatalf("the tray tooltip does not carry the label:\n%s", tooltip)
	}
}

// The add control walks the count up to the maximum and then goes away; the
// remove control walks it back down and goes away at one.
func TestTheAccountControlsWalkTheCountBetweenOneAndFive(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.Actions.WebAuthAvailable = true
	view.Show(SettingsScreen)
	last := func() model.ProviderID { return model.ClaudeAccountIDs()[claudeAccountCount(view.config)-1] }
	for expect := 2; expect <= settings.MaxClaudeAccounts; expect++ {
		card := connectionRowForTest(t, view, last())
		if card.addButton == nil {
			t.Fatalf("no add control on the last card at %d accounts", expect-1)
		}
		card.addButton.Tapped(nil)
		if got := view.config.ClaudeAccounts; got != expect {
			t.Fatalf("after adding, count = %d, want %d", got, expect)
		}
	}
	if card := connectionRowForTest(t, view, last()); card.addButton != nil {
		t.Fatal("the add control is still offered at the maximum")
	}
	for expect := settings.MaxClaudeAccounts - 1; expect >= 1; expect-- {
		card := connectionRowForTest(t, view, last())
		if card.removeButton == nil {
			t.Fatalf("no remove control on the last card at %d accounts", expect+1)
		}
		card.removeButton.Tapped(nil)
		if got := view.config.ClaudeAccounts; got != expect {
			t.Fatalf("after removing, count = %d, want %d", got, expect)
		}
	}
	if card := connectionRowForTest(t, view, model.ProviderClaude); card.removeButton != nil || view.config.ShowClaudeAuth {
		t.Fatal("a single account still offers a remove control or keeps the second-account flag")
	}
}

// Only the CLI is exclusive across accounts. The token variable may serve as
// many accounts as the user points at it, and the browser has a profile each.
func TestOnlyTheCLIIsExclusiveAcrossFiveAccounts(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ClaudeAccounts = 5
	view.SetConfig(config)
	view.setConnectionMethod(model.ProviderClaude, connectionMethodCLI)
	for _, id := range []model.ProviderID{model.ProviderClaudeAuth, model.ProviderClaude3, model.ProviderClaude5} {
		if !view.claudeMethodOccupiedByOther(id, connectionMethodCLI) {
			t.Fatalf("%s does not see the CLI as taken", id)
		}
		if view.claudeMethodOccupiedByOther(id, connectionMethodOther) || view.claudeMethodOccupiedByOther(id, connectionMethodAuth) {
			t.Fatalf("%s sees a shareable route as taken", id)
		}
	}
	view.setConnectionMethod(model.ProviderClaude3, connectionMethodOther)
	view.setConnectionMethod(model.ProviderClaude4, connectionMethodOther)
	if view.config.ConnectionMethods["claude-3"] != "other" || view.config.ConnectionMethods["claude-4"] != "other" {
		t.Fatalf("two accounts on the token variable were refused: %v", view.config.ConnectionMethods)
	}
	// Handing the CLI to the fifth account takes it from the first.
	view.setConnectionMethod(model.ProviderClaude, connectionMethodAuth)
	view.setConnectionMethod(model.ProviderClaude5, connectionMethodCLI)
	if !view.claudeMethodOccupiedByOther(model.ProviderClaude, connectionMethodCLI) {
		t.Fatal("the first account does not see the CLI as taken by the fifth")
	}
}
