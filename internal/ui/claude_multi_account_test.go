package ui

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

type multiAccountTestProvider struct {
	model.Provider
	additional map[model.ProviderID]model.Provider
}

type sourceModeTestProvider struct {
	phase2Provider
	mode string
}

func (p *sourceModeTestProvider) SetSourceMode(mode string) { p.mode = mode }

type sourceModeTestCollection struct {
	*sourceModeTestProvider
	additional map[model.ProviderID]model.Provider
}

func (p sourceModeTestCollection) AdditionalProviders() map[model.ProviderID]model.Provider {
	return p.additional
}

func (p multiAccountTestProvider) AdditionalProviders() map[model.ProviderID]model.Provider {
	return p.additional
}

func TestControllerPopulatesTheAdditionalClaudeAccount(t *testing.T) {
	root := phase2Provider{
		snapshot: model.UsageSnapshot{Provider: model.ProviderClaude, Limits: []model.UsageLimit{{Label: "CLI", UsedPercent: 11}}},
		inspect:  model.StatusConnected,
	}
	web := phase2Provider{
		snapshot: model.UsageSnapshot{Provider: model.ProviderClaudeAuth, Limits: []model.UsageLimit{{Label: "Auth", UsedPercent: 22}}},
		inspect:  model.StatusConnected,
	}
	collection := multiAccountTestProvider{
		Provider: root,
		additional: map[model.ProviderID]model.Provider{
			model.ProviderClaudeAuth: web,
		},
	}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderClaude: collection,
	}}, settings.Default())
	state := controller.Refresh(context.Background())
	if len(state.Lanes) < 5 || len(state.Lanes[4].Rows) != 1 || state.Lanes[4].Rows[0].Percent != 22 {
		t.Fatalf("additional Claude account was not populated: %+v", state.Lanes)
	}
}

func TestControllerAppliesIndependentClaudeSourceModes(t *testing.T) {
	root := &sourceModeTestProvider{phase2Provider: phase2Provider{id: model.ProviderClaude}}
	auth := &sourceModeTestProvider{phase2Provider: phase2Provider{id: model.ProviderClaudeAuth}}
	collection := sourceModeTestCollection{
		sourceModeTestProvider: root,
		additional: map[model.ProviderID]model.Provider{
			model.ProviderClaudeAuth: auth,
		},
	}
	config := settings.Default()
	config.ConnectionMethods = map[string]string{"claude": "other"}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderClaude: collection,
	}}, config)
	if root.mode != settings.ConnectionMethodOther || auth.mode != settings.ConnectionMethodAuth {
		t.Fatalf("initial source modes root/auth=%q/%q", root.mode, auth.mode)
	}

	config = controller.Config()
	config.ConnectionMethods = map[string]string{"claude": "auth", "claude-auth": "other"}
	controller.SetConfig(config)
	if root.mode != settings.ConnectionMethodAuth || auth.mode != settings.ConnectionMethodOther {
		t.Fatalf("updated source modes root/auth=%q/%q", root.mode, auth.mode)
	}
}

func TestClaudeCLIAndAuthLanesDisplayTogether(t *testing.T) {
	config := settings.Default()
	config.ShowClaudeAuth = true
	view := &View{config: config, state: ViewState{Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Status: model.StatusConnected},
		{Provider: model.ProviderCodex, Name: "Codex", Status: model.StatusConnected},
		{Provider: model.ProviderClaudeAuth, Name: "Claude Auth", Status: model.StatusConnected, Source: model.SourceWebSignIn},
	}}}

	lanes := view.visibleLanes()
	if len(lanes) != 3 {
		t.Fatalf("visible lane count = %d, want 3", len(lanes))
	}
	if lanes[0].Provider != model.ProviderClaude || lanes[0].Name != "Claude CLI" ||
		lanes[1].Provider != model.ProviderClaudeAuth || lanes[1].Name != "Claude Auth" {
		t.Fatalf("Claude account lanes = %+v / %+v", lanes[0], lanes[1])
	}
}

