package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

type normalBodyView struct {
	signature    string
	columnHeader *fyne.Container
	usageHeader  *canvas.Text
	resetHeader  *canvas.Text
	headers      []normalHeaderView
	statuses     []*canvas.Text
	rows         []normalUsageView
	dividers     []*canvas.Rectangle
}

type normalHeaderView struct {
	name    *canvas.Text
	plan    *canvas.Text
	credits *canvas.Text
}

type normalUsageView struct {
	row           *fyne.Container
	meterStack    *fyne.Container
	label         *canvas.Text
	meter         *SegmentedMeter
	resetBar      *SlimProgressBar
	percent       *canvas.Text
	percentSymbol *canvas.Text
	until         *canvas.Text
	resetAt       []*canvas.Text
}

type compactBodyView struct {
	signature    string
	labelWidth   float32
	columnHeader *fyne.Container
	usageHeader  *canvas.Text
	resetHeader  *canvas.Text
	statuses     []*canvas.Text
	rows         []compactUsageView
	dividers     []*canvas.Rectangle
}

type compactUsageView struct {
	background  *canvas.Rectangle
	icon        *canvas.Image
	label       *canvas.Text
	meter       *SegmentedMeter
	reset       *SlimProgressBar
	percent     *fyne.Container
	number      *canvas.Text
	symbol      *canvas.Text
	resetUntil  *canvas.Text
	resetRegion *TooltipRegion
}

type nanoBodyView struct {
	signature string
	cells     []nanoCellView
}

type nanoCellView struct {
	background *canvas.Rectangle
	icon       *canvas.Image
	rows       []nanoUsageView
}

type nanoUsageView struct {
	label  *canvas.Text
	bar    *SlimProgressBar
	reset  *SlimProgressBar
	region *TooltipRegion
}

type nanoCellState struct {
	key       string
	name      string
	kind      ProviderIconKind
	connected bool
	rows      []nanoUsageState
}

type nanoUsageState struct {
	label     string
	row       UsageRowState
	available bool
}

type connectionView struct {
	id             model.ProviderID
	status         *canvas.Text
	dot            *canvas.Circle
	detail         *canvas.Text
	install        *fyne.Container
	installRow     fyne.CanvasObject
	installButton  *SmallButton
	installVisible bool
	testButton     *SmallButton
	reconnect      *SmallButton
	helpButton     *SmallButton
	actionRow      *fyne.Container
}

const (
	claudeInstallURL = "https://code.claude.com/docs/en/quickstart"
	codexInstallURL  = "https://developers.openai.com/codex/cli/"
)

func (v *View) renderCurrentScreen() {
	switch v.screen {
	case CompactScreen:
		v.renderCompactBody()
	case NanoScreen:
		v.renderNanoBody()
	case SettingsScreen:
		v.renderConnectionSettings()
	default:
		v.renderNormalBody()
	}
}

func (v *View) syncNormalBody() {
	if v.normalBody == nil {
		return
	}
	lanes := v.visibleLanes()
	now := time.Now()
	signature := v.normalBodySignature(lanes, now)
	if v.normalCache == nil || v.normalCache.signature != signature {
		v.rebuildNormalBody(lanes, now, signature)
		return
	}
	v.updateUsageColumnHeaders()

	headerIndex, statusIndex, rowIndex := 0, 0, 0
	for _, lane := range lanes {
		v.updateNormalHeader(v.normalCache.headers[headerIndex], lane)
		headerIndex++
		if len(lane.Rows) == 0 {
			setCanvasText(v.normalCache.statuses[statusIndex], v.laneStatusText(lane), v.colors.Secondary)
			statusIndex++
			continue
		}
		for _, row := range lane.Rows {
			v.updateNormalUsage(v.normalCache.rows[rowIndex], lane, row, now)
			rowIndex++
		}
	}
}

func (v *View) rebuildNormalBody(lanes []LaneState, now time.Time, signature string) {
	// Size the label column for the labels this language actually shows before
	// any row or header is built, so every column shares the same geometry.
	normalRowColumns = normalRowColumnsFor(v.normalLabelWidth(lanes))
	cache := &normalBodyView{signature: signature}
	columnHeader, usageHeader, resetHeader := v.makeNormalColumnHeader()
	cache.columnHeader = columnHeader
	cache.usageHeader = usageHeader
	cache.resetHeader = resetHeader
	if v.normalHeaderWrap != nil {
		v.normalHeaderWrap.Objects = []fyne.CanvasObject{columnHeader}
		v.normalHeaderWrap.Refresh()
	}
	objects := make([]fyne.CanvasObject, 0, len(lanes)*2)
	for laneIndex, lane := range lanes {
		// Separate provider groups with the same thin line compact mode uses,
		// so the lane boundaries read the same way in every display mode.
		if laneIndex > 0 {
			divider, line := v.makeProviderGroupDivider()
			cache.dividers = append(cache.dividers, line)
			objects = append(objects, divider)
		}
		header, handles := v.makeLaneHeader(lane)
		cache.headers = append(cache.headers, handles)
		objects = append(objects, header)
		if len(lane.Rows) == 0 {
			status, text := v.makeNormalStatusRow(lane)
			cache.statuses = append(cache.statuses, text)
			objects = append(objects, status)
			continue
		}
		for _, row := range lane.Rows {
			object, handles := v.makeNormalUsageRow(lane, row, now)
			cache.rows = append(cache.rows, handles)
			objects = append(objects, object)
		}
	}
	v.normalBody.Objects = objects
	v.normalCache = cache
	v.normalBody.Refresh()
}

