package ui

// Compact reset column, header caption, and hover tooltip use the
// shared date/time formatter and the passive tooltip layer.

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestCompactResetColumnAlignsAndFollowsDateTimeFormat(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	state := sampleState()
	v.SetState(state)
	v.Show(CompactScreen)
	window.Resize(v.MinimumSize(CompactScreen))

	rows := compactTestRows(v)
	if len(rows) != len(v.compactCache.rows) || len(rows) == 0 {
		t.Fatalf("compact rows=%d cache=%d", len(rows), len(v.compactCache.rows))
	}
	now := time.Now()
	rowStates := make([]UsageRowState, 0, len(rows))
	for _, lane := range v.visibleLanes() {
		rowStates = append(rowStates, lane.Rows...)
	}
	var resetRight float32 = -1
	for index, handles := range v.compactCache.rows {
		content := compactTestRowContent(t, rows[index])
		if len(content.Objects) != 5 {
			t.Fatalf("row %d columns=%d, want icon/label/meter/percent/reset", index, len(content.Objects))
		}
		// The row rendered moments before this assertion, so allow the countdown
		// to match either the current minute or the previous one.
		until, resetAt := resetStrings(rowStates[index], now, v.config)
		untilEarlier, _ := resetStrings(rowStates[index], now.Add(-time.Minute), v.config)
		if handles.resetUntil.Text != until && handles.resetUntil.Text != untilEarlier {
			t.Fatalf("row %d reset text=%q, want %q or %q", index, handles.resetUntil.Text, until, untilEarlier)
		}
		if handles.resetUntil.Alignment != fyne.TextAlignTrailing || !handles.resetUntil.TextStyle.Monospace {
			t.Fatalf("row %d reset text alignment=%v mono=%t, want trailing mono", index, handles.resetUntil.Alignment, handles.resetUntil.TextStyle.Monospace)
		}
		if handles.resetRegion.tooltipValue() != resetAt {
			t.Fatalf("row %d reset tooltip=%q, want %q", index, handles.resetRegion.tooltipValue(), resetAt)
		}
		column := content.Objects[4]
		right := column.Position().X + column.Size().Width
		if resetRight < 0 {
			resetRight = right
		}
		if right != resetRight {
			t.Fatalf("row %d reset column right edge=%.1f, want %.1f on every row", index, right, resetRight)
		}
	}

	// The Reset caption sits over the reset column with matching geometry.
	header := v.compactCache.resetHeader
	first := compactTestRowContent(t, rows[0])
	if header.Position().X != first.Objects[4].Position().X || header.Size().Width != first.Objects[4].Size().Width {
		t.Fatalf("reset caption x/width=%.1f/%.1f, column=%.1f/%.1f", header.Position().X, header.Size().Width, first.Objects[4].Position().X, first.Objects[4].Size().Width)
	}

	// Switching the date/time format immediately reformats the hover value.
	before := v.compactCache.rows[0].resetRegion.tooltipValue()
	cfg := v.config
	cfg.DateTimeFormat = settings.Format24HourDateDay
	v.SetConfig(cfg)
	after := v.compactCache.rows[0].resetRegion.tooltipValue()
	if before == after || after == "" {
		t.Fatalf("reset tooltip did not follow date/time format: %q -> %q", before, after)
	}

	// Hovering shows the value in the passive layer; leaving clears it.
	region := v.compactCache.rows[0].resetRegion
	region.MouseIn(nil)
	v.showTooltip(region)
	if v.tooltipObject == nil || len(v.tooltipLayer.Objects) != 1 {
		t.Fatal("reset region hover did not show the passive tooltip")
	}
	region.MouseOut()
	if v.tooltipObject != nil || len(v.tooltipLayer.Objects) != 0 {
		t.Fatal("reset region leave did not dismiss the tooltip")
	}
}

