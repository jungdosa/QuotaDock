package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/settings"
	"github.com/jungdosa/QuotaDock/internal/ui"
)

func TestCustomTitlebarWindowDisablesDefaultCanvasPadding(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	window := a.NewWindow("QuotaDock")
	defer window.Close()

	configureMainWindow(window)

	paddedWindow, ok := window.(interface{ Padded() bool })
	if !ok {
		t.Fatal("test window does not expose its padding state")
	}
	if paddedWindow.Padded() {
		t.Fatal("custom titlebar window retained Fyne's default outer padding")
	}
}

func TestFyneDoMigrationIsAppliedAtRuntimeWithoutDroppingMetadata(t *testing.T) {
	original := fyne.AppMetadata{
		ID:         "com.jungdosa.quotadock",
		Name:       "QuotaDock",
		Version:    "0.4.0",
		Migrations: map[string]bool{"existing": true},
	}
	got := withFyneDoMigration(original)
	if !got.Migrations["fyneDo"] || !got.Migrations["existing"] {
		t.Fatalf("runtime migrations=%v", got.Migrations)
	}
	if got.ID != original.ID || got.Name != original.Name || got.Version != original.Version {
		t.Fatalf("runtime metadata changed: %+v", got)
	}
	if _, exists := original.Migrations["fyneDo"]; exists {
		t.Fatal("runtime migration mutated the source metadata map")
	}
}

func TestFyneDoMigrationIsDeclared(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "FyneApp.toml"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := string(raw)
	if !strings.Contains(metadata, "[Migrations]") || !strings.Contains(metadata, "fyneDo = true") {
		t.Fatal("FyneApp.toml does not declare the fyne.Do threading migration")
	}
}

func TestPhase3IVersionMetadataMatchesRuntime(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "FyneApp.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if version != "0.7.11" || !strings.Contains(string(raw), `Version = "0.7.11"`) {
		t.Fatalf("runtime version=%q metadata=%q", version, raw)
	}
}

func TestDemoModeDoesNotSaveSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "QuotaDock", "settings.json")
	if got := loadSettings(path, false); got.WarningPercent != settings.Default().WarningPercent {
		t.Fatalf("default warning percent = %v", got.WarningPercent)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("demo load created settings file: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	originalConfig := settings.Default()
	originalConfig.Theme = settings.ThemeLight
	originalConfig.WarningPercent = 81
	originalConfig.DangerPercent = 93
	if err := settings.Save(path, originalConfig); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded := loadSettings(path, false)
	demo := ui.DemoConfig(loaded)
	if demo.WarningPercent != 75 || demo.Theme != settings.ThemeDark {
		t.Fatalf("demo fixture = %+v", demo)
	}
	if err := saveSettings(path, demo, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("demo save changed settings: %q", after)
	}
}