func (v *View) makeNormalColumnHeader() (*fyne.Container, *canvas.Text, *canvas.Text) {
	// Centre each caption inside its own column so "Remaining" sits over the
	// meters and "Reset" over the reset times.
	usage := textLabel(v.usageColumnHeaderText(), UsageHeaderTextSize, v.colors.Secondary, false, false)
	usage.Alignment = fyne.TextAlignCenter
	reset := textLabel(v.text(i18n.KeyResetsAt), UsageHeaderTextSize, v.colors.Secondary, false, false)
	reset.Alignment = fyne.TextAlignCenter
	columns := container.New(
		NewColumnLayout(normalRowColumns, NormalRowGap, UsageHeaderRowHeight),
		layout.NewSpacer(),
		usage,
		reset,
	)
	return v.headerStrip(columns, 12, 12), usage, reset
}

// headerStrip paints a caption row on the titlebar tone across the full window
// width, merging the captions into the header block instead of leaving a band
// of body background between the titlebar and the tinted rows (W3).
func (v *View) headerStrip(columns fyne.CanvasObject, padLeft, padRight float32) *fyne.Container {
	background := canvas.NewRectangle(v.colors.TitleBottom)
	return container.NewStack(background, container.New(layout.NewCustomPaddedLayout(0, 0, padLeft, padRight), columns))
}

func (v *View) normalBodySignature(lanes []LaneState, now time.Time) string {
	var signature strings.Builder
	for _, lane := range lanes {
		hasPlan := lane.Plan != model.PlanUnknown && lane.Plan != ""
		fmt.Fprintf(&signature, "lane:%q:plan=%t:credits=%t:rows=%d|", lane.Provider, hasPlan, v.laneCreditsVisible(lane), len(lane.Rows))
		for _, row := range lane.Rows {
			_, resetAt := resetStrings(row, now, v.config)
			resetLines := wrapMonospace(resetAt, normalRowColumns[2], NormalMetaTextSize)
			fmt.Fprintf(&signature, "row:%q:%q:%d:reset-lines=%d|", row.Label, row.DisplayLabel, row.WindowMinutes, len(resetLines))
		}
	}
	return signature.String()
}

func (v *View) makeLaneHeader(lane LaneState) (fyne.CanvasObject, normalHeaderView) {
	// The provider name is the strongest label in the row group: slightly larger
	// and in the full text colour, while row labels stay a shade lighter.
	name := textLabel(lane.Name, LaneHeaderTextSize, v.colors.Text, true, false)
	handles := normalHeaderView{name: name}
	objects := []fyne.CanvasObject{name}
	if lane.Plan != model.PlanUnknown && lane.Plan != "" {
		handles.plan = textLabel(string(lane.Plan), PlanChipTextSize, v.colors.PlanChipText, true, false)
		chipBackground := canvas.NewRectangle(v.colors.PlanChip)
		chipBackground.CornerRadius = 7
		chip := container.NewStack(chipBackground, container.New(layout.NewCustomPaddedLayout(PlanChipPaddingY, PlanChipPaddingY, PlanChipPaddingX, PlanChipPaddingX), container.NewCenter(handles.plan)))
		// Hold a 6px gap between the provider name and its plan chip by padding
		// the chip, so the header keeps its expected two-object structure.
		objects = append(objects, container.New(layout.NewCustomPaddedLayout(0, 0, LaneHeaderChipGap, 0), chip))
	}
	if v.laneCreditsVisible(lane) {
		// The paid extra-usage balance sits as quiet meta text after the plan
		// chip; providers without credit reporting render nothing here.
		handles.credits = textLabel(v.creditsText(lane.Credits), CreditsTextSize, v.colors.Secondary, false, false)
		objects = append(objects, container.New(layout.NewCustomPaddedLayout(0, 0, LaneHeaderChipGap, 0), handles.credits))
	}
	return container.NewHBox(objects...), handles
}

// creditsDisplayEnabled gates the whole paid-credit surface: the lane-header
// meta text, the CONNECTIONS detail entry, and the Settings toggles. The Codex
// pipeline already delivers the balance, but the surface stays off until the
// Claude browser-login path can report credits too, so both providers launch
// together (plan: docs/REMAINING-WORK.md).
const creditsDisplayEnabled = false

// laneCreditsVisible combines "the provider reported a balance" with the
// per-provider visibility toggle from Settings.
func (v *View) laneCreditsVisible(lane LaneState) bool {
	if !creditsDisplayEnabled || lane.Credits == nil {
		return false
	}
	switch lane.Provider {
	case model.ProviderClaude:
		return v.config.ShowClaudeCredits
	case model.ProviderCodex:
		return v.config.ShowCodexCredits
	}
	return true
}

func (v *View) creditsText(credits *model.Credits) string {
	if credits == nil {
		return ""
	}
	if credits.Unlimited {
		return v.text(i18n.KeyCreditsUnlimited)
	}
	return fmt.Sprintf(v.text(i18n.KeyCredits), strconv.FormatFloat(credits.Balance, 'f', -1, 64))
}

func (v *View) updateNormalHeader(handles normalHeaderView, lane LaneState) {
	setCanvasText(handles.name, lane.Name, v.colors.Text)
	if handles.plan != nil {
		setCanvasText(handles.plan, string(lane.Plan), v.colors.PlanChipText)
	}
	if handles.credits != nil && lane.Credits != nil {
		setCanvasText(handles.credits, v.creditsText(lane.Credits), v.colors.Secondary)
	}
}

func (v *View) makeNormalStatusRow(lane LaneState) (fyne.CanvasObject, *canvas.Text) {
	status := textLabel(v.laneStatusText(lane), NormalLabelTextSize, v.colors.Secondary, false, false)
	return container.New(
		NewColumnLayout(normalRowColumns, NormalRowGap, NormalRowHeight),
		status,
		layout.NewSpacer(),
		layout.NewSpacer(),
	), status
}

