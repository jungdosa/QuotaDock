package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestUsageAndRemainingConversion(t *testing.T) {
	used, remaining := PercentPair(35, false)
	if used != 35 || remaining != 65 {
		t.Fatalf("used input = (%v, %v)", used, remaining)
	}
	used, remaining = PercentPair(25, true)
	if used != 75 || remaining != 25 {
		t.Fatalf("remaining input = (%v, %v)", used, remaining)
	}
}

func TestWarningThresholdBoundaries(t *testing.T) {
	cases := []struct {
		value float64
		want  AlertLevel
	}{{79.99, AlertNormal}, {80, AlertWarning}, {89.99, AlertWarning}, {90, AlertDanger}, {110, AlertDanger}}
	for _, tc := range cases {
		if got := ClassifyUsage(tc.value, 80, 90); got != tc.want {
			t.Errorf("ClassifyUsage(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestDisallowedPlanStringsRejected(t *testing.T) {
	if got := NormalizePlan(ProviderClaude, "max_5x"); got != "MAX 5X" {
		t.Fatalf("known plan = %q", got)
	}
	for _, raw := range []string{"person@example.invalid", "VIP CUSTOM", "127.0.0.1:9999"} {
		if got := NormalizePlan(ProviderClaude, raw); got != PlanUnknown {
			t.Errorf("NormalizePlan(%q) = %q", raw, got)
		}
	}
}

func TestCodexPlanAllowlistMatchesOfficialPlanTypes(t *testing.T) {
	cases := map[string]Plan{
		"go":                              "GO",
		"prolite":                         "PRO LITE",
		"self_serve_business_usage_based": "BUSINESS",
		"enterprise_cbp_usage_based":      "ENTERPRISE",
	}
	for raw, want := range cases {
		if got := NormalizePlan(ProviderCodex, raw); got != want {
			t.Errorf("NormalizePlan(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := NormalizePlan(ProviderCodex, "internal_custom_plan"); got != PlanUnknown {
		t.Fatalf("unapproved Codex plan = %q", got)
	}
}

func TestUsageWindowLabelBoundaries(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{{0, "Other"}, {-1, "Other"}, {1, "Session"}, {360, "Session"}, {361, "Weekly"}, {10080, "Weekly"}, {10081, "Monthly"}}
	for _, tc := range cases {
		if got := UsageWindowLabel(tc.minutes); got != tc.want {
			t.Errorf("UsageWindowLabel(%d) = %q, want %q", tc.minutes, got, tc.want)
		}
	}
}

func TestPublicSnapshotSurfaceContainsNoSecretFields(t *testing.T) {
	forbidden := []string{"token", "cookie", "csrf", "credential", "email", "port", "command"}
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.ToLower(field.Name)
			for _, word := range forbidden {
				if strings.Contains(name, word) {
					t.Errorf("public field %s contains forbidden word %q", field.Name, word)
				}
			}
			walk(field.Type)
		}
	}
	walk(reflect.TypeOf(UsageSnapshot{}))
	walk(reflect.TypeOf(ConnectionState{}))
}
