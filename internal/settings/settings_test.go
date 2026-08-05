package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/i18n"
)

func TestSettingsSaveRestoreAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	config := Default()
	config.Language = Language(i18n.Korean)
	config.DateTimeFormat = Format24HourDateDay
	config.WarningPercent = -10
	config.DangerPercent = 200
	config.WarningColor = "#ff0000"
	config.ProviderColors["claude"] = "url(evil)"
	if err := Save(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Language != Language(i18n.Korean) || loaded.DateTimeFormat != Format24HourDateDay {
		t.Fatalf("stable settings not restored: %+v", loaded)
	}
	if loaded.WarningPercent != 1 || loaded.DangerPercent != 99 {
		t.Fatalf("range not clamped: %+v", loaded)
	}
	if loaded.WarningColor != "amber" || loaded.ProviderColors["claude"] != "orange" || loaded.ProviderColors["antigravity-gemini"] != "violet" {
		t.Fatalf("palette not validated: %+v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"token", "cookie", "csrf", "email"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Errorf("settings contain forbidden term %q", forbidden)
		}
	}
}

func TestWarningAndDangerThresholdsClampToOneThroughNinetyNine(t *testing.T) {
	tests := []struct {
		name        string
		warning     float64
		danger      float64
		wantWarning float64
		wantDanger  float64
	}{
		{name: "zero and one hundred", warning: 0, danger: 100, wantWarning: 1, wantDanger: 99},
		{name: "below and above", warning: -20, danger: 120, wantWarning: 1, wantDanger: 99},
		{name: "danger follows warning", warning: 80, danger: 20, wantWarning: 80, wantDanger: 81},
		{name: "warning leaves room below maximum", warning: 99, danger: 99, wantWarning: 98, wantDanger: 99},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Default()
			config.WarningPercent = test.warning
			config.DangerPercent = test.danger
			got := config.Validated()
			if got.WarningPercent != test.wantWarning || got.DangerPercent != test.wantDanger {
				t.Fatalf("thresholds = %.0f/%.0f, want %.0f/%.0f", got.WarningPercent, got.DangerPercent, test.wantWarning, test.wantDanger)
			}
		})
	}
}