// Nano rows carry a whole-row hover target that shows a two-line
// tooltip (remaining time + reset moment) in the passive layer.
func TestNanoRowTooltipTwoLinesInPassiveLayer(t *testing.T) {
	v, window := newTestView(t)
	defer window.Close()
	v.SetState(sampleState())
	v.Show(NanoScreen)
	window.Resize(v.MinimumSize(NanoScreen))

	now := time.Now()
	states := v.nanoCellStates()
	var probe *TooltipRegion
	for cellIndex, cell := range v.nanoCache.cells {
		for rowIndex, row := range cell.rows {
			state := states[cellIndex].rows[rowIndex]
			if row.region == nil {
				t.Fatalf("nano cell %d row %d has no hover region", cellIndex, rowIndex)
			}
			value := row.region.tooltipValue()
			if !state.available {
				if value != "" {
					t.Fatalf("nano cell %d row %d unavailable but has tooltip %q", cellIndex, rowIndex, value)
				}
				continue
			}
			until, resetAt := resetStrings(state.row, now, v.config)
			untilEarlier, _ := resetStrings(state.row, now.Add(-time.Minute), v.config)
			title := states[cellIndex].name + " " + state.label
			if value != title+"\n"+until+"\n"+resetAt && value != title+"\n"+untilEarlier+"\n"+resetAt {
				t.Fatalf("nano cell %d row %d tooltip=%q, want %q / %q / reset moment", cellIndex, rowIndex, value, title, until)
			}
			if probe == nil {
				probe = row.region
			}
		}
	}
	if probe == nil {
		t.Fatal("no available nano row to probe")
	}
	probe.MouseIn(nil)
	v.showTooltip(probe)
	if v.tooltipObject == nil || len(v.tooltipLayer.Objects) != 1 {
		t.Fatal("nano hover did not show the passive tooltip")
	}
	lines := 0
	walkCanvasObject(v.tooltipObject, func(object fyne.CanvasObject) {
		if _, ok := object.(*canvas.Text); ok {
			lines++
		}
		switch object.(type) {
		case fyne.Tappable, fyne.SecondaryTappable, fyne.Draggable:
			t.Fatalf("nano tooltip contains event-consuming object %T", object)
		}
	})
	// Title ("Claude 5h"), remaining time, reset moment.
	if lines != 3 {
		t.Fatalf("nano tooltip lines=%d, want 3", lines)
	}
	position := v.tooltipObject.Position()
	size := v.tooltipObject.Size()
	canvasSize := v.Canvas.Size()
	if position.X < TooltipMargin || position.Y < TooltipMargin ||
		position.X+size.Width > canvasSize.Width-TooltipMargin || position.Y+size.Height > canvasSize.Height {
		t.Fatalf("nano tooltip escaped window: pos=%v size=%v canvas=%v", position, size, canvasSize)
	}
	probe.MouseOut()
	if v.tooltipObject != nil {
		t.Fatal("nano hover leave did not dismiss the tooltip")
	}
}

// Every caption strip paints the titlebar tone edge to edge, so the
// header reads as one block with the titlebar in both themes. Nano has no
// caption strip, so normal and compact are the only screens to verify.
func TestHeaderStripsShareTitlebarTone(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	for _, mode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark} {
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(mode))
		cfg := v.config
		cfg.Theme = mode
		v.SetConfig(cfg)
		for _, screen := range []struct {
			name  string
			mode  Screen
			wrap  func() *fyne.Container
			width float32
		}{
			{"normal", NormalScreen, func() *fyne.Container { return v.normalHeaderWrap }, NormalWidth},
			{"compact", CompactScreen, func() *fyne.Container { return v.compactHeaderWrap }, CompactWidth},
		} {
			v.Show(screen.mode)
			window.Resize(v.MinimumSize(screen.mode))
			strip, ok := screen.wrap().Objects[0].(*fyne.Container)
			if !ok || len(strip.Objects) != 2 {
				t.Fatalf("%s %s header strip structure=%T", mode, screen.name, screen.wrap().Objects[0])
			}
			background, ok := strip.Objects[0].(*canvas.Rectangle)
			if !ok || !sameColor(background.FillColor, v.colors.TitleBottom) {
				t.Fatalf("%s %s header strip is not on the titlebar tone: %v", mode, screen.name, background)
			}
			if strip.Size().Width != screen.width {
				t.Fatalf("%s %s header strip width=%.1f, want full window %.1f", mode, screen.name, strip.Size().Width, screen.width)
			}
		}
	}
}
