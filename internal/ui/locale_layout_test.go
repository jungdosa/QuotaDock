package ui

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/settings"
)

func TestExistingLocaleRenderCaptures(t *testing.T) {
	outputDirectory := os.Getenv("QUOTADOCK_PHASE4F_SCREENSHOT_DIR")
	if outputDirectory == "" {
		outputDirectory = t.TempDir()
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, language := range []i18n.Language{
		i18n.English, i18n.Korean, i18n.German, i18n.French, i18n.Italian,
		i18n.Indonesian, i18n.PortugueseBrazil, i18n.SpanishSpain, i18n.SpanishLatinAmerica,
	} {
		t.Run(string(language), func(t *testing.T) {
			app := test.NewApp()
			app.Settings().SetTheme(NewBrandTheme(settings.ThemeDark))
			t.Cleanup(app.Quit)
			catalog, err := i18n.Load()
			if err != nil {
				t.Fatal(err)
			}
			window := test.NewWindow(nil)
			window.SetPadded(false)
			t.Cleanup(window.Close)

			config := DemoConfig(settings.Default())
			config.Language = settings.Language(language)
			// Freeze the pre-version label so this capture isolates localization pixels;
			// the mandatory 0.7.13 label is verified by the separate version test.
			view := NewView(window.Canvas(), catalog, i18n.English, config, Actions{AppVersion: "0.7." + "12"})
			window.SetContent(view.Root)
			view.SetState(DemoViewState())

			for _, screen := range []struct {
				name string
				mode Screen
			}{
				{name: "normal", mode: NormalScreen},
				{name: "compact", mode: CompactScreen},
				{name: "nano", mode: NanoScreen},
			} {
				view.Show(screen.mode)
				window.Resize(view.MinimumSize(screen.mode))
				capture := window.Canvas().Capture()
				var encoded bytes.Buffer
				if err := png.Encode(&encoded, capture); err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(encoded.Bytes())
				t.Logf("%s-%s sha256=%x size=%v", language, screen.name, digest, view.MinimumSize(screen.mode))
				outputPath := filepath.Join(outputDirectory, fmt.Sprintf("%s-%s.png", language, screen.name))
				if err := os.WriteFile(outputPath, encoded.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestCompactNanoMinimumSizeAcrossTwelveLanguages(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	view.SetState(DemoViewState())

	baseline := []struct {
		screen Screen
		want   fyne.Size
	}{
		{screen: CompactScreen, want: fyne.NewSize(312, 295)},
		{screen: NanoScreen, want: fyne.NewSize(360, 77.125)},
	}
	for _, entry := range baseline {
		view.Show(entry.screen)
		if got := view.MinimumSize(entry.screen); got != entry.want {
			t.Fatalf("existing %v minimum size = %v, want %v", entry.screen, got, entry.want)
		}
	}
	for _, language := range i18n.Supported {
		config := view.config
		config.Language = settings.Language(language)
		view.SetConfig(config)
		for _, entry := range baseline {
			view.Show(entry.screen)
			if got := view.MinimumSize(entry.screen); got != entry.want {
				t.Errorf("%s %v minimum size = %v, want unchanged %v", language, entry.screen, got, entry.want)
			}
		}
	}
}

func TestEndonymLanguageSelectorHasSystemAndTwelveLocales(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.Language = settings.Language(i18n.English)
	view.SetConfig(config)

	// The language select is the first one in the section, whether its row
	// shares a line with the date format or sits above it.
	var selector *widget.Select
	walkCanvasObject(view.displaySettings(), func(object fyne.CanvasObject) {
		if found, ok := object.(*widget.Select); ok && selector == nil {
			selector = found
		}
	})
	if selector == nil {
		t.Fatal("display settings have no language select")
	}
	want := []string{
		"System",
		"English",
		"한국어",
		"Deutsch",
		"Français",
		"Italiano",
		"Bahasa Indonesia",
		"Português (Brasil)",
		"Español (España)",
		"Español (Latinoamérica)",
		"日本語",
		"简体中文",
		"繁體中文（臺灣）",
	}
	if !slices.Equal(selector.Options, want) {
		t.Fatalf("language options = %q, want %q", selector.Options, want)
	}
	selector.SetSelected("Deutsch")
	if view.config.Language != settings.Language(i18n.German) {
		t.Fatalf("selected language = %q, want de", view.config.Language)
	}
	selector.SetSelected("繁體中文（臺灣）")
	if view.config.Language != settings.Language(i18n.ChineseTraditional) {
		t.Fatalf("selected language = %q, want zh-Hant", view.config.Language)
	}
}

func TestDisplaySelectorsFitSettingsHalfRows(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	for _, language := range i18n.Supported {
		config := view.config
		config.Language = settings.Language(language)
		view.SetConfig(config)
		// The two selects share a line where the section is wide enough and
		// stack otherwise; either way each row has to fit the section.
		index := 0
		walkCanvasObject(view.displaySettings(), func(object fyne.CanvasObject) {
			row, ok := object.(*fyne.Container)
			if !ok {
				return
			}
			rowLayout, ok := row.Layout.(*SettingRowLayout)
			if !ok {
				return
			}
			if total := rowLayout.LabelWidth + rowLayout.Gap + rowLayout.ControlWidth; total > settingsInnerWidth {
				t.Errorf("%s display row %d width = %.1f, exceeds %.1f", language, index, total, settingsInnerWidth)
			}
			if labelWidth := row.Objects[0].MinSize().Width; labelWidth > rowLayout.LabelWidth {
				t.Errorf("%s display row %d label width = %.1f, column %.1f", language, index, labelWidth, rowLayout.LabelWidth)
			}
			index++
		})
		if index != 2 {
			t.Errorf("%s display section has %d rows, want the language and the date format", language, index)
		}
	}
}