// rowVisual carries every mode-independent visual decision for a usage row:
// colours, severity, percent, reset progress, and formatted strings. The mode
// renderers consume these tokens instead of re-deriving them, so cross-mode
// rules (achromatic reset bars, danger-bold numbers, meter direction, row
// tinting) live in exactly one place and cannot drift apart again.
// Mode-specific differences are limited to layout tokens: sizes, and which of
// these tokens a mode chooses to show.
type rowVisual struct {
	label         string
	percent       float64
	resetPercent  float64
	danger        bool
	meterColor    color.Color
	percentColor  color.Color
	resetBarColor color.Color
	background    color.Color
	until         string
	resetAt       string
	iconKind      ProviderIconKind
	colorKey      string
}

func (v *View) rowVisual(lane LaneState, row UsageRowState, now time.Time) rowVisual {
	level, severityColor := v.severity(row.Percent)
	providerColor := v.colors.PaletteColor(providerColorID(lane.Provider, row, v.config))
	// The meter and number carry the state: they switch to the severity colour
	// while the icon and row tint keep the provider colour (W9).
	meterColor := providerColor
	var percentColor color.Color = v.colors.PercentNormal
	if level != model.AlertNormal {
		meterColor = severityColor
		percentColor = severityColor
	}
	until, resetAt := resetStrings(row, now, v.config)
	return rowVisual{
		label:         v.usageRowLabel(lane, row),
		percent:       v.displayPercent(row),
		resetPercent:  v.resetRemainingPercent(row, now),
		danger:        level == model.AlertDanger,
		meterColor:    meterColor,
		percentColor:  percentColor,
		resetBarColor: v.resetBarColor(),
		background:    compactRowBackground(v.colors.Background, providerColor),
		until:         until,
		resetAt:       resetAt,
		iconKind:      providerIconKind(lane, row),
		colorKey:      providerColorKey(lane.Provider, row),
	}
}

func (v *View) makeNormalUsageRow(lane LaneState, row UsageRowState, now time.Time) (fyne.CanvasObject, normalUsageView) {
	visual := v.rowVisual(lane, row, now)
	label := textLabel(visual.label, NormalLabelTextSize, v.colors.Label, true, false)
	meter := NewSegmentedMeter(20, visual.percent, visual.meterColor, v.colors.Track)
	meter.Height = NormalMeterHeight
	meter.OnTapped = func() { v.showProviderPalette(meter, visual.colorKey) }
	percentText := textLabel(fmt.Sprintf("%.0f", visual.percent), NormalPercentTextSize, visual.percentColor, visual.danger, false)
	percentText.Alignment = fyne.TextAlignTrailing
	percentSymbol := textLabel("%", NormalPercentTextSize-1, visual.percentColor, visual.danger, false)
	percentBox := container.New(
		&CompactPercentLayout{RightInset: NormalPercentInset},
		percentText,
		percentSymbol,
	)
	// The reset-time bar (W2) is the continuous line under the segmented quota
	// meter: same span, achromatic, and always showing the remaining time so
	// it reads the same way in every mode and display method.
	resetBar := NewSlimProgressBar(visual.resetPercent, visual.resetBarColor, v.colors.Track)
	resetBar.Height = NormalResetBarHeight
	untilText := textLabel(visual.until, NormalMetaTextSize, v.colors.Text, true, true)
	resetLines := []fyne.CanvasObject{untilText}
	handles := normalUsageView{
		label:         label,
		meter:         meter,
		resetBar:      resetBar,
		percent:       percentText,
		percentSymbol: percentSymbol,
		until:         untilText,
	}
	for _, line := range wrapMonospace(visual.resetAt, normalRowColumns[2], NormalMetaTextSize) {
		text := textLabel(line, NormalMetaTextSize, v.colors.Secondary, false, true)
		handles.resetAt = append(handles.resetAt, text)
		resetLines = append(resetLines, text)
	}
	// Pull the reset date a touch closer to the countdown above it.
	reset := container.New(&CompactRowsLayout{Gap: NormalResetLineGap}, resetLines...)
	// Sit the percentage just above the right end of the meter, as in the design:
	// the number keeps its size while the % sign is a shade smaller.
	meterWithPercent := container.New(
		&NormalMeterStackLayout{PercentHeight: NormalPercentBandHeight, MeterHeight: NormalMeterHeight, BarHeight: NormalResetBarHeight, Gap: NormalResetBarGap},
		percentBox,
		meter,
		resetBar,
	)
	object := container.New(
		NewColumnLayout(normalRowColumns, NormalRowGap, NormalRowHeight),
		label,
		meterWithPercent,
		container.NewCenter(reset),
	)
	handles.row = object
	handles.meterStack = meterWithPercent
	return object, handles
}

func (v *View) updateNormalUsage(handles normalUsageView, lane LaneState, row UsageRowState, now time.Time) {
	visual := v.rowVisual(lane, row, now)
	setCanvasText(handles.label, visual.label, v.colors.Label)
	handles.meter.SetValue(visual.percent, visual.meterColor)
	handles.resetBar.SetValue(visual.resetPercent, visual.resetBarColor, v.colors.Track)
	setPercentWeight(handles.percent, handles.percentSymbol, visual.danger)
	setCanvasText(handles.percent, fmt.Sprintf("%.0f", visual.percent), visual.percentColor)
	setCanvasText(handles.percentSymbol, "%", visual.percentColor)
	setCanvasText(handles.until, visual.until, v.colors.Text)
	for index, line := range wrapMonospace(visual.resetAt, normalRowColumns[2], NormalMetaTextSize) {
		setCanvasText(handles.resetAt[index], line, v.colors.Secondary)
	}
}

// setPercentWeight applies the danger-only bold cue (W9) in place.
func setPercentWeight(number, symbol *canvas.Text, bold bool) {
	if number.TextStyle.Bold == bold && symbol.TextStyle.Bold == bold {
		return
	}
	number.TextStyle.Bold = bold
	symbol.TextStyle.Bold = bold
	number.Refresh()
	symbol.Refresh()
}

