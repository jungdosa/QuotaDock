// Package i18n loads stable translation keys from embedded resources.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Language string

const (
	System              Language = "system"
	English             Language = "en"
	Korean              Language = "ko"
	German              Language = "de"
	French              Language = "fr"
	Italian             Language = "it"
	Indonesian          Language = "id"
	PortugueseBrazil    Language = "pt-BR"
	SpanishSpain        Language = "es-ES"
	SpanishLatinAmerica Language = "es-419"
	Japanese            Language = "ja"
	ChineseSimplified   Language = "zh-Hans"
	ChineseTraditional  Language = "zh-Hant"
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
	KeyCreditsSpend              = "usage.credits_spend"
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
	KeyStartMinimized            = "settings.start_minimized"
	KeyRefreshInterval           = "settings.refresh_interval"
	KeyAlwaysOnTop               = "settings.always_on_top"
	KeyPromoteTray               = "settings.promote_tray"
	KeyShowInTaskbar             = "settings.show_in_taskbar"
	KeyTheme                     = "settings.theme"
	KeyThemeLight                = "settings.theme_light"
	KeyThemeDark                 = "settings.theme_dark"
	KeyThemeSystem               = "settings.theme_system"
	KeyLanguage                  = "settings.language"
	KeyLanguageSystem            = "settings.language_system"
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
	KeyUpdateNow                 = "action.update_now"
	KeyUpdateLater               = "action.update_later"
	KeyUpdateChecking            = "update.checking"
	KeyUpdateUpToDate            = "update.up_to_date"
	KeyUpdateAvailable           = "update.available"
	KeyUpdateCheckFailed         = "update.check_failed"
	KeyUpdateDownloading         = "update.downloading"
	KeyUpdateVerifying           = "update.verifying"
	KeyUpdateHashMismatch        = "update.hash_mismatch"
	KeyUpdateInstalling          = "update.installing"
	KeyUpdatePortableNotice      = "update.portable_notice"
	KeyUpdateUnsignedNotice      = "update.unsigned_notice"
	KeyDone                      = "action.done"
	KeyTooltipTheme              = "tooltip.theme"
	KeyTooltipThemeLight         = "tooltip.theme_light"
	KeyTooltipThemeDark          = "tooltip.theme_dark"
	KeyTooltipThemeSystem        = "tooltip.theme_system"
	KeyTooltipDisplay            = "tooltip.display"
	KeyTooltipRefresh            = "tooltip.refresh"
	KeyTooltipBack               = "tooltip.back"
	KeyTooltipUpdatePending      = "tooltip.update_pending"
	KeyTooltipPromoteTray        = "tooltip.promote_tray"
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

const (
	KeyConnectionMethodCLI         = "connection.method_cli"
	KeyConnectionMethodAuth        = "connection.method_auth"
	KeyConnectionMethodIDE         = "connection.method_ide"
	KeyConnectionMethodOther       = "connection.method_other"
	KeyConnectionStateActive       = "connection.state_active"
	KeyConnectionStateAvailable    = "connection.state_available"
	KeyConnectionStateMissing      = "connection.state_missing"
	KeyConnectionStatePlanned      = "connection.state_planned"
	KeyConnectionPanelInstallTitle = "connection.panel_install_title"
	KeyConnectionInstallStep1      = "connection.install_step_1"
	KeyConnectionInstallStep2      = "connection.install_step_2"
	KeyConnectionInstallStep3      = "connection.install_step_3"
	KeyConnectionSearchPaths       = "connection.search_paths"
	KeyConnectionAutoDetect        = "connection.auto_detect"
	KeyConnectionAuthPlanned       = "connection.auth_planned"
	KeyConnectionEnvConfigured     = "connection.env_configured"
	KeyConnectionEnvHint           = "connection.env_hint"
	KeyConnectionPanelClose        = "connection.panel_close"
	KeyRescan                      = "action.rescan"
	KeyOpenInstallDocs             = "action.open_install_docs"
)

var Supported = []Language{
	English,
	Korean,
	German,
	French,
	Italian,
	Indonesian,
	PortugueseBrazil,
	SpanishSpain,
	SpanishLatinAmerica,
	Japanese,
	ChineseSimplified,
	ChineseTraditional,
}

var endonyms = map[Language]string{
	English:             "English",
	Korean:              "한국어",
	German:              "Deutsch",
	French:              "Français",
	Italian:             "Italiano",
	Indonesian:          "Bahasa Indonesia",
	PortugueseBrazil:    "Português (Brasil)",
	SpanishSpain:        "Español (España)",
	SpanishLatinAmerica: "Español (Latinoamérica)",
	Japanese:            "日本語",
	ChineseSimplified:   "简体中文",
	ChineseTraditional:  "繁體中文（臺灣）",
}

func Endonym(language Language) string {
	return endonyms[language]
}

func IsSupported(language Language) bool {
	for _, candidate := range Supported {
		if language == candidate {
			return true
		}
	}
	return false
}

// MatchSystemLanguage maps an OS locale to a supported QuotaDock language.
// It is deliberately pure so platform locale detection remains independently testable.
func MatchSystemLanguage(raw string) Language {
	normalized := strings.TrimSpace(strings.ReplaceAll(raw, "_", "-"))
	if index := strings.IndexAny(normalized, ".@"); index >= 0 {
		normalized = normalized[:index]
	}
	for _, language := range Supported {
		if strings.EqualFold(normalized, string(language)) {
			return language
		}
	}
	lower := strings.ToLower(normalized)
	if lower == "zh-hant" || strings.HasPrefix(lower, "zh-hant-") {
		return ChineseTraditional
	}
	switch lower {
	case "zh-tw", "zh-hk", "zh-mo":
		return ChineseTraditional
	case "zh", "zh-cn", "zh-sg", "zh-hans":
		return ChineseSimplified
	}
	if strings.HasPrefix(lower, "zh-hans-") {
		return ChineseSimplified
	}
	base := strings.ToLower(strings.Split(normalized, "-")[0])
	switch base {
	case "en":
		return English
	case "ko":
		return Korean
	case "ja":
		return Japanese
	case "de":
		return German
	case "fr":
		return French
	case "it":
		return Italian
	case "id":
		return Indonesian
	case "pt":
		return PortugueseBrazil
	case "es":
		if strings.EqualFold(normalized, "es") || strings.EqualFold(normalized, "es-ES") {
			return SpanishSpain
		}
		return SpanishLatinAmerica
	default:
		return English
	}
}

func FormatDecimal(language Language, value float64) string {
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	switch language {
	case German, French, Italian, Indonesian, PortugueseBrazil, SpanishSpain, SpanishLatinAmerica:
		return strings.Replace(formatted, ".", ",", 1)
	default:
		return formatted
	}
}

var RequiredKeys = []string{
	KeyTrayShow,
	KeyTraySettings,
	KeyTrayNormal,
	KeyTrayCompact,
	KeyTrayNano,
	KeyTrayQuit,
	KeyUntilReset,
	KeyResetsAt,
	KeyCreditsSpend,
	KeyErrorCLINotInstalled,
	KeyErrorNotLoggedIn,
	KeyErrorCLIOutdated,
	KeyErrorInitializationFailed,
	KeyErrorTimeout,
	KeyErrorInvalidResponse,
	KeyErrorUsageUnavailable,
	KeyErrorQuotaExhausted,
	KeyNotificationWarning,
	KeyDate12,
	KeyDate12Day,
	KeyDate24,
	KeyDate24Day,
	KeyAppTitle,
	KeySettingsTitle,
	KeyGroupUsage,
	KeyGroupBehavior,
	KeyGroupDisplay,
	KeyGroupConnections,
	KeyShowClaude,
	KeyShowCodex,
	KeyShowAGGemini,
	KeyShowAGClaude,
	KeyUsageMode,
	KeyUsageUsed,
	KeyUsageRemaining,
	KeyLastRefresh,
	KeyWarnings,
	KeyWarningThreshold,
	KeyDangerThreshold,
	KeyAutoStart,
	KeyStartMinimized,
	KeyRefreshInterval,
	KeyAlwaysOnTop,
	KeyPromoteTray,
	KeyShowInTaskbar,
	KeyTheme,
	KeyThemeLight,
	KeyThemeDark,
	KeyThemeSystem,
	KeyLanguage,
	KeyLanguageSystem,
	KeyDateTime,
	KeyConnected,
	KeyDisconnected,
	KeyConnect,
	KeyReconnect,
	KeyTestConnection,
	KeyInstall,
	KeyCLIPath,
	KeyCLIVersion,
	KeySource,
	KeyLocalLSP,
	KeyInstallClaude,
	KeyInstallCodex,
	KeyConnectionMethodCLI,
	KeyConnectionMethodAuth,
	KeyConnectionMethodIDE,
	KeyConnectionMethodOther,
	KeyConnectionStateActive,
	KeyConnectionStateAvailable,
	KeyConnectionStateMissing,
	KeyConnectionStatePlanned,
	KeyConnectionPanelInstallTitle,
	KeyConnectionInstallStep1,
	KeyConnectionInstallStep2,
	KeyConnectionInstallStep3,
	KeyConnectionSearchPaths,
	KeyConnectionAutoDetect,
	KeyConnectionAuthPlanned,
	KeyConnectionEnvConfigured,
	KeyConnectionEnvHint,
	KeyConnectionPanelClose,
	KeyRescan,
	KeyOpenInstallDocs,
	KeyClose,
	KeyMinimize,
	KeyRefresh,
	KeyDisplayMode,
	KeyDisplayNormal,
	KeyCompact,
	KeyNano,
	KeySettings,
	KeyHelp,
	KeyUpdate,
	KeyUpdatePending,
	KeyUpdateNow,
	KeyUpdateLater,
	KeyUpdateChecking,
	KeyUpdateUpToDate,
	KeyUpdateAvailable,
	KeyUpdateCheckFailed,
	KeyUpdateDownloading,
	KeyUpdateVerifying,
	KeyUpdateHashMismatch,
	KeyUpdateInstalling,
	KeyUpdatePortableNotice,
	KeyUpdateUnsignedNotice,
	KeyDone,
	KeyTooltipTheme,
	KeyTooltipThemeLight,
	KeyTooltipThemeDark,
	KeyTooltipThemeSystem,
	KeyTooltipDisplay,
	KeyTooltipRefresh,
	KeyTooltipBack,
	KeyTooltipUpdatePending,
	KeyTooltipPromoteTray,
	KeyClaudeColor,
	KeyCodexColor,
	KeyAGGeminiColor,
	KeyAGClaudeColor,
	KeyHelpTitle,
	KeyHelpIntro,
	KeyHelpClaudeTitle,
	KeyHelpClaude,
	KeyHelpClaudeRetry,
	KeyHelpCodexTitle,
	KeyHelpCodex,
	KeyHelpCodexRetry,
	KeyHelpAntigravityTitle,
	KeyHelpAntigravity,
	KeyHelpAntigravityRetry,
	KeyHelpCredentials,
}

//go:embed locales/*.json
var resources embed.FS

type Catalog struct {
	translations map[Language]map[string]string
}

func Load() (*Catalog, error) {
	catalog := &Catalog{translations: make(map[Language]map[string]string)}
	for _, language := range Supported {
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
	for _, language := range Supported {
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
	if !IsSupported(language) {
		language = English
	}
	values, loaded := c.translations[language]
	if !loaded {
		values = c.translations[English]
	}
	if text := values[key]; text != "" {
		return text
	}
	return key
}
