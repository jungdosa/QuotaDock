// Package i18n loads stable translation keys from embedded resources.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

type Language string

const (
	System  Language = "system"
	Korean  Language = "ko"
	English Language = "en"
)
const (
	KeyTrayShow                  = "tray.show"
	KeyTraySettings              = "tray.settings"
	KeyTrayNormal                = "tray.normal"
	KeyTrayCompact               = "tray.compact"
	KeyTrayNano                  = "tray.nano"
	KeyTrayQuit                  = "tray.quit"
	KeyUntilReset                = "usage.until_reset"
	KeyResetsAt                  = "usage.resets_at"
	KeyCredits                   = "usage.credits"
	KeyCreditsUnlimited          = "usage.credits_unlimited"
	KeyCreditsToggle             = "settings.credits"
	KeyErrorCLINotInstalled      = "error.cli_not_installed"
	KeyErrorNotLoggedIn          = "error.not_logged_in"
	KeyErrorCLIOutdated          = "error.cli_outdated"
	KeyErrorInitializationFailed = "error.initialization_failed"
	KeyErrorTimeout              = "error.timeout"
	KeyErrorInvalidResponse      = "error.invalid_response"
	KeyErrorUsageUnavailable     = "error.usage_unavailable"
	KeyErrorQuotaExhausted       = "error.quota_exhausted"
	KeyNotificationWarning       = "notification.warning"
	KeyDate12                    = "datetime.12h_date"
	KeyDate12Day                 = "datetime.12h_date_day"
	KeyDate24                    = "datetime.24h_date"
	KeyDate24Day                 = "datetime.24h_date_day"
	KeyAppTitle                  = "app.title"
	KeySettingsTitle             = "settings.title"
	KeyGroupUsage                = "settings.group.usage"
	KeyGroupBehavior             = "settings.group.behavior"
	KeyGroupDisplay              = "settings.group.display"
	KeyGroupConnections          = "settings.group.connections"
	KeyShowClaude                = "settings.show_claude"
	KeyShowCodex                 = "settings.show_codex"
	KeyShowAGGemini              = "settings.show_ag_gemini"
	KeyShowAGClaude              = "settings.show_ag_claude"
	KeyUsageMode                 = "settings.usage_mode"
	KeyUsageUsed                 = "settings.usage_used"
	KeyUsageRemaining            = "settings.usage_remaining"
	KeyLastRefresh               = "usage.last_refresh"
	KeyWarnings                  = "settings.warnings"
	KeyWarningThreshold          = "settings.warning_threshold"
	KeyDangerThreshold           = "settings.danger_threshold"
	KeyAutoStart                 = "settings.auto_start"
	KeyRefreshInterval           = "settings.refresh_interval"
	KeyAlwaysOnTop               = "settings.always_on_top"
	KeyShowInTaskbar             = "settings.show_in_taskbar"
	KeyTheme                     = "settings.theme"
	KeyThemeLight                = "settings.theme_light"
	KeyThemeDark                 = "settings.theme_dark"
	KeyThemeSystem               = "settings.theme_system"
	KeyLanguage                  = "settings.language"
	KeyLanguageSystem            = "settings.language_system"
	KeyLanguageKorean            = "settings.language_korean"
	KeyLanguageEnglish           = "settings.language_english"
	KeyDateTime                  = "settings.datetime"
	KeyConnected                 = "status.connected"
	KeyDisconnected              = "status.disconnected"
	KeyConnect                   = "action.connect"
	KeyReconnect                 = "action.reconnect"
	KeyTestConnection            = "action.test_connection"
	KeyInstall                   = "action.install"
	KeyCLIPath                   = "connection.cli_path"
	KeyCLIVersion                = "connection.cli_version"
	KeySource                    = "connection.source"
	KeyLocalLSP                  = "connection.local_lsp"
	KeyInstallClaude             = "connection.install_claude"
	KeyInstallCodex              = "connection.install_codex"
	KeyClose                     = "action.close"
	KeyMinimize                  = "action.minimize"
	KeyRefresh                   = "action.refresh"
	KeyDisplayMode               = "action.display_mode"
	KeyDisplayNormal             = "action.display_normal"
	KeyCompact                   = "action.compact"
	KeyNano                      = "action.nano"
	KeySettings                  = "action.settings"
	KeyHelp                      = "action.help"
	KeyUpdate                    = "action.update"
	KeyUpdatePending             = "action.update_pending"
	KeyDone                      = "action.done"
	KeyTooltipTheme              = "tooltip.theme"
	KeyTooltipThemeLight         = "tooltip.theme_light"
	KeyTooltipThemeDark          = "tooltip.theme_dark"
	KeyTooltipThemeSystem        = "tooltip.theme_system"
	KeyTooltipDisplay            = "tooltip.display"
	KeyTooltipRefresh            = "tooltip.refresh"
	KeyTooltipBack               = "tooltip.back"
	KeyTooltipUpdatePending      = "tooltip.update_pending"
	KeyClaudeColor               = "settings.claude_color"
	KeyCodexColor                = "settings.codex_color"
	KeyAGGeminiColor             = "settings.ag_gemini_color"
	KeyAGClaudeColor             = "settings.ag_claude_color"
	KeyHelpTitle                 = "help.title"
	KeyHelpIntro                 = "help.intro"
	KeyHelpClaudeTitle           = "help.claude_title"
	KeyHelpClaude                = "help.claude"
	KeyHelpClaudeRetry           = "help.claude_retry"
	KeyHelpCodexTitle            = "help.codex_title"
	KeyHelpCodex                 = "help.codex"
	KeyHelpCodexRetry            = "help.codex_retry"
	KeyHelpAntigravityTitle      = "help.antigravity_title"
	KeyHelpAntigravity           = "help.antigravity"
	KeyHelpAntigravityRetry      = "help.antigravity_retry"
	KeyHelpCredentials           = "help.credentials"
)

