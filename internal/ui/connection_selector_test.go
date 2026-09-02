package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// A config written before the selector shipped must behave exactly as it did.
func TestAbsentConnectionMethodKeepsProviderDefault(t *testing.T) {
	config := settings.Default().Validated()
	if config.ConnectionMethods != nil {
		t.Fatalf("default config carries connection methods: %v", config.ConnectionMethods)
	}
	for id, want := range map[model.ProviderID]connectionMethod{
		model.ProviderClaude:      connectionMethodCLI,
		model.ProviderCodex:       connectionMethodCLI,
		model.ProviderAntigravity: connectionMethodIDE,
		model.ProviderGrok:        connectionMethodCLI,
	} {
		if got := selectedConnectionMethod(config, id); got != want {
			t.Fatalf("%s default method = %q, want %q", id, got, want)
		}
	}
}

func TestClaudeAccountAddButtonAndCLIExclusivity(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.Actions.WebAuthAvailable = true
	view.Show(SettingsScreen)

	root := connectionRowForTest(t, view, model.ProviderClaude)
	if root.addButton == nil || view.config.ShowClaudeAuth {
		t.Fatal("single Claude account did not expose the add button")
	}
	root.addButton.Tapped(nil)
	if !view.config.ShowClaudeAuth {
		t.Fatal("add account button did not enable Claude Auth")
	}
	auth := connectionRowForTest(t, view, model.ProviderClaudeAuth)
	if auth.methods[0].button.State != connectionMethodOccupied || !strings.Contains(auth.methods[0].button.Tooltip, view.text(i18n.KeyConnectionCLIInUse)) {
		t.Fatalf("second-account CLI state/tooltip = %v / %q", auth.methods[0].button.State, auth.methods[0].button.Tooltip)
	}
	auth.methods[0].button.Tapped(nil)
	if view.config.ConnectionMethods[string(model.ProviderClaudeAuth)] == string(connectionMethodCLI) {
		t.Fatal("occupied CLI method was selectable")
	}

	view.setConnectionMethod(model.ProviderClaude, connectionMethodAuth)
	auth = connectionRowForTest(t, view, model.ProviderClaudeAuth)
	if auth.methods[0].button.State != connectionMethodAvailable {
		t.Fatalf("released CLI state = %v, want available", auth.methods[0].button.State)
	}
	view.setConnectionMethod(model.ProviderClaudeAuth, connectionMethodCLI)
	if got := view.config.ConnectionMethods[string(model.ProviderClaudeAuth)]; got != string(connectionMethodCLI) {
		t.Fatalf("second account method = %q, want cli", got)
	}
	root = connectionRowForTest(t, view, model.ProviderClaude)
	if root.methods[0].button.State != connectionMethodOccupied {
		t.Fatalf("root CLI state = %v after transfer, want occupied", root.methods[0].button.State)
	}
}

func TestClaudeAuthAndOtherMethodsMayOverlap(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	view.setConnectionMethod(model.ProviderClaude, connectionMethodOther)
	view.setConnectionMethod(model.ProviderClaudeAuth, connectionMethodOther)
	if view.config.ConnectionMethods["claude"] != "other" || view.config.ConnectionMethods["claude-auth"] != "other" {
		t.Fatalf("overlapping Other methods were rejected: %v", view.config.ConnectionMethods)
	}
	view.setConnectionMethod(model.ProviderClaude, connectionMethodAuth)
	view.setConnectionMethod(model.ProviderClaudeAuth, connectionMethodAuth)
	if view.config.ConnectionMethods["claude"] != "auth" || view.config.ConnectionMethods["claude-auth"] != "auth" {
		t.Fatalf("overlapping Auth methods were rejected: %v", view.config.ConnectionMethods)
	}
}

func TestAccountLabelEntriesPersistUserAliasesOnly(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	view.Show(SettingsScreen)
	root := connectionRowForTest(t, view, model.ProviderClaude)
	auth := connectionRowForTest(t, view, model.ProviderClaudeAuth)
	root.labelEntry.SetText("Work")
	auth.labelEntry.SetText("Personal")
	if view.config.AccountLabels["claude"] != "Work" || view.config.AccountLabels["claude-auth"] != "Personal" {
		t.Fatalf("account labels were not stored: %v", view.config.AccountLabels)
	}
	view.SetState(dualClaudeTestState())
	lanes := view.visibleLanes()
	if len(lanes) < 2 || lanes[0].Name != "Work" || lanes[1].Name != "Personal" {
		t.Fatalf("stored account labels were not applied to dual Claude lanes: %+v", lanes)
	}
}

