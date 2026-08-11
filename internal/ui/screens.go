package ui

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	qdatetime "github.com/jungdosa/QuotaDock/internal/datetime"
	"github.com/jungdosa/QuotaDock/internal/diagnostics"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/security"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"image/color"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Screen int

const (
	NormalScreen Screen = iota
	CompactScreen
	NanoScreen
	SettingsScreen
)

func ScreenForDisplayMode(mode settings.DisplayMode) Screen {
	switch mode {
	case settings.ModeCompact:
		return CompactScreen
	case settings.ModeNano:
		return NanoScreen
	default:
		return NormalScreen
	}
}

const (
	NormalWidth float32 = 539
	// CompactWidth grew with the reset column: the previous 260 budget is
	// preserved for icon/label/meter/percent and the measured reset column plus
	// one extra column gap is added on top, so meters keep their old width.
	CompactWidth           float32 = 312
	NanoWidth              float32 = 360
	SettingsWidth          float32 = 620
	SettingsHeight         float32 = 680
	WindowCornerRadius     float32 = 8
	CompactIconWidth       float32 = 16
	CompactColumnGap       float32 = 2
	CompactPaddingLeft     float32 = 4
	CompactPaddingRight    float32 = 8
	CompactLabelPadding    float32 = 10
	CompactLabelMinWidth   float32 = 40
	CompactLabelMaxWidth   float32 = 140
	CompactPercentTextSize float32 = 12
	CompactHundredTextSize float32 = 11
	CompactPercentMargin   float32 = 8
	CompactPercentLead     float32 = 2
	CompactSymbolTextSize  float32 = 9
	CompactPercentOffset   float32 = -2
	CompactResetTextSize   float32 = 10
	CompactResetPadding    float32 = 6
	CompactMeterMinWidth   float32 = 46
	CompactMeterHeight     float32 = 7
	CompactMeterGap        float32 = 2
	CompactDividerInset    float32 = 7
	CompactDividerPaddingY float32 = 3
	CompactDividerAlpha    uint8   = 0x3D
	CompactRowHeight       float32 = 22
	UsageHeaderTextSize    float32 = 14
	UsageHeaderRowHeight   float32 = 19
	NanoBodyHeight float32 = 26
	// NanoLabelTextSize is nano-scoped only: compact/normal type tokens
	// are separate constants, so this bump never leaks into other modes. The
	// row line height stays pinned to the former 7.5px metric (nanoRowHeight),
	// so the nano window height does not change.
	NanoLabelTextSize          float32 = 8.5
	nanoLabelReferenceTextSize float32 = 7.5
	NanoUsageBarHeight     float32 = 6
	NanoResetBarHeight     float32 = 2
	NanoResetGap           float32 = 1
	NanoLineGap            float32 = 2
	NormalRowGap           float32 = 5
	NormalRowHeight        float32 = 38
	NormalMeterHeight      float32 = 10
	// The normal-mode reset bar is thicker than compact/nano's 2px: the
	// row has the room, and 2px vanishes on high-DPI and dark themes.
	NormalResetBarHeight float32 = 3
	NormalResetBarGap    float32 = 1
	TitleBarHeight         float32 = 38
	TitleTextSize          float32 = 14
	TitleVersionTextSize   float32 = 11
	LaneHeaderTextSize     float32 = 14
	PlanChipTextSize       float32 = 9
	CreditsTextSize        float32 = 9.5
	PlanChipPaddingX       float32 = 5
	PlanChipPaddingY       float32 = 1.5
	LaneHeaderChipGap      float32 = 6
	NormalResetLineGap     float32 = -2
	NormalLabelTextSize    float32 = 12
	NormalMetaTextSize     float32 = 11.5
	CompactLabelTextSize   float32 = 12
	SettingsTextSize       float32 = 11
	ThresholdTextSize      float32 = 9
	// ButtonLabelPadding keeps translated button labels from touching the border.
	ButtonLabelPadding float32 = 9
	// The percentage sits in a band directly above the meter, flush with its right end.
	NormalPercentBandHeight float32 = 15
	NormalPercentInset      float32 = 1
	TooltipTextSize         float32 = 10.5
	TooltipGap              float32 = 4
	TooltipMargin           float32 = 4
	TooltipDelay                    = 400 * time.Millisecond
)

// Normal-mode label column sizing mirrors compact mode: the column is only as
// wide as the widest label of the *current* language plus a fixed gap, so adding
// a locale never widens the layout for everyone.
const (
	NormalLabelPadding    float32 = 8
	NormalLabelMinWidth   float32 = 56
	NormalLabelMaxWidth   float32 = 150
	NormalPercentTextSize float32 = 13
)

var normalFixedColumns = []float32{0, 140}

func (v *View) normalLabelWidth(lanes []LaneState) float32 {
	maximum := float32(0)
	for _, lane := range lanes {
		for _, row := range lane.Rows {
			width := fyne.MeasureText(v.usageRowLabel(lane, row), NormalLabelTextSize, fyne.TextStyle{Bold: true}).Width
			maximum = max(maximum, width)
		}
		if len(lane.Rows) == 0 {
			maximum = max(maximum, fyne.MeasureText(v.laneStatusText(lane), NormalLabelTextSize, fyne.TextStyle{Bold: true}).Width)
		}
	}
	return min(NormalLabelMaxWidth, max(NormalLabelMinWidth, float32(math.Ceil(float64(maximum+NormalLabelPadding)))))
}

func normalRowColumnsFor(labelWidth float32) []float32 {
	columns := make([]float32, 0, len(normalFixedColumns)+1)
	columns = append(columns, labelWidth)
	return append(columns, normalFixedColumns...)
}

var normalRowColumns = normalRowColumnsFor(120)

type Actions struct {
	BeginWindowDrag func() (int, int, error)
	MoveWindow      func(int, int) error
	EndWindowDrag   func()
	ToggleCompact   func()
	SetDisplayMode  func(settings.DisplayMode)
	OpenContextMenu func(fyne.Position)
	Refresh         func()
	ResizeWindow    func(fyne.Size)
	OpenSettings    func()
	Minimize        func()
	Close           func()
	CloseSettings   func()
	ConfigChanged   func(settings.Config)
	Inspect         func(model.ProviderID)
	Reconnect       func(model.ProviderID)
	CheckUpdate     func()
	OpenURL         func(string) error
	Activity        func()
	AppVersion               string
	DemoMode                 bool
	TrayPromotionSupported   bool
}
type View struct {
	Canvas                  fyne.Canvas
	Catalog                 *i18n.Catalog
	SystemLanguage          i18n.Language
	Actions                 Actions
	config                  settings.Config
	colors                  BrandColors
	state                   ViewState
	Root                    *fyne.Container
	Normal                  *fyne.Container
	Compact                 *fyne.Container
	Nano                    *fyne.Container
	Settings                *fyne.Container
	normalBody, compactBody *fyne.Container
	// Header wraps hold the full-width caption strips: the strip sits
	// outside the padded body so its titlebar tone reaches both window edges.
	normalHeaderWrap  *fyne.Container
	compactHeaderWrap *fyne.Container
	nanoBody          *fyne.Container
	lastRefreshText   *canvas.Text
	helpPopup               *widget.PopUp
	palettePopup            *widget.PopUp
	tooltipLayer            *fyne.Container
	tooltipObject           fyne.CanvasObject
	tooltipTimer            *time.Timer
	tooltipOwner            tooltipAnchor
	tooltipMu               sync.Mutex
	refreshButtons          []*SmallButton
	refreshing              bool
	connectionsBody         *fyne.Container
	normalCache             *normalBodyView
	compactCache            *compactBodyView
	nanoCache               *nanoBodyView
	connectionCache         []*connectionView
	openConnectionPanel     connectionPanelSelection
	warningEntry            *widget.Entry
	dangerEntry             *widget.Entry
	warningSlider           *widget.Slider
	dangerSlider            *widget.Slider
	lastResizeRequest       fyne.Size
	screen                  Screen
}

