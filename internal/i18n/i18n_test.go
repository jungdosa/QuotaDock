package i18n

import (
	"slices"
	"strings"
	"testing"
)

func TestTranslationMissingKeyDetection(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if missing := catalog.MissingKeys(); len(missing) != 0 {
		t.Fatalf("missing translations: %v", missing)
	}
	if got := catalog.Text(Korean, English, KeyUntilReset); got != "재설정까지" {
		t.Fatalf("Korean wording = %q", got)
	}
	if got := catalog.Text(Korean, English, KeyHelpTitle); got != "사용량 표시 안내" {
		t.Fatalf("Korean help title = %q", got)
	}
	if got := catalog.Text(Korean, English, KeyHelpIntro); got != "각 서비스는 서로 독립적으로 연결되며, 한 서비스가 실패해도 나머지 사용량은 계속 표시됩니다." {
		t.Fatalf("Korean help intro = %q", got)
	}
	for _, values := range catalog.translations {
		for _, value := range values {
			if strings.Contains(value, "쿨타임") {
				t.Fatalf("forbidden wording in %q", value)
			}
		}
	}
}

func TestTranslationMissingKeyDetectionIncludesLocaleKeySetDifferences(t *testing.T) {
	catalog := &Catalog{translations: map[Language]map[string]string{
		Korean:  {"only.in.korean": "한국어"},
		English: {},
	}}
	missing := catalog.MissingKeys()
	if !slices.Contains(missing[English], "only.in.korean") {
		t.Fatalf("locale key-set difference was not detected: %v", missing)
	}
}

func TestLoadAllNineLocalesWithoutMissingKeys(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []Language{English, Korean, German, French, Italian, Indonesian, PortugueseBrazil, SpanishSpain, SpanishLatinAmerica}
	if !slices.Equal(Supported, want) {
		t.Fatalf("Supported = %v, want %v", Supported, want)
	}
	if len(catalog.translations) != 9 {
		t.Fatalf("loaded locales = %d, want 9", len(catalog.translations))
	}
	if missing := catalog.MissingKeys(); len(missing) != 0 {
		t.Fatalf("missing translations: %v", missing)
	}
}

func TestAllLocalesHaveSame138Keys(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	englishKeys := make([]string, 0, len(catalog.translations[English]))
	for key := range catalog.translations[English] {
		englishKeys = append(englishKeys, key)
	}
	slices.Sort(englishKeys)
	if len(englishKeys) != 138 {
		t.Fatalf("English key count = %d, want 138", len(englishKeys))
	}
	for _, language := range Supported {
		keys := make([]string, 0, len(catalog.translations[language]))
		for key := range catalog.translations[language] {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, englishKeys) {
			t.Errorf("%s key set differs from English", language)
		}
	}
}

func TestMatchSystemLanguage(t *testing.T) {
	cases := map[string]Language{
		"pt":    PortugueseBrazil,
		"pt-BR": PortugueseBrazil,
		"es":    SpanishSpain,
		"es-ES": SpanishSpain,
		"es-MX": SpanishLatinAmerica,
		"es-AR": SpanishLatinAmerica,
		"es-CO": SpanishLatinAmerica,
		"de-DE": German,
		"id_ID": Indonesian,
		"ja":    English,
		"":      English,
	}
	for raw, want := range cases {
		if got := MatchSystemLanguage(raw); got != want {
			t.Errorf("MatchSystemLanguage(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestTextFallsBackToEnglishForUnloadedLanguage(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Text(Language("ja"), Korean, KeyHelp); got != "Help" {
		t.Fatalf("unsupported language text = %q, want English", got)
	}
	if got := catalog.Text(System, Language("hi"), KeyHelp); got != "Help" {
		t.Fatalf("unsupported system language text = %q, want English", got)
	}
}

func TestFormatDecimalUsesLocaleSeparator(t *testing.T) {
	for _, language := range []Language{English, Korean} {
		if got := FormatDecimal(language, 2.39); got != "2.39" {
			t.Errorf("FormatDecimal(%s) = %q, want 2.39", language, got)
		}
	}
	for _, language := range []Language{German, French, Italian, Indonesian, PortugueseBrazil, SpanishSpain, SpanishLatinAmerica} {
		if got := FormatDecimal(language, 2.39); got != "2,39" {
			t.Errorf("FormatDecimal(%s) = %q, want 2,39", language, got)
		}
	}
}