func TestClaudeWebFallbackIsNotRenderedTwice(t *testing.T) {
	config := settings.Default()
	config.ShowClaudeAuth = true
	view := &View{config: config, state: ViewState{Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Status: model.StatusConnected, Source: model.SourceWebSignIn},
		{Provider: model.ProviderClaudeAuth, Name: "Claude Auth", Status: model.StatusConnected, Source: model.SourceWebSignIn},
	}}}

	lanes := view.visibleLanes()
	if len(lanes) != 1 || lanes[0].Provider != model.ProviderClaudeAuth {
		t.Fatalf("browser fallback was duplicated: %+v", lanes)
	}
}

func TestClaudeAuthUsesClaudeVisualIdentity(t *testing.T) {
	row := UsageRowState{Label: "Fable 7 day"}
	lane := LaneState{Provider: model.ProviderClaudeAuth}
	if providerColorKey(lane.Provider, row) != "claude-auth" || providerIconKind(lane, row) != ProviderIconClaude {
		t.Fatal("Claude Auth drifted from the Claude color or icon")
	}
}

func TestClaudeAccountLabelsOnlyAppearWithTwoVisibleAccounts(t *testing.T) {
	config := settings.Default()
	config.AccountLabels = map[string]string{"claude": "Work", "claude-auth": "Personal"}
	view := &View{config: config, state: ViewState{Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Status: model.StatusConnected},
		{Provider: model.ProviderClaudeAuth, Name: "Claude Auth", Status: model.StatusConnected, Source: model.SourceWebSignIn},
	}}}
	if lanes := view.visibleLanes(); len(lanes) != 1 || lanes[0].Name != "Claude" {
		t.Fatalf("single account label leaked into the lane: %+v", lanes)
	}
	view.config.ShowClaudeAuth = true
	lanes := view.visibleLanes()
	if len(lanes) != 2 || lanes[0].Name != "Work" || lanes[1].Name != "Personal" {
		t.Fatalf("dual account labels = %+v", lanes)
	}
}

func TestClaudeAuthColorContrastsInBothThemes(t *testing.T) {
	for name, colors := range map[string]BrandColors{"light": LightBrandColors, "dark": DarkBrandColors} {
		account := colors.PaletteColor("white")
		if contrast := wcagContrastRatio(account, colors.Background); contrast < 3 {
			t.Fatalf("%s Claude Auth contrast = %.2f, want >= 3", name, contrast)
		}
		if sameColor(account, colors.PaletteColor("amber")) || sameColor(account, colors.PaletteColor("red")) {
			t.Fatalf("%s Claude Auth color overlaps warning or danger", name)
		}
	}
}

func TestDualClaudeCompactGroupingAndNanoWidth(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	view.SetState(dualClaudeTestState())

	view.Show(CompactScreen)
	if view.compactCache == nil || len(view.compactCache.dividers) != 3 || len(view.compactCache.accountHeaders) != 2 {
		t.Fatalf("dual-account compact groups/dividers = %+v", view.compactCache)
	}
	if got := []string{view.compactCache.accountHeaders[0].label.Text, view.compactCache.accountHeaders[1].label.Text}; got[0] != "Claude CLI" || got[1] != "Claude Auth" {
		t.Fatalf("default compact account headers=%v", got)
	}
	view.Show(NanoScreen)
	if cells := len(view.nanoCellStates()); cells != 5 {
		t.Fatalf("dual-account nano cells=%d, want 5", cells)
	}
	if width := view.MinimumSize(NanoScreen).Width; width != 5*NanoCellMinimumWidth {
		t.Fatalf("five-cell nano width=%.1f, want %.1f", width, 5*NanoCellMinimumWidth)
	}

	config = view.config
	config.ShowGrok = true
	view.SetConfig(config)
	if cells := len(view.nanoCellStates()); cells != 6 {
		t.Fatalf("dual-account nano cells with Grok=%d, want 6", cells)
	}
	if width := view.MinimumSize(NanoScreen).Width; width != 6*NanoCellMinimumWidth {
		t.Fatalf("six-cell nano width=%.1f, want %.1f", width, 6*NanoCellMinimumWidth)
	}
}