func NewView(c fyne.Canvas, catalog *i18n.Catalog, systemLanguage i18n.Language, config settings.Config, actions Actions) *View {
	validated := config.Validated()
	v := &View{Canvas: c, Catalog: catalog, SystemLanguage: systemLanguage, Actions: actions, config: validated, colors: currentBrandColors(validated.Theme), state: defaultViewState()}
	v.build()
	v.Show(NormalScreen)
	return v
}
func (v *View) build() {
	v.Normal = v.buildNormal()
	v.Compact = v.buildCompact()
	v.Nano = v.buildNano()
	v.Settings = v.buildSettings()
	// The tooltip layer sits above every screen but is made of plain canvas
	// objects only, so pointer events fall straight through to the controls
	// underneath (the Fyne equivalent of pointer-events: none). A widget.PopUp
	// must never be used here: its full-canvas overlay consumes the first click.
	v.tooltipLayer = container.NewWithoutLayout()
	v.Root = container.NewStack(v.Normal, v.Compact, v.Nano, v.Settings, v.tooltipLayer)
}
func (v *View) Show(screen Screen) {
	v.dismissTooltip()
	v.dismissPalette()
	v.noteActivity()
	v.screen = screen
	switch screen {
	case NormalScreen:
		v.config.DisplayMode = settings.ModeNormal
	case CompactScreen:
		v.config.DisplayMode = settings.ModeCompact
	case NanoScreen:
		v.config.DisplayMode = settings.ModeNano
	}
	v.renderCurrentScreen()
	v.Normal.Hide()
	v.Compact.Hide()
	v.Nano.Hide()
	v.Settings.Hide()
	switch screen {
	case CompactScreen:
		v.Compact.Show()
	case NanoScreen:
		v.Nano.Show()
	case SettingsScreen:
		v.Settings.Show()
	default:
		v.Normal.Show()
	}
}
func (v *View) Screen() Screen { return v.screen }
func (v *View) SetState(state ViewState) {
	v.dismissPalette()
	v.noteActivity()
	v.state = cloneState(state)
	for i := range v.state.Lanes {
		lane := &v.state.Lanes[i]
		sortLaneRows(lane.Provider, lane.Rows)
		assignUniqueDisplayLabels(lane.Rows)
	}
	v.renderCurrentScreen()
	v.refreshLastRefreshText()
	v.resizeCurrentWidget()
}
func (v *View) SetConfig(config settings.Config) {
	v.noteActivity()
	oldLanguage := v.config.Language
	oldTheme := v.config.Theme
	oldWarningsEnabled := v.config.WarningsEnabled
	current := v.screen

	v.config = config.Validated()
	v.colors = currentBrandColors(v.config.Theme)
	v.updateUsageColumnHeaders()
	v.syncThresholdEntries()
	if v.Actions.ConfigChanged != nil {
		v.Actions.ConfigChanged(v.config)
	}
	if (oldLanguage != v.config.Language || oldTheme != v.config.Theme || oldWarningsEnabled != v.config.WarningsEnabled) && v.Root != nil {
		v.rebuildScreens(current)
		v.resizeCurrentWidget()
		return
	}
	v.renderCurrentScreen()
	// Timestamps follow the date/time setting, so a format change must
	// repaint the footer and refresh tooltips without waiting for new data.
	v.refreshLastRefreshText()
	v.resizeCurrentWidget()
}

// RefreshTheme rebuilds custom canvas objects after the system theme variant
// changes. Standard Fyne widgets refresh themselves through app settings.
func (v *View) RefreshTheme() {
	if v.config.Theme != settings.ThemeSystem || v.Root == nil {
		return
	}
	nextColors := currentBrandColors(v.config.Theme)
	if nextColors.Background == v.colors.Background {
		return
	}
	current := v.screen

	v.colors = nextColors
	v.rebuildScreens(current)
}

func (v *View) rebuildScreens(current Screen) {
	v.dismissTooltip()
	v.dismissPalette()
	root := v.Root
	v.normalCache = nil
	v.compactCache = nil
	v.nanoCache = nil
	v.connectionCache = nil
	v.refreshButtons = nil
	v.Normal = v.buildNormal()
	v.Compact = v.buildCompact()
	v.Nano = v.buildNano()
	v.Settings = v.buildSettings()
	root.Objects = []fyne.CanvasObject{v.Normal, v.Compact, v.Nano, v.Settings, v.tooltipLayer}
	v.Root = root
	v.Root.Refresh()
	v.Show(current)
}
func (v *View) resizeCurrentWidget() {
	if v.Actions.ResizeWindow == nil {
		return
	}
	size := v.MinimumSize(v.screen)
	if size == v.lastResizeRequest {
		return
	}
	v.lastResizeRequest = size
	v.Actions.ResizeWindow(size)
}
func (v *View) text(key string) string {
	lang := i18n.Language(v.config.Language)
	return v.Catalog.Text(lang, v.SystemLanguage, key)
}

func (v *View) resolvedLanguage() i18n.Language {
	language := i18n.Language(v.config.Language)
	if language == i18n.System {
		language = v.SystemLanguage
	}
	if !i18n.IsSupported(language) {
		return i18n.English
	}
	return language
}
func (v *View) windowTitle(mode settings.DisplayMode) *fyne.Container {
	title := textLabel(v.text(i18n.KeyAppTitle), TitleTextSize, v.colors.Text, true, false)
	titleObjects := []fyne.CanvasObject{title, v.titleVersionLabel(v.text(i18n.KeyAppTitle), TitleTextSize, v.Actions.AppVersion)}
	if v.Actions.DemoMode {
		demoText := textLabel("DEMO", 10, v.colors.Accent, true, false)
		demoBackground := canvas.NewRectangle(v.colors.PlanChip)
		demoBackground.CornerRadius = 6
		titleObjects = append(titleObjects, container.NewStack(demoBackground, container.New(layout.NewCustomPaddedLayout(2, 2, 5, 5), container.NewCenter(demoText))))
	}
	titleGroup := container.NewHBox(titleObjects...)
	buttons := []*SmallButton{
		NewSmallIconButton(displayModeResource(mode, v.colors), v.displayModeTooltip(mode), v.Actions.ToggleCompact, v.colors),
		v.newRefreshButton(),
		v.newThemeButton(),
		NewSmallIconButton(theme.SettingsIcon(), v.text(i18n.KeySettings), v.Actions.OpenSettings, v.colors),
		NewSmallIconButton(theme.WindowMinimizeIcon(), v.text(i18n.KeyMinimize), v.Actions.Minimize, v.colors),
		NewSmallIconButton(theme.CancelIcon(), v.text(i18n.KeyClose), v.Actions.Close, v.colors),
	}
	buttonObjects := make([]fyne.CanvasObject, 0, len(buttons))
	for _, button := range buttons {
		buttonObjects = append(buttonObjects, v.bindTitleButton(button))
	}
	dragTitle := container.NewStack(
		NewDragSurface(v.Actions.BeginWindowDrag, v.Actions.MoveWindow, v.Actions.EndWindowDrag),
		container.New(layout.NewCustomPaddedLayout(0, 0, 8, 0), titleGroup),
	)
	widths := []float32{0, 24, 24, 24, 24, 24, 24}
	row := container.New(NewColumnLayout(widths, 2, TitleBarHeight), append([]fyne.CanvasObject{dragTitle}, buttonObjects...)...)
	gradient := canvas.NewLinearGradient(v.colors.TitleTop, v.colors.TitleBottom, 0)
	divider := canvas.NewRectangle(v.colors.TitleDivider)
	divider.SetMinSize(fyne.NewSize(1, 1))
	dividerOverlay := container.NewBorder(nil, divider, nil, nil)
	background := container.NewStack(gradient, dividerOverlay)
	return container.NewStack(background, row)
}

// titleVersionLabel renders "vX.Y.Z" in small secondary type, bottom-aligned
// with the neighbouring title text: both labels are centred by their em box,
// so padding the smaller one down by the height difference lines their
// bottoms up instead of their centres.
func (v *View) titleVersionLabel(titleText string, titleSize float32, appVersion string) fyne.CanvasObject {
	versionLabel := textLabel("v"+appVersion, TitleVersionTextSize, v.colors.Secondary, false, false)
	titleHeight := fyne.MeasureText(titleText, titleSize, fyne.TextStyle{Bold: true}).Height
	versionHeight := fyne.MeasureText("v"+appVersion, TitleVersionTextSize, fyne.TextStyle{}).Height
	drop := max(float32(0), titleHeight-versionHeight)
	return container.New(layout.NewCustomPaddedLayout(drop, 0, 4, 0), versionLabel)
}

func (v *View) displayModeTooltip(mode settings.DisplayMode) string {
	mode = settings.NextDisplayMode(mode)
	key := i18n.KeyDisplayNormal
	switch mode {
	case settings.ModeCompact:
		key = i18n.KeyCompact
	case settings.ModeNano:
		key = i18n.KeyNano
	}
	return fmt.Sprintf(v.text(i18n.KeyTooltipDisplay), v.text(key))
}

func (v *View) newRefreshButton() *SmallButton {
	icon := theme.ViewRefreshIcon()
	if v.refreshing {
		icon = busyResource(v.colors)
	}
	button := NewSmallIconButton(icon, v.refreshTooltip(), v.Actions.Refresh, v.colors)
	button.Disabled = v.refreshing
	v.refreshButtons = append(v.refreshButtons, button)
	return button
}

func (v *View) newThemeButton() *SmallButton {
	return NewSmallIconButton(themeModeResource(v.config.Theme, v.colors), v.themeTooltip(), v.cycleTheme, v.colors)
}

func (v *View) themeTooltip() string {
	key := i18n.KeyTooltipThemeLight
	switch v.config.Theme {
	case settings.ThemeDark:
		key = i18n.KeyTooltipThemeDark
	case settings.ThemeSystem:
		key = i18n.KeyTooltipThemeSystem
	}
	return fmt.Sprintf(v.text(i18n.KeyTooltipTheme), v.text(key))
}

func (v *View) refreshTooltip() string {
	return fmt.Sprintf(v.text(i18n.KeyTooltipRefresh), v.formatTimestamp(v.state.LastRefresh))
}

// formatTimestamp renders every UI timestamp through the shared date/time
// setting, matching resetStrings so no surface bypasses the user's format.
func (v *View) formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	lang := v.resolvedLanguage()
	formatted, err := qdatetime.FormatUnix(value.Unix(), time.Local, lang, qdatetime.Format(v.config.DateTimeFormat))
	if err != nil {
		return value.Local().Format("15:04")
	}
	return formatted
}

