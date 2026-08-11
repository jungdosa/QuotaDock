// Package settings owns the versioned, non-secret QuotaDock configuration.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/security"
)

const (
	CurrentSchemaVersion       = 4
	MaxFileSize          int64 = 256 << 10
)

// Language remains a distinct public string type so settings.json keeps its
// stable wire representation. Supported locale constants live in i18n.
type Language string

// LanguageSystem is a settings-only choice rather than a concrete locale.
const LanguageSystem Language = "system"

type DateTimeFormat string

const (
	Format12HourDate    DateTimeFormat = "12h-date"
	Format12HourDateDay DateTimeFormat = "12h-date-day"
	Format24HourDate    DateTimeFormat = "24h-date"
	Format24HourDateDay DateTimeFormat = "24h-date-day"
)

type Theme string

const (
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
	ThemeSystem Theme = "system"
)

type UsageMode string

const (
	UsageUsed      UsageMode = "used"
	UsageRemaining UsageMode = "remaining"
)

type DisplayMode string

const (
	ModeNormal  DisplayMode = "normal"
	ModeCompact DisplayMode = "compact"
	ModeNano    DisplayMode = "nano"
)

func NextDisplayMode(mode DisplayMode) DisplayMode {
	switch mode {
	case ModeNormal:
		return ModeCompact
	case ModeCompact:
		return ModeNano
	default:
		return ModeNormal
	}
}

type Config struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Language         Language          `json:"language"`
	DateTimeFormat   DateTimeFormat    `json:"dateTimeFormat"`
	Theme            Theme             `json:"theme"`
	UsageMode        UsageMode         `json:"usageMode"`
	RefreshSeconds   int               `json:"refreshSeconds"`
	WarningsEnabled  bool              `json:"warningsEnabled"`
	WarningPercent   float64           `json:"warningPercent"`
	DangerPercent    float64           `json:"dangerPercent"`
	WarningColor     string            `json:"warningColor"`
	DangerColor      string            `json:"dangerColor"`
	ProviderColors   map[string]string `json:"providerColors"`
	ShowClaude       bool              `json:"showClaude"`
	ShowCodex        bool              `json:"showCodex"`
	ShowAGGemini     bool              `json:"showAGGemini"`
	ShowAGClaude     bool              `json:"showAGClaude"`
	// ShowGrok defaults to off: the lane would only report "sign in" noise
	// for users without the Grok CLI, and existing screens stay unchanged.
	ShowGrok bool `json:"showGrok"`
	// Per-provider paid-credit visibility. Claude has no credit feed yet, but
	// the toggle is wired so the surface lights up the moment data exists.
	ShowClaudeCredits bool `json:"showClaudeCredits"`
	ShowCodexCredits  bool `json:"showCodexCredits"`
	AutoStart bool `json:"autoStart"`
	// StartMinimized controls whether a launch at Windows startup goes straight
	// to the tray. It defaults to false: a widget the user asked to start with
	// Windows should be on screen, and the old always-hidden behaviour left
	// people wondering whether the app had started at all.
	StartMinimized   bool              `json:"startMinimized"`
	AlwaysOnTop      bool              `json:"alwaysOnTop"`
	ShowInTaskbar    bool              `json:"showInTaskbar"`
	PromoteTrayIcon  bool              `json:"promoteTrayIcon"`
	DisplayMode      DisplayMode       `json:"displayMode"`
	WindowX          int               `json:"windowX,omitempty"`
	WindowY          int               `json:"windowY,omitempty"`
	WindowPositioned bool              `json:"windowPositioned,omitempty"`
}

// Default provider hues follow the official brand-logo icons so the icon and
// its resting meter read as one colour: Claude orange, Codex gray, AG Gemini
// violet, AG Claude slate. (An earlier draft banned warm provider hues;
// that reservation was withdrawn when defaults were matched to the logos.)
func Default() Config {
	return Config{SchemaVersion: CurrentSchemaVersion, Language: LanguageSystem, DateTimeFormat: Format12HourDate, Theme: ThemeLight, UsageMode: UsageUsed, RefreshSeconds: 300, WarningsEnabled: true, WarningPercent: 80, DangerPercent: 90, WarningColor: "amber", DangerColor: "red", ProviderColors: map[string]string{"claude": "orange", "codex": "gray", "antigravity": "slate", "antigravity-gemini": "violet", "grok": "sky"}, ShowClaude: true, ShowCodex: true, ShowAGGemini: true, ShowAGClaude: true, ShowGrok: false, ShowClaudeCredits: true, ShowCodexCredits: true, ShowInTaskbar: true, PromoteTrayIcon: true, DisplayMode: ModeNormal}
}

