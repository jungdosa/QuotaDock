package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/settings"
	"github.com/jungdosa/QuotaDock/internal/webview"
)

func TestMigrateFirstClaudeProfile(t *testing.T) {
	for _, tc := range []struct {
		name         string
		firstMethod  string
		secondMethod string
		accounts     int
		shared       bool
		sharedFile   bool
		own          bool
		wantMoved    bool
	}{
		{name: "only first Auth", firstMethod: settings.ConnectionMethodAuth, accounts: 1, shared: true, wantMoved: true},
		{name: "second Auth", firstMethod: settings.ConnectionMethodAuth, secondMethod: settings.ConnectionMethodAuth, accounts: 2, shared: true},
		{name: "second unset", firstMethod: settings.ConnectionMethodAuth, accounts: 2, shared: true},
		{name: "second unknown", firstMethod: settings.ConnectionMethodAuth, secondMethod: "unknown", accounts: 2, shared: true},
		{name: "second CLI", firstMethod: settings.ConnectionMethodAuth, secondMethod: settings.ConnectionMethodCLI, accounts: 2, shared: true, wantMoved: true},
		{name: "second Other", firstMethod: settings.ConnectionMethodAuth, secondMethod: settings.ConnectionMethodOther, accounts: 2, shared: true, wantMoved: true},
		{name: "destination exists", firstMethod: settings.ConnectionMethodAuth, accounts: 1, shared: true, own: true},
		{name: "source absent", firstMethod: settings.ConnectionMethodAuth, accounts: 1},
		{name: "source is a file", firstMethod: settings.ConnectionMethodAuth, accounts: 1, sharedFile: true},
		{name: "first Auto", accounts: 1, shared: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			shared := filepath.Join(dataDir, webview.DefaultUserDataDir)
			own := filepath.Join(dataDir, webview.DefaultUserDataDir+"-1")
			if tc.shared {
				if err := os.Mkdir(shared, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.sharedFile {
				if err := os.WriteFile(shared, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.own {
				if err := os.Mkdir(own, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			cfg := settings.Default()
			cfg.ClaudeAccounts = tc.accounts
			cfg.ConnectionMethods = map[string]string{"claude": tc.firstMethod, "claude-auth": tc.secondMethod}
			moved, err := migrateFirstClaudeProfile(dataDir, cfg)
			if err != nil || moved != tc.wantMoved {
				t.Fatalf("moved=%v err=%v, want moved=%v", moved, err, tc.wantMoved)
			}
			if _, err := os.Stat(shared); (err == nil) != ((tc.shared || tc.sharedFile) && !tc.wantMoved) {
				t.Fatalf("shared profile state: %v", err)
			}
			if _, err := os.Stat(own); (err == nil) != (tc.own || tc.wantMoved) {
				t.Fatalf("first profile state: %v", err)
			}
			if moved, err := migrateFirstClaudeProfile(dataDir, cfg); moved || err != nil {
				t.Fatalf("second migration moved=%v err=%v", moved, err)
			}
		})
	}
}