// tooltipAnchor is any canvas object that can own the shared passive tooltip.
// The tooltip re-reads the anchor's state at show time, so hovering again after
// a click always displays the freshest text.
type tooltipAnchor interface {
	fyne.CanvasObject
	tooltipActive() bool
	tooltipValue() string
}

func (v *View) bindTitleButton(button *SmallButton) *SmallButton {
	if button == nil {
		return nil
	}
	button.OnHoverStart = v.scheduleTooltip
	button.OnHoverEnd = v.dismissTooltipFor
	return button
}

func (v *View) bindToggle(toggle *Toggle) *Toggle {
	if toggle == nil {
		return nil
	}
	toggle.OnHoverStart = func(anchor *Toggle) { v.scheduleAnchorTooltip(anchor) }
	toggle.OnHoverEnd = func(anchor *Toggle) { v.dismissAnchorTooltip(anchor) }
	return toggle
}

func (v *View) bindConnectionMethodButton(button *ConnectionMethodButton) *ConnectionMethodButton {
	if button == nil {
		return nil
	}
	button.OnHoverStart = func(anchor *ConnectionMethodButton) { v.scheduleAnchorTooltip(anchor) }
	button.OnHoverEnd = func(anchor *ConnectionMethodButton) { v.dismissAnchorTooltip(anchor) }
	return button
}

func (v *View) scheduleTooltip(button *SmallButton) { v.scheduleAnchorTooltip(button) }

func (v *View) scheduleAnchorTooltip(anchor tooltipAnchor) {
	v.dismissTooltip()
	if anchor == nil || anchor.tooltipValue() == "" || v.Canvas == nil {
		return
	}
	v.tooltipMu.Lock()
	v.tooltipOwner = anchor
	v.tooltipTimer = diagnostics.AfterFunc(TooltipDelay, "tooltip_delay", func() {
		fyne.Do(func() { v.showTooltip(anchor) })
	})
	v.tooltipMu.Unlock()
}

func (v *View) showTooltip(anchor tooltipAnchor) {
	v.tooltipMu.Lock()
	if anchor == nil || v.tooltipOwner != anchor || !anchor.tooltipActive() || anchor.tooltipValue() == "" || v.Canvas == nil || v.tooltipLayer == nil {
		v.tooltipMu.Unlock()
		return
	}
	timer := v.tooltipTimer
	v.tooltipTimer = nil
	v.tooltipMu.Unlock()
	if timer != nil {
		timer.Stop()
	}

	content := v.tooltipContent(anchor.tooltipValue())
	size := content.MinSize()
	origin := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	position := tooltipPosition(origin, anchor.Size(), size, v.Canvas.Size())

	v.tooltipMu.Lock()
	if v.tooltipOwner != anchor || !anchor.tooltipActive() {
		v.tooltipMu.Unlock()
		return
	}
	v.tooltipObject = content
	v.tooltipMu.Unlock()
	content.Resize(size)
	content.Move(position)
	v.tooltipLayer.Add(content)
	v.tooltipLayer.Refresh()
}

func (v *View) tooltipContent(value string) fyne.CanvasObject {
	background := canvas.NewRectangle(v.colors.Card)
	background.CornerRadius = 4
	background.StrokeColor = v.colors.CardBorder
	background.StrokeWidth = 1
	lines := strings.Split(value, "\n")
	labels := make([]fyne.CanvasObject, 0, len(lines))
	for _, line := range lines {
		labels = append(labels, textLabel(line, TooltipTextSize, v.colors.Text, false, false))
	}
	body := container.New(&CompactRowsLayout{Gap: 1}, labels...)
	return container.NewStack(background, container.New(layout.NewCustomPaddedLayout(3, 3, 6, 6), body))
}

func tooltipPosition(anchor fyne.Position, anchorSize, tooltipSize, canvasSize fyne.Size) fyne.Position {
	x := anchor.X + (anchorSize.Width-tooltipSize.Width)/2
	y := anchor.Y + anchorSize.Height + TooltipGap
	maxX := max(TooltipMargin, canvasSize.Width-tooltipSize.Width-TooltipMargin)
	x = min(maxX, max(TooltipMargin, x))
	if y+tooltipSize.Height > canvasSize.Height-TooltipMargin {
		y = anchor.Y - tooltipSize.Height - TooltipGap
	}
	y = max(TooltipMargin, y)
	return fyne.NewPos(x, y)
}

func (v *View) dismissTooltipFor(button *SmallButton) { v.dismissAnchorTooltip(button) }

func (v *View) dismissAnchorTooltip(anchor tooltipAnchor) {
	v.tooltipMu.Lock()
	matches := v.tooltipOwner == anchor
	v.tooltipMu.Unlock()
	if matches {
		v.dismissTooltip()
	}
}

func (v *View) dismissTooltip() {
	v.tooltipMu.Lock()
	timer := v.tooltipTimer
	object := v.tooltipObject
	v.tooltipTimer = nil
	v.tooltipObject = nil
	v.tooltipOwner = nil
	v.tooltipMu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if object != nil && v.tooltipLayer != nil {
		v.tooltipLayer.Remove(object)
		v.tooltipLayer.Refresh()
	}
}

func (v *View) cycleTheme() {
	cfg := v.config
	switch cfg.Theme {
	case settings.ThemeLight:
		cfg.Theme = settings.ThemeDark
	case settings.ThemeDark:
		cfg.Theme = settings.ThemeSystem
	default:
		cfg.Theme = settings.ThemeLight
	}
	v.SetConfig(cfg)
}

func (v *View) SetRefreshing(refreshing bool) {
	if v.refreshing == refreshing {
		return
	}
	v.refreshing = refreshing
	v.noteActivity()
	for _, button := range v.refreshButtons {
		if refreshing {
			button.SetIcon(busyResource(v.colors))
		} else {
			button.SetIcon(theme.ViewRefreshIcon())
		}
		button.SetDisabled(refreshing)
	}
}

func (v *View) noteActivity() {
	if v.Actions.Activity != nil {
		v.Actions.Activity()
	}
}

type DragSurface struct {
	widget.BaseWidget
	OnDragStart func() (int, int, error)
	OnDrag      func(int, int) error
	OnDragEnd   func()
	dragging    bool
	grabOffsetX int
	grabOffsetY int
}

var _ fyne.Draggable = (*DragSurface)(nil)

func NewDragSurface(onDragStart func() (int, int, error), onDrag func(int, int) error, onDragEnd func()) *DragSurface {
	d := &DragSurface{OnDragStart: onDragStart, OnDrag: onDrag, OnDragEnd: onDragEnd}
	d.ExtendBaseWidget(d)
	return d
}
func (d *DragSurface) Dragged(_ *fyne.DragEvent) {
	if !d.dragging {
		if d.OnDragStart == nil {
			return
		}
		offsetX, offsetY, err := d.OnDragStart()
		if err != nil {
			return
		}
		d.grabOffsetX = offsetX
		d.grabOffsetY = offsetY
		d.dragging = true
	}
	if d.OnDrag != nil {
		_ = d.OnDrag(d.grabOffsetX, d.grabOffsetY)
	}
}
func (d *DragSurface) DragEnd() {
	if !d.dragging {
		return
	}
	d.dragging = false
	d.grabOffsetX = 0
	d.grabOffsetY = 0
	if d.OnDragEnd != nil {
		d.OnDragEnd()
	}
}
func (d *DragSurface) CreateRenderer() fyne.WidgetRenderer {
	return &singleRenderer{object: canvas.NewRectangle(color.Transparent), min: fyne.NewSize(1, TitleBarHeight)}
}

type NanoSurface struct {
	widget.BaseWidget
	drag        *DragSurface
	onTapped    func()
	onSecondary func(fyne.Position)
}

var _ fyne.Tappable = (*NanoSurface)(nil)
var _ fyne.SecondaryTappable = (*NanoSurface)(nil)
var _ fyne.Draggable = (*NanoSurface)(nil)

func NewNanoSurface(onDragStart func() (int, int, error), onDrag func(int, int) error, onDragEnd, onTapped func(), onSecondary func(fyne.Position)) *NanoSurface {
	s := &NanoSurface{drag: NewDragSurface(onDragStart, onDrag, onDragEnd), onTapped: onTapped, onSecondary: onSecondary}
	s.ExtendBaseWidget(s)
	return s
}

func (s *NanoSurface) Tapped(*fyne.PointEvent) {
	if s.onTapped != nil {
		s.onTapped()
	}
}

func (s *NanoSurface) TappedSecondary(event *fyne.PointEvent) {
	if s.onSecondary != nil && event != nil {
		s.onSecondary(event.Position)
	}
}

func (s *NanoSurface) Dragged(event *fyne.DragEvent) { s.drag.Dragged(event) }
func (s *NanoSurface) DragEnd()                      { s.drag.DragEnd() }
func (s *NanoSurface) CreateRenderer() fyne.WidgetRenderer {
	return &singleRenderer{object: canvas.NewRectangle(color.Transparent), min: fyne.NewSize(1, NanoBodyHeight)}
}

func (v *View) roundedScreen(background color.Color, content fyne.CanvasObject) *fyne.Container {
	page := canvas.NewRectangle(background)
	page.CornerRadius = WindowCornerRadius
	return container.NewStack(page, content)
}

