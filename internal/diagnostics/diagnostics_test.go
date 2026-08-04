package diagnostics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var millisecondTimestamp = regexp.MustCompile("^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}\\.\\d{3}(Z|[+-]\\d{2}:\\d{2})$")

func readJSONLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for index, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", index+1, err, line)
		}
		records = append(records, record)
	}
	return records
}

func TestJSONLinesAlwaysCarryCommonFieldsAndRunChanges(t *testing.T) {
	directory := t.TempDir()
	first, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "run-one", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	first.LogStart("windows_build", 26100)
	first.Logger().Info("provider.refresh", "provider", "codex", "ok", true, "err", "", "ms", 7)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "run-two", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	second.LogStart("windows_build", 26100)
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	records := readJSONLines(t, filepath.Join(directory, NormalLogName))
	if len(records) != 3 {
		t.Fatalf("records=%d, want 3", len(records))
	}
	for index, record := range records {
		for _, key := range []string{"time", "level", "msg", "ver", "run"} {
			if _, ok := record[key]; !ok {
				t.Errorf("record %d missing %q: %#v", index, key, record)
			}
		}
		if !millisecondTimestamp.MatchString(record["time"].(string)) {
			t.Errorf("timestamp=%q", record["time"])
		}
		if record["ver"] != "9.9.9-test" {
			t.Errorf("version=%v", record["ver"])
		}
	}
	if records[0]["run"] != "run-one" || records[1]["run"] != "run-one" || records[2]["run"] != "run-two" {
		t.Fatalf("run ids=%v, %v, %v", records[0]["run"], records[1]["run"], records[2]["run"])
	}
}

func TestDebugLevelOnlyComesFromExplicitEnvironmentValue(t *testing.T) {
	if got := LevelFromEnvironment(func(string) string { return "" }); got != slog.LevelInfo {
		t.Fatalf("default level=%v", got)
	}
	if got := LevelFromEnvironment(func(string) string { return "DEBUG" }); got != slog.LevelDebug {
		t.Fatalf("debug level=%v", got)
	}
}

func TestNormalLogMasksSecretsAndRejectsUnstructuredMessages(t *testing.T) {
	directory := t.TempDir()
	logger, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "mask", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	logger.Logger().Info("provider.refresh",
		"provider", "codex",
		"ok", false,
		"err", "token=fake-secret user@example.invalid",
		"nested", []string{"Authorization: Bearer nested-secret"},
		"ms", 1,
	)
	logger.Logger().Warn("provider output user@example.invalid", "token", "fake-secret")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(directory, NormalLogName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "fake-secret") || strings.Contains(text, "nested-secret") || strings.Contains(text, "user@example.invalid") {
		t.Fatalf("secret retained in log: %s", text)
	}
	if strings.Contains(text, "provider output") {
		t.Fatalf("unstructured message retained in log: %s", text)
	}
	if len(readJSONLines(t, filepath.Join(directory, NormalLogName))) != 1 {
		t.Fatal("unexpected normal log record count")
	}
}

func TestCappedWriterDropsWholeOldLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded.log")
	writer, err := newCappedWriter(path, 512)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 80; index++ {
		line := fmt.Sprintf("{\"index\":%d,\"value\":\"%s\"}\n", index, strings.Repeat("x", 48))
		if _, err := writer.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 512 {
		t.Fatalf("bounded log size=%d", info.Size())
	}
	for _, record := range readJSONLines(t, path) {
		if _, ok := record["index"]; !ok {
			t.Fatalf("partial line survived trim: %#v", record)
		}
	}
}