func TestCompactClaudeAccountHeadersOnlyAppearForDualAccounts(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.SetState(dualClaudeTestState())
	view.Show(CompactScreen)

	singleCache := view.compactCache
	singleHeight := view.MinimumSize(CompactScreen).Height
	if len(singleCache.accountHeaders) != 0 {
		t.Fatalf("single Claude account rendered compact headers: %+v", singleCache.accountHeaders)
	}
	config := view.config
	config.AccountLabels = map[string]string{"claude": "Work", "claude-auth": "Personal"}
	view.SetConfig(config)
	if view.compactCache != singleCache || len(view.compactCache.accountHeaders) != 0 || view.MinimumSize(CompactScreen).Height != singleHeight {
		t.Fatal("hidden second Claude account changed the compact cache or geometry")
	}

	config = view.config
	config.ShowClaudeAuth = true
	view.SetConfig(config)
	window.Resize(view.MinimumSize(CompactScreen))
	lanes := view.visibleLanes()
	if len(view.compactCache.accountHeaders) != 2 || len(lanes) < 2 {
		t.Fatalf("dual Claude compact headers/lanes=%d/%d, want 2/at least 2", len(view.compactCache.accountHeaders), len(lanes))
	}
	for index, want := range []string{lanes[0].Name, lanes[1].Name} {
		header := view.compactCache.accountHeaders[index]
		if header.label.Text != want {
			t.Fatalf("compact header %d=%q, normal lane name=%q", index, header.label.Text, want)
		}
		// The name reads as quietly as before but is no longer the smallest
		// thing on the screen: it now matches the row labels in size and stays
		// subordinate to them through its colour and its lighter weight.
		if !sameColor(header.label.Color, view.colors.Secondary) || header.label.TextSize > CompactLabelTextSize {
			t.Fatalf("compact header %d color/size=%v/%.1f, want Secondary and no larger than %.1f", index, header.label.Color, header.label.TextSize, CompactLabelTextSize)
		}
		if header.label.TextStyle.Bold {
			t.Fatalf("compact header %d is bold, which would outweigh the row labels it introduces", index)
		}
	}
	rowCount := 0
	for _, lane := range lanes {
		rowCount += len(lane.Rows)
	}
	if len(view.compactCache.rows) != rowCount {
		t.Fatalf("compact usage cache rows=%d, want %d without account headers", len(view.compactCache.rows), rowCount)
	}
	if got, want := len(view.compactBody.Objects), rowCount+len(view.compactCache.dividers)+len(view.compactCache.accountHeaders); got != want {
		t.Fatalf("compact body objects=%d, want rows+dividers+headers=%d", got, want)
	}
	rowsLayout := view.compactBody.Layout.(*CompactRowsLayout)
	withoutHeaders := make([]fyne.CanvasObject, 0, len(view.compactBody.Objects)-len(view.compactCache.accountHeaders))
	headerHeight := float32(0)
	for _, object := range view.compactBody.Objects {
		isHeader := false
		for _, header := range view.compactCache.accountHeaders {
			if object == header.row {
				isHeader = true
				headerHeight += header.row.MinSize().Height
				break
			}
		}
		if !isHeader {
			withoutHeaders = append(withoutHeaders, object)
		}
	}
	wantBodyHeight := rowsLayout.MinSize(withoutHeaders).Height + headerHeight + rowsLayout.Gap*float32(len(view.compactCache.accountHeaders))
	if got := view.compactBody.MinSize().Height; got != wantBodyHeight {
		t.Fatalf("compact body height=%.1f, want %.1f including two account header lines", got, wantBodyHeight)
	}

	for index, firstRow := range []int{0, len(lanes[0].Rows)} {
		header := view.compactCache.accountHeaders[index]
		headerPosition, headerFound := objectPosition(view.Compact, header.row, fyne.NewPos(0, 0))
		rowPosition, rowFound := objectPosition(view.Compact, view.compactCache.rows[firstRow].row, fyne.NewPos(0, 0))
		if !headerFound || !rowFound || headerPosition.Y+header.row.Size().Height > rowPosition.Y {
			t.Fatalf("compact group %d header/row geometry found=%t/%t pos=%v/%v size=%v", index, headerFound, rowFound, headerPosition, rowPosition, header.row.Size())
		}
	}
	capture := window.Canvas().Capture()
	if capture.Bounds().Dy() != int(math.Ceil(float64(view.MinimumSize(CompactScreen).Height))) {
		t.Fatalf("dual compact capture height=%d, minimum=%.1f", capture.Bounds().Dy(), view.MinimumSize(CompactScreen).Height)
	}
	for index, header := range view.compactCache.accountHeaders {
		if ink, found := phase3WHeaderInk(capture, view.Compact, header.label, view.colors.Background); !found || ink == 0 {
			t.Fatalf("compact account header %d is clipped or not rasterized: found=%t ink=%d", index, found, ink)
		}
	}
}