func (v *View) buildNormal() *fyne.Container {
	v.normalBody = container.NewVBox()
	v.normalHeaderWrap = container.NewStack()
	v.renderNormalBody()
	padded := container.New(layout.NewCustomPaddedLayout(4, 8, 12, 12), v.normalBody)
	v.lastRefreshText = textLabel(v.lastRefreshLabel(), 9.5, v.colors.Text, false, true)
	v.lastRefreshText.Alignment = fyne.TextAlignTrailing
	footer := container.New(layout.NewCustomPaddedLayout(0, 6, 12, 12), v.lastRefreshText)
	body := container.NewBorder(v.normalHeaderWrap, footer, nil, nil, padded)
	return v.roundedScreen(v.colors.Background, container.NewBorder(v.windowTitle(settings.ModeNormal), nil, nil, nil, body))
}
func (v *View) buildCompact() *fyne.Container {
	v.compactBody = container.New(&CompactRowsLayout{Gap: 1})
	v.compactHeaderWrap = container.NewStack()
	v.renderCompactBody()
	rows := container.New(layout.NewCustomPaddedLayout(2, 2, CompactPaddingLeft, CompactPaddingRight), v.compactBody)
	content := container.NewBorder(v.compactHeaderWrap, nil, nil, nil, rows)
	return v.roundedScreen(v.colors.Background, container.NewBorder(v.windowTitle(settings.ModeCompact), nil, nil, nil, content))
}
func (v *View) buildNano() *fyne.Container {
	v.nanoBody = container.New(layout.NewGridLayoutWithColumns(1))
	v.renderNanoBody()
	visual := container.New(layout.NewCustomPaddedLayout(3, 3, 4, 4), v.nanoBody)
	gestures := NewNanoSurface(v.Actions.BeginWindowDrag, v.Actions.MoveWindow, v.Actions.EndWindowDrag, func() {
		if v.Actions.SetDisplayMode != nil {
			v.Actions.SetDisplayMode(settings.ModeCompact)
		}
	}, v.Actions.OpenContextMenu)
	body := container.NewStack(visual, gestures)
	return v.roundedScreen(v.colors.Background, container.NewBorder(v.windowTitle(settings.ModeNano), nil, nil, nil, body))
}
func (v *View) renderNormalBody()  { v.syncNormalBody() }
func (v *View) renderCompactBody() { v.syncCompactBody() }
func (v *View) renderNanoBody()    { v.syncNanoBody() }
func (v *View) lastRefreshLabel() string {
	return fmt.Sprintf(v.text(i18n.KeyLastRefresh), v.formatTimestamp(v.state.LastRefresh))
}
func (v *View) refreshLastRefreshText() {
	if v.lastRefreshText != nil {
		setCanvasText(v.lastRefreshText, v.lastRefreshLabel(), v.colors.Text)
	}
	tooltip := v.refreshTooltip()
	dismiss := false
	for _, button := range v.refreshButtons {
		button.Tooltip = tooltip
		v.tooltipMu.Lock()
		dismiss = dismiss || v.tooltipOwner == button
		v.tooltipMu.Unlock()
	}
	if dismiss {
		v.dismissTooltip()
	}
}
func (v *View) visibleLanes() []LaneState {
	out := []LaneState{}
	for _, lane := range v.state.Lanes {
		show := lane.Provider == model.ProviderClaude && v.config.ShowClaude || lane.Provider == model.ProviderCodex && v.config.ShowCodex || lane.Provider == model.ProviderAntigravity && (v.config.ShowAGGemini || v.config.ShowAGClaude) || lane.Provider == model.ProviderGrok && v.config.ShowGrok
		if show {
			if lane.Provider == model.ProviderAntigravity {
				filtered := lane
				filtered.Rows = nil
				for _, row := range lane.Rows {
					gemini := antigravityRowIsGemini(row)
					if gemini && v.config.ShowAGGemini || !gemini && v.config.ShowAGClaude {
						filtered.Rows = append(filtered.Rows, row)
					}
				}
				lane = filtered
			}
			out = append(out, lane)
		}
	}
	return out
}
func antigravityRowIsGemini(row UsageRowState) bool {
	return strings.Contains(strings.ToLower(row.Label), "gemini")
}
func (v *View) laneHeader(lane LaneState) fyne.CanvasObject {
	object, _ := v.makeLaneHeader(lane)
	return object
}
func (v *View) laneStatusText(l LaneState) string {
	if l.ErrorKey != "" {
		return v.text(l.ErrorKey)
	}
	return "—"
}
func (v *View) normalStatusRow(lane LaneState) fyne.CanvasObject {
	object, _ := v.makeNormalStatusRow(lane)
	return object
}
func (v *View) compactStatusRow(lane LaneState) fyne.CanvasObject {
	object, _ := v.makeCompactStatusRow(lane)
	return object
}
func (v *View) displayPercent(row UsageRowState) float64 {
	if v.config.UsageMode == settings.UsageRemaining {
		return 100 - row.Percent
	}
	return row.Percent
}

// resetRemainingPercent drives every reset-time bar: it always shows the
// remaining share of the window, draining toward the reset moment, regardless
// of the Used/Remaining display method. One consistent reading in all modes:
// a full bar means the window just reset, an empty bar means reset is due.
func (v *View) resetRemainingPercent(row UsageRowState, now time.Time) float64 {
	return 100 - resetProgress(row, now)
}

// resetBarColor keeps every reset-time bar achromatic and quiet: the bar
// never borrows provider or severity hues, so it stays readable next to a
// warning-colored meter in all three display modes.
func (v *View) resetBarColor() color.Color {
	return buttonAlpha(v.colors.Text, 0x48)
}
func (v *View) severity(percent float64) (model.AlertLevel, color.Color) {
	level := model.AlertNormal
	if v.config.WarningsEnabled {
		level = model.ClassifyUsage(percent, v.config.WarningPercent, v.config.DangerPercent)
	}
	var clr color.Color = v.colors.PercentNormal
	switch level {
	case model.AlertWarning:
		clr = v.colors.PaletteColor(v.config.WarningColor)
	case model.AlertDanger:
		clr = v.colors.PaletteColor(v.config.DangerColor)
	}
	return level, clr
}
func providerColorID(id model.ProviderID, row UsageRowState, c settings.Config) string {
	return c.ProviderColors[providerColorKey(id, row)]
}
func providerColorKey(id model.ProviderID, row UsageRowState) string {
	if id == model.ProviderClaude {
		return "claude"
	}
	if id == model.ProviderCodex {
		return "codex"
	}
	if id == model.ProviderGrok {
		return "grok"
	}
	if antigravityRowIsGemini(row) {
		return "antigravity-gemini"
	}
	return "antigravity"
}
func (v *View) showProviderPalette(meter *SegmentedMeter, key string) {
	if v.Canvas == nil || meter == nil {
		return
	}
	current := v.config.ProviderColors[key]
	defaultID := settings.Default().ProviderColors[key]
	selectColor := func(id string) {
		cfg := v.config
		cfg.ProviderColors[key] = id
		v.SetConfig(cfg)
	}
	v.showColorPalette(meter, current, defaultID, true, selectColor)
}

// showColorPalette is the single palette surface for provider meters and
// warning/danger controls. Callers choose whether the provider-default reset
// swatch is present; the 16 allowlisted colors and popup geometry stay shared.
func (v *View) showColorPalette(anchor fyne.CanvasObject, current, defaultID string, includeReset bool, selectColor func(string)) {
	if v.Canvas == nil || anchor == nil {
		return
	}
	v.dismissPalette()
	v.noteActivity()
	choose := func(id string) {
		v.dismissPalette()
		if selectColor != nil {
			selectColor(id)
		}
	}
	objects := make([]fyne.CanvasObject, 0, 20)
	if includeReset {
		objects = append(objects, NewPaletteSwatch(v.colors.PaletteColor(defaultID), current == defaultID, true, func() { choose(defaultID) }, v.colors))
	}
	for _, paletteID := range security.PaletteIDs() {
		id := paletteID
		selected := current == id && (!includeReset || current != defaultID)
		objects = append(objects, NewPaletteSwatch(v.colors.PaletteColor(id), selected, false, func() { choose(id) }, v.colors))
	}
	for len(objects)%5 != 0 {
		objects = append(objects, layout.NewSpacer())
	}
	grid := container.NewGridWithColumns(5, objects...)
	background := canvas.NewRectangle(v.colors.Card)
	background.CornerRadius = 8
	background.StrokeColor = v.colors.CardBorder
	background.StrokeWidth = 1
	content := container.NewStack(background, container.New(layout.NewCustomPaddedLayout(7, 7, 7, 7), grid))
	popup := widget.NewPopUp(content, v.Canvas)
	size := content.MinSize()
	popup.Resize(size)
	position := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	x := position.X
	y := position.Y + anchor.Size().Height + 3
	canvasSize := v.Canvas.Size()
	if x+size.Width > canvasSize.Width {
		x = canvasSize.Width - size.Width - 4
	}
	if x < 4 {
		x = 4
	}
	if y+size.Height > canvasSize.Height {
		y = position.Y - size.Height - 3
	}
	if y < 4 {
		y = 4
	}
	v.palettePopup = popup
	popup.ShowAtPosition(fyne.NewPos(x, y))
}
func (v *View) dismissPalette() {
	if v.palettePopup != nil {
		v.palettePopup.Hide()
		v.palettePopup = nil
	}
}
func (v *View) normalUsageRow(lane LaneState, row UsageRowState) fyne.CanvasObject {
	object, _ := v.makeNormalUsageRow(lane, row, time.Now())
	return object
}
func (v *View) compactUsageRow(lane LaneState, row UsageRowState, showIcon bool) fyne.CanvasObject {
	object, _ := v.makeCompactUsageRow(lane, row, showIcon, v.compactLabelWidth(v.visibleLanes()), time.Now())
	return object
}
func providerIconKind(lane LaneState, row UsageRowState) ProviderIconKind {
	switch lane.Provider {
	case model.ProviderClaude:
		return ProviderIconClaude
	case model.ProviderCodex:
		return ProviderIconCodex
	case model.ProviderAntigravity:
		if antigravityRowIsGemini(row) {
			return ProviderIconGemini
		}
		return ProviderIconAGClaude
	case model.ProviderGrok:
		return ProviderIconGrok
	}
	return ProviderIconClaude
}
func (v *View) usageRowLabel(lane LaneState, row UsageRowState) string {
	language := i18n.Language(v.config.Language)
	if language == i18n.System {
		language = v.SystemLanguage
	}
	if language == i18n.Korean {
		return koreanUsageLabel(lane, row)
	}
	return englishUsageLabel(lane, row)
}