func TestAccountLabelEntriesFollowDualClaudeVisibilityImmediately(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.Show(SettingsScreen)

	root := connectionRowForTest(t, view, model.ProviderClaude)
	if root.labelEntry != nil || root.labelText != nil {
		t.Fatal("single Claude account rendered a display-name control")
	}

	config := view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	root = connectionRowForTest(t, view, model.ProviderClaude)
	auth := connectionRowForTest(t, view, model.ProviderClaudeAuth)
	for _, row := range []*connectionView{root, auth} {
		if row.labelEntry == nil || row.labelText == nil || row.labelRow == nil {
			t.Fatalf("dual Claude row %s is missing its display-name control", row.id)
		}
		if row.labelText.Text != view.text(i18n.KeyConnectionAccountLabel) {
			t.Fatalf("dual Claude row %s label=%q", row.id, row.labelText.Text)
		}
		if row.labelEntry.PlaceHolder != view.text(i18n.KeyConnectionAccountLabelHint) {
			t.Fatalf("dual Claude row %s placeholder=%q", row.id, row.labelEntry.PlaceHolder)
		}
	}

	config = view.config
	config.ShowClaudeAuth = false
	config.ClaudeAccounts = 1
	view.SetConfig(config)
	if row := connectionRowForTest(t, view, model.ProviderClaude); row.labelEntry != nil {
		t.Fatal("display-name control remained after Claude Auth was hidden")
	}

	config = view.config
	config.ShowClaude = false
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	root = connectionRowForTest(t, view, model.ProviderClaude)
	auth = connectionRowForTest(t, view, model.ProviderClaudeAuth)
	if root.labelEntry != nil || auth.labelEntry != nil || root.name.Text != "Claude" || auth.name.Text != "Claude" {
		t.Fatalf("single visible Claude account retained dual controls/names: root=%q auth=%q", root.name.Text, auth.name.Text)
	}
}

func TestAccountLabelEntryFillsRemainingConnectionRowWithoutOverlap(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.Language = settings.Language(i18n.Korean)
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	view.Show(SettingsScreen)
	window.Resize(view.MinimumSize(SettingsScreen))

	root := connectionRowForTest(t, view, model.ProviderClaude)
	detailPosition, detailFound := objectPosition(view.Settings, root.detail, fyne.NewPos(0, 0))
	labelPosition, labelFound := objectPosition(view.Settings, root.labelText, fyne.NewPos(0, 0))
	entryPosition, entryFound := objectPosition(view.Settings, root.labelEntry, fyne.NewPos(0, 0))
	if !detailFound || !labelFound || !entryFound {
		t.Fatal("display-name row geometry was not found in settings")
	}
	if detailPosition.X+root.detail.Size().Width > labelPosition.X || labelPosition.X+root.labelText.Size().Width > entryPosition.X {
		t.Fatalf("display-name row overlaps: detail=%v/%v label=%v/%v entry=%v/%v", detailPosition, root.detail.Size(), labelPosition, root.labelText.Size(), entryPosition, root.labelEntry.Size())
	}
	if root.labelEntry.Size().Width <= 170 {
		t.Fatalf("display-name entry width=%.1f, want remaining width above legacy 170px", root.labelEntry.Size().Width)
	}
	expandedWidth := root.labelEntry.Size().Width
	if right := entryPosition.X + root.labelEntry.Size().Width; right > view.Settings.Size().Width {
		t.Fatalf("display-name entry right edge %.1f exceeds settings width %.1f", right, view.Settings.Size().Width)
	}

	capture := window.Canvas().Capture()
	if capture.Bounds().Dx() == 0 || capture.Bounds().Dy() == 0 {
		t.Fatal("settings render capture is empty")
	}

	root.labelRow.Resize(fyne.NewSize(240, root.labelRow.MinSize().Height))
	root.labelRow.Layout.Layout(root.labelRow.Objects, root.labelRow.Size())
	narrowLabelPosition, narrowLabelFound := objectPosition(root.labelRow, root.labelText, fyne.NewPos(0, 0))
	narrowEntryPosition, narrowEntryFound := objectPosition(root.labelRow, root.labelEntry, fyne.NewPos(0, 0))
	if !narrowLabelFound || !narrowEntryFound || narrowLabelPosition.X+root.labelText.Size().Width > narrowEntryPosition.X || narrowEntryPosition.X+root.labelEntry.Size().Width > root.labelRow.Size().Width {
		t.Fatalf("narrow display-name row overlaps or clips: label=%v/%v entry=%v/%v row=%v", narrowLabelPosition, root.labelText.Size(), narrowEntryPosition, root.labelEntry.Size(), root.labelRow.Size())
	}

	t.Logf("settings capture=%dx%d, display-name entry width=%.1f", capture.Bounds().Dx(), capture.Bounds().Dy(), expandedWidth)
}

