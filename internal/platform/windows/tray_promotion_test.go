package windows

import (
	"testing"
	"time"
)

func TestTrayPromotionWindows11BuildThreshold(t *testing.T) {
	tests := []struct {
		build int
		want  bool
	}{
		{build: 19045, want: false},
		{build: 22000, want: true},
		{build: 26200, want: true},
	}
	for _, test := range tests {
		if got := isWindows11Build(test.build); got != test.want {
			t.Errorf("isWindows11Build(%d)=%v, want %v", test.build, got, test.want)
		}
	}
}

func TestTrayPromotionNormalizesExecutablePaths(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "extended drive prefix", value: "\\\\?\\C:\\a\\b.exe", want: "c:\\a\\b.exe"},
		{name: "extended UNC prefix", value: "\\\\?\\UNC\\srv\\s\\b.exe", want: "\\\\srv\\s\\b.exe"},
		{name: "mixed slashes and case", value: "C:/A\\b.EXE", want: "c:\\a\\b.exe"},
		{name: "trailing slash", value: "C:\\A\\b.exe\\", want: "c:\\a\\b.exe"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeExecutablePath(test.value); got != test.want {
				t.Fatalf("normalizeExecutablePath(%q)=%q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestTrayPromotionSelectsExactExecutablePath(t *testing.T) {
	entries := []trayIconRegistryEntry{
		{subkey: "old", executablePath: "C:\\old\\quotadock.exe"},
		{subkey: "current", executablePath: "\\\\?\\C:\\Apps\\QuotaDock\\quotadock.exe"},
	}
	got, found := selectTrayIconSubkey("c:/apps/quotadock/QUOTADOCK.EXE", entries)
	if !found || got != "current" {
		t.Fatalf("exact path match=%q/%v, want current/true", got, found)
	}

	if got, found := selectTrayIconSubkey("C:\\Apps\\QuotaDock\\other.exe", entries); found {
		t.Fatalf("different executable in the same directory matched subkey %q", got)
	}
}

func TestTrayPromotionFilenameFallbackRequiresSingleCandidate(t *testing.T) {
	single := []trayIconRegistryEntry{
		{subkey: "only", executablePath: "D:\\portable\\quotadock.exe"},
	}
	if got, found := selectTrayIconSubkey("C:\\Apps\\QuotaDock\\quotadock.exe", single); !found || got != "only" {
		t.Fatalf("single filename fallback=%q/%v, want only/true", got, found)
	}

	ambiguous := append(single, trayIconRegistryEntry{subkey: "second", executablePath: "E:\\qa\\quotadock.exe"})
	if got, found := selectTrayIconSubkey("C:\\Apps\\QuotaDock\\quotadock.exe", ambiguous); found {
		t.Fatalf("ambiguous filename fallback matched subkey %q", got)
	}
}

func TestTrayPromotionDemotesOnlyOnEnabledToDisabledTransition(t *testing.T) {
	tests := []struct {
		previous bool
		current  bool
		want     bool
	}{
		{previous: false, current: false, want: false},
		{previous: false, current: true, want: false},
		{previous: true, current: true, want: false},
		{previous: true, current: false, want: true},
	}
	for _, test := range tests {
		if got := shouldDemoteTrayIcon(test.previous, test.current); got != test.want {
			t.Errorf("shouldDemoteTrayIcon(%v, %v)=%v, want %v", test.previous, test.current, got, test.want)
		}
	}
}

func TestTrayPromotionRetryContinuesOnlyWhenEntryIsMissing(t *testing.T) {
	tests := []struct {
		name   string
		result TrayPromotionResult
		want   bool
	}{
		{name: "entry not found", result: TrayPromotionEntryNotFound, want: true},
		{name: "applied", result: TrayPromotionApplied, want: false},
		{name: "unsupported", result: TrayPromotionUnsupported, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got := NextTrayIconPromotionRetry(test.result, 1)
			if got != test.want {
				t.Fatalf("NextTrayIconPromotionRetry(%v, 1) retry=%v, want %v", test.result, got, test.want)
			}
		})
	}
}

func TestTrayPromotionRetryAttemptsAreCapped(t *testing.T) {
	attempts := 1
	var previousDelay time.Duration
	for {
		delay, retry := NextTrayIconPromotionRetry(TrayPromotionEntryNotFound, attempts)
		if !retry {
			break
		}
		if delay <= previousDelay {
			t.Fatalf("retry delay %v after attempt %d did not increase from %v", delay, attempts, previousDelay)
		}
		previousDelay = delay
		attempts++
	}
	if attempts != maxTrayPromotionAttempts {
		t.Fatalf("tray promotion attempts=%d, want %d", attempts, maxTrayPromotionAttempts)
	}
}