func (v *View) compactLabelWidth(lanes []LaneState) float32 {
	maximum := float32(0)
	for _, lane := range lanes {
		for _, row := range lane.Rows {
			width := fyne.MeasureText(v.usageRowLabel(lane, row), CompactLabelTextSize, fyne.TextStyle{Bold: true}).Width
			maximum = max(maximum, width)
		}
	}
	return min(CompactLabelMaxWidth, max(CompactLabelMinWidth, float32(math.Ceil(float64(maximum+CompactLabelPadding)))))
}

type compactWidthBudget struct {
	Icon          float32
	Label         float32
	Meter         float32
	MeterMinimum  float32
	Percent       float32
	Reset         float32
	Gaps          float32
	Padding       float32
	RequiredTotal float32
	Total         float32
}

func compactPercentTextWidth() float32 {
	maximum := float32(0)
	for value := 0; value <= 100; value++ {
		number := strconv.Itoa(value)
		numberSize := CompactPercentTextSize
		if value == 100 {
			numberSize = CompactHundredTextSize
		}
		width := fyne.MeasureText(number, numberSize, fyne.TextStyle{Bold: true}).Width +
			fyne.MeasureText("%", CompactSymbolTextSize, fyne.TextStyle{Bold: true}).Width
		maximum = max(maximum, width)
	}
	return maximum
}

func compactPercentColumnWidth() float32 {
	return float32(math.Ceil(float64(compactPercentTextWidth() + CompactPercentMargin)))
}

// compactResetColumnWidth sizes the reset column for the widest countdown
// resetStrings can produce, so every row's reset time right-aligns on one edge.
func compactResetColumnWidth() float32 {
	maximum := float32(0)
	for _, pattern := range []string{"23h 59m", "6d 23h"} {
		maximum = max(maximum, fyne.MeasureText(pattern, CompactResetTextSize, fyne.TextStyle{Monospace: true}).Width)
	}
	return float32(math.Ceil(float64(maximum + CompactResetPadding)))
}

func compactLayoutWidthBudget(labelWidth float32) compactWidthBudget {
	budget := compactWidthBudget{
		Icon:         CompactIconWidth,
		Label:        labelWidth,
		MeterMinimum: CompactMeterMinWidth,
		Percent:      compactPercentColumnWidth(),
		Reset:        compactResetColumnWidth(),
		Gaps:         4 * CompactColumnGap,
		Padding:      CompactPaddingLeft + CompactPaddingRight,
	}
	budget.RequiredTotal = budget.Icon + budget.Label + budget.MeterMinimum + budget.Percent + budget.Reset + budget.Gaps + budget.Padding
	budget.Meter = max(float32(1), CompactWidth-budget.Icon-budget.Label-budget.Percent-budget.Reset-budget.Gaps-budget.Padding)
	budget.Total = budget.Icon + budget.Label + budget.Meter + budget.Percent + budget.Reset + budget.Gaps + budget.Padding
	return budget
}

func compactRowColumns(labelWidth float32) []float32 {
	budget := compactLayoutWidthBudget(labelWidth)
	return []float32{budget.Icon, budget.Label, 0, budget.Percent, budget.Reset}
}
func koreanUsageLabel(lane LaneState, row UsageRowState) string {
	if lane.Provider == model.ProviderClaude && strings.Contains(strings.ToLower(row.Label), "fable") {
		return "Fable 주간"
	}
	if lane.Provider == model.ProviderAntigravity {
		group := "Claude"
		if antigravityRowIsGemini(row) {
			group = "Gemini"
		}
		if period := koreanUsagePeriodLabel(row.WindowMinutes); period != "" {
			return group + " " + period
		}
		return group
	}
	if row.DisplayLabel != "" {
		localized := koreanDisplayLabel(row.DisplayLabel)
		if localized != row.DisplayLabel || strings.Contains(row.DisplayLabel, " · ") || strings.Contains(row.DisplayLabel, " (") {
			return localized
		}
	}
	if period := koreanUsagePeriodLabel(row.WindowMinutes); period != "" {
		return period
	}
	if row.DisplayLabel != "" {
		return row.DisplayLabel
	}
	return usageLabel(row, false)
}
func koreanDisplayLabel(label string) string {
	for _, base := range []string{"Weekly", "주간"} {
		if strings.HasPrefix(label, base+" · ") || strings.HasPrefix(label, base+" (") || label == base {
			return "주간" + strings.TrimPrefix(label, base)
		}
	}
	for _, base := range []string{"Session", "세션"} {
		if strings.HasPrefix(label, base+" · ") || strings.HasPrefix(label, base+" (") || label == base {
			return "세션" + strings.TrimPrefix(label, base)
		}
	}
	return label
}
func koreanUsagePeriodLabel(minutes int) string {
	switch {
	case minutes == 300:
		return "세션"
	case minutes >= 10080:
		return "주간"
	default:
		return ""
	}
}
func englishUsageLabel(lane LaneState, row UsageRowState) string {
	if lane.Provider == model.ProviderClaude && strings.Contains(strings.ToLower(row.Label), "fable") {
		return "Fable Weekly"
	}
	if lane.Provider == model.ProviderAntigravity {
		group := "Claude"
		if antigravityRowIsGemini(row) {
			group = "Gemini"
		}
		if period := usagePeriodLabel(row.WindowMinutes); period != "" {
			return group + " " + period
		}
		return group
	}
	if strings.Contains(row.DisplayLabel, " · ") || strings.Contains(row.DisplayLabel, "(") {
		return row.DisplayLabel
	}
	if period := usagePeriodLabel(row.WindowMinutes); period != "" {
		return period
	}
	if row.DisplayLabel != "" {
		return row.DisplayLabel
	}
	return usageLabel(row, false)
}
func usagePeriodLabel(minutes int) string {
	switch {
	case minutes == 300:
		return "Session"
	case minutes >= 10080:
		return "Weekly"
	default:
		return ""
	}
}
func usageLabel(row UsageRowState, compact bool) string {
	if !compact && row.DisplayLabel != "" {
		return row.DisplayLabel
	}
	value := row.Label
	if compact {
		switch value {
		case "Claude and GPT models", "Claude/GPT Models":
			return "AG Claude"
		case "Gemini Models":
			return "AG Gemini"
		}
	} else {
		suffix := ""
		if row.WindowMinutes == 300 {
			suffix = " 5H"
		} else if row.WindowMinutes >= 10080 {
			suffix = " 7D"
		}
		switch value {
		case "Claude and GPT models", "Claude/GPT Models":
			return "Claude/GPT" + suffix
		case "Gemini Models":
			return "Gemini" + suffix
		}
	}
	if len([]rune(value)) > 14 {
		return string([]rune(value)[:14])
	}
	return value
}

