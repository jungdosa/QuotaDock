package ui

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/provider"
	"github.com/jungdosa/QuotaDock/internal/security"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

// Keep capture pixels fixed at the pre-refactor label while version plumbing changes.
const testAppVersion = "0.7." + "17"

type phase2Provider struct {
	id       model.ProviderID
	snapshot model.UsageSnapshot
	err      error
	inspect  model.ConnectionStatus
}

func (p phase2Provider) Inspect(context.Context) model.ConnectionState {
	if p.inspect != "" {
		return model.ConnectionState{Status: p.inspect}
	}
	if p.err != nil {
		return model.ConnectionState{Status: model.StatusError, Error: model.ErrUnavailable, ErrorKey: "error.unavailable"}
	}
	return model.ConnectionState{Status: model.StatusConnected}
}
func (p phase2Provider) Refresh(context.Context) (model.UsageSnapshot, error) {
	return p.snapshot, p.err
}
func (p phase2Provider) Reconnect(ctx context.Context) (model.UsageSnapshot, error) {
	return p.Refresh(ctx)
}
func (p phase2Provider) Close() error { return nil }

func sampleState() ViewState {
	reset := time.Now().Add(3 * time.Hour)
	return ViewState{LastRefresh: time.Date(2026, 7, 25, 15, 42, 18, 0, time.Local), Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Plan: "MAX 5X", Status: model.StatusConnected, Rows: []UsageRowState{
			{Label: "5H SESSION", Percent: 42, ResetsAt: reset, WindowMinutes: 300},
			{Label: "7D WEEKLY", Percent: 84, ResetsAt: reset.Add(24 * time.Hour), WindowMinutes: 10080},
		}},
		{Provider: model.ProviderCodex, Name: "Codex", Plan: "PLUS", Status: model.StatusConnected, CLIPath: `C:\Users\demo\.codex\bin\codex.exe`, CLIVersion: "0.145.0", Rows: []UsageRowState{
			{Label: "PRIMARY", Percent: 93, ResetsAt: reset, WindowMinutes: 300},
		}},
		{Provider: model.ProviderAntigravity, Name: "Antigravity", Plan: "AI ULTRA", Status: model.StatusConnected, Source: "Local LSP", Rows: []UsageRowState{
			{Label: "Gemini Models", Percent: 31, ResetsAt: reset, WindowMinutes: 300},
			{Label: "Claude and GPT models", Percent: 70, ResetsAt: reset, WindowMinutes: 300},
		}},
	}}
}

func newTestView(t *testing.T) (*View, fyne.Window) {
	return newTestViewWithActions(t, Actions{})
}

func newTestViewWithActions(t *testing.T, actions Actions) (*View, fyne.Window) {
	t.Helper()
	if actions.AppVersion == "" {
		actions.AppVersion = testAppVersion
	}
	a := test.NewApp()
	a.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	t.Cleanup(a.Quit)
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	w.SetPadded(false)
	config := settings.Default()
	config.Theme = settings.ThemeDark
	v := NewView(w.Canvas(), catalog, i18n.English, config, actions)
	w.SetContent(v.Root)
	v.SetState(sampleState())
	return v, w
}

func TestNormalCompactNanoRenderingAndExactlyFourScreens(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	// Four screens plus the passive tooltip layer stacked on top (W1).
	if len(v.Root.Objects) != 5 || v.Root.Objects[4] != v.tooltipLayer {
		t.Fatalf("root objects=%d, want 4 screens + tooltip layer on top", len(v.Root.Objects))
	}
	if v.MinimumSize(NormalScreen).Width != NormalWidth || v.MinimumSize(CompactScreen).Width != CompactWidth {
		t.Fatal("fixed screen widths mismatch")
	}
	v.Show(CompactScreen)
	if !v.Compact.Visible() || v.Normal.Visible() || v.Nano.Visible() || v.Settings.Visible() {
		t.Fatal("compact visibility mismatch")
	}
	v.Show(NanoScreen)
	if !v.Nano.Visible() || v.Normal.Visible() || v.Compact.Visible() || v.Settings.Visible() {
		t.Fatal("nano visibility mismatch")
	}
	v.Show(SettingsScreen)
	if !v.Settings.Visible() {
		t.Fatal("settings is not visible")
	}
}

func TestSettingsBackgroundRefreshPreservesScreenWithoutScroll(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	settingsRoot := v.Settings
	next := sampleState()
	next.Lanes[0].Rows[0].Percent = 91
	v.SetState(next)
	if v.Settings != settingsRoot {
		t.Fatal("provider refresh recreated settings screen")
	}
	scrolls := 0
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		if _, ok := object.(*container.Scroll); ok {
			scrolls++
		}
	})
	if scrolls != 0 {
		t.Fatalf("settings contains %d scroll containers, want none", scrolls)
	}
}

func TestSettingsToggleColumnLeavesRightMargin(t *testing.T) {
	rowLayout := &SettingRowLayout{LabelWidth: 122, Gap: 10, ControlWidth: 38, Height: 28}
	objects := []fyne.CanvasObject{canvas.NewRectangle(color.Transparent), NewToggle(false, nil)}
	rowLayout.Layout(objects, fyne.NewSize(560, 28))
	if objects[1].Position().X != 132 {
		t.Fatalf("toggle x=%v, want 132", objects[1].Position().X)
	}
	if objects[1].Position().X+objects[1].Size().Width >= 560 {
		t.Fatal("toggle was attached to the right edge")
	}
}

func TestSettingsUsesRequiredTwoColumnRows(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	assertPair := func(name string, object fyne.CanvasObject) {
		t.Helper()
		pair, ok := object.(*fyne.Container)
		if !ok || len(pair.Objects) != 2 {
			t.Fatalf("%s is not a two-column row", name)
		}
		pair.Resize(fyne.NewSize(560, 30))
		pair.Layout.Layout(pair.Objects, pair.Size())
		if pair.Objects[0].Position().X != 0 || pair.Objects[1].Position().X <= pair.Objects[0].Position().X {
			t.Fatalf("%s columns overlap: left=%v right=%v", name, pair.Objects[0].Position(), pair.Objects[1].Position())
		}
		if pair.Objects[1].Position().X+pair.Objects[1].Size().Width > pair.Size().Width {
			t.Fatalf("%s right column is clipped: position=%v size=%v pair=%v", name, pair.Objects[1].Position(), pair.Objects[1].Size(), pair.Size())
		}
	}

	usage := v.usageSettings().(*fyne.Container)
	if len(usage.Objects) != 4 {
		t.Fatalf("expanded usage rows=%d, want 4", len(usage.Objects))
	}
	for index, name := range map[int]string{0: "provider row 1", 1: "provider row 2", 2: "display and alerts", 3: "thresholds"} {
		assertPair(name, usage.Objects[index])
	}

	behavior := v.behaviorSettings().(*fyne.Container)
	if len(behavior.Objects) != 2 {
		t.Fatalf("behavior rows=%d, want 2", len(behavior.Objects))
	}
	assertPair("behavior row 1", behavior.Objects[0])
	assertPair("behavior row 2", behavior.Objects[1])

	display := v.displaySettings().(*fyne.Container)
	if len(display.Objects) != 1 {
		t.Fatalf("display rows=%d, want 1", len(display.Objects))
	}
	assertPair("display language and date/time", display.Objects[0])
}

func TestTitlebarDragRegionsDoNotOverlapButtons(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	hasDragSurface := func(root fyne.CanvasObject) bool {
		found := false
		walkCanvasObject(root, func(object fyne.CanvasObject) {
			if _, ok := object.(*DragSurface); ok {
				found = true
			}
		})
		return found
	}
	assertSeparated := func(name string, bar *fyne.Container, width float32, dragIndex int, buttonIndexes ...int) {
		t.Helper()
		bar.Resize(fyne.NewSize(width, 42))
		if len(bar.Objects) != 2 {
			t.Fatalf("%s titlebar stack objects=%d, want background and content row", name, len(bar.Objects))
		}
		var row *fyne.Container
		walkCanvasObject(bar.Objects[1], func(object fyne.CanvasObject) {
			candidate, ok := object.(*fyne.Container)
			if !ok {
				return
			}
			switch candidate.Layout.(type) {
			case *ColumnLayout, *GapColumnLayout:
				row = candidate
			}
		})
		if row == nil {
			t.Fatalf("%s titlebar has no column content row", name)
		}
		if !hasDragSurface(row.Objects[dragIndex]) {
			t.Fatalf("%s title region has no DragSurface", name)
		}
		for _, index := range buttonIndexes {
			if hasDragSurface(row.Objects[index]) {
				t.Fatalf("%s button %d shares a subtree with DragSurface", name, index)
			}
			if _, ok := row.Objects[index].(*SmallButton); !ok {
				t.Fatalf("%s button %d is %T", name, index, row.Objects[index])
			}
		}
		dragRegion := row.Objects[dragIndex]
		for _, index := range buttonIndexes {
			button := row.Objects[index]
			if index < dragIndex && button.Position().X+button.Size().Width > dragRegion.Position().X {
				t.Fatalf("%s left button %d ends at %.1f but drag region starts at %.1f", name, index, button.Position().X+button.Size().Width, dragRegion.Position().X)
			}
			if index > dragIndex && dragRegion.Position().X+dragRegion.Size().Width > button.Position().X {
				t.Fatalf("%s drag region ends at %.1f but right button %d starts at %.1f", name, dragRegion.Position().X+dragRegion.Size().Width, index, button.Position().X)
			}
		}
	}

	assertSeparated("widget", v.windowTitle(settings.ModeNormal), NormalWidth, 0, 1, 2, 3, 4, 5, 6)
	back := NewSmallIconButton(theme.NavigateBackIcon(), v.text(i18n.KeyClose), nil, v.colors)
	assertSeparated("settings", v.settingsTitleBar(back, testAppVersion, nil), SettingsWidth, 1, 0, 2, 3, 5, 6)
}

func TestTitlebarButtonsReceiveCanvasClicks(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	defer a.Quit()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	clicks := map[string]int{}
	v := NewView(nil, catalog, i18n.English, settings.Default(), Actions{
		ToggleCompact: func() { clicks["compact"]++ },
		Refresh:       func() { clicks["refresh"]++ },
		OpenSettings:  func() { clicks["settings"]++ },
		Minimize:      func() { clicks["minimize"]++ },
		Close:         func() { clicks["close"]++ },
	})
	window := test.NewWindow(nil)
	defer window.Close()
	window.SetPadded(false)
	bar := v.windowTitle(settings.ModeNormal)
	window.SetContent(bar)
	window.Resize(fyne.NewSize(NormalWidth, TitleBarHeight))
	row := bar.Objects[1].(*fyne.Container)
	for name, index := range map[string]int{"compact": 1, "refresh": 2, "settings": 4, "minimize": 5, "close": 6} {
		button := row.Objects[index]
		test.TapCanvas(window.Canvas(), fyne.NewPos(button.Position().X+button.Size().Width/2, TitleBarHeight/2))
		if clicks[name] != 1 {
			t.Fatalf("%s button canvas click count=%d, want 1", name, clicks[name])
		}
	}
}

func TestDragSurfaceUsesGrabOffsetInsteadOfFyneDelta(t *testing.T) {
	starts, moves, ends := 0, 0, 0
	var gotX, gotY int
	surface := NewDragSurface(func() (int, int, error) {
		starts++
		return 41, 19, nil
	}, func(offsetX, offsetY int) error {
		moves++
		gotX, gotY = offsetX, offsetY
		return nil
	}, func() {
		ends++
	})
	surface.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(7.5, -3.25)})
	surface.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-500, 900)})
	if starts != 1 || moves != 2 || gotX != 41 || gotY != 19 {
		t.Fatalf("drag callbacks starts=%d moves=%d offset=(%d,%d)", starts, moves, gotX, gotY)
	}
	surface.DragEnd()
	if ends != 1 {
		t.Fatalf("drag ends=%d, want 1", ends)
	}
	surface.Dragged(&fyne.DragEvent{})
	if starts != 2 {
		t.Fatalf("drag start state was not reset: starts=%d", starts)
	}
}