func TestRecoveredPanicWritesMaskedCrashJSON(t *testing.T) {
	directory := t.TempDir()
	runtime, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "panic-run", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	restore := runtime.Install()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic was not rethrown")
			}
		}()
		defer Recover("test_worker")
		panic("token=fake-secret user@example.invalid")
	}()
	restore()
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	records := readJSONLines(t, filepath.Join(directory, CrashLogName))
	if len(records) != 1 {
		t.Fatalf("crash records=%d", len(records))
	}
	record := records[0]
	if record["msg"] != "crash" || record["ver"] != "9.9.9-test" || record["run"] != "panic-run" {
		t.Fatalf("crash common fields=%#v", record)
	}
	if record["stack"] == "" || record["component"] != "test_worker" {
		t.Fatalf("crash evidence=%#v", record)
	}
	encoded, _ := json.Marshal(record)
	if strings.Contains(string(encoded), "fake-secret") || strings.Contains(string(encoded), "user@example.invalid") {
		t.Fatalf("crash retained secret: %s", encoded)
	}
}

func TestCrashLogStaysSingleAndBounded(t *testing.T) {
	directory := t.TempDir()
	runtime, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "bounded-crash", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 32; index++ {
		if err := runtime.RecordPanic("test", strings.Repeat("s", 20<<10), []byte("stack")); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directory, CrashLogName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > CrashMaxBytes {
		t.Fatalf("crash log size=%d", info.Size())
	}
	matches, err := filepath.Glob(filepath.Join(directory, "crash*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("crash files=%v", matches)
	}
	if len(readJSONLines(t, filepath.Join(directory, CrashLogName))) == 0 {
		t.Fatal("bounded crash log lost all records")
	}
}

func TestNormalSessionRemovesEmptyMarkerWithoutCrashRecord(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.Local)
	runtime, err := NewRuntime(RuntimeOptions{
		Directory: directory,
		Version:   "9.9.9-test",
		RunID:     "normal",
		Level:     slog.LevelInfo,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.BeginSession(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, SessionFileName)
	info, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("marker size=%d", info.Size())
	}
	now = now.Add(75 * time.Second)
	if err := runtime.EndSession("tray_quit"); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, CrashLogName)); !os.IsNotExist(err) {
		t.Fatalf("normal session created crash log: %v", err)
	}
	records := readJSONLines(t, filepath.Join(directory, NormalLogName))
	if records[len(records)-1]["msg"] != "app.exit" || records[len(records)-1]["uptime_s"] != float64(75) {
		t.Fatalf("exit record=%#v", records[len(records)-1])
	}
}

func TestStaleMarkerRecordsPreviousUncleanExit(t *testing.T) {
	directory := t.TempDir()
	first, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "first", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.BeginSession(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "second", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.BeginSession(); err != nil {
		t.Fatal(err)
	}
	if err := second.EndSession("test"); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	records := readJSONLines(t, filepath.Join(directory, CrashLogName))
	if len(records) != 1 || records[0]["kind"] != "previous_unclean" {
		t.Fatalf("unclean records=%#v", records)
	}
}

func TestLocalDataDirectoryUsesUserCacheNotConfiguration(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := LocalDataDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(cache, "QuotaDock") {
		t.Fatalf("local diagnostic path=%q", got)
	}
}

func TestNormalLogOneMegabyteLimitKeepsJSONLinesParsable(t *testing.T) {
	directory := t.TempDir()
	runtime, err := NewRuntime(RuntimeOptions{Directory: directory, Version: "9.9.9-test", RunID: "normal-cap", Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 7000; index++ {
		runtime.LogStart("sequence", index, "padding", strings.Repeat("x", 160))
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, NormalLogName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > NormalMaxBytes {
		t.Fatalf("normal log size=%d", info.Size())
	}
	records := readJSONLines(t, path)
	if len(records) == 0 {
		t.Fatal("normal log lost all JSON records")
	}
	for _, record := range records {
		if record["run"] != "normal-cap" {
			t.Fatalf("trimmed record run=%v", record["run"])
		}
	}
}

func TestGeneratedRunIDChangesAcrossProcesses(t *testing.T) {
	first, err := NewRunID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRunID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 8 || len(second) != 8 || first == second {
		t.Fatalf("run ids first=%q second=%q", first, second)
	}
}