func wrapMonospace(value string, maxWidth, textSize float32) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{"—"}
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if current != "" && fyne.MeasureText(candidate, textSize, fyne.TextStyle{Monospace: true}).Width > maxWidth {
			lines = append(lines, current)
			current = word
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func resetProgress(row UsageRowState, now time.Time) float64 {
	if row.DisplayOverride {
		return 100 - clampPercent(row.DisplayRemainingPercent)
	}
	if row.ResetsAt.IsZero() || row.WindowMinutes <= 0 {
		return 0
	}
	remaining := row.ResetsAt.Sub(now).Minutes()
	return 100 - clampPercent(remaining/float64(row.WindowMinutes)*100)
}
func resetStrings(row UsageRowState, now time.Time, c settings.Config, systemLanguages ...i18n.Language) (string, string) {
	if row.DisplayOverride {
		return row.DisplayRemaining, row.DisplayReset
	}
	if row.ResetsAt.IsZero() {
		return "—", "—"
	}
	d := row.ResetsAt.Sub(now)
	if d < 0 {
		d = 0
	}
	totalHours := int(d.Hours())
	var until string
	if totalHours >= 24 {
		until = fmt.Sprintf("%dd %dh", totalHours/24, totalHours%24)
	} else {
		until = fmt.Sprintf("%dh %02dm", totalHours, int(d.Minutes())%60)
	}
	systemLanguage := i18n.English
	if len(systemLanguages) > 0 {
		systemLanguage = systemLanguages[0]
	}
	language := i18n.Language(c.Language)
	if language == i18n.System {
		language = systemLanguage
	}
	if !i18n.IsSupported(language) {
		language = i18n.English
	}
	at, _ := qdatetime.FormatUnix(row.ResetsAt.Unix(), time.Local, language, qdatetime.Format(c.DateTimeFormat))
	return until, at
}

func (v *View) buildSettings() *fyne.Container {
	content := container.NewVBox(
		v.section(v.text(i18n.KeyGroupUsage), v.usageSettings()),
		v.section(v.text(i18n.KeyGroupBehavior), v.behaviorSettings()),
		v.section(v.text(i18n.KeyGroupDisplay), v.displaySettings()),
		v.section(v.text(i18n.KeyGroupConnections), v.connectionSettings()),
	)
	closeSettings := func() {
		if v.Actions.CloseSettings != nil {
			v.Actions.CloseSettings()
			return
		}
		v.Show(ScreenForDisplayMode(v.config.DisplayMode))
	}
	back := NewSmallIconButton(theme.NavigateBackIcon(), v.text(i18n.KeyTooltipBack), closeSettings, v.colors)
	bar := v.settingsTitleBar(back, v.Actions.AppVersion, closeSettings)
	page := canvas.NewRectangle(v.colors.SettingsBackground)
	page.CornerRadius = WindowCornerRadius
	body := container.NewBorder(bar, nil, nil, nil, container.New(layout.NewCustomPaddedLayout(4, 8, 12, 12), content))
	return container.NewStack(page, body)
}

func (v *View) settingsTitleBar(back *SmallButton, appVersion string, closeSettings func()) *fyne.Container {
	// Identical product mark to the widget titlebars — same size and the same
	// all-caps text (mixed case reads smaller at an equal point size) — then
	// the version on the shared baseline and the screen name.
	product := container.NewHBox(
		textLabel(v.text(i18n.KeyAppTitle), TitleTextSize, v.colors.Text, true, false),
		v.titleVersionLabel(v.text(i18n.KeyAppTitle), TitleTextSize, appVersion),
		container.New(layout.NewCustomPaddedLayout(0, 0, 6, 0), textLabel(v.text(i18n.KeySettingsTitle), TitleTextSize, v.colors.Text, true, false)),
	)
	dragProduct := container.NewStack(
		NewDragSurface(v.Actions.BeginWindowDrag, v.Actions.MoveWindow, v.Actions.EndWindowDrag),
		product,
	)
	back = v.bindTitleButton(back)
	help := v.bindTitleButton(NewSmallIconButton(theme.HelpIcon(), v.text(i18n.KeyHelp), v.showHelp, v.colors))
	themeButton := v.bindTitleButton(v.newThemeButton())
	update := v.bindTitleButton(NewOutlinedSmallButton(v.text(i18n.KeyUpdate), v.text(i18n.KeyUpdate), func() {
		v.noteActivity()
		if v.Actions.CheckUpdate != nil {
			v.Actions.CheckUpdate()
		}
	}, v.colors))
	done := v.bindTitleButton(NewPrimarySmallButton(v.text(i18n.KeyDone), v.text(i18n.KeyDone), closeSettings, v.colors))
	separator := canvas.NewRectangle(buttonAlpha(v.colors.TitleDivider, 0xA0))
	separator.SetMinSize(fyne.NewSize(1, 16))
	separatorCell := container.NewCenter(separator)
	widths := []float32{24, 0, 24, 24, 1, buttonWidthFor(update, 68), buttonWidthFor(done, 48)}
	gaps := []float32{6, 6, 6, 8, 8, 6}
	title := container.New(NewGapColumnLayout(widths, gaps, TitleBarHeight), back, dragProduct, themeButton, help, separatorCell, update, done)
	divider := canvas.NewRectangle(v.colors.TitleDivider)
	divider.SetMinSize(fyne.NewSize(1, 1))
	content := container.NewBorder(nil, divider, nil, nil, title)
	gradient := canvas.NewLinearGradient(v.colors.TitleTop, v.colors.TitleBottom, 0)
	return container.NewStack(gradient, content)
}
func helpRichText(text string, colorName fyne.ThemeColorName, bold bool, width, height float32) fyne.CanvasObject {
	content := widget.NewRichText(&widget.TextSegment{
		Style: widget.RichTextStyle{
			ColorName: colorName,
			Inline:    true,
			TextStyle: fyne.TextStyle{Bold: bold},
		},
		Text: text,
	})
	content.Wrapping = fyne.TextWrapWord
	return container.NewGridWrap(fyne.NewSize(width, height), content)
}

func (v *View) helpProviderCard(title, description, retry string, accent color.Color) fyne.CanvasObject {
	background := canvas.NewRectangle(v.colors.Card)
	background.CornerRadius = 7
	background.StrokeColor = v.colors.CardBorder
	background.StrokeWidth = 1

	providerBorder := canvas.NewRectangle(accent)
	providerBorder.CornerRadius = 2
	providerBorder.SetMinSize(fyne.NewSize(4, 1))
	text := container.NewVBox(
		textLabel(title, 13, v.colors.Text, true, false),
		helpRichText(description, theme.ColorNameForeground, false, 498, 48),
		helpRichText(retry, theme.ColorNameDisabled, false, 498, 34),
	)
	content := container.New(layout.NewCustomPaddedLayout(8, 8, 10, 10), text)
	bordered := container.NewBorder(nil, nil, providerBorder, nil, content)
	return container.NewStack(background, bordered)
}

func (v *View) showHelp() {
	if v.Canvas == nil {
		return
	}
	v.noteActivity()
	if v.helpPopup != nil {
		v.helpPopup.Hide()
	}
	closeHelp := widget.NewButton(v.text(i18n.KeyDone), func() {
		if v.helpPopup != nil {
			v.helpPopup.Hide()
			v.helpPopup = nil
		}
	})
	privacyBackground := canvas.NewRectangle(v.colors.TitleBottom)
	privacyBackground.CornerRadius = 6
	privacyBackground.StrokeColor = v.colors.CardBorder
	privacyBackground.StrokeWidth = 1
	privacy := container.NewStack(
		privacyBackground,
		container.New(layout.NewCustomPaddedLayout(6, 6, 10, 10),
			helpRichText(v.text(i18n.KeyHelpCredentials), theme.ColorNameDisabled, false, 506, 32),
		),
	)
	body := container.NewVBox(
		textLabel(v.text(i18n.KeyHelpTitle), 15, v.colors.Text, true, false),
		helpRichText(v.text(i18n.KeyHelpIntro), theme.ColorNameForeground, false, 526, 42),
		v.helpProviderCard(
			v.text(i18n.KeyHelpClaudeTitle),
			v.text(i18n.KeyHelpClaude),
			v.text(i18n.KeyHelpClaudeRetry),
			v.colors.PaletteColor(v.config.ProviderColors["claude"]),
		),
		v.helpProviderCard(
			v.text(i18n.KeyHelpCodexTitle),
			v.text(i18n.KeyHelpCodex),
			v.text(i18n.KeyHelpCodexRetry),
			v.colors.PaletteColor(v.config.ProviderColors["codex"]),
		),
		v.helpProviderCard(
			v.text(i18n.KeyHelpAntigravityTitle),
			v.text(i18n.KeyHelpAntigravity),
			v.text(i18n.KeyHelpAntigravityRetry),
			v.colors.PaletteColor(v.config.ProviderColors["antigravity"]),
		),
		privacy,
		container.NewHBox(layout.NewSpacer(), closeHelp),
	)
	background := canvas.NewRectangle(v.colors.SettingsBackground)
	background.CornerRadius = WindowCornerRadius
	background.StrokeColor = v.colors.CardBorder
	background.StrokeWidth = 1
	card := container.NewStack(background, container.New(layout.NewCustomPaddedLayout(16, 16, 16, 16), body))
	v.helpPopup = widget.NewModalPopUp(card, v.Canvas)
	v.helpPopup.Resize(fyne.NewSize(580, 620))
	v.helpPopup.Show()
}

func (v *View) showConnectionHelp(id model.ProviderID) {
	if v.Canvas == nil {
		return
	}
	v.noteActivity()
	if v.helpPopup != nil {
		v.helpPopup.Hide()
	}
	titleKey, descriptionKey, retryKey := connectionHelpKeys(id)
	closeHelp := widget.NewButton(v.text(i18n.KeyDone), func() {
		if v.helpPopup != nil {
			v.helpPopup.Hide()
			v.helpPopup = nil
		}
	})
	body := container.NewVBox(
		v.helpProviderCard(
			v.text(titleKey),
			v.text(descriptionKey),
			v.text(retryKey),
			v.colors.PaletteColor(v.config.ProviderColors[connectionColorKey(id)]),
		),
		container.NewHBox(layout.NewSpacer(), closeHelp),
	)
	background := canvas.NewRectangle(v.colors.SettingsBackground)
	background.CornerRadius = WindowCornerRadius
	background.StrokeColor = v.colors.CardBorder
	background.StrokeWidth = 1
	card := container.NewStack(background, container.New(layout.NewCustomPaddedLayout(14, 14, 14, 14), body))
	v.helpPopup = widget.NewModalPopUp(card, v.Canvas)
	v.helpPopup.Resize(fyne.NewSize(550, 190))
	v.helpPopup.Show()
}

func connectionHelpKeys(id model.ProviderID) (string, string, string) {
	switch id {
	case model.ProviderCodex:
		return i18n.KeyHelpCodexTitle, i18n.KeyHelpCodex, i18n.KeyHelpCodexRetry
	case model.ProviderAntigravity:
		return i18n.KeyHelpAntigravityTitle, i18n.KeyHelpAntigravity, i18n.KeyHelpAntigravityRetry
	default:
		return i18n.KeyHelpClaudeTitle, i18n.KeyHelpClaude, i18n.KeyHelpClaudeRetry
	}
}
func (v *View) section(title string, body fyne.CanvasObject) fyne.CanvasObject {
	card := canvas.NewRectangle(v.colors.Card)
	card.CornerRadius = 10
	card.StrokeColor = v.colors.CardBorder
	card.StrokeWidth = 1
	heading := textLabel(trackedUpper(title), 11, v.colors.Accent, true, false)
	return container.NewStack(card, container.New(layout.NewCustomPaddedLayout(9, 10, 10, 10), container.NewVBox(heading, body)))
}
func (v *View) settingRow(label string, control fyne.CanvasObject, width float32) fyne.CanvasObject {
	return v.settingRowSized(label, control, 122, 10, width)
}
func (v *View) settingRowSized(label string, control fyne.CanvasObject, labelWidth, gap, controlWidth float32) fyne.CanvasObject {
	return container.New(&SettingRowLayout{LabelWidth: labelWidth, Gap: gap, ControlWidth: controlWidth, Height: 28}, textLabel(label, SettingsTextSize, v.colors.Label, true, false), control)
}
func settingsPair(left, right fyne.CanvasObject) fyne.CanvasObject {
	return container.NewGridWithColumns(2, left, right)
}

const (
	halfSettingGap      float32 = 8
	halfSettingRowWidth float32 = 280
	thresholdTrackWidth float32 = 76
	// settingLabelGap is the breathing room kept between the longest settings
	// label and its control.
	settingLabelGap     float32 = 10
	settingLabelMinimum float32 = 96
	settingLabelMaximum float32 = 190
)

// settingLabelKeys lists every label that shares the settings label column.
// Measuring all of them in the active language keeps controls on one vertical
// line for any translation, instead of the per-row hardcoded widths that kept
// leaving single rows (e.g. "Start with Windows") out of alignment.
var settingLabelKeys = []string{
	i18n.KeyShowClaude, i18n.KeyShowCodex, i18n.KeyShowAGGemini, i18n.KeyShowAGClaude, i18n.KeyShowGrok,
	i18n.KeyUsageMode, i18n.KeyWarnings,
	i18n.KeyAutoStart, i18n.KeyAlwaysOnTop, i18n.KeyPromoteTray, i18n.KeyRefreshInterval,
	i18n.KeyLanguage, i18n.KeyDateTime,
}

func (v *View) settingLabelWidth() float32 {
	maximum := float32(0)
	for _, key := range settingLabelKeys {
		width := fyne.MeasureText(v.text(key), SettingsTextSize, fyne.TextStyle{Bold: true}).Width
		maximum = max(maximum, width)
	}
	return min(settingLabelMaximum, max(settingLabelMinimum, float32(math.Ceil(float64(maximum+settingLabelGap)))))
}

func (v *View) halfToggleRow(key string, value bool, labelWidth float32, set func(*settings.Config, bool)) fyne.CanvasObject {
	return v.settingRowSized(v.text(key), NewToggle(value, func(on bool) { cfg := v.config; set(&cfg, on); v.SetConfig(cfg) }, v.colors), labelWidth, 8, 38)
}
func (v *View) halfTooltipToggleRow(key, tooltipKey string, value bool, labelWidth float32, set func(*settings.Config, bool)) fyne.CanvasObject {
	toggle := NewTooltipToggle(value, v.text(tooltipKey), func(on bool) { cfg := v.config; set(&cfg, on); v.SetConfig(cfg) }, v.colors)
	return v.settingRowSized(v.text(key), v.bindToggle(toggle), labelWidth, 8, 38)
}

// showWithCreditsRow is a provider-visibility row with a second, smaller
// "credits" toggle after it, controlling the paid-balance meta text.
func (v *View) showWithCreditsRow(key string, shown bool, setShown func(*settings.Config, bool), creditsOn bool, setCredits func(*settings.Config, bool)) fyne.CanvasObject {
	showToggle := NewToggle(shown, func(on bool) { cfg := v.config; setShown(&cfg, on); v.SetConfig(cfg) }, v.colors)
	creditsLabel := textLabel(v.text(i18n.KeyCreditsToggle), SettingsTextSize, v.colors.Label, false, false)
	creditsToggle := NewToggle(creditsOn, func(on bool) { cfg := v.config; setCredits(&cfg, on); v.SetConfig(cfg) }, v.colors)
	labelWidth := float32(math.Ceil(float64(fyne.MeasureText(v.text(i18n.KeyCreditsToggle), SettingsTextSize, fyne.TextStyle{}).Width))) + 2
	control := container.New(NewGapColumnLayout([]float32{38, labelWidth, 38}, []float32{16, 6}, 28), showToggle, creditsLabel, creditsToggle)
	return v.settingRowSized(v.text(key), control, v.settingLabelWidth(), 8, 0)
}
func (v *View) toggleRow(key string, value bool, set func(*settings.Config, bool)) fyne.CanvasObject {
	return v.settingRow(v.text(key), NewToggle(value, func(on bool) { cfg := v.config; set(&cfg, on); v.SetConfig(cfg) }, v.colors), 38)
}
func (v *View) usageSettings() fyne.CanvasObject {
	remainingLabel := v.text(i18n.KeyUsageRemaining)
	usageLabel := v.text(i18n.KeyUsageUsed)
	selectedMode := usageLabel
	if v.config.UsageMode == settings.UsageRemaining {
		selectedMode = remainingLabel
	}
	mode := NewRadioGroup([]string{remainingLabel, usageLabel}, selectedMode, func(value string) {
		cfg := v.config
		if value == remainingLabel {
			cfg.UsageMode = settings.UsageRemaining
		} else {
			cfg.UsageMode = settings.UsageUsed
		}
		v.SetConfig(cfg)
	}, v.colors)

	claudeRow := v.halfToggleRow(i18n.KeyShowClaude, v.config.ShowClaude, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.ShowClaude = b })
	codexRow := v.halfToggleRow(i18n.KeyShowCodex, v.config.ShowCodex, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.ShowCodex = b })
	if creditsDisplayEnabled {
		claudeRow = v.showWithCreditsRow(i18n.KeyShowClaude, v.config.ShowClaude, func(c *settings.Config, b bool) { c.ShowClaude = b },
			v.config.ShowClaudeCredits, func(c *settings.Config, b bool) { c.ShowClaudeCredits = b })
		codexRow = v.showWithCreditsRow(i18n.KeyShowCodex, v.config.ShowCodex, func(c *settings.Config, b bool) { c.ShowCodex = b },
			v.config.ShowCodexCredits, func(c *settings.Config, b bool) { c.ShowCodexCredits = b })
	}
	rows := []fyne.CanvasObject{
		settingsPair(claudeRow, codexRow),
		settingsPair(
			v.halfToggleRow(i18n.KeyShowAGGemini, v.config.ShowAGGemini, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.ShowAGGemini = b }),
			v.halfToggleRow(i18n.KeyShowAGClaude, v.config.ShowAGClaude, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.ShowAGClaude = b }),
		),
		settingsPair(
			v.halfToggleRow(i18n.KeyShowGrok, v.config.ShowGrok, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.ShowGrok = b }),
			layout.NewSpacer(),
		),
		settingsPair(
			v.settingRowSized(v.text(i18n.KeyUsageMode), mode, v.settingLabelWidth(), halfSettingGap, 0),
			v.halfToggleRow(i18n.KeyWarnings, v.config.WarningsEnabled, v.settingLabelWidth(), func(c *settings.Config, on bool) {
				c.WarningsEnabled = on
			}),
		),
	}
	if v.config.WarningsEnabled {
		thresholds := container.New(
			&CompactRowsLayout{Gap: 0},
			v.thresholdSliderControl(i18n.KeyWarningThreshold, v.config.WarningPercent, v.config.WarningColor, true),
			v.thresholdSliderControl(i18n.KeyDangerThreshold, v.config.DangerPercent, v.config.DangerColor, false),
		)
		rows = append(rows, settingsPair(layout.NewSpacer(), thresholds))
	} else {
		v.warningEntry = nil
		v.dangerEntry = nil
		v.warningSlider = nil
		v.dangerSlider = nil
	}
	return container.NewVBox(rows...)
}