func TestDragSurfaceStartsOnFirstDraggedAndIsNotMouseable(t *testing.T) {
	cursorX, cursorY := 130, 75
	windowX, windowY := 100, 50
	starts, ends := 0, 0
	surface := NewDragSurface(func() (int, int, error) {
		starts++
		return cursorX - windowX, cursorY - windowY, nil
	}, func(offsetX, offsetY int) error {
		windowX = cursorX - offsetX
		windowY = cursorY - offsetY
		return nil
	}, func() {
		ends++
	})

	if _, mouseable := any(surface).(desktop.Mouseable); mouseable {
		t.Fatal("drag surface implements desktop.Mouseable, which prevents Fyne drag gesture delivery")
	}
	cursorX, cursorY = 145, 84
	surface.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-500, 900)})
	if starts != 1 || windowX != 100 || windowY != 50 {
		t.Fatalf("first dragged callback starts=%d window=(%d,%d), want 1 and (100,50)", starts, windowX, windowY)
	}
	cursorX, cursorY = 151, 79
	surface.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(900, -500)})
	if windowX != 106 || windowY != 45 || starts != 1 {
		t.Fatalf("absolute drag moved window to (%d,%d), starts=%d; want (106,45), 1", windowX, windowY, starts)
	}
	surface.DragEnd()
	if ends != 1 {
		t.Fatalf("drag end callbacks=%d, want 1", ends)
	}
}
func TestCanvasDragTargetsTitlebarSurface(t *testing.T) {
	starts, moves, ends := 0, 0, 0
	var gotX, gotY int
	view, window := newTestView(t)
	defer window.Close()
	view.Actions.BeginWindowDrag = func() (int, int, error) {
		starts++
		return 23, 11, nil
	}
	view.Actions.MoveWindow = func(offsetX, offsetY int) error {
		moves++
		gotX, gotY = offsetX, offsetY
		return nil
	}
	view.Actions.EndWindowDrag = func() { ends++ }
	// Rebuild so the titlebars capture the updated action.
	view.build()
	window.SetContent(view.Root)
	window.Resize(view.MinimumSize(NormalScreen))
	test.Drag(window.Canvas(), fyne.NewPos(300, 17), 18, 11)
	if starts != 1 || moves == 0 || ends != 1 || gotX != 23 || gotY != 11 {
		t.Fatalf("canvas drag callbacks starts=%d moves=%d ends=%d offset=(%d,%d)", starts, moves, ends, gotX, gotY)
	}
}

func TestCanvasClickTargetsReachTitlebarAndSettingsControls(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	defer a.Quit()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	defer w.Close()
	w.SetPadded(false)
	clicks := map[string]int{}
	actions := Actions{
		ToggleCompact: func() { clicks["compact"]++ },
		Refresh:       func() { clicks["refresh"]++ },
		OpenSettings:  func() { clicks["settings"]++ },
		Minimize:      func() { clicks["minimize"]++ },
		Close:         func() { clicks["close"]++ },
		ConfigChanged: func(settings.Config) { clicks["config"]++ },
	}
	v := NewView(w.Canvas(), catalog, i18n.English, settings.Default(), actions)
	w.SetContent(v.Root)
	v.Show(NormalScreen)
	w.Resize(v.MinimumSize(NormalScreen))
	titleBar := v.windowTitle(settings.ModeNormal)
	w.SetContent(titleBar)
	w.Resize(fyne.NewSize(NormalWidth, TitleBarHeight))
	row := titleBar.Objects[1].(*fyne.Container)
	for name, index := range map[string]int{"compact": 1, "refresh": 2, "settings": 4, "minimize": 5, "close": 6} {
		button := row.Objects[index]
		position := fyne.NewPos(button.Position().X+button.Size().Width/2, TitleBarHeight/2)
		test.TapCanvas(w.Canvas(), position)
		if clicks[name] != 1 {
			t.Fatalf("%s titlebar click count=%d, want 1", name, clicks[name])
		}
	}

	w.SetContent(v.Root)
	v.Show(SettingsScreen)
	w.Resize(v.MinimumSize(SettingsScreen))
	before := v.config.ShowClaude
	test.TapCanvas(w.Canvas(), fyne.NewPos(168, 107))
	if v.config.ShowClaude == before || clicks["config"] == 0 {
		t.Fatal("settings toggle did not receive the canvas click")
	}
	v.SetState(sampleState())
	v.Show(NormalScreen)
	w.Resize(v.MinimumSize(NormalScreen))
	meter := v.normalCache.rows[0].meter
	meterPosition := fyne.CurrentApp().Driver().AbsolutePositionForObject(meter)
	test.TapCanvas(w.Canvas(), meterPosition.Add(fyne.NewPos(meter.Size().Width/2, meter.Size().Height/2)))
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("palette click did not open a popover")
	}
}

func TestProviderCoordinatorFeedsNormalizedUIOnly(t *testing.T) {
	snap := model.UsageSnapshot{Provider: model.ProviderClaude, Plan: "PRO", FetchedAt: time.Now(), Limits: []model.UsageLimit{{ID: "five", Label: "5H", UsedPercent: 40, RemainingPercent: 60}}}
	c := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderClaude: phase2Provider{id: model.ProviderClaude, snapshot: snap}}}, settings.Default())
	state := c.Refresh(context.Background())
	if len(state.Lanes[0].Rows) != 1 || state.Lanes[0].Rows[0].Percent != 40 {
		t.Fatalf("normalized state=%+v", state)
	}
	for _, word := range []string{"token", "cookie", "csrf", "email", "credential"} {
		if strings.Contains(strings.ToLower(reflect.TypeOf(ViewState{}).String()), word) {
			t.Fatalf("UI state type contains %q", word)
		}
	}
}

func TestRawLimitIDNeverEntersUIStateOrRenderedText(t *testing.T) {
	if _, ok := reflect.TypeOf(UsageRowState{}).FieldByName("ID"); ok {
		t.Fatal("UI row state exposes a raw limit ID field")
	}
	snap := model.UsageSnapshot{Provider: model.ProviderCodex, Plan: "GO", Limits: []model.UsageLimit{{ID: "codex_bengalfo", Label: "Weekly", UsedPercent: 54, WindowMinutes: 10080}}}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderCodex: phase2Provider{snapshot: snap}}}, settings.Default())
	state := controller.Refresh(context.Background())
	if len(state.Lanes[1].Rows) != 1 || state.Lanes[1].Rows[0].Label != "Weekly" {
		t.Fatalf("normalized Codex UI row = %+v", state.Lanes[1].Rows)
	}
	v, w := newTestView(t)
	defer w.Close()
	v.SetState(state)
	walkCanvasObject(v.Root, func(object fyne.CanvasObject) {
		if text, ok := object.(*canvas.Text); ok && strings.Contains(text.Text, "codex_bengalfo") {
			t.Fatalf("raw limit ID rendered as %q", text.Text)
		}
	})
}

func TestAntigravityRowsAreSortedByModelGroupThenAscendingWindowDuration(t *testing.T) {
	snap := model.UsageSnapshot{Provider: model.ProviderAntigravity, Limits: []model.UsageLimit{
		{Label: "Claude/GPT Models", UsedPercent: 30, WindowMinutes: 300},
		{Label: "Gemini Models", UsedPercent: 70, WindowMinutes: 10080},
		{Label: "Claude/GPT Models", UsedPercent: 80, WindowMinutes: 10080},
		{Label: "Gemini Models", UsedPercent: 20, WindowMinutes: 300},
	}}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderAntigravity: phase2Provider{snapshot: snap}}}, settings.Default())
	rows := controller.Refresh(context.Background()).Lanes[2].Rows
	got := make([]string, len(rows))
	for i, row := range rows {
		got[i] = usageLabel(row, false)
	}
	want := []string{"Gemini 5H", "Gemini 7D", "Claude/GPT 5H", "Claude/GPT 7D"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Antigravity rows=%v, want model group then window %v", got, want)
	}
}

func TestDuplicateLaneLabelsUseUniqueHumanReadableWindowDurations(t *testing.T) {
	snap := model.UsageSnapshot{Provider: model.ProviderCodex, Limits: []model.UsageLimit{
		{ID: "internal_alpha", Label: "Weekly", UsedPercent: 60, WindowMinutes: 8640},
		{ID: "internal_beta", Label: "Weekly", UsedPercent: 0, WindowMinutes: 10080},
	}}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderCodex: phase2Provider{snapshot: snap}}}, settings.Default())
	rows := controller.Refresh(context.Background()).Lanes[1].Rows
	seen := make(map[string]bool, len(rows))
	got := make([]string, len(rows))
	for i, row := range rows {
		label := usageLabel(row, false)
		if seen[label] {
			t.Fatalf("duplicate rendered label %q", label)
		}
		seen[label] = true
		got[i] = label
		if strings.Contains(label, "internal_") {
			t.Fatalf("raw server ID leaked through label %q", label)
		}
		if width := fyne.MeasureText(label, NormalLabelTextSize, fyne.TextStyle{Bold: true}).Width; width+NormalLabelPadding > NormalLabelMaxWidth {
			t.Fatalf("label %q width %.1f + padding exceeds max column %.1f", label, width, NormalLabelMaxWidth)
		}
	}
	want := []string{"Weekly · 6d", "Weekly · 7d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex labels=%v, want %v", got, want)
	}
}

func TestEqualCodexWindowDurationsUseResetOrderedOrdinals(t *testing.T) {
	reset := time.Now().UTC().Truncate(time.Second)
	snap := model.UsageSnapshot{Provider: model.ProviderCodex, Limits: []model.UsageLimit{
		{ID: "internal_later", Label: "Weekly", UsedPercent: 38, WindowMinutes: 10080, ResetsAt: reset.Add(24 * time.Hour)},
		{ID: "internal_sooner", Label: "Weekly", UsedPercent: 62, WindowMinutes: 10080, ResetsAt: reset},
	}}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderCodex: phase2Provider{snapshot: snap}}}, settings.Default())
	rows := controller.Refresh(context.Background()).Lanes[1].Rows
	if len(rows) != 2 {
		t.Fatalf("Codex rows=%d, want 2", len(rows))
	}
	got := []string{usageLabel(rows[0], false), usageLabel(rows[1], false)}
	want := []string{"Weekly (1)", "Weekly (2)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("equal-duration Codex labels=%v, want %v", got, want)
	}
	if rows[0].Percent != 62 || !rows[0].ResetsAt.Equal(reset) {
		t.Fatalf("first ordinal is not the sooner reset: %+v", rows[0])
	}
	for _, label := range got {
		if strings.Contains(label, "internal_") {
			t.Fatalf("raw server ID leaked through label %q", label)
		}
		if width := fyne.MeasureText(label, NormalLabelTextSize, fyne.TextStyle{Bold: true}).Width; width+NormalLabelPadding > NormalLabelMaxWidth {
			t.Fatalf("label %q width %.1f + padding exceeds max column %.1f", label, width, NormalLabelMaxWidth)
		}
	}
}

func TestKoreanCodexDuplicateWeeklyRowsUseResetOrderedOrdinals(t *testing.T) {
	reset := time.Now().UTC().Truncate(time.Second)
	snap := model.UsageSnapshot{Provider: model.ProviderCodex, Limits: []model.UsageLimit{
		{Label: "Weekly", UsedPercent: 38, WindowMinutes: 10080, ResetsAt: reset.Add(24 * time.Hour)},
		{Label: "Weekly", UsedPercent: 62, WindowMinutes: 10080, ResetsAt: reset},
	}}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{model.ProviderCodex: phase2Provider{snapshot: snap}}}, settings.Default())
	state := controller.Refresh(context.Background())
	v, window := newTestView(t)
	defer window.Close()
	config := v.config
	config.Language = settings.LanguageKorean
	v.SetConfig(config)
	v.SetState(state)

	rows := v.state.Lanes[1].Rows
	got := []string{v.usageRowLabel(v.state.Lanes[1], rows[0]), v.usageRowLabel(v.state.Lanes[1], rows[1])}
	want := []string{"주간 (1)", "주간 (2)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Korean Codex labels=%v, want %v", got, want)
	}
	if rows[0].Percent != 62 || !rows[0].ResetsAt.Equal(reset) {
		t.Fatalf("first Korean ordinal is not the sooner reset: %+v", rows[0])
	}
}

func TestClaudeWeeklySortsBeforeFableWeeklyAtEqualDuration(t *testing.T) {
	rows := []UsageRowState{
		{Label: "seven_day_fable", WindowMinutes: 10080},
		{Label: "Weekly", WindowMinutes: 10080},
		{Label: "Session", WindowMinutes: 300},
	}
	sortLaneRows(model.ProviderClaude, rows)
	got := []string{rows[0].Label, rows[1].Label, rows[2].Label}
	want := []string{"Session", "Weekly", "seven_day_fable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Claude row order=%v, want %v", got, want)
	}
}