func connectionRowForTest(t *testing.T, view *View, id model.ProviderID) *connectionView {
	t.Helper()
	for _, row := range view.connectionCache {
		if row.id == id {
			return row
		}
	}
	t.Fatalf("connection row %s not found", id)
	return nil
}

func TestStoredConnectionMethodIsHonoredWhenAvailable(t *testing.T) {
	config := settings.Default()
	config.ConnectionMethods = map[string]string{"claude": "other"}
	config = config.Validated()
	if got := selectedConnectionMethod(config, model.ProviderClaude); got != connectionMethodOther {
		t.Fatalf("stored method = %q, want other", got)
	}
}

// A method the provider does not offer must fall back rather than leave the
// row with no active route at all.
func TestUnavailableStoredMethodFallsBackToDefault(t *testing.T) {
	config := settings.Default()
	config.ConnectionMethods = map[string]string{"codex": "auth", "grok": "ide"}
	config = config.Validated()
	if got := selectedConnectionMethod(config, model.ProviderCodex); got != connectionMethodCLI {
		t.Fatalf("Codex fallback = %q, want cli", got)
	}
	if got := selectedConnectionMethod(config, model.ProviderGrok); got != connectionMethodCLI {
		t.Fatalf("Grok fallback = %q, want cli", got)
	}
}

func TestValidatedDropsUnknownProvidersAndMethods(t *testing.T) {
	config := settings.Default()
	config.ConnectionMethods = map[string]string{
		"claude": "cli",
		"bogus":  "cli",
		"codex":  "teleport",
	}
	config = config.Validated()
	if len(config.ConnectionMethods) != 1 || config.ConnectionMethods["claude"] != "cli" {
		t.Fatalf("validated methods = %v, want only claude:cli", config.ConnectionMethods)
	}
}

// Only the picked route reads as active; other working routes read as
// available so the row shows which one is actually in use.
func TestOnlySelectedMethodReadsAsActive(t *testing.T) {
	lane := LaneState{Provider: model.ProviderClaude, Status: model.StatusConnected}
	if got := connectionMethodStateSelected(lane, connectionMethodCLI, true, connectionMethodCLI); got != connectionMethodActive {
		t.Fatalf("selected CLI = %v, want active", got)
	}
	if got := connectionMethodStateSelected(lane, connectionMethodCLI, true, connectionMethodOther); got != connectionMethodAvailable {
		t.Fatalf("unselected but working CLI = %v, want available", got)
	}
	// Selecting something does not invent readiness for a planned method.
	if got := connectionMethodStateSelected(lane, connectionMethodAuth, true, connectionMethodAuth); got != connectionMethodPlanned {
		t.Fatalf("planned auth = %v, want planned", got)
	}
}

func TestSelectingAnUnofferedMethodIsIgnored(t *testing.T) {
	view := &View{config: settings.Default()}
	view.state = ViewState{Lanes: []LaneState{{Provider: model.ProviderCodex, Status: model.StatusConnected}}}
	view.setConnectionMethod(model.ProviderCodex, connectionMethodAuth)
	if view.config.ConnectionMethods["codex"] != "" {
		t.Fatalf("unoffered method was stored: %v", view.config.ConnectionMethods)
	}
}