func (v *View) syncCompactBody() {
	if v.compactBody == nil {
		return
	}
	lanes := v.visibleLanes()
	signature := v.compactBodySignature(lanes)
	if v.compactCache == nil || v.compactCache.signature != signature {
		v.rebuildCompactBody(lanes, signature)
		return
	}
	v.updateUsageColumnHeaders()

	now := time.Now()
	statusIndex, rowIndex := 0, 0
	for _, lane := range lanes {
		if len(lane.Rows) == 0 {
			setCanvasText(v.compactCache.statuses[statusIndex], v.laneStatusText(lane), v.colors.Secondary)
			statusIndex++
			continue
		}
		for _, row := range lane.Rows {
			v.updateCompactUsage(v.compactCache.rows[rowIndex], lane, row, now)
			rowIndex++
		}
	}
}

func (v *View) rebuildCompactBody(lanes []LaneState, signature string) {
	labelWidth := v.compactLabelWidth(lanes)
	cache := &compactBodyView{signature: signature, labelWidth: labelWidth}
	objects := make([]fyne.CanvasObject, 0)
	columnHeader, usageHeader, resetHeader := v.makeCompactColumnHeader(labelWidth)
	cache.columnHeader = columnHeader
	cache.usageHeader = usageHeader
	cache.resetHeader = resetHeader
	if v.compactHeaderWrap != nil {
		v.compactHeaderWrap.Objects = []fyne.CanvasObject{columnHeader}
		v.compactHeaderWrap.Refresh()
	}
	now := time.Now()
	for laneIndex, lane := range lanes {
		if laneIndex > 0 {
			divider, line := v.makeProviderGroupDivider()
			cache.dividers = append(cache.dividers, line)
			objects = append(objects, divider)
		}
		for _, row := range lane.Rows {
			object, handles := v.makeCompactUsageRow(lane, row, true, labelWidth, now)
			cache.rows = append(cache.rows, handles)
			objects = append(objects, object)
		}
		if len(lane.Rows) == 0 {
			status, text := v.makeCompactStatusRow(lane)
			cache.statuses = append(cache.statuses, text)
			objects = append(objects, status)
		}
	}
	v.compactBody.Objects = objects
	v.compactCache = cache
	v.compactBody.Refresh()
}

func (v *View) makeCompactColumnHeader(labelWidth float32) (*fyne.Container, *canvas.Text, *canvas.Text) {
	// Centre the usage caption over the meter column and right-align the reset
	// caption over the reset times (W11); the whole strip sits on the titlebar
	// tone so it reads as part of the header block (W3).
	usage := textLabel(v.usageColumnHeaderText(), UsageHeaderTextSize, v.colors.Secondary, false, false)
	usage.Alignment = fyne.TextAlignCenter
	reset := textLabel(v.text(i18n.KeyResetsAt), UsageHeaderTextSize, v.colors.Secondary, false, false)
	reset.Alignment = fyne.TextAlignTrailing
	columns := container.New(
		NewColumnLayout(compactRowColumns(labelWidth), CompactColumnGap, UsageHeaderRowHeight),
		layout.NewSpacer(),
		layout.NewSpacer(),
		usage,
		layout.NewSpacer(),
		reset,
	)
	return v.headerStrip(columns, CompactPaddingLeft, CompactPaddingRight), usage, reset
}

func (v *View) usageColumnHeaderText() string {
	if v.config.UsageMode == settings.UsageRemaining {
		return v.text(i18n.KeyUsageRemaining)
	}
	return v.text(i18n.KeyUsageUsed)
}

func (v *View) updateUsageColumnHeaders() {
	usage := v.usageColumnHeaderText()
	if v.normalCache != nil {
		setCanvasText(v.normalCache.usageHeader, usage, v.colors.Secondary)
		setCanvasText(v.normalCache.resetHeader, v.text(i18n.KeyResetsAt), v.colors.Secondary)
	}
	if v.compactCache != nil {
		setCanvasText(v.compactCache.usageHeader, usage, v.colors.Secondary)
		setCanvasText(v.compactCache.resetHeader, v.text(i18n.KeyResetsAt), v.colors.Secondary)
	}
}

// makeProviderGroupDivider draws the thin translucent rule between provider
// groups; normal and compact mode share it so the separation reads identically.
func (v *View) makeProviderGroupDivider() (fyne.CanvasObject, *canvas.Rectangle) {
	foreground := color.NRGBAModel.Convert(v.colors.Text).(color.NRGBA)
	line := canvas.NewRectangle(color.NRGBA{
		R: foreground.R,
		G: foreground.G,
		B: foreground.B,
		A: CompactDividerAlpha,
	})
	line.SetMinSize(fyne.NewSize(1, 1))
	return container.New(
		layout.NewCustomPaddedLayout(
			CompactDividerPaddingY,
			CompactDividerPaddingY,
			CompactDividerInset,
			CompactDividerInset,
		),
		line,
	), line
}

func (v *View) compactBodySignature(lanes []LaneState) string {
	var signature strings.Builder
	for _, lane := range lanes {
		fmt.Fprintf(&signature, "lane:%q:rows=%d|", lane.Provider, len(lane.Rows))
		for _, row := range lane.Rows {
			fmt.Fprintf(&signature, "row:%q:%q:%d:icon=%q", row.Label, row.DisplayLabel, row.WindowMinutes, providerIconKind(lane, row))
			signature.WriteByte('|')
		}
	}
	return signature.String()
}

func (v *View) makeCompactStatusRow(lane LaneState) (fyne.CanvasObject, *canvas.Text) {
	status := textLabel(v.laneStatusText(lane), CompactLabelTextSize, v.colors.Secondary, false, false)
	return container.New(layout.NewCustomPaddedLayout(5, 0, 5, 0), status), status
}