func TestConnectedUsageUnavailableShowsStatusWithoutZeroPercentOrMeter(t *testing.T) {
	snapshot := model.UsageSnapshot{Provider: model.ProviderClaude, Plan: "MAX"}
	unavailable := model.SafeError{Code: model.ErrUsageUnavailable, Key: i18n.KeyErrorUsageUnavailable}
	controller := NewController(provider.Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderClaude: phase2Provider{snapshot: snapshot, err: unavailable, inspect: model.StatusConnected},
	}}, settings.Default())
	state := controller.Refresh(context.Background())
	lane := state.Lanes[0]
	if lane.Status != model.StatusConnected || lane.Error != model.ErrUsageUnavailable || lane.Plan != "MAX" {
		t.Fatalf("connected usage-unavailable lane = %+v", lane)
	}
	v, w := newTestView(t)
	defer w.Close()
	v.SetState(state)
	// Compact content is synchronized lazily when it becomes visible.
	v.Show(CompactScreen)
	statusCount, zeroCount, meterCount := 0, 0, 0
	walkCanvasObject(v.Root, func(object fyne.CanvasObject) {
		switch value := object.(type) {
		case *canvas.Text:
			if value.Text == "Usage information unavailable" {
				statusCount++
			}
			if value.Text == "0%" || value.Text == "  0%" {
				zeroCount++
			}
		case *SegmentedMeter:
			meterCount++
		}
	})
	if statusCount < 2 || zeroCount != 0 || meterCount != 0 {
		t.Fatalf("usage unavailable rendered status=%d zero=%d meters=%d", statusCount, zeroCount, meterCount)
	}
}

func TestNormalStatusRowMatchesUsageRowIndentAndHeight(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	lane := LaneState{Provider: model.ProviderClaude, Name: "Claude", ErrorKey: i18n.KeyErrorUsageUnavailable}
	status := v.normalStatusRow(lane).(*fyne.Container)
	usage := v.normalUsageRow(lane, UsageRowState{Label: "Weekly", Percent: 25, WindowMinutes: 10080}).(*fyne.Container)
	status.Resize(fyne.NewSize(NormalWidth, status.MinSize().Height))
	usage.Resize(fyne.NewSize(NormalWidth, usage.MinSize().Height))
	if status.MinSize().Height != usage.MinSize().Height || status.MinSize().Height != NormalRowHeight {
		t.Fatalf("status height=%v usage height=%v want=%v", status.MinSize().Height, usage.MinSize().Height, NormalRowHeight)
	}
	if len(status.Objects) != len(usage.Objects) || status.Objects[0].Position().X != usage.Objects[0].Position().X || status.Objects[0].Size().Width != usage.Objects[0].Size().Width {
		t.Fatalf("status first column position/width=%v/%v, usage=%v/%v", status.Objects[0].Position().X, status.Objects[0].Size().Width, usage.Objects[0].Position().X, usage.Objects[0].Size().Width)
	}
}

func TestLaneHeaderShowsAllowedPlanWithoutDecorativeConnectionDot(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	header := v.laneHeader(LaneState{Provider: model.ProviderCodex, Name: "Codex", Plan: "GO", Status: model.StatusConnected})
	header.Resize(header.MinSize())
	foundPlan, foundDot := false, false
	walkCanvasObject(header, func(object fyne.CanvasObject) {
		switch value := object.(type) {
		case *canvas.Text:
			foundPlan = foundPlan || value.Text == "GO"
		case *canvas.Circle:
			foundDot = true
		}
	})
	if !foundPlan || foundDot {
		t.Fatalf("lane header plan=%v dot=%v", foundPlan, foundDot)
	}
}

func TestPaletteControllerHasSixteenAllowedColorsAndRejectsCSS(t *testing.T) {
	if len(security.PaletteIDs()) != 16 {
		t.Fatalf("palette size=%d", len(security.PaletteIDs()))
	}
	b := NewPaletteButton("blue", nil, nil)
	b.SetID("#fff")
	if b.ID != "blue" {
		t.Fatal("arbitrary CSS color was accepted")
	}
	b.SetID("amber")
	if b.ID != "amber" {
		t.Fatal("allowed color was rejected")
	}
	b.Dismiss()
}

func TestLongestLocalizedTextMeasuredInsideReservedColumns(t *testing.T) {
	for _, label := range []string{"Claude/GPT 5H", "Fable Weekly", "Gemini Session", "Claude 주간"} {
		usage := fyne.MeasureText(label, NormalLabelTextSize, fyne.TextStyle{Bold: true})
		if usage.Width+NormalLabelPadding > NormalLabelMaxWidth {
			t.Fatalf("usage label %q width + padding %.1f exceeds dynamic maximum %.1f", label, usage.Width+NormalLabelPadding, NormalLabelMaxWidth)
		}
	}
	for _, metadata := range []string{"2h 32m 남음", "7.25 오전 3:00", "초기화 7.29 15:00"} {
		for _, line := range wrapMonospace(metadata, normalRowColumns[2], NormalMetaTextSize) {
			if fyne.MeasureText(line, NormalMetaTextSize, fyne.TextStyle{Monospace: true}).Width > normalRowColumns[2] {
				t.Fatalf("reset metadata line %q exceeds %.1f", line, normalRowColumns[2])
			}
		}
	}
}

func TestPhase3HTypographyMeetsReadableMinimums(t *testing.T) {
	if TitleTextSize < 13 {
		t.Fatalf("title text size=%.1f, want at least 13", TitleTextSize)
	}
	if PlanChipTextSize != 9 {
		t.Fatalf("plan chip text size=%.1f, want Phase 3T size 9", PlanChipTextSize)
	}
	for name, size := range map[string]float32{
		"lane header":     LaneHeaderTextSize,
		"normal label":    NormalLabelTextSize,
		"normal metadata": NormalMetaTextSize,
		"compact label":   CompactLabelTextSize,
		"settings text":   SettingsTextSize,
	} {
		if size < 11 {
			t.Fatalf("%s text size=%.1f, want at least 11", name, size)
		}
	}
}

func TestPlanChipTextIsVerticallyCentered(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	header := v.laneHeader(LaneState{Provider: model.ProviderClaude, Name: "Claude", Plan: "MAX 20X", Status: model.StatusConnected}).(*fyne.Container)
	header.Resize(header.MinSize())
	chip := header.Objects[1].(*fyne.Container)
	chip.Resize(fyne.NewSize(chip.MinSize().Width, chip.MinSize().Height+8))

	var textCenter float32
	var findText func(fyne.CanvasObject, fyne.Position)
	findText = func(object fyne.CanvasObject, offset fyne.Position) {
		if value, ok := object.(*canvas.Text); ok {
			textCenter = offset.Y + value.Size().Height/2
			return
		}
		if value, ok := object.(*fyne.Container); ok {
			for _, child := range value.Objects {
				findText(child, offset.Add(child.Position()))
			}
		}
	}
	findText(chip, fyne.NewPos(0, 0))
	if delta := math.Abs(float64(textCenter - chip.Size().Height/2)); delta > 1 {
		t.Fatalf("plan chip text center=%.1f chip center=%.1f delta=%.1f", textCenter, chip.Size().Height/2, delta)
	}
}

func phase2DTestView(t *testing.T) (*View, fyne.Window) {
	t.Helper()
	v, w := newTestView(t)
	cfg := v.config
	cfg.Language = settings.LanguageEnglish
	v.SetConfig(cfg)
	v.SetState(DemoViewState())
	v.Show(CompactScreen)
	return v, w
}

func compactTestRows(v *View) []*fyne.Container {
	rows := make([]*fyne.Container, 0, len(v.compactBody.Objects))
	for _, object := range v.compactBody.Objects {
		// The caption strip is also a background+content pair, so skip it
		// explicitly instead of relying on the child count alone.
		if v.compactCache != nil && object == v.compactCache.columnHeader {
			continue
		}
		if row, ok := object.(*fyne.Container); ok && len(row.Objects) == 2 {
			rows = append(rows, row)
		}
	}
	return rows
}

func compactTestRowContent(t *testing.T, row *fyne.Container) *fyne.Container {
	t.Helper()
	if len(row.Objects) != 2 {
		t.Fatalf("compact row objects=%d, want background + content", len(row.Objects))
	}
	content, ok := row.Objects[1].(*fyne.Container)
	if !ok {
		t.Fatalf("compact row content=%T, want *fyne.Container", row.Objects[1])
	}
	return content
}

func TestPhase2DNormalEnglishLabelsMatchTerminology(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	got := make([]string, 0, 9)
	for _, lane := range v.state.Lanes {
		for _, row := range lane.Rows {
			got = append(got, v.usageRowLabel(lane, row))
		}
	}
	want := []string{"Session", "Weekly", "Fable Weekly", "Session", "Weekly", "Gemini Session", "Gemini Weekly", "Claude Session", "Claude Weekly"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normal English labels=%v, want %v", got, want)
	}
}

func TestPhase2DCompactLabelsMatchNormalLabels(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	for _, lane := range v.state.Lanes {
		for _, row := range lane.Rows {
			compact := v.compactUsageRow(lane, row, false).(*fyne.Container)
			content := compactTestRowContent(t, compact)
			if got, want := content.Objects[1].(*canvas.Text).Text, v.usageRowLabel(lane, row); got != want {
				t.Fatalf("compact label=%q, normal label=%q", got, want)
			}
		}
	}
}

func TestPhase3KCompactUsesVerifiedProviderIconOnEveryRow(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	rows := compactTestRows(v)
	for index, row := range rows {
		content := compactTestRowContent(t, row)
		icon, hasIcon := content.Objects[0].(*canvas.Image)
		if !hasIcon {
			t.Fatalf("compact row %d icon=%T, want *canvas.Image", index, content.Objects[0])
		}
		if icon.MinSize() != fyne.NewSize(providerIconSize, providerIconSize) {
			t.Fatalf("compact row %d icon min size=%v", index, icon.MinSize())
		}
		if icon.Image == nil || countOpaque(icon.Image) == 0 || icon.FillMode != canvas.ImageFillContain {
			t.Fatalf("compact row %d verified fallback rendered no ink", index)
		}
	}
}

func TestPhase3KProviderIconsUseMappedEmbeddedAssets(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	rows := compactTestRows(v)
	want := []struct {
		index int
		kind  ProviderIconKind
		asset string
	}{{0, ProviderIconClaude, "claude.svg"}, {3, ProviderIconCodex, "openai.svg"}, {5, ProviderIconGemini, "gemini.svg"}, {7, ProviderIconAGClaude, "claude.svg"}}
	for _, expected := range want {
		_ = compactTestRowContent(t, rows[expected.index]).Objects[0].(*canvas.Image)
		resource := providerIconResource(expected.kind)
		if providerIconAsset(expected.kind) != expected.asset || !strings.Contains(string(resource.Content()), "<svg") {
			t.Fatalf("row %d icon source asset=%q resource=%q", expected.index, providerIconAsset(expected.kind), resource.Name())
		}
	}
}

func TestPhase3KCompactRowsHaveResetBarsAndProviderTints(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	if len(v.compactCache.rows) != 9 {
		t.Fatalf("compact cache rows=%d, want 9", len(v.compactCache.rows))
	}
	for index, handles := range v.compactCache.rows {
		if handles.reset == nil || handles.reset.Height < 1 || handles.reset.Height > 2 {
			t.Fatalf("compact row %d reset bar=%v", index, handles.reset)
		}
	}
	for _, pair := range [][2]int{{0, 3}, {3, 5}, {5, 7}} {
		if sameColor(v.compactCache.rows[pair[0]].background.FillColor, v.compactCache.rows[pair[1]].background.FillColor) {
			t.Fatalf("provider row backgrounds %d/%d are identical", pair[0], pair[1])
		}
	}
	// Reset bars always show the remaining share of the window.
	if got, want := v.compactCache.rows[0].reset.Value, 100-resetProgress(v.state.Lanes[0].Rows[0], time.Now()); math.Abs(got-want) > 0.01 {
		t.Fatalf("reset progress=%.2f, want %.2f", got, want)
	}
}