func (v *View) thresholdSliderControl(key string, value float64, colorID string, warning bool) fyne.CanvasObject {
	apply := func(cfg *settings.Config, next float64) {
		if warning {
			cfg.WarningPercent = math.Round(next)
		} else {
			cfg.DangerPercent = math.Round(next)
		}
	}
	slider := widget.NewSlider(1, 99)
	slider.Step = 1
	slider.Value = value
	slider.OnChanged = func(next float64) {
		cfg := v.config
		apply(&cfg, next)
		v.SetConfig(cfg)
	}
	entry := v.thresholdEntry(value, apply)
	dot := NewPaletteDotButton(colorID, v.Canvas, nil, v.colors)
	dot.OnShowPalette = func() {
		v.showColorPalette(dot, dot.ID, "", false, func(id string) {
			dot.ID = id
			dot.Refresh()
			cfg := v.config
			if warning {
				cfg.WarningColor = id
			} else {
				cfg.DangerColor = id
			}
			v.SetConfig(cfg)
		})
	}
	if warning {
		v.warningEntry = entry
		v.warningSlider = slider
	} else {
		v.dangerEntry = entry
		v.dangerSlider = slider
	}
	entryControl := container.NewGridWrap(fyne.NewSize(34, 28), entry)
	controls := container.New(
		NewGapColumnLayout(
			[]float32{48, thresholdTrackWidth, 34, 10, 22},
			[]float32{6, 8, 2, 8},
			28,
		),
		textLabel(shortThresholdLabel(v.text(key)), ThresholdTextSize, v.colors.Label, true, false),
		slider,
		entryControl,
		textLabel("%", ThresholdTextSize, v.colors.Label, true, false),
		dot,
	)
	return controls
}