func (v *View) makeCompactUsageRow(lane LaneState, row UsageRowState, showIcon bool, labelWidth float32, now time.Time) (fyne.CanvasObject, compactUsageView) {
	visual := v.rowVisual(lane, row, now)
	var icon fyne.CanvasObject = layout.NewSpacer()
	var iconImage *canvas.Image
	if showIcon {
		// The icon holds the provider identity (W9): it wears the official
		// brand-logo colour, fixed even while the meter switches to warning or
		// danger. The default palette hues mirror these logo colours, so the
		// icon and its resting meter still read as one colour.
		iconImage = NewProviderIcon(visual.iconKind, v.config.Theme)
		icon = iconImage
	}
	label := textLabel(visual.label, CompactLabelTextSize, v.colors.Text, true, false)
	meter := NewSegmentedMeter(1, visual.percent, visual.meterColor, v.colors.Track)
	meter.Height = CompactMeterHeight
	meter.Gap = CompactMeterGap
	meter.SetSquareSegments(32)
	meter.OnTapped = func() { v.showProviderPalette(meter, visual.colorKey) }
	reset := NewSlimProgressBar(visual.resetPercent, visual.resetBarColor, v.colors.Track)
	reset.Height = 2
	meterStack := container.New(
		&CompactMeterLayout{MeterHeight: CompactMeterHeight, BarHeight: 2, Gap: 1},
		meter,
		reset,
	)
	numberText := textLabel("", CompactPercentTextSize, visual.percentColor, visual.danger, false)
	symbolText := textLabel("%", CompactSymbolTextSize, visual.percentColor, visual.danger, false)
	numberText.Alignment = fyne.TextAlignLeading
	symbolText.Alignment = fyne.TextAlignLeading
	percentColumn := container.New(
		&CompactPercentLayout{OffsetY: CompactPercentOffset, RightInset: CompactPercentMargin},
		numberText,
		symbolText,
	)
	setCompactPercent(percentColumn, numberText, symbolText, visual.percent, visual.percentColor)
	// The reset column (W11) shows the countdown; hovering it reveals the full
	// reset moment, already rendered through the shared date/time format (W4).
	untilText := textLabel(visual.until, CompactResetTextSize, v.colors.Text, false, true)
	untilText.Alignment = fyne.TextAlignTrailing
	resetRegion := NewTooltipRegion(v, visual.resetAt)
	resetColumn := container.NewStack(untilText, resetRegion)
	content := container.New(
		NewColumnLayout(compactRowColumns(labelWidth), CompactColumnGap, CompactRowHeight),
		icon,
		label,
		meterStack,
		percentColumn,
		resetColumn,
	)
	background := canvas.NewRectangle(visual.background)
	background.CornerRadius = 4
	object := container.NewStack(background, content)
	return object, compactUsageView{
		background:  background,
		icon:        iconImage,
		label:       label,
		meter:       meter,
		reset:       reset,
		percent:     percentColumn,
		number:      numberText,
		symbol:      symbolText,
		resetUntil:  untilText,
		resetRegion: resetRegion,
	}
}

func (v *View) syncNanoBody() {
	if v.nanoBody == nil {
		return
	}
	cells := v.nanoCellStates()
	signature := nanoBodySignature(cells)
	now := time.Now()
	if v.nanoCache == nil || v.nanoCache.signature != signature {
		v.rebuildNanoBody(cells, signature, now)
		return
	}
	for index, cell := range cells {
		v.updateNanoCell(v.nanoCache.cells[index], cell, now)
	}
}

func (v *View) rebuildNanoBody(cells []nanoCellState, signature string, now time.Time) {
	cache := &nanoBodyView{signature: signature}
	objects := make([]fyne.CanvasObject, 0, len(cells))
	for _, cell := range cells {
		object, handles := v.makeNanoCell(cell, now)
		objects = append(objects, object)
		cache.cells = append(cache.cells, handles)
	}
	columns := max(1, len(objects))
	v.nanoBody.Layout = layout.NewGridLayoutWithColumns(columns)
	v.nanoBody.Objects = objects
	v.nanoCache = cache
	v.nanoBody.Refresh()
}

func nanoBodySignature(cells []nanoCellState) string {
	var signature strings.Builder
	for _, cell := range cells {
		fmt.Fprintf(&signature, "cell:%s:rows=%d|", cell.key, len(cell.rows))
		for _, row := range cell.rows {
			fmt.Fprintf(&signature, "row:%s|", row.label)
		}
	}
	return signature.String()
}

func (v *View) makeNanoCell(cell nanoCellState, now time.Time) (fyne.CanvasObject, nanoCellView) {
	providerColor := v.colors.PaletteColor(v.config.ProviderColors[cell.key])
	background := canvas.NewRectangle(compactRowBackground(v.colors.Background, providerColor))
	background.CornerRadius = 4
	// The icon holds the provider identity (W9): it wears the official brand
	// colour, fixed even while the usage bar switches to warning or danger.
	icon := NewProviderIcon(cell.kind, v.config.Theme)
	if !cell.connected {
		icon.Translucency = 0.55
	}
	handles := nanoCellView{background: background, icon: icon}
	lines := make([]fyne.CanvasObject, 0, len(cell.rows))
	for _, state := range cell.rows {
		value, active := v.nanoUsageValue(state, providerColor)
		label := textLabel(state.label, NanoLabelTextSize, v.colors.Label, true, false)
		bar := NewSlimProgressBar(value, active, v.colors.Track)
		bar.Height = NanoUsageBarHeight
		reset := NewSlimProgressBar(v.nanoResetPercent(state, now), v.resetBarColor(), v.colors.Track)
		reset.Height = NanoResetBarHeight
		line := container.New(
			&NanoUsageLayout{LabelWidth: 14, Gap: 2, BarHeight: NanoUsageBarHeight, ResetHeight: NanoResetBarHeight, ResetGap: NanoResetGap},
			label,
			bar,
			reset,
		)
		// The whole row is the hover target (W12): the bars alone are a few
		// pixels tall, far too small to hit reliably.
		region := NewTooltipRegion(v, v.nanoRowTooltip(cell, state, now))
		lines = append(lines, container.NewStack(line, region))
		handles.rows = append(handles.rows, nanoUsageView{label: label, bar: bar, reset: reset, region: region})
	}
	stack := container.New(&NanoLinesLayout{Gap: NanoLineGap}, lines...)
	content := container.New(&NanoCellLayout{IconSize: providerIconSize, Gap: 3}, icon, stack)
	return container.NewStack(background, container.New(layout.NewCustomPaddedLayout(3, 3, 3, 3), content)), handles
}