func TestPhase3WCompactShowsUsageModeHeaderOnce(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.UsageMode = settings.UsageRemaining
	v.SetConfig(cfg)
	matches := 0
	walkCanvasObject(v.Compact, func(object fyne.CanvasObject) {
		if label, ok := object.(*canvas.Text); ok && label.Text == "Remaining" {
			matches++
		}
	})
	if matches != 1 || v.compactCache.usageHeader.Text != "Remaining" {
		t.Fatalf("compact Remaining headers=%d cache=%q, want one top-row header", matches, v.compactCache.usageHeader.Text)
	}
}

func TestPhase2DCompactLabelsUseCurrentLanguageDynamicWidth(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	if got := v.MinimumSize(CompactScreen).Width; got != CompactWidth {
		t.Fatalf("compact width=%v, want %v", got, CompactWidth)
	}
	if got, limit := v.MinimumSize(CompactScreen).Height, float32(263)+UsageHeaderRowHeight+1+4*CompactDividerPaddingY; got > limit {
		t.Fatalf("compact height=%v exceeds Phase 3W divider+header budget %.0f", got, limit)
	}
	for _, language := range []settings.Language{settings.LanguageEnglish, settings.LanguageKorean} {
		cfg := v.config
		cfg.Language = language
		v.SetConfig(cfg)
		v.Show(CompactScreen)
		w.Resize(v.MinimumSize(CompactScreen))
		lanes := v.visibleLanes()
		maximum := float32(0)
		for _, lane := range lanes {
			for _, row := range lane.Rows {
				maximum = max(maximum, fyne.MeasureText(v.usageRowLabel(lane, row), CompactLabelTextSize, fyne.TextStyle{Bold: true}).Width)
			}
		}
		want := min(CompactLabelMaxWidth, max(CompactLabelMinWidth, float32(math.Ceil(float64(maximum+CompactLabelPadding)))))
		if got := v.compactCache.labelWidth; got != want || got < maximum {
			t.Fatalf("%s compact label width=%.1f, want %.1f for longest %.1f", language, got, want, maximum)
		}
		for index, row := range compactTestRows(v) {
			content := compactTestRowContent(t, row)
			label := content.Objects[1].(*canvas.Text)
			if label.Size().Width != want || label.MinSize().Width > label.Size().Width {
				t.Fatalf("%s row %d label %q size/min=%.1f/%.1f, want column %.1f", language, index, label.Text, label.Size().Width, label.MinSize().Width, want)
			}
			contentWidth := CompactWidth - CompactPaddingLeft - CompactPaddingRight
			if row.MinSize().Width > contentWidth {
				t.Fatalf("%s compact row %d min width %.1f exceeds content width %.1f", language, index, row.MinSize().Width, contentWidth)
			}
		}
		t.Logf("%s compact label longest=%.2f padding=%.0f calculated=%.0f", language, maximum, CompactLabelPadding, want)
	}

	before := v.compactCache.labelWidth
	cfg := v.config
	cfg.ShowAGGemini = false
	cfg.ShowAGClaude = false
	v.SetConfig(cfg)
	if got, want := v.compactCache.labelWidth, v.compactLabelWidth(v.visibleLanes()); got != want || got == before {
		t.Fatalf("provider toggle label width=%.1f, want recalculated %.1f (before %.1f)", got, want, before)
	}
}

func TestDemoModeTitlebarIsIdentifiable(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
	defer a.Quit()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	defer w.Close()
	v := NewView(w.Canvas(), catalog, i18n.English, settings.Default(), Actions{DemoMode: true})
	w.SetContent(v.Root)
	found := false
	walkCanvasObject(v.Normal, func(object fyne.CanvasObject) {
		if text, ok := object.(*canvas.Text); ok && text.Text == "DEMO" {
			found = true
		}
	})
	if !found {
		t.Fatal("demo titlebar marker is missing")
	}
}

func TestConnectionActionButtonsMatchProviderCards(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	countButtons := func(label string) int {
		count := 0
		walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
			if button, ok := object.(*SmallButton); ok && button.Label == label {
				count++
			}
		})
		return count
	}
	if got := countButtons(v.text(i18n.KeyReconnect)); got != 3 {
		t.Fatalf("reconnect buttons=%d, want one per provider", got)
	}
	if got := countButtons(v.text(i18n.KeyTestConnection)); got != 3 {
		t.Fatalf("connection test buttons=%d, want one per provider", got)
	}
	if got := countButtons(v.text(i18n.KeyConnect)); got != 0 {
		t.Fatalf("legacy connect buttons=%d, want none", got)
	}
}

func TestSettingsHeaderHelpAndProviderColorsMovedToMeters(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)
	texts := make(map[string]bool)
	buttons := make(map[string]*SmallButton)
	var helpButton *SmallButton
	paletteCount := 0
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		switch value := object.(type) {
		case *canvas.Text:
			texts[value.Text] = true
		case *SmallButton:
			buttons[value.Label] = value
			if value.Tooltip == v.text(i18n.KeyHelp) && value.Icon != nil {
				helpButton = value
			}
		case *PaletteButton:
			paletteCount++
		}
	})
	// The header shows the same all-caps product mark as the widget titlebars,
	// the version in smaller secondary type, and the screen name.
	for _, label := range []string{"QuotaDock", "v" + testAppVersion, "SETTINGS"} {
		if !texts[label] {
			t.Fatalf("settings header is missing %q", label)
		}
	}
	for _, label := range []string{"Update", "Done"} {
		if buttons[label] == nil {
			t.Fatalf("settings header is missing %q", label)
		}
	}
	if helpButton == nil || helpButton.Outlined {
		t.Fatal("settings header is missing the borderless help icon button")
	}
	if buttons["Update"].Disabled || !buttons["Update"].Outlined || !buttons["Done"].Primary {
		t.Fatal("settings update/done button hierarchy is incorrect")
	}
	for _, label := range []string{"Theme", "Claude color", "Codex color", "AG Gemini color", "AG Claude color"} {
		if texts[label] {
			t.Fatalf("settings still contains moved control %q", label)
		}
	}
	if paletteCount != 2 {
		t.Fatalf("palette controls=%d, want only 2 severity controls", paletteCount)
	}

	v.showHelp()
	if v.helpPopup == nil || !v.helpPopup.Visible() {
		t.Fatal("help button content did not open")
	}
	helpTexts := make(map[string]bool)
	providerBorders := 0
	walkCanvasObject(v.helpPopup.Content, func(object fyne.CanvasObject) {
		switch value := object.(type) {
		case *canvas.Text:
			helpTexts[value.Text] = true
		case *widget.RichText:
			for _, segment := range value.Segments {
				helpTexts[segment.Textual()] = true
			}
		case *canvas.Rectangle:
			for _, id := range []string{"orange", "gray", "slate"} {
				if sameColor(value.FillColor, v.colors.PaletteColor(id)) {
					providerBorders++
				}
			}
		}
	})
	for _, text := range []string{
		"Usage display guide",
		"Each service connects independently. If one service fails, usage for the others continues to be displayed.",
		"Selecting Claude sign-in opens an authorized Claude login window. After sign-in, usage is queried through the web session, so you never need to enter a password or token directly in the app.",
		"If the session expires and 'Sign in required' appears, sign in to Claude again.",
		"The official Codex CLI must be installed and signed in. The app automatically detects the installed CLI and local app-server and never asks for an authentication token.",
		"Check codex --version in a terminal. If it is not detected, complete Codex CLI sign-in and reconnect.",
		"Antigravity IDE must be running and signed in. QuotaDock connects only to the local LSP to read Gemini and Claude usage.",
		"If 'Not found' appears, start Antigravity IDE and reconnect.",
		"Raw authentication information is never shown in Settings or the usage widget.",
	} {
		if !helpTexts[text] {
			t.Fatalf("help content is missing %q", text)
		}
	}
	if providerBorders < 3 {
		t.Fatalf("help provider color borders=%d, want at least 3", providerBorders)
	}
}

func TestAntigravityGroupsHideIndependently(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.ShowAGGemini = false
	cfg.ShowAGClaude = true
	v.SetConfig(cfg)
	lanes := v.visibleLanes()
	ag := lanes[len(lanes)-1]
	if len(ag.Rows) != 1 || antigravityRowIsGemini(ag.Rows[0]) {
		t.Fatalf("AG Claude-only rows=%v", ag.Rows)
	}
	cfg.ShowAGGemini = true
	cfg.ShowAGClaude = false
	v.SetConfig(cfg)
	lanes = v.visibleLanes()
	ag = lanes[len(lanes)-1]
	if len(ag.Rows) != 1 || !antigravityRowIsGemini(ag.Rows[0]) {
		t.Fatalf("AG Gemini-only rows=%v", ag.Rows)
	}
}

func walkCanvasObject(object fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	visit(object)
	switch value := object.(type) {
	case *fyne.Container:
		for _, child := range value.Objects {
			walkCanvasObject(child, visit)
		}
	case *container.Scroll:
		walkCanvasObject(value.Content, visit)
	}
}

func TestLanguageSwitchRebuildsSettingsWithoutScroll(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)
	cfg := v.config
	cfg.Language = settings.LanguageKorean
	v.SetConfig(cfg)
	if v.text(i18n.KeySettingsTitle) != "설정" || len(v.Root.Objects) != 5 {
		t.Fatalf("language update title=%q root objects=%d (4 screens + tooltip layer)", v.text(i18n.KeySettingsTitle), len(v.Root.Objects))
	}
	scrolls := 0
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		if _, ok := object.(*container.Scroll); ok {
			scrolls++
		}
	})
	if scrolls != 0 {
		t.Fatalf("rebuilt settings contains %d scroll containers", scrolls)
	}
}

func TestThresholdEntriesShowValuesAndReflectCoupledClamp(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)

	v.warningEntry.OnSubmitted("95")

	if v.config.WarningPercent != 95 || v.config.DangerPercent != 96 {
		t.Fatalf("saved thresholds=%.0f/%.0f, want 95/96", v.config.WarningPercent, v.config.DangerPercent)
	}
	if v.warningEntry.Text != "95" || v.dangerEntry.Text != "96" {
		t.Fatalf("threshold entries=%q/%q, want 95/96", v.warningEntry.Text, v.dangerEntry.Text)
	}
	v.dangerEntry.OnSubmitted("not-a-number")
	if v.dangerEntry.Text != "96" {
		t.Fatalf("invalid danger threshold was not restored: %q", v.dangerEntry.Text)
	}
}
func TestPalettePopupKeyboardEscapeAndDismiss(t *testing.T) {
	_, w := newTestView(t)
	defer w.Close()
	button := NewPaletteButton("blue", w.Canvas(), nil)
	w.SetContent(button)
	button.ShowPalette()
	if button.popup == nil || !button.popup.Visible() {
		t.Fatal("palette popup did not open")
	}
	button.popup.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if button.popup.Visible() {
		t.Fatal("Escape did not close palette")
	}
	button.ShowPalette()
	button.Dismiss()
	if button.popup != nil {
		t.Fatal("outside-dismiss path retained popup")
	}
}