func shortThresholdLabel(label string) string {
	label = strings.TrimSuffix(label, " threshold")
	return strings.TrimSuffix(label, " 임계값")
}

func (v *View) thresholdEntry(value float64, apply func(*settings.Config, float64)) *widget.Entry {
	entry := widget.NewEntry()
	entry.OnChanged = func(string) { v.noteActivity() }
	entry.SetText(formatThreshold(value))
	entry.OnSubmitted = func(raw string) {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			v.syncThresholdEntries()
			return
		}
		cfg := v.config
		apply(&cfg, parsed)
		v.SetConfig(cfg)
	}
	return entry
}

func formatThreshold(value float64) string {
	return strconv.Itoa(int(math.Round(value)))
}

func (v *View) syncThresholdEntries() {
	if v.warningEntry != nil {
		value := formatThreshold(v.config.WarningPercent)
		if v.warningEntry.Text != value {
			v.warningEntry.SetText(value)
		}
	}
	if v.dangerEntry != nil {
		value := formatThreshold(v.config.DangerPercent)
		if v.dangerEntry.Text != value {
			v.dangerEntry.SetText(value)
		}
	}
	if v.warningSlider != nil && !approxEqual(v.warningSlider.Value, v.config.WarningPercent) {
		v.warningSlider.Value = v.config.WarningPercent
		v.warningSlider.Refresh()
	}
	if v.dangerSlider != nil && !approxEqual(v.dangerSlider.Value, v.config.DangerPercent) {
		v.dangerSlider.Value = v.config.DangerPercent
		v.dangerSlider.Refresh()
	}
}
func (v *View) behaviorSettings() fyne.CanvasObject {
	refresh := widget.NewSelect([]string{"15s", "30s", "60s", "5m", "15m"}, func(value string) {
		mapping := map[string]int{"15s": 15, "30s": 30, "60s": 60, "5m": 300, "15m": 900}
		cfg := v.config
		cfg.RefreshSeconds = mapping[value]
		v.SetConfig(cfg)
	})
	selectedRefresh := map[int]string{15: "15s", 30: "30s", 60: "60s", 300: "5m", 900: "15m"}[v.config.RefreshSeconds]
	if selectedRefresh == "" {
		selectedRefresh = "5m"
	}
	refresh.SetSelected(selectedRefresh)
	var promoteTrayIcon fyne.CanvasObject = layout.NewSpacer()
	if v.Actions.TrayPromotionSupported {
		promoteTrayIcon = v.halfTooltipToggleRow(i18n.KeyPromoteTray, i18n.KeyTooltipPromoteTray, v.config.PromoteTrayIcon, v.settingLabelWidth(), func(c *settings.Config, b bool) {
			c.PromoteTrayIcon = b
		})
	}
	return container.NewVBox(
		settingsPair(
			v.halfToggleRow(i18n.KeyAutoStart, v.config.AutoStart, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.AutoStart = b }),
			v.settingRowSized(v.text(i18n.KeyRefreshInterval), refresh, v.settingLabelWidth(), halfSettingGap, 110),
		),
		settingsPair(
			// Start-minimized sits directly under the startup toggle it belongs
			// to: it only means anything when starting with Windows is on, and
			// putting an unrelated row between them hid that relationship.
			//
			// The taskbar toggle was removed from the UI: ✕ already hides to
			// the tray, so the entry only added noise. The stored setting and
			// platform behaviour remain (default: visible in taskbar).
			v.halfToggleRow(i18n.KeyStartMinimized, v.config.StartMinimized, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.StartMinimized = b }),
			promoteTrayIcon,
		),
		settingsPair(
			v.halfToggleRow(i18n.KeyAlwaysOnTop, v.config.AlwaysOnTop, v.settingLabelWidth(), func(c *settings.Config, b bool) { c.AlwaysOnTop = b }),
			layout.NewSpacer(),
		),
	)
}
func (v *View) displaySettings() fyne.CanvasObject {
	languages := []string{v.text(i18n.KeyLanguageSystem)}
	languageValues := []settings.Language{settings.LanguageSystem}
	for _, supported := range i18n.Supported {
		languages = append(languages, i18n.Endonym(supported))
		languageValues = append(languageValues, settings.Language(supported))
	}
	language := widget.NewSelect(languages, func(value string) {
		cfg := v.config
		cfg.Language = settings.LanguageSystem
		for index, label := range languages {
			if value == label {
				cfg.Language = languageValues[index]
				break
			}
		}
		v.SetConfig(cfg)
	})
	selectedLanguage := settings.LanguageSystem
	for _, supported := range i18n.Supported {
		if settings.Language(supported) == v.config.Language {
			selectedLanguage = v.config.Language
			break
		}
	}
	for index, value := range languageValues {
		if value == selectedLanguage {
			language.SetSelected(languages[index])
			break
		}
	}
	dateKeys := []settings.DateTimeFormat{settings.Format12HourDate, settings.Format12HourDateDay, settings.Format24HourDate, settings.Format24HourDateDay}
	examples := make([]string, 4)
	lang := v.resolvedLanguage()
	for i, key := range dateKeys {
		examples[i], _ = qdatetime.FormatUnix(time.Now().Unix(), time.Local, lang, qdatetime.Format(key))
	}
	dateSelect := widget.NewSelect(examples, func(value string) {
		for i, example := range examples {
			if value == example {
				cfg := v.config
				cfg.DateTimeFormat = dateKeys[i]
				v.SetConfig(cfg)
			}
		}
	})
	for i, key := range dateKeys {
		if key == v.config.DateTimeFormat {
			dateSelect.SetSelected(examples[i])
		}
	}
	languageWidth := selectWidth(languages)
	dateWidth := selectWidth(examples)
	displayLabelWidth := min(v.settingLabelWidth(), halfSettingRowWidth-halfSettingGap-max(languageWidth, dateWidth))
	return container.NewVBox(
		settingsPair(
			v.settingRowSized(v.text(i18n.KeyLanguage), language, displayLabelWidth, halfSettingGap, languageWidth),
			v.settingRowSized(v.text(i18n.KeyDateTime), dateSelect, displayLabelWidth, halfSettingGap, dateWidth),
		),
	)
}

func selectWidth(options []string) float32 {
	maximum := float32(0)
	for _, option := range options {
		maximum = max(maximum, fyne.MeasureText(option, theme.TextSize(), fyne.TextStyle{}).Width)
	}
	chrome := theme.IconInlineSize() + theme.InnerPadding()*4
	return float32(math.Ceil(float64(maximum + chrome)))
}
func (v *View) connectionSettings() fyne.CanvasObject {
	v.connectionsBody = container.NewVBox()
	v.connectionCache = nil
	v.renderConnectionSettings()
	return v.connectionsBody
}

func (v *View) renderConnectionSettings() { v.syncConnections() }
func (v *View) MinimumSize(screen Screen) fyne.Size {
	switch screen {
	case CompactScreen:
		if v.Compact != nil {
			return fyne.NewSize(CompactWidth, v.Compact.MinSize().Height)
		}
		return fyne.NewSize(CompactWidth, 1)
	case NanoScreen:
		if v.Nano != nil {
			return fyne.NewSize(NanoWidth, max(v.Nano.MinSize().Height, TitleBarHeight+NanoBodyHeight))
		}
		return fyne.NewSize(NanoWidth, TitleBarHeight+NanoBodyHeight)
	case SettingsScreen:
		height := SettingsHeight
		if v.Settings != nil && v.Settings.MinSize().Height > height {
			height = v.Settings.MinSize().Height
		}
		return fyne.NewSize(SettingsWidth, height)
	default:
		if v.Normal != nil {
			return fyne.NewSize(NormalWidth, v.Normal.MinSize().Height)
		}
		return fyne.NewSize(NormalWidth, 1)
	}
}
func approxEqual(a, b float64) bool { return math.Abs(a-b) < 0.001 }