func TestCompactClaudeAccountLabelChangeInvalidatesSignature(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.ShowClaudeAuth = true
	config.AccountLabels = map[string]string{"claude": "Work", "claude-auth": "Personal"}
	view.SetConfig(config)
	view.SetState(dualClaudeTestState())
	view.Show(CompactScreen)

	before := view.compactCache
	config = view.config
	config.AccountLabels = map[string]string{"claude": "Office", "claude-auth": "Home"}
	view.SetConfig(config)
	if view.compactCache == before {
		t.Fatal("compact cache was not rebuilt after Claude account labels changed")
	}
	if len(view.compactCache.accountHeaders) != 2 {
		t.Fatalf("compact account headers after rename=%d, want 2", len(view.compactCache.accountHeaders))
	}
	if got := []string{view.compactCache.accountHeaders[0].label.Text, view.compactCache.accountHeaders[1].label.Text}; got[0] != "Office" || got[1] != "Home" {
		t.Fatalf("compact account headers after rename=%v", got)
	}

	view.Show(NormalScreen)
	if got := []string{view.normalCache.headers[0].name.Text, view.normalCache.headers[1].name.Text}; got[0] != "Office" || got[1] != "Home" {
		t.Fatalf("normal account names after rename=%v, want compact names", got)
	}
}

func TestAbsentClaudeAccountSettingsPreserveSingleLaneAndNanoWidth(t *testing.T) {
	view := &View{config: settings.Default(), state: dualClaudeTestState()}
	lanes := view.visibleLanes()
	if len(lanes) != 3 || lanes[0].Provider != model.ProviderClaude || lanes[0].Name != "Claude" {
		t.Fatalf("default visible lanes changed: %+v", lanes)
	}
	if cells := len(view.nanoCellStates()); cells != 4 {
		t.Fatalf("default nano cells=%d, want 4", cells)
	}
	if width := view.MinimumSize(NanoScreen).Width; width != NanoWidth {
		t.Fatalf("default nano width=%.1f, want %.1f", width, NanoWidth)
	}
}

func TestDualClaudeCompactAndNanoRenderCapture(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_CLAUDE_ACCOUNT_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_CLAUDE_ACCOUNT_SCREENSHOT_DIR to retain render captures")
	}
	view, window := newTestView(t)
	defer window.Close()
	for _, themeMode := range []struct {
		name   string
		theme  settings.Theme
		colors BrandColors
	}{
		{name: "light", theme: settings.ThemeLight, colors: LightBrandColors},
		{name: "dark", theme: settings.ThemeDark, colors: DarkBrandColors},
	} {
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(themeMode.theme))
		config := settings.Default()
		config.Theme = themeMode.theme
		config.ShowClaudeAuth = true
		config.AccountLabels = map[string]string{"claude": "Work", "claude-auth": "Personal"}
		view.SetConfig(config)
		view.SetState(dualClaudeTestState())
		for _, screen := range []struct {
			name   string
			screen Screen
		}{
			{name: "compact", screen: CompactScreen},
			{name: "nano", screen: NanoScreen},
		} {
			view.Show(screen.screen)
			window.Resize(view.MinimumSize(screen.screen))
			capture := window.Canvas().Capture()
			if pixels := countExactPixels(capture, themeMode.colors.PaletteColor("white")); pixels == 0 {
				t.Fatalf("%s %s capture has no Claude Auth color pixels", themeMode.name, screen.name)
			}
			writePhase4BCapture(t, filepath.Join(directory, "dual-claude-"+themeMode.name+"-"+screen.name+".png"), capture)
		}
	}
}

func dualClaudeTestState() ViewState {
	state := sampleState()
	state.Lanes = append(state.Lanes, LaneState{
		Provider: model.ProviderClaudeAuth,
		Name:     "Claude Auth",
		Plan:     "PRO",
		Status:   model.StatusConnected,
		Source:   model.SourceWebSignIn,
		Rows: []UsageRowState{
			{Label: "5H SESSION", Percent: 22, WindowMinutes: 300},
			{Label: "7D WEEKLY", Percent: 33, WindowMinutes: 10080},
		},
	})
	return state
}