// nanoRowTooltip is the three-line nano hover text (W12): the provider and
// window ("Claude 5h") on top, then the remaining time, then the reset moment
// through the shared date/time format (W4).
func (v *View) nanoRowTooltip(cell nanoCellState, state nanoUsageState, now time.Time) string {
	if !state.available {
		return ""
	}
	until, resetAt := resetStrings(state.row, now, v.config)
	return cell.name + " " + state.label + "\n" + until + "\n" + resetAt
}

func (v *View) updateNanoCell(handles nanoCellView, cell nanoCellState, now time.Time) {
	providerColor := v.colors.PaletteColor(v.config.ProviderColors[cell.key])
	setRectangleFill(handles.background, compactRowBackground(v.colors.Background, providerColor))
	translucency := float64(0)
	if !cell.connected {
		translucency = 0.55
	}
	if handles.icon.Translucency != translucency {
		handles.icon.Translucency = translucency
		handles.icon.Refresh()
	}
	for index, state := range cell.rows {
		value, active := v.nanoUsageValue(state, providerColor)
		setCanvasText(handles.rows[index].label, state.label, v.colors.Label)
		handles.rows[index].bar.SetValue(value, active, v.colors.Track)
		handles.rows[index].reset.SetValue(v.nanoResetPercent(state, now), v.resetBarColor(), v.colors.Track)
		handles.rows[index].region.SetValue(v.nanoRowTooltip(cell, state, now))
	}
}

func (v *View) nanoResetPercent(state nanoUsageState, now time.Time) float64 {
	if !state.available {
		return 0
	}
	return v.resetRemainingPercent(state.row, now)
}

func (v *View) nanoUsageValue(state nanoUsageState, providerColor color.Color) (float64, color.Color) {
	if !state.available {
		return 0, providerColor
	}
	active := providerColor
	if level, severityColor := v.severity(state.row.Percent); level != model.AlertNormal {
		active = severityColor
	}
	return v.displayPercent(state.row), active
}

func (v *View) nanoCellStates() []nanoCellState {
	lanes := make(map[model.ProviderID]LaneState, len(v.state.Lanes))
	for _, lane := range v.state.Lanes {
		lanes[lane.Provider] = lane
	}
	cells := make([]nanoCellState, 0, 4)
	if v.config.ShowClaude {
		lane := lanes[model.ProviderClaude]
		cells = append(cells, nanoCellState{key: "claude", name: "Claude", kind: ProviderIconClaude, connected: lane.Status == model.StatusConnected, rows: selectNanoRows(lane.Rows, false)})
	}
	if v.config.ShowCodex {
		lane := lanes[model.ProviderCodex]
		cells = append(cells, nanoCellState{key: "codex", name: "Codex", kind: ProviderIconCodex, connected: lane.Status == model.StatusConnected, rows: selectNanoRows(lane.Rows, true)})
	}
	if v.config.ShowAGGemini {
		lane := lanes[model.ProviderAntigravity]
		cells = append(cells, nanoCellState{key: "antigravity-gemini", name: "AG Gemini", kind: ProviderIconGemini, connected: lane.Status == model.StatusConnected, rows: selectNanoRows(filterNanoRows(lane.Rows, true), false)})
	}
	if v.config.ShowAGClaude {
		lane := lanes[model.ProviderAntigravity]
		cells = append(cells, nanoCellState{key: "antigravity", name: "AG Claude", kind: ProviderIconAGClaude, connected: lane.Status == model.StatusConnected, rows: selectNanoRows(filterNanoRows(lane.Rows, false), false)})
	}
	return cells
}