func (c Config) Validated() Config {
	defaults := Default()
	c.SchemaVersion = CurrentSchemaVersion
	if c.Language != LanguageSystem && !i18n.IsSupported(i18n.Language(c.Language)) {
		c.Language = defaults.Language
	}
	switch c.DateTimeFormat {
	case Format12HourDate, Format12HourDateDay, Format24HourDate, Format24HourDateDay:
	default:
		c.DateTimeFormat = defaults.DateTimeFormat
	}
	switch c.Theme {
	case ThemeLight, ThemeDark, ThemeSystem:
	default:
		c.Theme = defaults.Theme
	}
	switch c.UsageMode {
	case UsageUsed, UsageRemaining:
	default:
		c.UsageMode = defaults.UsageMode
	}
	switch c.DisplayMode {
	case ModeNormal, ModeCompact, ModeNano:
	default:
		c.DisplayMode = defaults.DisplayMode
	}
	c.RefreshSeconds = security.ClampInt(c.RefreshSeconds, 15, 86400)
	c.WarningPercent = security.ClampFloat(c.WarningPercent, 1, 99)
	c.DangerPercent = security.ClampFloat(c.DangerPercent, 1, 99)
	if c.DangerPercent <= c.WarningPercent {
		if c.WarningPercent >= 99 {
			c.WarningPercent = 98
			c.DangerPercent = 99
		} else {
			c.DangerPercent = c.WarningPercent + 1
		}
	}
	if !security.IsPaletteID(c.WarningColor) {
		c.WarningColor = defaults.WarningColor
	}
	if !security.IsPaletteID(c.DangerColor) {
		c.DangerColor = defaults.DangerColor
	}
	output := make(map[string]string, len(defaults.ProviderColors))
	for provider, fallback := range defaults.ProviderColors {
		value := c.ProviderColors[provider]
		if !security.IsPaletteID(value) {
			value = fallback
		}
		output[provider] = value
	}
	c.ProviderColors = output
	return c
}

func Decode(reader io.Reader) (Config, error) {
	var data json.RawMessage
	if err := security.DecodeJSONLimited(reader, MaxFileSize, &data); err != nil {
		return Default(), err
	}
	config := Default()
	if err := json.Unmarshal(data, &config); err != nil {
		return Default(), err
	}
	var taskbar struct {
		HideTaskbar   *bool        `json:"hideTaskbar"`
		ShowInTaskbar *bool        `json:"showInTaskbar"`
		DisplayMode   *DisplayMode `json:"displayMode"`
		Compact       *bool        `json:"compact"`
	}
	if err := json.Unmarshal(data, &taskbar); err != nil {
		return Default(), err
	}
	switch {
	case taskbar.ShowInTaskbar != nil:
		config.ShowInTaskbar = *taskbar.ShowInTaskbar
	case taskbar.HideTaskbar != nil:
		config.ShowInTaskbar = !*taskbar.HideTaskbar
	default:
		config.ShowInTaskbar = Default().ShowInTaskbar
	}
	switch {
	case taskbar.DisplayMode != nil:
		config.DisplayMode = *taskbar.DisplayMode
	case taskbar.Compact != nil && *taskbar.Compact:
		config.DisplayMode = ModeCompact
	default:
		config.DisplayMode = ModeNormal
	}
	return config.Validated(), nil
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("open settings: %w", err)
	}
	defer file.Close()
	return Decode(file)
}

func Save(path string, config Config) error {
	data, err := json.MarshalIndent(config.Validated(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if int64(len(data)) > MaxFileSize {
		return errors.New("encoded settings exceed size limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".quotadock-settings-*")
	if err != nil {
		return fmt.Errorf("create temporary settings: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("protect temporary settings: %w", err)
	}
	if _, err := io.Copy(temp, bytes.NewReader(data)); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary settings: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary settings: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary settings: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}
	return nil
}
