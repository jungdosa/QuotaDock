package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

func TestPhase3QProviderIconSourcesTintsAndNativeResources(t *testing.T) {
	tests := []struct {
		kind        ProviderIconKind
		asset       string
		tint        string
		sourceColor string
	}{
		{ProviderIconClaude, "claude.svg", "#D97757", ""},
		{ProviderIconCodex, "openai.svg", "#1F2328", ""},
		{ProviderIconGemini, "gemini.svg", "#8E75FF", ""},
		{ProviderIconAGClaude, "claude.svg", "#6B7280", ""},
	}
	for _, test := range tests {
		resource := providerIconResource(test.kind)
		content := string(resource.Content())
		if providerIconAsset(test.kind) != test.asset || !strings.HasSuffix(resource.Name(), ".svg") {
			t.Fatalf("%s source asset=%q resource=%q, want source %q", test.kind, providerIconAsset(test.kind), resource.Name(), test.asset)
		}
		if test.tint != "" {
			if strings.Contains(content, "currentColor") || !strings.Contains(content, test.tint) {
				t.Fatalf("%s monochrome tint was not applied: %q", test.kind, test.tint)
			}
		} else if !strings.Contains(content, test.sourceColor) {
			t.Fatalf("%s source color %q was not preserved", test.kind, test.sourceColor)
		}
		if strings.Contains(content, `height="1em"`) || strings.Contains(content, `width="1em"`) ||
			!strings.Contains(content, `height="24"`) || !strings.Contains(content, `width="24"`) {
			t.Fatalf("%s SVG dimensions were not normalized to 24", test.kind)
		}
		icon := newNativeProviderIcon(test.kind)
		if icon.Resource != resource || icon.Image != nil || icon.FillMode != canvas.ImageFillContain || icon.MinSize() != fyne.NewSquareSize(providerIconSize) {
			t.Fatalf("%s does not use the Fyne native SVG resource path", test.kind)
		}
	}
}

func TestPhase3QCompactPercentColumnAndSpacing(t *testing.T) {
	v, w := phase2DTestView(t)
	defer w.Close()
	state := DemoViewState()
	state.Lanes[0].Rows[0].Percent = 100
	v.SetState(state)
	w.Resize(v.MinimumSize(CompactScreen))

	rows := compactTestRows(v)
	first := compactTestRowContent(t, rows[0])
	second := compactTestRowContent(t, rows[1])
	meter := first.Objects[2]
	percentColumn, ok := first.Objects[3].(*fyne.Container)
	if !ok {
		t.Fatalf("percent column=%T, want *fyne.Container", first.Objects[3])
	}
	if len(percentColumn.Objects) != 2 {
		t.Fatalf("percent parts=%d, want number + symbol", len(percentColumn.Objects))
	}
	number := percentColumn.Objects[0].(*canvas.Text)
	symbol := percentColumn.Objects[1].(*canvas.Text)

	wantMeterX := CompactIconWidth + CompactColumnGap + v.compactCache.labelWidth + CompactColumnGap
	if got := meter.Position().X; got != wantMeterX {
		t.Fatalf("meter x=%.1f, want %.1f from dynamic label width %.1f", got, wantMeterX, v.compactCache.labelWidth)
	}
	if gap := percentColumn.Position().X - (meter.Position().X + meter.Size().Width); gap != CompactColumnGap {
		t.Fatalf("100%% meter-to-percent gap=%.1f, want %.1f", gap, CompactColumnGap)
	}
	percentWidth := compactPercentColumnWidth()
	if percentColumn.Size().Width != percentWidth || second.Objects[3].Size().Width != percentWidth || second.Objects[3].Position().X != percentColumn.Position().X {
		t.Fatalf("percent columns are not fixed at measured width %.1fpx", percentWidth)
	}
	if number.Text != "100" || symbol.Text != "%" || number.Alignment != fyne.TextAlignLeading || symbol.Alignment != fyne.TextAlignLeading {
		t.Fatalf("percent parts=%q/%q alignments=%v/%v", number.Text, symbol.Text, number.Alignment, symbol.Alignment)
	}
	if number.TextSize != CompactHundredTextSize || symbol.TextSize != CompactSymbolTextSize {
		t.Fatalf("compact 100%% fonts=%.1f/%.1f, want %.1f/%.1f", number.TextSize, symbol.TextSize, CompactHundredTextSize, CompactSymbolTextSize)
	}
	if width := number.MinSize().Width + symbol.MinSize().Width; width+CompactPercentMargin > percentWidth {
		t.Fatalf("100%% width + margin %.1f + %.1f exceeds fixed column %.1f", width, CompactPercentMargin, percentWidth)
	}
}

func TestPhase3QVisualReviewCaptures(t *testing.T) {
	directory := os.Getenv("QUOTADOCK_PHASE3Q_SCREENSHOT_DIR")
	if directory == "" {
		t.Skip("set QUOTADOCK_PHASE3Q_SCREENSHOT_DIR for Phase 3Q captures")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	v, w := phase2DTestView(t)
	defer w.Close()
	cfg := DemoConfig(v.config)
	v.SetConfig(cfg)
	state := DemoViewState()
	state.Lanes[0].Rows[0].Percent = 100
	v.SetState(state)

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
		size   fyne.Size
	}{
		{"compact-mode", CompactScreen, v.MinimumSize(CompactScreen)},
		{"nano-mode", NanoScreen, v.MinimumSize(NanoScreen)},
	} {
		v.Show(entry.screen)
		w.Resize(entry.size)
		save(entry.name)
	}
}