func filterNanoRows(rows []UsageRowState, gemini bool) []UsageRowState {
	filtered := make([]UsageRowState, 0, len(rows))
	for _, row := range rows {
		if antigravityRowIsGemini(row) == gemini {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// Nano unit-casing rule (W8) — deliberate, do not "fix" back to lowercase:
// nano labels show a single unit, and at 9-10px a lone "d" is easily confused
// with "h", so ONLY the day unit is uppercased here ("7D" vs "5h", "45m").
// Normal and compact show two units side by side ("4d 2h") where order makes
// them unambiguous, so they keep all-lowercase units (see resetStrings).
// Never uppercase minutes: "M" reads as months.
func selectNanoRows(rows []UsageRowState, weeklyOnly bool) []nanoUsageState {
	var session, weekly *UsageRowState
	for index := range rows {
		row := &rows[index]
		if strings.Contains(strings.ToLower(row.Label), "fable") {
			continue
		}
		switch {
		case row.WindowMinutes >= 6*24*60:
			if weekly == nil || row.WindowMinutes > weekly.WindowMinutes {
				weekly = row
			}
		case row.WindowMinutes > 0:
			if session == nil || row.WindowMinutes < session.WindowMinutes {
				session = row
			}
		}
	}
	if weeklyOnly {
		state := nanoUsageState{label: "7D"}
		if weekly != nil {
			state.row, state.available = *weekly, true
		}
		return []nanoUsageState{state}
	}
	states := []nanoUsageState{{label: "5h"}, {label: "7D"}}
	if session != nil {
		states[0].row, states[0].available = *session, true
	}
	if weekly != nil {
		states[1].row, states[1].available = *weekly, true
	}
	return states
}

func (v *View) updateCompactUsage(handles compactUsageView, lane LaneState, row UsageRowState, now time.Time) {
	visual := v.rowVisual(lane, row, now)
	setRectangleFill(handles.background, visual.background)
	setCanvasText(handles.label, visual.label, v.colors.Text)
	handles.meter.SetValue(visual.percent, visual.meterColor)
	handles.reset.SetValue(visual.resetPercent, visual.resetBarColor, v.colors.Track)
	setPercentWeight(handles.number, handles.symbol, visual.danger)
	setCompactPercent(handles.percent, handles.number, handles.symbol, visual.percent, visual.percentColor)
	setCanvasText(handles.resetUntil, visual.until, v.colors.Text)
	handles.resetRegion.SetValue(visual.resetAt)
}

func setCompactPercent(column *fyne.Container, number, symbol *canvas.Text, percent float64, textColor color.Color) {
	numberValue := fmt.Sprintf("%.0f", percent)
	numberSize := CompactPercentTextSize
	if numberValue == "100" {
		numberSize = CompactHundredTextSize
	}
	number.TextSize = numberSize
	setCanvasText(number, numberValue, textColor)
	symbol.TextSize = CompactSymbolTextSize
	setCanvasText(symbol, "%", textColor)
	column.Refresh()
}

func compactRowBackground(background, tint color.Color) color.Color {
	base := color.NRGBAModel.Convert(background).(color.NRGBA)
	accent := color.NRGBAModel.Convert(tint).(color.NRGBA)
	const tintRatio = 0.09
	mix := func(left, right uint8) uint8 {
		return uint8(float64(left)*(1-tintRatio) + float64(right)*tintRatio)
	}
	return color.NRGBA{R: mix(base.R, accent.R), G: mix(base.G, accent.G), B: mix(base.B, accent.B), A: 0xFF}
}

func (v *View) syncConnections() {
	if v.connectionsBody == nil {
		return
	}
	if len(v.connectionCache) == 0 {
		v.buildConnectionRows()
		return
	}
	for _, handles := range v.connectionCache {
		lane := v.connectionLane(handles.id)
		statusColor := v.colors.Secondary
		dotColor := v.colors.Disconnected
		if lane.Status == model.StatusConnected {
			statusColor = v.colors.Connected
			dotColor = v.colors.Connected
		}
		setCanvasText(handles.status, v.connectionStatusText(lane), statusColor)
		setCircleFill(handles.dot, dotColor)
		setCanvasText(handles.detail, v.connectionDetailText(lane), v.colors.Secondary)
		showInstall := v.connectionNeedsInstall(lane)
		if showInstall == handles.installVisible {
			continue
		}
		handles.installVisible = showInstall
		if showInstall {
			handles.install.Objects = []fyne.CanvasObject{handles.installRow}
		} else {
			handles.install.Objects = nil
		}
		handles.install.Refresh()
	}
}

func (v *View) buildConnectionRows() {
	descriptors := []struct {
		id   model.ProviderID
		name string
	}{{model.ProviderClaude, "Claude"}, {model.ProviderCodex, "Codex"}, {model.ProviderAntigravity, "Antigravity"}}
	objects := make([]fyne.CanvasObject, 0, len(descriptors))
	v.connectionCache = make([]*connectionView, 0, len(descriptors))
	for _, descriptor := range descriptors {
		id := descriptor.id
		lane := v.connectionLane(id)
		helpButton := NewOutlinedSmallIconButton(theme.HelpIcon(), v.text(i18n.KeyHelp), func() { v.showConnectionHelp(id) }, v.colors)
		testButton := NewOutlinedSmallButton(v.text(i18n.KeyTestConnection), v.text(i18n.KeyTestConnection), func() {
			if v.Actions.Inspect != nil {
				v.Actions.Inspect(id)
			}
		}, v.colors)
		reconnect := NewOutlinedSmallButton(v.text(i18n.KeyReconnect), v.text(i18n.KeyReconnect), func() {
			if v.Actions.Reconnect != nil {
				v.Actions.Reconnect(id)
			}
		}, v.colors)
		installButton := NewOutlinedSmallButton(v.text(i18n.KeyInstall), v.text(i18n.KeyInstall), func() {
			if v.Actions.OpenURL != nil {
				_ = v.Actions.OpenURL(connectionInstallURL(id))
			}
		}, v.colors)

		actionWidths := []float32{buttonWidthFor(testButton, 92), buttonWidthFor(reconnect, 74), 24}
		actions := []fyne.CanvasObject{connectionButton(testButton, 92), connectionButton(reconnect, 74), connectionButton(helpButton, 24)}
		actionRow := container.New(NewGapColumnLayout(actionWidths, []float32{6, 6}, SmallButtonHeight), actions...)

		statusColor := v.colors.Secondary
		dotColor := v.colors.Disconnected
		if lane.Status == model.StatusConnected {
			statusColor = v.colors.Connected
			dotColor = v.colors.Connected
		}
		status := textLabel(v.connectionStatusText(lane), SettingsTextSize, statusColor, false, false)
		dot := canvas.NewCircle(dotColor)
		dot.Resize(fyne.NewSize(7, 7))
		identity := container.NewHBox(
			textLabel(descriptor.name, SettingsTextSize, v.colors.Label, true, false),
			container.NewCenter(container.NewGridWrap(fyne.NewSize(7, 7), dot)),
			status,
		)
		header := container.NewBorder(nil, nil, identity, actionRow)
		detail := textLabel(v.connectionDetailText(lane), 9.5, v.colors.Secondary, false, false)
		install := container.NewStack()
		installRow := v.connectionInstallRow(id, installButton)
		installVisible := v.connectionNeedsInstall(lane)
		if installVisible {
			install.Objects = []fyne.CanvasObject{installRow}
		}

		accent := canvas.NewRectangle(v.colors.PaletteColor(v.config.ProviderColors[connectionColorKey(id)]))
		accent.CornerRadius = 2
		accent.SetMinSize(fyne.NewSize(3, 1))
		background := canvas.NewRectangle(v.colors.SettingsBackground)
		background.CornerRadius = 6
		background.StrokeColor = v.colors.CardBorder
		background.StrokeWidth = 1
		content := container.NewVBox(header, detail, install)
		card := container.NewStack(background, container.NewBorder(nil, nil, accent, nil, container.New(layout.NewCustomPaddedLayout(4, 4, 7, 7), content)))
		objects = append(objects, card)
		v.connectionCache = append(v.connectionCache, &connectionView{id: id, status: status, dot: dot, detail: detail, install: install, installRow: installRow, installButton: installButton, installVisible: installVisible, testButton: testButton, reconnect: reconnect, helpButton: helpButton, actionRow: actionRow})
	}
	v.connectionsBody.Objects = objects
	v.connectionsBody.Refresh()
}

// connectionButton sizes a button to its own label so translations never get
// clipped. The caller's width acts as a minimum, not a fixed size.
func connectionButton(button *SmallButton, width float32) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(buttonWidthFor(button, width), SmallButtonHeight), button)
}

// buttonWidthFor measures a button's label at its own font size and adds
// symmetric padding, clamped to at least the requested minimum.
func buttonWidthFor(button *SmallButton, minimum float32) float32 {
	if button == nil || button.Label == "" {
		return minimum
	}
	size := float32(13)
	if button.Outlined {
		size = SettingsTextSize
	}
	measured := fyne.MeasureText(button.Label, size, fyne.TextStyle{Bold: true}).Width
	return max(minimum, measured+2*ButtonLabelPadding)
}

func (v *View) connectionLane(id model.ProviderID) LaneState {
	for _, lane := range v.state.Lanes {
		if lane.Provider == id {
			return lane
		}
	}
	return LaneState{Provider: id, Status: model.StatusUnavailable}
}

func (v *View) connectionStatusText(lane LaneState) string {
	if lane.Status != model.StatusConnected && lane.ErrorKey != "" {
		return v.text(lane.ErrorKey)
	}
	switch lane.Status {
	case model.StatusConnected:
		return v.text(i18n.KeyConnected)
	case model.StatusLoggedOut:
		return v.text(i18n.KeyErrorNotLoggedIn)
	case model.StatusOutdated:
		return v.text(i18n.KeyErrorCLIOutdated)
	default:
		return v.text(i18n.KeyDisconnected)
	}
}

func (v *View) connectionDetailText(lane LaneState) string {
	lastRefresh := v.formatTimestamp(v.state.LastRefresh)
	details := make([]string, 0, 3)
	if lane.Provider == model.ProviderCodex {
		if lane.CLIPath != "" {
			details = append(details, fmt.Sprintf(v.text(i18n.KeyCLIPath), compactConnectionPath(lane.CLIPath)))
		}
		if lane.CLIVersion != "" {
			details = append(details, fmt.Sprintf(v.text(i18n.KeyCLIVersion), lane.CLIVersion))
		}
		if creditsDisplayEnabled && lane.Credits != nil && v.config.ShowCodexCredits {
			details = append(details, v.creditsText(lane.Credits))
		}
	}
	if lane.Provider == model.ProviderAntigravity {
		source := lane.Source
		if source == "" || source == "Local LSP" {
			source = v.text(i18n.KeyLocalLSP)
		}
		details = append(details, fmt.Sprintf(v.text(i18n.KeySource), source))
	}
	details = append(details, fmt.Sprintf(v.text(i18n.KeyLastRefresh), lastRefresh))
	return strings.Join(details, "  ·  ")
}

func compactConnectionPath(path string) string {
	const maximum = 50
	if len([]rune(path)) <= maximum {
		return path
	}
	runes := []rune(path)
	return string(runes[:18]) + "…" + string(runes[len(runes)-(maximum-19):])
}

func (v *View) connectionNeedsInstall(lane LaneState) bool {
	return (lane.Provider == model.ProviderClaude || lane.Provider == model.ProviderCodex) && lane.Error == model.ErrCLINotInstalled
}

func (v *View) connectionInstallRow(id model.ProviderID, button *SmallButton) fyne.CanvasObject {
	key := i18n.KeyInstallCodex
	if id == model.ProviderClaude {
		key = i18n.KeyInstallClaude
	}
	message := textLabel(v.text(key), 9.5, v.colors.Secondary, false, false)
	return container.NewBorder(nil, nil, message, connectionButton(button, 74))
}

func connectionInstallURL(id model.ProviderID) string {
	if id == model.ProviderClaude {
		return claudeInstallURL
	}
	return codexInstallURL
}

func connectionColorKey(id model.ProviderID) string {
	if id == model.ProviderAntigravity {
		return "antigravity"
	}
	return string(id)
}

func setCanvasText(text *canvas.Text, value string, textColor color.Color) {
	if text.Text == value && sameColor(text.Color, textColor) {
		return
	}
	text.Text = value
	text.Color = textColor
	text.Refresh()
}

func setRectangleFill(rectangle *canvas.Rectangle, fill color.Color) {
	if sameColor(rectangle.FillColor, fill) {
		return
	}
	rectangle.FillColor = fill
	rectangle.Refresh()
}

func setCircleFill(circle *canvas.Circle, fill color.Color) {
	if sameColor(circle.FillColor, fill) {
		return
	}
	circle.FillColor = fill
	circle.Refresh()
}
