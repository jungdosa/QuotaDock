package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/jungdosa/QuotaDock/internal/settings"
	"github.com/jungdosa/QuotaDock/internal/webview"
)

// migrateFirstClaudeProfile moves an existing shared sign-in to the first
// account's own folder when only the first account was using it.
func migrateFirstClaudeProfile(dataDir string, cfg settings.Config) (moved bool, err error) {
	if cfg.ConnectionMethods["claude"] != settings.ConnectionMethodAuth {
		return false, nil
	}
	if cfg.ClaudeAccounts >= 2 {
		secondMethod := cfg.ConnectionMethods["claude-auth"]
		if secondMethod != settings.ConnectionMethodCLI && secondMethod != settings.ConnectionMethodOther {
			return false, nil
		}
	}
	destination := filepath.Join(dataDir, webview.DefaultUserDataDir+"-1")
	if _, err := os.Stat(destination); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	source := filepath.Join(dataDir, webview.DefaultUserDataDir)
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	if err := os.Rename(source, destination); err != nil {
		return false, err
	}
	return true, nil
}

// signInOutcome names how a sign-in window ended, for the diagnostics log.
func signInOutcome(err error) string {
	switch {
	case err == nil:
		return "signed_in"
	case errors.Is(err, webview.ErrSignInCancelled):
		return "closed"
	case errors.Is(err, webview.ErrSignInBlocked):
		return "blocked"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	default:
		return "failed"
	}
}