func TestMeterPaletteUpdatesColorWithoutReplacingCachedMeter(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(NormalScreen)
	w.Resize(v.MinimumSize(NormalScreen))
	meter := v.normalCache.rows[0].meter
	meter.Tapped(nil)
	if v.palettePopup == nil || !v.palettePopup.Visible() {
		t.Fatal("meter tap did not open the provider palette")
	}
	var swatches []*PaletteSwatch
	walkCanvasObject(v.palettePopup.Content, func(object fyne.CanvasObject) {
		if swatch, ok := object.(*PaletteSwatch); ok {
			swatches = append(swatches, swatch)
		}
	})
	if len(swatches) != 17 {
		t.Fatalf("provider palette swatches=%d, want reset + 16 colors", len(swatches))
	}
	selected := 0
	var purple *PaletteSwatch
	for _, swatch := range swatches {
		if swatch.Selected {
			selected++
		}
		if !swatch.Reset && sameColor(swatch.Fill, v.colors.PaletteColor("purple")) {
			purple = swatch
		}
	}
	if selected != 1 || purple == nil {
		t.Fatalf("selected swatches=%d purple=%v", selected, purple != nil)
	}
	purple.Tapped(nil)
	if v.config.ProviderColors["claude"] != "purple" {
		t.Fatalf("Claude color=%q, want purple", v.config.ProviderColors["claude"])
	}
	if v.palettePopup != nil {
		t.Fatal("selected provider palette retained its popup")
	}
	if v.normalCache.rows[0].meter != meter {
		t.Fatal("provider color update replaced the cached segmented meter")
	}

	meter.Tapped(nil)
	var reset *PaletteSwatch
	walkCanvasObject(v.palettePopup.Content, func(object fyne.CanvasObject) {
		if swatch, ok := object.(*PaletteSwatch); ok && swatch.Reset {
			reset = swatch
		}
	})
	if reset == nil {
		t.Fatal("provider palette has no reset swatch")
	}
	reset.Tapped(nil)
	if v.config.ProviderColors["claude"] != settings.Default().ProviderColors["claude"] {
		t.Fatalf("reset color=%q, want provider default", v.config.ProviderColors["claude"])
	}
	if v.normalCache.rows[0].meter != meter {
		t.Fatal("provider reset replaced the cached segmented meter")
	}
	for i := 0; i < 25; i++ {
		meter.Tapped(nil)
		if v.palettePopup == nil || len(w.Canvas().Overlays().List()) != 1 {
			t.Fatalf("palette repetition %d retained overlays=%d", i, len(w.Canvas().Overlays().List()))
		}
		v.dismissPalette()
		if v.palettePopup != nil || len(w.Canvas().Overlays().List()) != 0 {
			t.Fatalf("palette repetition %d did not release overlay", i)
		}
	}
	if v.normalCache.rows[0].meter != meter {
		t.Fatal("repeated palette opens replaced the cached segmented meter")
	}
}

func TestCompactMeterReusesProviderPaletteAndCachedMeter(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	var saved settings.Config
	v.Actions.ConfigChanged = func(config settings.Config) { saved = config }
	v.Show(CompactScreen)
	w.Resize(v.MinimumSize(CompactScreen))
	meter := v.compactCache.rows[0].meter
	if meter.OnTapped == nil {
		t.Fatal("compact meter has no shared palette tap handler")
	}
	meter.Tapped(nil)
	if v.palettePopup == nil || !v.palettePopup.Visible() {
		t.Fatal("compact meter tap did not open the provider palette")
	}
	var purple *PaletteSwatch
	walkCanvasObject(v.palettePopup.Content, func(object fyne.CanvasObject) {
		if swatch, ok := object.(*PaletteSwatch); ok && !swatch.Reset && sameColor(swatch.Fill, v.colors.PaletteColor("purple")) {
			purple = swatch
		}
	})
	if purple == nil {
		t.Fatal("compact provider palette has no purple swatch")
	}
	purple.Tapped(nil)
	if v.config.ProviderColors["claude"] != "purple" {
		t.Fatalf("compact palette saved Claude color=%q", v.config.ProviderColors["claude"])
	}
	if saved.ProviderColors["claude"] != "purple" {
		t.Fatal("compact palette selection did not invoke the shared persistence handler")
	}
	if v.compactCache.rows[0].meter != meter {
		t.Fatal("compact palette selection replaced the cached meter")
	}
}

func TestDisplayModeStateSurvivesSettingsChanges(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(NanoScreen)
	v.Show(SettingsScreen)
	cfg := v.config
	cfg.AlwaysOnTop = true
	v.SetConfig(cfg)
	if v.config.DisplayMode != settings.ModeNano {
		t.Fatalf("settings change reset display mode to %q", v.config.DisplayMode)
	}
}

func TestPhase3K2NanoStripUsesProviderCellsSeverityAndViewCache(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.WarningPercent = 75
	cfg.DangerPercent = 90
	v.SetConfig(cfg)
	v.Show(NanoScreen)
	w.Resize(v.MinimumSize(NanoScreen))

	if got := v.MinimumSize(NanoScreen); got.Width != NanoWidth || got.Height > TitleBarHeight+40 {
		t.Fatalf("nano minimum size=%v", got)
	}
	if len(v.nanoCache.cells) != 4 {
		t.Fatalf("nano cells=%d, want Claude/Codex/Gemini/AG Claude", len(v.nanoCache.cells))
	}
	wantRows := []int{2, 1, 2, 2}
	for index, want := range wantRows {
		if got := len(v.nanoCache.cells[index].rows); got != want {
			t.Fatalf("nano cell %d rows=%d, want %d", index, got, want)
		}
	}
	if v.nanoCache.cells[0].rows[0].label.Text != "5h" || v.nanoCache.cells[0].rows[1].label.Text != "7D" || v.nanoCache.cells[1].rows[0].label.Text != "7D" {
		t.Fatal("nano 5h/7D labels do not match the provider row rules")
	}
	if !sameColor(v.nanoCache.cells[3].rows[0].bar.Active, v.colors.PaletteColor(v.config.DangerColor)) {
		t.Fatal("nano danger row did not use the shared danger color")
	}
	if !sameColor(v.nanoCache.cells[3].rows[1].bar.Active, v.colors.PaletteColor(v.config.WarningColor)) {
		t.Fatal("nano warning row did not use the shared warning color")
	}

	cache := v.nanoCache
	firstBar := cache.cells[0].rows[0].bar
	next := DemoViewState()
	next.Lanes[0].Rows[0].Percent = 43
	v.SetState(next)
	if v.nanoCache != cache || v.nanoCache.cells[0].rows[0].bar != firstBar || firstBar.Value != 43 {
		t.Fatal("stable nano refresh replaced cached widgets instead of updating in place")
	}
}

func TestPhase3K2NanoBodyTapReturnsToCompactAndSupportsDrag(t *testing.T) {
	selected := settings.ModeNano
	starts, moves, ends := 0, 0, 0
	surface := NewNanoSurface(func() (int, int, error) {
		starts++
		return 3, 4, nil
	}, func(int, int) error {
		moves++
		return nil
	}, func() { ends++ }, func() { selected = settings.ModeCompact }, nil)
	surface.Tapped(nil)
	if selected != settings.ModeCompact {
		t.Fatalf("nano body tap selected mode=%q", selected)
	}
	surface.Dragged(&fyne.DragEvent{})
	surface.DragEnd()
	if starts != 1 || moves != 1 || ends != 1 {
		t.Fatalf("nano drag callbacks=%d/%d/%d", starts, moves, ends)
	}
}

func TestPhase3K2DisplayModeIconsAndScreenMapping(t *testing.T) {
	if ScreenForDisplayMode(settings.ModeNormal) != NormalScreen || ScreenForDisplayMode(settings.ModeCompact) != CompactScreen || ScreenForDisplayMode(settings.ModeNano) != NanoScreen {
		t.Fatal("display mode screen mapping mismatch")
	}
	for _, entry := range []struct {
		current settings.DisplayMode
		want    string
		marker  string
	}{
		// W5: rectangles for normal/compact targets, a line for the nano target.
		{settings.ModeNormal, "display-compact.svg", "<rect"},
		{settings.ModeCompact, "display-nano.svg", "<path"},
		{settings.ModeNano, "display-normal.svg", "<rect"},
	} {
		resource := displayModeResource(entry.current, DarkBrandColors)
		if resource.Name() != entry.want || !strings.Contains(string(resource.Content()), entry.marker) {
			t.Fatalf("current mode %q icon=%q, want %q containing %q", entry.current, resource.Name(), entry.want, entry.marker)
		}
	}
}

func TestPhase3K2VisualReviewCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3K2_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3K2_SCREENSHOT_DIR for Phase 3K-2 captures")
	}
	v, w := phase2DTestView(t)
	defer w.Close()
	cfg := DemoConfig(v.config)
	v.SetConfig(cfg)
	v.SetState(DemoViewState())

	save := func(name string) {
		t.Helper()
		path := filepath.Join(directory, name+".png")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, w.Canvas().Capture()); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	for _, entry := range []struct {
		name   string
		screen Screen
	}{{"normal-mode", NormalScreen}, {"compact-mode", CompactScreen}, {"nano-mode", NanoScreen}} {
		v.Show(entry.screen)
		w.Resize(v.MinimumSize(entry.screen))
		save(entry.name)
	}
	v.Show(CompactScreen)
	w.Resize(v.MinimumSize(CompactScreen))
	v.compactCache.rows[0].meter.Tapped(nil)
	save("compact-palette")
}

func TestPhase3PConnectionSettingsCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3P_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3P_SCREENSHOT_DIR for Phase 3P captures")
	}
	for _, entry := range []struct {
		name  string
		theme settings.Theme
	}{{"light-settings", settings.ThemeLight}, {"dark-settings", settings.ThemeDark}} {
		capturePhase3PConnectionSettings(t, entry.theme, filepath.Join(directory, entry.name+".png"))
	}
}

func capturePhase3PConnectionSettings(t *testing.T, mode settings.Theme, path string) {
	t.Helper()
	a := test.NewApp()
	a.Settings().SetTheme(NewBrandTheme(mode))
	defer a.Quit()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	w.SetPadded(false)
	defer w.Close()
	config := settings.Default()
	config.Theme = mode
	v := NewView(w.Canvas(), catalog, i18n.English, config, Actions{})
	w.SetContent(v.Root)
	state := sampleState()
	state.Lanes[0].Status = model.StatusLoggedOut
	state.Lanes[0].Error = model.ErrNotLoggedIn
	state.Lanes[0].ErrorKey = i18n.KeyErrorNotLoggedIn
	v.SetState(state)
	v.Show(SettingsScreen)
	w.Resize(v.MinimumSize(SettingsScreen))
	if v.MinimumSize(SettingsScreen).Height < SettingsHeight || len(v.connectionCache) != 3 || len(v.connectionsBody.Objects) != 3 {
		t.Fatalf("settings layout height=%.1f cards=%d/%d", v.MinimumSize(SettingsScreen).Height, len(v.connectionCache), len(v.connectionsBody.Objects))
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, w.Canvas().Capture()); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestThemeCyclesFromTitlebarAndSettingsContainsNoThemeSelect(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	config := settings.Default()
	config.Theme = settings.ThemeLight
	var saved []settings.Theme
	v := NewView(nil, catalog, i18n.English, config, Actions{ConfigChanged: func(cfg settings.Config) {
		saved = append(saved, cfg.Theme)
	}})
	for _, want := range []settings.Theme{settings.ThemeDark, settings.ThemeSystem, settings.ThemeLight} {
		v.cycleTheme()
		if v.config.Theme != want {
			t.Fatalf("cycled theme=%q, want %q", v.config.Theme, want)
		}
		if len(saved) == 0 || saved[len(saved)-1] != want {
			t.Fatalf("last saved theme=%v, want %q", saved, want)
		}
	}
	themeSelects := 0
	walkCanvasObject(v.displaySettings(), func(object fyne.CanvasObject) {
		if _, ok := object.(*widget.Select); ok {
			themeSelects++
		}
	})
	if themeSelects != 2 {
		t.Fatalf("display selects=%d, want language + date/time only", themeSelects)
	}
}

func TestRefreshFeedbackAndLastRefreshLabel(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.SetRefreshing(true)
	if len(v.refreshButtons) != 3 {
		t.Fatalf("refresh buttons=%d, want normal + compact + nano", len(v.refreshButtons))
	}
	for _, button := range v.refreshButtons {
		if !button.Disabled || button.Icon == nil || button.Icon.Name() != "refresh-busy.svg" {
			t.Fatalf("busy refresh button disabled=%v icon=%v", button.Disabled, button.Icon)
		}
	}
	v.SetRefreshing(false)
	for _, button := range v.refreshButtons {
		if button.Disabled || button.Icon == nil || button.Icon.Name() == "refresh-busy.svg" {
			t.Fatalf("completed refresh button disabled=%v icon=%v", button.Disabled, button.Icon)
		}
	}
	state := sampleState()
	state.LastRefresh = time.Date(2026, 7, 25, 3, 4, 5, 0, time.Local)
	v.SetState(state)
	// The footer follows the global date/time setting (W4): default 12h-date.
	if v.lastRefreshText == nil || !strings.Contains(v.lastRefreshText.Text, "Jul 25 3:04 AM") {
		t.Fatalf("last refresh label=%v", v.lastRefreshText)
	}
	cfg := v.config
	cfg.DateTimeFormat = settings.Format24HourDate
	v.SetConfig(cfg)
	if !strings.Contains(v.lastRefreshText.Text, "Jul 25 03:04") {
		t.Fatalf("last refresh label did not follow format change: %v", v.lastRefreshText.Text)
	}
}

func TestTitlebarIconsAreSixteenPixels(t *testing.T) {
	button := NewSmallIconButton(theme.SettingsIcon(), "settings", nil, DarkBrandColors)
	renderer := button.CreateRenderer().(*smallButtonRenderer)
	renderer.Layout(fyne.NewSize(24, 24))
	if renderer.icon.Size() != fyne.NewSize(16, 16) {
		t.Fatalf("titlebar icon size=%v, want 16x16", renderer.icon.Size())
	}
}

func TestSeverityIsTripleEncoded(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	_, normal := v.severity(42)
	_, warning := v.severity(85)
	_, danger := v.severity(95)
	if !sameColor(normal, ColorPercentNormal) || sameColor(warning, danger) || !sameColor(warning, PaletteColor("amber")) || !sameColor(danger, PaletteColor("red")) {
		t.Fatal("severity palette mismatch")
	}
}

func TestDefaultProviderAndPlanChipColors(t *testing.T) {
	config := settings.Default()
	cases := []struct {
		name string
		id   model.ProviderID
		row  UsageRowState
		want color.Color
	}{
		// Defaults mirror the brand-logo icon hues: Claude orange, Codex gray,
		// AG Gemini violet, AG Claude slate.
		{"Claude", model.ProviderClaude, UsageRowState{}, color.NRGBA{R: 0xDD, G: 0x9A, B: 0x63, A: 0xFF}},
		{"Codex", model.ProviderCodex, UsageRowState{}, color.NRGBA{R: 0x9A, G: 0xA7, B: 0xB7, A: 0xFF}},
		{"Antigravity Gemini", model.ProviderAntigravity, UsageRowState{Label: "Gemini Models"}, color.NRGBA{R: 0x9B, G: 0x8C, B: 0xEC, A: 0xFF}},
		{"Antigravity Claude/GPT", model.ProviderAntigravity, UsageRowState{Label: "Claude/GPT Models"}, color.NRGBA{R: 0x7E, G: 0x8F, B: 0xA6, A: 0xFF}},
	}
	// 네 공급자 그룹의 기본색은 서로 달라야 한다. 같으면 한 화면에서 구분되지 않는다.
	seen := map[color.Color]string{}
	for _, tc := range cases {
		got := PaletteColor(providerColorID(tc.id, tc.row, config))
		for previous, name := range seen {
			if sameColor(got, previous) {
				t.Fatalf("%s shares its default color with %s", tc.name, name)
			}
		}
		seen[got] = tc.name
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PaletteColor(providerColorID(tc.id, tc.row, config))
			if !sameColor(got, tc.want) {
				t.Fatalf("provider color=%v, want %v", got, tc.want)
			}
		})
	}
	if !sameColor(ColorPlanChip, color.NRGBA{R: 0x78, G: 0x8C, B: 0xA3, A: 0xFF}) ||
		!sameColor(ColorPlanChipText, ColorBackground) {
		t.Fatal("plan chip colors do not match PLAN §25.5")
	}

	saved := config
	saved.ProviderColors = map[string]string{"claude": "purple", "codex": "lime", "antigravity": "cyan"}
	demo := DemoConfig(saved)
	if demo.ProviderColors["claude"] != "orange" || demo.ProviderColors["codex"] != "gray" ||
		demo.ProviderColors["antigravity"] != "slate" || demo.ProviderColors["antigravity-gemini"] != "violet" {
		t.Fatalf("demo provider colors are not deterministic: %v", demo.ProviderColors)
	}
}