func TestSettingsUnknownFieldsAndTypeErrors(t *testing.T) {
	config, err := Decode(strings.NewReader(`{"schemaVersion":99,"language":"en","unknown":{"x":1}}`))
	if err != nil || config.Language != Language(i18n.English) || config.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("unknown field handling = %+v, %v", config, err)
	}
	if _, err := Decode(strings.NewReader(`{"refreshSeconds":"often"}`)); err == nil {
		t.Fatal("expected type validation error")
	}
	oversize := strings.NewReader(`{"unknown":"` + strings.Repeat("x", int(MaxFileSize)) + `"}`)
	if _, err := Decode(oversize); err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestDecodeMissingFieldsPreservesDefaults(t *testing.T) {
	t.Run("theme only", func(t *testing.T) {
		config, err := Decode(strings.NewReader(`{"theme":"light"}`))
		if err != nil {
			t.Fatal(err)
		}
		if config.Theme != ThemeLight || config.WarningPercent != 80 || config.DangerPercent != 90 {
			t.Fatalf("theme=%q thresholds=%.0f/%.0f, want light 80/90", config.Theme, config.WarningPercent, config.DangerPercent)
		}
	})

	t.Run("empty object", func(t *testing.T) {
		config, err := Decode(strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(config, Default()) {
			t.Fatalf("empty object decoded to %#v, want defaults %#v", config, Default())
		}
	})
}
func TestPhaseTwoSettingsValidation(t *testing.T) {
	config := Default()
	config.Theme = Theme("neon")
	config.UsageMode = UsageMode("raw")
	config.ShowClaude = false
	config.AlwaysOnTop = true
	got := config.Validated()
	if got.Theme != ThemeLight || got.UsageMode != UsageUsed {
		t.Fatalf("invalid display values survived: %+v", got)
	}
	if got.ShowClaude || !got.AlwaysOnTop {
		t.Fatal("existing booleans were not preserved")
	}
}

func TestTaskbarSettingMigrationInvertsLegacyValue(t *testing.T) {
	tests := []struct {
		name string
		hide bool
		show bool
	}{
		{name: "legacy hidden", hide: true, show: false},
		{name: "legacy visible", hide: false, show: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := Decode(strings.NewReader(`{"schemaVersion":2,"hideTaskbar":` + fmt.Sprint(test.hide) + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if config.SchemaVersion != CurrentSchemaVersion || config.ShowInTaskbar != test.show {
				t.Fatalf("migrated taskbar setting = version %d, show %v", config.SchemaVersion, config.ShowInTaskbar)
			}
		})
	}
}

func TestTaskbarSettingUsesNewKeyAndVisibleDefault(t *testing.T) {
	if !Default().ShowInTaskbar {
		t.Fatal("taskbar must be visible by default")
	}
	config, err := Decode(strings.NewReader(`{"schemaVersion":3,"showInTaskbar":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.ShowInTaskbar {
		t.Fatal("new showInTaskbar value was not preserved")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Save(path, config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"showInTaskbar": false`) || strings.Contains(string(data), "hideTaskbar") {
		t.Fatalf("saved taskbar keys = %s", data)
	}
}

func TestDisplayModeCyclesAndMigratesLegacyCompactSetting(t *testing.T) {
	mode := ModeNormal
	for _, want := range []DisplayMode{ModeCompact, ModeNano, ModeNormal} {
		mode = NextDisplayMode(mode)
		if mode != want {
			t.Fatalf("cycled display mode=%q, want %q", mode, want)
		}
	}

	legacy, err := Decode(strings.NewReader(`{"schemaVersion":3,"compact":true}`))
	if err != nil || legacy.DisplayMode != ModeCompact {
		t.Fatalf("legacy compact migration mode=%q err=%v", legacy.DisplayMode, err)
	}
	modern, err := Decode(strings.NewReader(`{"schemaVersion":4,"displayMode":"nano","compact":false}`))
	if err != nil || modern.DisplayMode != ModeNano {
		t.Fatalf("modern display mode=%q err=%v", modern.DisplayMode, err)
	}
	invalid := Default()
	invalid.DisplayMode = DisplayMode("giant")
	if got := invalid.Validated().DisplayMode; got != ModeNormal {
		t.Fatalf("invalid display mode=%q, want normal", got)
	}

	path := filepath.Join(t.TempDir(), "settings.json")
	modern.DisplayMode = ModeNano
	if err := Save(path, modern); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"displayMode": "nano"`) || strings.Contains(string(data), `"compact"`) {
		t.Fatalf("saved display mode keys=%s", data)
	}
}

func TestTrayPromotionDefaultsTrueAndMissingKeyStaysTrue(t *testing.T) {
	if !Default().PromoteTrayIcon {
		t.Fatal("tray icon promotion must be enabled by default")
	}
	config, err := Decode(strings.NewReader("{\"schemaVersion\":4,\"alwaysOnTop\":true}"))
	if err != nil {
		t.Fatal(err)
	}
	if !config.PromoteTrayIcon {
		t.Fatal("settings without promoteTrayIcon did not use the enabled default")
	}
}

func TestTrayPromotionStoredFalseOverridesDefault(t *testing.T) {
	config, err := Decode(strings.NewReader("{\"schemaVersion\":4,\"promoteTrayIcon\":false}"))
	if err != nil {
		t.Fatal(err)
	}
	if config.PromoteTrayIcon {
		t.Fatal("stored false promoteTrayIcon setting was overwritten by the default")
	}
}

func TestSettingsLanguageSchemaPreservesLegacyStoredBytes(t *testing.T) {
	storedValues := []string{"system", "en", "ko", "de", "fr", "it", "id", "pt-BR", "es-ES", "es-419"}
	for _, stored := range storedValues {
		config, err := Decode(strings.NewReader(fmt.Sprintf(`{"schemaVersion":4,"language":%q}`, stored)))
		if err != nil {
			t.Fatalf("Decode(%s): %v", stored, err)
		}
		if got := string(config.Language); got != stored {
			t.Errorf("Decode(%s) language bytes = %q", stored, got)
		}
		encoded, err := json.Marshal(config.Language)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", stored, err)
		}
		if got, want := string(encoded), fmt.Sprintf("%q", stored); got != want {
			t.Errorf("Marshal(%s) = %s, want byte-stable %s", stored, got, want)
		}
	}
}

func TestSettingsLanguageSchemaAcceptsPhase4KCJKValues(t *testing.T) {
	for _, language := range []i18n.Language{i18n.Japanese, i18n.ChineseSimplified, i18n.ChineseTraditional} {
		config, err := Decode(strings.NewReader(fmt.Sprintf(`{"schemaVersion":4,"language":%q}`, language)))
		if err != nil {
			t.Fatalf("Decode(%s): %v", language, err)
		}
		if got := i18n.Language(config.Language); got != language {
			t.Errorf("Decode(%s) language = %s", language, got)
		}
	}
}
