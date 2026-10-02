package settings

import (
	"github.com/jungdosa/QuotaDock/internal/model"
	"slices"
	"strings"
	"testing"
)

func TestCodexAccountsMigrateWithoutChangingExistingPreferences(t *testing.T) {
	cfg, err := Decode(strings.NewReader(`{"schemaVersion":7,"showCodex":false,"claudeAccounts":3,"laneOrder":["grok","codex","claude"],"providerColors":{"codex":"teal"},"accountLabels":{"claude":"Work"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CodexAccounts != 1 || cfg.ShowCodex || cfg.ClaudeAccounts != 3 || cfg.ProviderColors["codex"] != "teal" || cfg.AccountLabels["claude"] != "Work" {
		t.Fatalf("migration changed preferences: %+v", cfg)
	}
	if cfg.SchemaVersion != 8 || !slices.Equal(cfg.LaneOrder[:6], []string{"grok", "codex", "codex-2", "codex-3", "codex-4", "codex-5"}) {
		t.Fatalf("migration order=%v", cfg.LaneOrder)
	}
}

func TestCodexAccountSettingsAreBoundedAndPreserved(t *testing.T) {
	cfg := Default()
	cfg.CodexAccounts = 99
	cfg.AccountLabels = map[string]string{"codex": "Work", "codex-2": "Personal", "codex-6": "invalid"}
	cfg.ConnectionMethods = map[string]string{"codex-2": "auth"}
	cfg = cfg.Validated()
	if cfg.CodexAccounts != 5 || cfg.AccountLabels["codex-2"] != "Personal" || cfg.AccountLabels["codex-6"] != "" || cfg.ConnectionMethods["codex-2"] != "auth" {
		t.Fatal("account settings not preserved/validated")
	}
	for _, id := range model.CodexAccountIDs() {
		if cfg.ProviderColors[string(id)] == "" {
			t.Fatalf("missing color: %s", id)
		}
	}
	cfg.CodexAccounts = 1
	cfg = cfg.Validated()
	cfg.CodexAccounts = 2
	cfg = cfg.Validated()
	if cfg.AccountLabels["codex-2"] != "Personal" {
		t.Fatal("removing/readding lost the alias")
	}
	if got := (Config{CodexAccounts: -1}).Validated().CodexAccounts; got != 1 {
		t.Fatal(got)
	}
}
