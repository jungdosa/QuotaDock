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

func TestPhase4FKoEnRenderCaptures(t *testing.T) {
	outputDirectory := os.Getenv("QUOTADOCK_PHASE4F_SCREENSHOT_DIR")
	if outputDirectory == "" {
		outputDirectory = t.TempDir()
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, language := range []settings.Language{settings.LanguageKorean, settings.LanguageEnglish} {
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
			config.Language = language
			// Freeze the pre-version label so this capture isolates Phase 4F i18n pixels;
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

func TestPhase4FCompactNanoMinimumSizeAcrossNineLanguages(t *testing.T) {
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

func TestPhase4FEndonymLanguageSelectorHasSystemAndNineLocales(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	config := view.config
	config.Language = settings.LanguageEnglish
	view.SetConfig(config)

	display := view.displaySettings().(*fyne.Container)
	pair := display.Objects[0].(*fyne.Container)
	languageRow := pair.Objects[0].(*fyne.Container)
	selector := languageRow.Objects[1].(*widget.Select)
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
	}
	if !slices.Equal(selector.Options, want) {
		t.Fatalf("language options = %q, want %q", selector.Options, want)
	}
	selector.SetSelected("Deutsch")
	if view.config.Language != settings.LanguageGerman {
		t.Fatalf("selected language = %q, want de", view.config.Language)
	}
}

func TestPhase4FDisplaySelectorsFitSettingsHalfRows(t *testing.T) {
	view, window := newTestView(t)
	defer window.Close()
	for _, language := range i18n.Supported {
		config := view.config
		config.Language = settings.Language(language)
		view.SetConfig(config)
		display := view.displaySettings().(*fyne.Container)
		pair := display.Objects[0].(*fyne.Container)
		for index, object := range pair.Objects {
			row := object.(*fyne.Container)
			rowLayout := row.Layout.(*SettingRowLayout)
			if total := rowLayout.LabelWidth + rowLayout.Gap + rowLayout.ControlWidth; total > halfSettingRowWidth {
				t.Errorf("%s display row %d width = %.1f, exceeds %.1f", language, index, total, halfSettingRowWidth)
			}
			if labelWidth := row.Objects[0].MinSize().Width; labelWidth > rowLayout.LabelWidth {
				t.Errorf("%s display row %d label width = %.1f, column %.1f", language, index, labelWidth, rowLayout.LabelWidth)
			}
		}
	}
}