func TestDemoSessionRingArcLengthsTrackElapsedWindow(t *testing.T) {
	state := DemoViewState()
	claude := state.Lanes[0].Rows[0]
	codex := state.Lanes[1].Rows[0]
	claudeProgress := resetProgress(claude, time.Time{})
	codexProgress := resetProgress(codex, time.Time{})
	if math.Abs(claudeProgress-44.33) > 0.02 || math.Abs(codexProgress-77.33) > 0.02 {
		t.Fatalf("session progress Claude=%.2f%% Codex=%.2f%%, want 44.33%%/77.33%%", claudeProgress, codexProgress)
	}

	stroke := RingStroke * 4
	claudePixels := countRingForeground(RenderRingImage(88, 88, claudeProgress, stroke, ColorLabel, ColorTrack), ColorLabel)
	codexPixels := countRingForeground(RenderRingImage(88, 88, codexProgress, stroke, ColorLabel, ColorTrack), ColorLabel)
	fullPixels := countRingForeground(RenderRingImage(88, 88, 100, stroke, ColorLabel, ColorTrack), ColorLabel)
	if math.Abs(float64(claudePixels)/float64(fullPixels)-claudeProgress/100) > 0.04 {
		t.Fatalf("Claude arc pixels=%d/%d do not represent %.2f%%", claudePixels, fullPixels, claudeProgress)
	}
	if math.Abs(float64(codexPixels)/float64(fullPixels)-codexProgress/100) > 0.04 {
		t.Fatalf("Codex arc pixels=%d/%d do not represent %.2f%%", codexPixels, fullPixels, codexProgress)
	}
	if float64(codexPixels) < float64(claudePixels)*1.5 {
		t.Fatalf("ring arcs are not visually distinct: Claude=%dpx Codex=%dpx", claudePixels, codexPixels)
	}
}

func countRingForeground(img image.Image, foreground color.Color) int {
	r, g, b, _ := foreground.RGBA()
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if pixel.A > 0 && pixel.R == uint8(r>>8) && pixel.G == uint8(g>>8) && pixel.B == uint8(b>>8) {
				count++
			}
		}
	}
	return count
}

func TestDemoStateMatchesVisualReviewFixture(t *testing.T) {
	state := DemoViewState()
	if len(state.Lanes) != 3 {
		t.Fatalf("lane count=%d, want 3", len(state.Lanes))
	}
	if state.Lanes[0].Plan != "MAX 20X" || state.Lanes[1].Plan != "PLUS" || state.Lanes[2].Plan != "AI ULTRA" {
		t.Fatalf("demo plans=%q/%q/%q", state.Lanes[0].Plan, state.Lanes[1].Plan, state.Lanes[2].Plan)
	}
	if len(state.Lanes[0].Rows) != 3 || len(state.Lanes[1].Rows) != 2 || len(state.Lanes[2].Rows) != 4 {
		t.Fatal("demo row counts do not match the review fixture")
	}
	if state.Lanes[2].Rows[2].Percent != 91 || state.Lanes[2].Rows[3].Percent != 77 {
		t.Fatal("demo warning and danger rows are missing")
	}
	for _, lane := range state.Lanes {
		if lane.Status != model.StatusConnected {
			t.Fatalf("%s is not connected", lane.Name)
		}
		for _, row := range lane.Rows {
			if !row.DisplayOverride || row.DisplayRemaining == "" || row.DisplayReset == "" {
				t.Fatalf("%s/%s is missing fixed display data", lane.Name, row.Label)
			}
		}
	}
}

func TestNormalHeightTracksRenderedContent(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	sampleHeight := v.MinimumSize(NormalScreen).Height
	v.SetState(DemoViewState())
	demoHeight := v.MinimumSize(NormalScreen).Height
	t.Logf("normal height sample=%.1f demo=%.1f", sampleHeight, demoHeight)
	if demoHeight <= sampleHeight {
		t.Fatalf("demo height=%v, want greater than sample height=%v", demoHeight, sampleHeight)
	}
	if demoHeight != v.Normal.MinSize().Height {
		t.Fatalf("window height=%v, content height=%v", demoHeight, v.Normal.MinSize().Height)
	}
}

func TestLightDarkSystemThemeContrastAndBrandBackground(t *testing.T) {
	tests := []struct {
		name    string
		mode    settings.Theme
		variant fyne.ThemeVariant
		colors  BrandColors
	}{
		{name: "forced light ignores dark variant", mode: settings.ThemeLight, variant: theme.VariantDark, colors: LightBrandColors},
		{name: "forced dark ignores light variant", mode: settings.ThemeDark, variant: theme.VariantLight, colors: DarkBrandColors},
		{name: "system light", mode: settings.ThemeSystem, variant: theme.VariantLight, colors: LightBrandColors},
		{name: "system dark", mode: settings.ThemeSystem, variant: theme.VariantDark, colors: DarkBrandColors},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			brand := NewBrandTheme(test.mode)
			if got := brand.Color(theme.ColorNameBackground, test.variant); !sameColor(got, test.colors.Background) {
				t.Fatalf("background=%v, want %v", got, test.colors.Background)
			}
			if got := brand.Color(theme.ColorNameForeground, test.variant); !sameColor(got, test.colors.Text) {
				t.Fatalf("foreground=%v, want %v", got, test.colors.Text)
			}
		})
	}
}

func TestBrandThemeUsesMeasurablyWiderBoldFace(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	theme := NewBrandTheme(settings.ThemeDark)
	a.Settings().SetTheme(theme)
	if theme.font == nil || theme.boldFont == nil {
		t.Fatal("embedded regular or bold Pretendard font is unavailable")
	}
	if theme.font.Name() != "Pretendard-Regular.ttf" || theme.boldFont.Name() != "Pretendard-Bold.ttf" {
		t.Fatalf("font resources=%q/%q, want embedded Pretendard regular/bold", theme.font.Name(), theme.boldFont.Name())
	}
	regular := fyne.MeasureText("QuotaDock 설정", 14, fyne.TextStyle{})
	bold := fyne.MeasureText("QuotaDock 설정", 14, fyne.TextStyle{Bold: true})
	t.Logf("MeasureText regular=%.2f bold=%.2f", regular.Width, bold.Width)
	if bold.Width <= regular.Width {
		t.Fatalf("bold width=%.2f, want greater than regular width=%.2f", bold.Width, regular.Width)
	}
}

func TestSettingsExpandsWithoutScrollAndShowsConnections(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)
	expanded := v.MinimumSize(SettingsScreen)
	cfg := v.config
	cfg.WarningsEnabled = false
	v.SetConfig(cfg)
	collapsed := v.MinimumSize(SettingsScreen)
	if collapsed.Height < SettingsHeight || expanded.Height < collapsed.Height {
		t.Fatalf("settings heights expanded=%.1f collapsed=%.1f, want expanded >= collapsed >= %.1f", expanded.Height, collapsed.Height, SettingsHeight)
	}
	scrolls := 0
	walkCanvasObject(v.Settings, func(object fyne.CanvasObject) {
		if _, ok := object.(*container.Scroll); ok {
			scrolls++
		}
	})
	if scrolls != 0 {
		t.Fatalf("settings contains %d scroll containers", scrolls)
	}
	if len(v.connectionCache) != 3 || len(v.connectionsBody.Objects) != 3 {
		t.Fatalf("collapsed settings connection rows=%d/%d, want 3/3", len(v.connectionCache), len(v.connectionsBody.Objects))
	}
}
func TestLightSeverityColorsMeetWCAGThreeToOneContrast(t *testing.T) {
	if got, want := LightBrandColors.PaletteColor("amber"), (color.NRGBA{R: 0xA6, G: 0x6C, B: 0x16, A: 0xFF}); !sameColor(got, want) {
		t.Fatalf("light warning color=%v, want %v", got, want)
	}
	if got, want := DarkBrandColors.PaletteColor("amber"), (color.NRGBA{R: 0xE0, G: 0xA9, B: 0x4E, A: 0xFF}); !sameColor(got, want) {
		t.Fatalf("dark warning color=%v, want %v", got, want)
	}
	for name, severity := range map[string]color.Color{
		"warning": LightBrandColors.PaletteColor("amber"),
		"danger":  LightBrandColors.PaletteColor("red"),
	} {
		for surface, background := range map[string]color.Color{
			"background": LightBrandColors.Background,
			"track":      LightBrandColors.Track,
		} {
			ratio := wcagContrastRatio(severity, background)
			t.Logf("%s vs %s contrast=%.4f:1", name, surface, ratio)
			if ratio < 3 {
				t.Errorf("%s vs %s contrast=%.4f:1, want >= 3:1", name, surface, ratio)
			}
		}
	}
}

