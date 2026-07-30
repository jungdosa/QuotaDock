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