var RequiredKeys = []string{KeyTrayShow, KeyTraySettings, KeyTrayNormal, KeyTrayCompact, KeyTrayNano, KeyTrayQuit, KeyUntilReset, KeyResetsAt, KeyErrorCLINotInstalled, KeyErrorNotLoggedIn, KeyErrorCLIOutdated, KeyErrorInitializationFailed, KeyErrorTimeout, KeyErrorInvalidResponse, KeyErrorUsageUnavailable, KeyErrorQuotaExhausted, KeyNotificationWarning, KeyDate12, KeyDate12Day, KeyDate24, KeyDate24Day, KeyAppTitle, KeySettingsTitle, KeyGroupUsage, KeyGroupBehavior, KeyGroupDisplay, KeyGroupConnections, KeyShowClaude, KeyShowCodex, KeyShowAGGemini, KeyShowAGClaude, KeyUsageMode, KeyUsageUsed, KeyUsageRemaining, KeyLastRefresh, KeyWarnings, KeyWarningThreshold, KeyDangerThreshold, KeyAutoStart, KeyRefreshInterval, KeyAlwaysOnTop, KeyShowInTaskbar, KeyTheme, KeyThemeLight, KeyThemeDark, KeyThemeSystem, KeyLanguage, KeyLanguageSystem, KeyLanguageKorean, KeyLanguageEnglish, KeyDateTime, KeyConnected, KeyDisconnected, KeyConnect, KeyReconnect, KeyTestConnection, KeyInstall, KeyCLIPath, KeyCLIVersion, KeySource, KeyLocalLSP, KeyInstallClaude, KeyInstallCodex, KeyClose, KeyMinimize, KeyRefresh, KeyDisplayMode, KeyDisplayNormal, KeyCompact, KeyNano, KeySettings, KeyHelp, KeyUpdate, KeyUpdatePending, KeyDone, KeyTooltipTheme, KeyTooltipThemeLight, KeyTooltipThemeDark, KeyTooltipThemeSystem, KeyTooltipDisplay, KeyTooltipRefresh, KeyTooltipBack, KeyTooltipUpdatePending, KeyClaudeColor, KeyCodexColor, KeyAGGeminiColor, KeyAGClaudeColor, KeyHelpTitle, KeyHelpIntro, KeyHelpClaudeTitle, KeyHelpClaude, KeyHelpClaudeRetry, KeyHelpCodexTitle, KeyHelpCodex, KeyHelpCodexRetry, KeyHelpAntigravityTitle, KeyHelpAntigravity, KeyHelpAntigravityRetry, KeyHelpCredentials}

//go:embed locales/*.json
var resources embed.FS

type Catalog struct {
	translations map[Language]map[string]string
}

func Load() (*Catalog, error) {
	catalog := &Catalog{translations: make(map[Language]map[string]string)}
	for _, language := range []Language{Korean, English} {
		data, err := resources.ReadFile("locales/" + string(language) + ".json")
		if err != nil {
			return nil, fmt.Errorf("read %s translations: %w", language, err)
		}
		var values map[string]string
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, fmt.Errorf("decode %s translations: %w", language, err)
		}
		catalog.translations[language] = values
	}
	if missing := catalog.MissingKeys(); len(missing) > 0 {
		return nil, fmt.Errorf("missing translation keys: %v", missing)
	}
	return catalog, nil
}
func (c *Catalog) MissingKeys() map[Language][]string {
	missing := make(map[Language][]string)
	keys := make(map[string]struct{}, len(RequiredKeys))
	for _, key := range RequiredKeys {
		keys[key] = struct{}{}
	}
	for _, values := range c.translations {
		for key := range values {
			keys[key] = struct{}{}
		}
	}
	for _, language := range []Language{Korean, English} {
		for key := range keys {
			if c.translations[language][key] == "" {
				missing[language] = append(missing[language], key)
			}
		}
		sort.Strings(missing[language])
	}
	return missing
}
func (c *Catalog) Text(language, systemLanguage Language, key string) string {
	if language == System {
		language = systemLanguage
	}
	if language != Korean && language != English {
		language = English
	}
	if text := c.translations[language][key]; text != "" {
		return text
	}
	return key
}