func TestProfessionalPaletteAndDedicatedToggleColors(t *testing.T) {
	if got, want := DarkBrandColors.PaletteColor("blue"), (color.NRGBA{R: 0x5B, G: 0x8D, B: 0xEF, A: 0xFF}); !sameColor(got, want) {
		t.Fatalf("dark professional blue=%v, want %v", got, want)
	}
	if got, want := DarkBrandColors.PaletteColor("red"), (color.NRGBA{R: 0xE0, G: 0x65, B: 0x6C, A: 0xFF}); !sameColor(got, want) {
		t.Fatalf("dark danger red=%v, want %v", got, want)
	}
	for _, test := range []struct {
		name   string
		colors BrandColors
	}{{"light", LightBrandColors}, {"dark", DarkBrandColors}} {
		t.Run(test.name, func(t *testing.T) {
			seen := make(map[color.NRGBA]string)
			for _, id := range security.PaletteIDs() {
				value := color.NRGBAModel.Convert(test.colors.PaletteColor(id)).(color.NRGBA)
				if previous, exists := seen[value]; exists {
					t.Fatalf("palette colors %q and %q are identical: %v", previous, id, value)
				}
				seen[value] = id
				if value.R == 0xFF || value.G == 0xFF || value.B == 0xFF {
					t.Fatalf("palette color %q uses a primary-channel extreme: %v", id, value)
				}
			}
			if sameColor(test.colors.ToggleOn, test.colors.Accent) {
				t.Fatal("ToggleOn is not separated from Accent")
			}
			toggle := NewToggle(true, nil, test.colors)
			renderer := toggle.CreateRenderer().(*toggleRenderer)
			renderer.Refresh()
			if !sameColor(renderer.track.FillColor, test.colors.ToggleOn) {
				t.Fatalf("toggle track=%v, want ToggleOn=%v", renderer.track.FillColor, test.colors.ToggleOn)
			}
			blue := color.NRGBAModel.Convert(test.colors.ToggleOn).(color.NRGBA)
			if blue.B <= blue.R || blue.B <= blue.G || blue.A == 0xFF {
				t.Fatalf("ToggleOn is not a muted translucent blue: %v", blue)
			}
			if sameColor(test.colors.TitleTop, test.colors.Background) || sameColor(test.colors.TitleDivider, test.colors.Background) {
				t.Fatal("title bar is not visually separated from the body")
			}
		})
	}
}
func wcagContrastRatio(first, second color.Color) float64 {
	firstLuminance := wcagRelativeLuminance(first)
	secondLuminance := wcagRelativeLuminance(second)
	if firstLuminance < secondLuminance {
		firstLuminance, secondLuminance = secondLuminance, firstLuminance
	}
	return (firstLuminance + 0.05) / (secondLuminance + 0.05)
}

func wcagRelativeLuminance(value color.Color) float64 {
	r, g, b, _ := value.RGBA()
	linear := func(component uint32) float64 {
		srgb := float64(component) / 65535
		if srgb <= 0.04045 {
			return srgb / 12.92
		}
		return math.Pow((srgb+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}
func TestLightAndDarkCanvasUseMatchingCustomPalettes(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	for _, test := range []struct {
		mode   settings.Theme
		colors BrandColors
	}{{settings.ThemeLight, LightBrandColors}, {settings.ThemeDark, DarkBrandColors}} {
		t.Run(string(test.mode), func(t *testing.T) {
			fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(test.mode))
			config := v.config
			config.Theme = test.mode
			v.SetConfig(config)
			v.SetState(DemoViewState())
			v.Show(NormalScreen)
			w.Resize(v.MinimumSize(NormalScreen))
			img := w.Canvas().Capture()
			if pixels := countExactPixels(img, test.colors.Background); pixels == 0 {
				t.Fatalf("capture has no %s background pixels", test.mode)
			}
			if pixels := countExactPixels(img, test.colors.Text); pixels == 0 {
				t.Fatalf("capture has no %s text pixels", test.mode)
			}
		})
	}
}

// iconInkMatches reports whether every solid pixel of an icon raster sits
// close to the expected colour. Anti-aliased edges (partial alpha) are ignored
// and solid pixels get a small tolerance: supersampled downscaling shifts the
// brand hue by a few units per channel.
func iconInkMatches(img image.Image, want color.NRGBA) bool {
	const tolerance = 12
	near := func(a, b uint8) bool {
		d := int(a) - int(b)
		return d >= -tolerance && d <= tolerance
	}
	found := false
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if pixel.A < 0xFA {
				continue
			}
			if !near(pixel.R, want.R) || !near(pixel.G, want.G) || !near(pixel.B, want.B) {
				return false
			}
			found = true
		}
	}
	return found
}

func TestScreenCaptureArtifacts(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	// Captures stand in for the shipping target, Windows 11, so OS-gated
	// settings render in their supported form rather than silently missing.
	v.Actions.TrayPromotionSupported = true
	outDir := os.Getenv("QUOTADOCK_SCREENSHOT_DIR")
	saveCapture := func(name string, img image.Image) {
		t.Helper()
		if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
			t.Fatalf("%s capture is empty", name)
		}
		if outDir == "" {
			return
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(outDir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, img); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	for _, mode := range []struct {
		name   string
		theme  settings.Theme
		colors BrandColors
	}{{"light", settings.ThemeLight, LightBrandColors}, {"dark", settings.ThemeDark, DarkBrandColors}} {
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(mode.theme))
		config := DemoConfig(v.config)
		config.Theme = mode.theme
		v.SetConfig(config)
		v.SetState(DemoViewState())
		screens := []struct {
			name   string
			screen Screen
			size   fyne.Size
		}{{"normal", NormalScreen, v.MinimumSize(NormalScreen)}, {"compact", CompactScreen, v.MinimumSize(CompactScreen)}, {"nano", NanoScreen, v.MinimumSize(NanoScreen)}, {"settings", SettingsScreen, v.MinimumSize(SettingsScreen)}}
		for _, entry := range screens {
			v.Show(entry.screen)
			w.Resize(entry.size)
			img := w.Canvas().Capture()
			if entry.screen == NormalScreen {
				wantColors := map[string]color.Color{
					"Claude orange": mode.colors.PaletteColor("orange"),
					"Codex gray":    mode.colors.PaletteColor("gray"),
					"Gemini violet": mode.colors.PaletteColor("violet"),
					"warning amber": mode.colors.PaletteColor("amber"),
					"danger red":    mode.colors.PaletteColor("red"),
					"plan slate":    mode.colors.PlanChip,
				}
				for name, target := range wantColors {
					pixels := countExactPixels(img, target)
					t.Logf("%s normal capture %s=%d exact pixels", mode.name, name, pixels)
					if pixels == 0 {
						t.Fatalf("%s normal capture is missing %s", mode.name, name)
					}
				}
			}
			if entry.screen == CompactScreen {
				// Both AG Claude demo rows sit in warning/danger, so their icon
				// keeping its fixed brand colour while the meter wears a
				// severity colour proves the W9 role split.
				agClaude := v.compactCache.rows[len(v.compactCache.rows)-1]
				if agClaude.icon == nil {
					t.Fatalf("%s compact AG Claude row has no provider icon", mode.name)
				}
				brand := color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}
				if !iconInkMatches(agClaude.icon.Image, brand) {
					t.Fatalf("%s compact AG Claude icon ink is not the fixed brand colour", mode.name)
				}
				if sameColor(agClaude.meter.Active, mode.colors.PaletteColor("slate")) {
					t.Fatalf("%s compact AG Claude meter did not switch to a severity colour", mode.name)
				}
			}
			saveCapture(mode.name+"-"+entry.name, img)
		}

		installState := DemoViewState()
		installState.Lanes[1].Status = model.StatusUnavailable
		installState.Lanes[1].Error = model.ErrCLINotInstalled
		installState.Lanes[1].ErrorKey = i18n.KeyErrorCLINotInstalled
		v.SetState(installState)
		v.Show(SettingsScreen)
		v.connectionCache[1].methods[0].button.Tapped(nil)
		w.Resize(v.MinimumSize(SettingsScreen))
		saveCapture(mode.name+"-settings-install", w.Canvas().Capture())
		v.closeConnectionPanel()
		v.SetState(DemoViewState())

		v.Show(SettingsScreen)
		w.Resize(v.MinimumSize(SettingsScreen))
		v.showHelp()
		saveCapture(mode.name+"-help", w.Canvas().Capture())
		v.helpPopup.Hide()
		v.helpPopup = nil
	}

	for _, mode := range []struct {
		name  string
		theme settings.Theme
	}{{"theme-light", settings.ThemeLight}, {"theme-dark", settings.ThemeDark}, {"theme-system", settings.ThemeSystem}} {
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(mode.theme))
		config := DemoConfig(v.config)
		config.Theme = mode.theme
		v.SetConfig(config)
		v.SetState(DemoViewState())
		v.Show(NormalScreen)
		w.Resize(v.MinimumSize(NormalScreen))
		saveCapture(mode.name, w.Canvas().Capture())
	}

	config := DemoConfig(v.config)
	config.Theme = settings.ThemeDark
	fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(config.Theme))
	v.SetConfig(config)
	v.SetState(DemoViewState())
	v.Show(NormalScreen)
	w.Resize(v.MinimumSize(NormalScreen))
	v.SetRefreshing(true)
	saveCapture("refresh-busy", w.Canvas().Capture())
	v.SetRefreshing(false)
	meter := v.normalCache.rows[0].meter
	meter.Tapped(nil)
	saveCapture("palette-popup", w.Canvas().Capture())
	var purple *PaletteSwatch
	walkCanvasObject(v.palettePopup.Content, func(object fyne.CanvasObject) {
		if swatch, ok := object.(*PaletteSwatch); ok && !swatch.Reset && sameColor(swatch.Fill, v.colors.PaletteColor("purple")) {
			purple = swatch
		}
	})
	if purple == nil {
		t.Fatal("capture palette has no purple swatch")
	}
	purple.Tapped(nil)
	saveCapture("palette-applied", w.Canvas().Capture())
	v.Show(SettingsScreen)
	w.Resize(v.MinimumSize(SettingsScreen))
	saveCapture("settings-clean", w.Canvas().Capture())
}
func TestPhase2BDefectReviewCapture(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	cfg := v.config
	cfg.Language = settings.LanguageKorean
	v.SetConfig(cfg)
	reset := time.Now().Add(3 * time.Hour)
	v.SetState(ViewState{Lanes: []LaneState{
		{Provider: model.ProviderClaude, Name: "Claude", Plan: "MAX", Status: model.StatusConnected, Error: model.ErrUsageUnavailable, ErrorKey: i18n.KeyErrorUsageUnavailable},
		{Provider: model.ProviderCodex, Name: "Codex", Plan: "PRO LITE", Status: model.StatusConnected, Rows: []UsageRowState{
			{Label: "Weekly", Percent: 60, WindowMinutes: 8640, ResetsAt: reset.Add(5 * 24 * time.Hour)},
			{Label: "Weekly", Percent: 0, WindowMinutes: 10080, ResetsAt: reset.Add(6 * 24 * time.Hour)},
		}},
		{Provider: model.ProviderAntigravity, Name: "Antigravity", Plan: "AI ULTRA", Status: model.StatusConnected, Rows: []UsageRowState{
			{Label: "Gemini Models", Percent: 8, WindowMinutes: 300, ResetsAt: reset},
			{Label: "Gemini Models", Percent: 24, WindowMinutes: 10080, ResetsAt: reset.Add(6 * 24 * time.Hour)},
			{Label: "Claude/GPT Models", Percent: 31, WindowMinutes: 300, ResetsAt: reset},
			{Label: "Claude/GPT Models", Percent: 70, WindowMinutes: 10080, ResetsAt: reset.Add(6 * 24 * time.Hour)},
		}},
	}})
	v.Show(NormalScreen)
	w.Resize(v.MinimumSize(NormalScreen))
	texts := make(map[string]bool)
	walkCanvasObject(v.Normal, func(object fyne.CanvasObject) {
		if value, ok := object.(*canvas.Text); ok {
			texts[value.Text] = true
		}
	})
	for _, expected := range []string{"사용량 정보 없음", "PRO LITE", "60", "%", "주간 · 6d", "주간 · 7d", "AI ULTRA", "Gemini 세션", "Gemini 주간", "Claude 세션", "Claude 주간"} {
		if !texts[expected] {
			t.Fatalf("Phase 2B review frame is missing %q", expected)
		}
	}
	for forbidden := range texts {
		if forbidden == "0%" || strings.Contains(forbidden, "codex_bengalfo") {
			t.Fatalf("Phase 2B review frame contains forbidden text %q", forbidden)
		}
	}
	img := w.Canvas().Capture()
	if countExactPixels(img, ColorPlanChip) == 0 {
		t.Fatal("Phase 2B review frame is missing plan chips")
	}
	if path := os.Getenv("QUOTADOCK_PHASE2B_SCREENSHOT"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, img); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func countExactPixels(img image.Image, target color.Color) int {
	r, g, b, a := target.RGBA()
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if pixel.R == uint8(r>>8) && pixel.G == uint8(g>>8) && pixel.B == uint8(b>>8) && pixel.A == uint8(a>>8) {
				count++
			}
		}
	}
	return count
}

