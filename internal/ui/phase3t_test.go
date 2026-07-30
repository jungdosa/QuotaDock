package ui

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestPhase3TPlanChipPaddingAndNormalVerticalCenters(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()

	header, handles := v.makeLaneHeader(LaneState{Provider: model.ProviderClaude, Name: "Claude", Plan: "MAX 20X", Status: model.StatusConnected})
	header.Resize(header.MinSize())
	headerContainer := header.(*fyne.Container)
	if len(headerContainer.Objects) != 2 {
		t.Fatalf("lane header objects=%d, want name + plan chip with no connection dot", len(headerContainer.Objects))
	}
	chip := headerContainer.Objects[1]
	if handles.plan.TextSize != PlanChipTextSize || PlanChipTextSize != 9 {
		t.Fatalf("plan chip font=%.1f, want 9", handles.plan.TextSize)
	}
	if got, want := chip.MinSize().Width-handles.plan.MinSize().Width, 2*PlanChipPaddingX+LaneHeaderChipGap; got != want {
		t.Fatalf("plan chip horizontal padding=%.1f, want %.1f", got, want)
	}
	if got, want := chip.MinSize().Height-handles.plan.MinSize().Height, 2*PlanChipPaddingY; got != want {
		t.Fatalf("plan chip vertical padding=%.1f, want %.1f", got, want)
	}

	v.Show(NormalScreen)
	window.Resize(v.MinimumSize(NormalScreen))
	handlesRow := v.normalCache.rows[0]
	row := handlesRow.row
	if row == nil {
		t.Fatal("normal usage row was not found")
	}
	if handlesRow.meter.Height != NormalMeterHeight || NormalMeterHeight != 10 {
		t.Fatalf("normal meter height=%.1f, want 10", handlesRow.meter.Height)
	}
	rowCenter := row.Size().Height / 2
	meterCenter := objectCenterY(row, handlesRow.meter)
	percentCenter := objectCenterY(row, handlesRow.percent)
	resetTop := objectTopY(row, handlesRow.until)
	resetBottom := objectBottomY(row, handlesRow.resetAt[len(handlesRow.resetAt)-1])
	resetCenter := (resetTop + resetBottom) / 2
	// The reset block stays centred on the row. The meter now shares its column
	// with a percentage band above it, so those two objects straddle the centre.
	if math.Abs(float64(resetCenter-rowCenter)) > 0.01 {
		t.Fatalf("normal reset block center=%.2f, row center=%.2f", resetCenter, rowCenter)
	}
	if percentCenter >= meterCenter {
		t.Fatalf("percentage should sit above the meter: percent center=%.2f, meter center=%.2f", percentCenter, meterCenter)
	}
	t.Logf("normal centers row/meter/percent/reset=%.2f/%.2f/%.2f/%.2f", rowCenter, meterCenter, percentCenter, resetCenter)
}

func findContainerWithObject(root *fyne.Container, target fyne.CanvasObject) *fyne.Container {
	for _, object := range root.Objects {
		container, ok := object.(*fyne.Container)
		if !ok {
			continue
		}
		for _, child := range container.Objects {
			if child == target {
				return container
			}
		}
		if found := findContainerWithObject(container, target); found != nil {
			return found
		}
	}
	return nil
}

func objectTopY(root *fyne.Container, target fyne.CanvasObject) float32 {
	position, ok := objectPosition(root, target, fyne.NewPos(0, 0))
	if !ok {
		return float32(math.NaN())
	}
	return position.Y
}

func objectBottomY(root *fyne.Container, target fyne.CanvasObject) float32 {
	return objectTopY(root, target) + target.Size().Height
}

func objectCenterY(root *fyne.Container, target fyne.CanvasObject) float32 {
	return objectTopY(root, target) + target.Size().Height/2
}

func objectPosition(root *fyne.Container, target fyne.CanvasObject, offset fyne.Position) (fyne.Position, bool) {
	for _, object := range root.Objects {
		position := offset.Add(object.Position())
		if object == target {
			return position, true
		}
		if container, ok := object.(*fyne.Container); ok {
			if found, yes := objectPosition(container, target, position); yes {
				return found, true
			}
		}
	}
	return fyne.Position{}, false
}

func TestPhase3TSoftwareRenderCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3T_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3T_SCREENSHOT_DIR for Phase 3T captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	v, window := phase2DTestView(t)
	defer window.Close()
	v.SetState(DemoViewState())

	for _, themeMode := range []settings.Theme{settings.ThemeLight, settings.ThemeDark} {
		cfg := v.config
		cfg.Theme = themeMode
		cfg.Language = settings.LanguageKorean
		fyne.CurrentApp().Settings().SetTheme(NewBrandTheme(themeMode))
		v.SetConfig(cfg)
		for _, screen := range []struct {
			name string
			mode Screen
		}{{"normal", NormalScreen}, {"compact", CompactScreen}, {"nano", NanoScreen}} {
			v.Show(screen.mode)
			window.Resize(v.MinimumSize(screen.mode))
			output := window.Canvas().Capture()
			path := filepath.Join(directory, string(themeMode)+"-"+screen.name+".png")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(file, output); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
			if output.Bounds().Dx() == 0 || output.Bounds().Dy() == 0 {
				t.Fatalf("empty software capture %s", path)
			}
		}
	}
}

func TestPhase3TCompactPercentFontAndOffsetsRemainExact(t *testing.T) {
	v, window := phase2DTestView(t)
	defer window.Close()
	v.Show(CompactScreen)
	window.Resize(v.MinimumSize(CompactScreen))
	for index, row := range v.compactCache.rows {
		if row.number.TextSize != CompactPercentTextSize || row.symbol.TextSize != CompactSymbolTextSize || CompactPercentTextSize != 12 || CompactSymbolTextSize != 9 {
			t.Fatalf("compact row %d percent fonts=%.1f/%.1f, want 12/9", index, row.number.TextSize, row.symbol.TextSize)
		}
		percentLayout := row.percent.Layout.(*CompactPercentLayout)
		if percentLayout.OffsetY != CompactPercentOffset || CompactPercentOffset != -2 {
			t.Fatalf("compact row %d percent offset=%.1f, want preserved -2", index, percentLayout.OffsetY)
		}
	}
}