func TestSetStateReusesVisibleWidgetsWhenStructureIsStable(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	cache := v.normalCache
	objects := append([]fyne.CanvasObject(nil), v.normalBody.Objects...)
	firstMeter := cache.rows[0].meter
	firstPercent := cache.rows[0].percent
	firstPercentSymbol := cache.rows[0].percentSymbol
	resizeCalls := 0
	v.Actions.ResizeWindow = func(fyne.Size) { resizeCalls++ }

	next := sampleState()
	next.Lanes[0].Rows[0].Percent = 83
	v.SetState(next)

	if v.normalCache != cache {
		t.Fatal("stable state replaced the normal widget cache")
	}
	if firstMeter != v.normalCache.rows[0].meter || firstPercent != v.normalCache.rows[0].percent ||
		firstPercentSymbol != v.normalCache.rows[0].percentSymbol {
		t.Fatal("stable state replaced row widget handles")
	}
	if len(objects) != len(v.normalBody.Objects) {
		t.Fatalf("normal object count changed from %d to %d", len(objects), len(v.normalBody.Objects))
	}
	for index := range objects {
		if objects[index] != v.normalBody.Objects[index] {
			t.Fatalf("normal object %d was replaced", index)
		}
	}
	v.SetState(next)
	if resizeCalls != 1 {
		t.Fatalf("stable minimum size triggered %d resize calls, want 1", resizeCalls)
	}
}

func TestPhase3KCompactSetStateReusesMetersResetBarsAndBackgrounds(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	cache := v.compactCache
	row := cache.rows[0]
	objects := append([]fyne.CanvasObject(nil), v.compactBody.Objects...)

	next := DemoViewState()
	next.Lanes[0].Rows[0].Percent = 43
	next.Lanes[0].Rows[0].DisplayRemainingPercent = 40
	v.SetState(next)

	if v.compactCache != cache {
		t.Fatal("stable compact state replaced the view cache")
	}
	updated := v.compactCache.rows[0]
	if updated.meter != row.meter || updated.reset != row.reset || updated.background != row.background || updated.label != row.label || updated.percent != row.percent {
		t.Fatal("stable compact state replaced cached row handles")
	}
	// The reset bar shows the remaining share directly (DisplayRemainingPercent).
	if updated.meter.Value != 43 || updated.reset.Value != 40 {
		t.Fatalf("cached values meter=%.0f reset=%.0f, want 43/40", updated.meter.Value, updated.reset.Value)
	}
	for index := range objects {
		if objects[index] != v.compactBody.Objects[index] {
			t.Fatalf("compact object %d was replaced", index)
		}
	}
}

func TestHiddenBodyDefersStructuralRebuildUntilShown(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	compactCache := v.compactCache
	compactObjects := append([]fyne.CanvasObject(nil), v.compactBody.Objects...)

	v.SetState(DemoViewState())
	if v.compactCache != compactCache {
		t.Fatal("hidden compact body was rebuilt during normal-screen refresh")
	}
	if len(v.compactBody.Objects) != len(compactObjects) {
		t.Fatal("hidden compact object structure changed before display")
	}

	v.Show(CompactScreen)
	if v.compactCache == compactCache {
		t.Fatal("compact body did not rebuild when its structure changed on display")
	}
	if len(v.compactCache.rows) != 9 {
		t.Fatalf("compact cached rows=%d, want 9", len(v.compactCache.rows))
	}
}

func TestConnectionCardsAndButtonsAreReused(t *testing.T) {
	v, w := newTestView(t)
	defer w.Close()
	v.Show(SettingsScreen)
	row := v.connectionCache[0]
	status := row.status
	detail := row.detail
	methodButton := row.methods[0].button
	panel := row.panel
	reconnect := row.reconnect
	help := row.helpButton

	state := sampleState()
	state.Lanes[0].Status = model.StatusLoggedOut
	v.SetState(state)
	if v.connectionCache[0] != row || row.status != status || row.detail != detail || row.methods[0].button != methodButton || row.panel != panel || row.reconnect != reconnect || row.helpButton != help {
		t.Fatal("connection state update replaced cached widgets")
	}
	if !strings.Contains(row.status.Text, "Sign in") {
		t.Fatalf("logged-out card status=%q", row.status.Text)
	}
	if methodButton.State != connectionMethodMissing {
		t.Fatalf("logged-out Claude CLI state=%v, want missing", methodButton.State)
	}

	state.Lanes[0].Status = model.StatusUnavailable
	state.Lanes[0].Error = model.ErrCLINotInstalled
	state.Lanes[0].ErrorKey = i18n.KeyErrorCLINotInstalled
	v.SetState(state)
	if len(row.panel.Objects) != 0 || row.panelOpen || methodButton.State != connectionMethodMissing {
		t.Fatal("missing CLI opened guidance before its method button was selected")
	}
	methodButton.Tapped(nil)
	if len(row.panel.Objects) != 1 || !row.panelOpen || row.panelView.rescanButton == nil || row.panelView.docsButton == nil {
		t.Fatal("cached CLI installation guidance was not attached inline")
	}

	state.Lanes[0].Status = model.StatusConnected
	state.Lanes[0].Error = model.ErrNone
	state.Lanes[0].ErrorKey = ""
	v.SetState(state)
	if len(row.panel.Objects) != 1 || !row.panelOpen || methodButton.State != connectionMethodActive || row.panelView.docsButton != nil {
		t.Fatal("selected CLI panel did not switch from install guidance to active details")
	}
}

func TestConnectionCardActionsHelpAndInstallLinks(t *testing.T) {
	var inspected, reconnected []model.ProviderID
	var opened []string
	v, w := newTestViewWithActions(t, Actions{
		Inspect:   func(id model.ProviderID) { inspected = append(inspected, id) },
		Reconnect: func(id model.ProviderID) { reconnected = append(reconnected, id) },
		OpenURL: func(raw string) error {
			opened = append(opened, raw)
			return nil
		},
	})
	defer w.Close()
	v.Show(SettingsScreen)

	v.connectionCache[1].testButton.Tapped(nil)
	v.connectionCache[2].testButton.Tapped(nil)
	v.connectionCache[0].reconnect.Tapped(nil)
	v.connectionCache[1].reconnect.Tapped(nil)
	if !reflect.DeepEqual(inspected, []model.ProviderID{model.ProviderCodex, model.ProviderAntigravity}) {
		t.Fatalf("inspect callbacks=%v", inspected)
	}
	if !reflect.DeepEqual(reconnected, []model.ProviderID{model.ProviderClaude, model.ProviderCodex}) {
		t.Fatalf("reconnect callbacks=%v", reconnected)
	}

	providers := []model.ProviderID{model.ProviderClaude, model.ProviderCodex, model.ProviderAntigravity}
	for index, id := range providers {
		v.connectionCache[index].helpButton.Tapped(nil)
		if v.helpPopup == nil || !v.helpPopup.Visible() {
			t.Fatalf("%s help popup is not visible", id)
		}
		texts := map[string]bool{}
		walkCanvasObject(v.helpPopup.Content, func(object fyne.CanvasObject) {
			switch value := object.(type) {
			case *canvas.Text:
				texts[value.Text] = true
			case *widget.RichText:
				for _, segment := range value.Segments {
					if text, ok := segment.(*widget.TextSegment); ok {
						texts[text.Text] = true
					}
				}
			}
		})
		_, descriptionKey, retryKey := connectionHelpKeys(id)
		if !texts[v.text(descriptionKey)] || !texts[v.text(retryKey)] {
			t.Fatalf("%s card help did not reuse provider-specific i18n text", id)
		}
		v.helpPopup.Hide()
		v.helpPopup = nil
	}

	state := sampleState()
	for _, index := range []int{0, 1} {
		state.Lanes[index].Status = model.StatusUnavailable
		state.Lanes[index].Error = model.ErrCLINotInstalled
		state.Lanes[index].ErrorKey = i18n.KeyErrorCLINotInstalled
	}
	v.SetState(state)
	if v.connectionCache[0].methods[0].button.State != connectionMethodMissing || v.connectionCache[1].methods[0].button.State != connectionMethodMissing {
		t.Fatal("Claude/Codex CLI methods are not in the missing state")
	}
	v.connectionCache[0].methods[0].button.Tapped(nil)
	v.connectionCache[0].panelView.rescanButton.Tapped(nil)
	v.connectionCache[0].panelView.docsButton.Tapped(nil)
	v.connectionCache[1].methods[0].button.Tapped(nil)
	v.connectionCache[1].panelView.rescanButton.Tapped(nil)
	v.connectionCache[1].panelView.docsButton.Tapped(nil)
	if !reflect.DeepEqual(inspected, []model.ProviderID{model.ProviderCodex, model.ProviderAntigravity, model.ProviderClaude, model.ProviderCodex}) {
		t.Fatalf("inspect callbacks after panel rescans=%v", inspected)
	}
	if !reflect.DeepEqual(opened, []string{claudeInstallURL, codexInstallURL}) {
		t.Fatalf("opened installation URLs=%v", opened)
	}
}

func TestRasterRingCachesImageBetweenPaintsAndEqualValues(t *testing.T) {
	ring := NewRasterRing(25, ColorAccent, ColorTrack)
	renderer := ring.CreateRenderer()
	imageObject := renderer.Objects()[0].(*canvas.Image)
	initial := imageObject.Image

	renderer.Refresh()
	ring.SetPercent(25, ColorAccent, ColorTrack)
	if imageObject.Image != initial {
		t.Fatal("paint or equal SetPercent regenerated the cached ring image")
	}

	ring.SetPercent(26, ColorAccent, ColorTrack)
	if imageObject.Image == initial {
		t.Fatal("changed percent did not regenerate the cached ring image")
	}
}
func TestRingDPIImagesStayContinuous(t *testing.T) {
	for _, size := range []int{22, 28, 33, 44} {
		img := RenderRingImage(size, size, 67, RingStroke*float64(size)/22, ColorAccent, ColorTrack)
		if countOpaque(img) < size*2 {
			t.Fatalf("%dpx ring has too few painted pixels", size)
		}
	}
}
func countOpaque(img image.Image) int {
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				count++
			}
		}
	}
	return count
}
func TestRingCandidatesImplementSameContract(t *testing.T) {
	candidates := []RingCandidate{NewRasterRing(25, ColorAccent, ColorTrack), NewFrameRing(25, ColorAccent, ColorTrack), NewExtensionRing(25, ColorAccent, ColorTrack)}
	for _, candidate := range candidates {
		candidate.Resize(fyne.NewSize(22, 22))
		candidate.SetPercent(91, ColorDanger, ColorTrack)
		if candidate.CandidateName() == "" {
			t.Fatal("candidate missing name")
		}
	}
}
func BenchmarkRingCandidates(b *testing.B) {
	b.Run("A_RasterFrame", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = RenderRingImage(88, 88, float64(i%101), RingStroke*4, ColorAccent, ColorTrack)
		}
	})
	frame := NewFrameRing(0, ColorAccent, ColorTrack)
	b.Run("B_101FrameSwap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			frame.SetPercent(float64(i%101), ColorAccent, ColorTrack)
		}
	})
	extension := NewExtensionRing(0, ColorAccent, ColorTrack)
	renderer := extension.CreateRenderer()
	renderer.Layout(fyne.NewSize(22, 22))
	b.Run("C_Vector72Refresh", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			extension.percent = float64(i % 101)
			renderer.Refresh()
		}
	})
}
func BenchmarkNineByTwentyMeterRefresh(b *testing.B) {
	meters := make([]*SegmentedMeter, 9)
	for i := range meters {
		meters[i] = NewSegmentedMeter(20, 0, color.White)
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, meter := range meters {
			meter.SetValue(float64(n%101), ColorAccent)
		}
	}
}
