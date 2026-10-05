//go:build windows

package antigravity

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/process"
)

func TestCLIExecutableDiscoveryOnlyAcceptsFixedLocations(t *testing.T) {
	root := t.TempDir()
	local, home := filepath.Join(root, "local"), filepath.Join(root, "home")
	getenv := func(key string) string {
		switch key {
		case "LOCALAPPDATA":
			return local
		case "USERPROFILE":
			return home
		}
		return ""
	}
	writeFake := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		// An empty inert file is sufficient: discovery must never execute it.
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rogue := filepath.Join(root, "rogue", "agy.exe")
	writeFake(rogue)
	look := func(string) (string, error) { return rogue, nil }
	if _, err := findAGYExecutable(getenv, look); !errors.Is(err, errCLIUnavailable) {
		t.Fatal("arbitrary PATH executable accepted")
	}
	second := filepath.Join(home, ".local", "bin", "agy.exe")
	writeFake(second)
	if got, err := findAGYExecutable(getenv, look); err != nil || got != second {
		t.Fatal("fixed home candidate not used")
	}
	first := filepath.Join(local, "agy", "bin", "agy.exe")
	writeFake(first)
	if got, err := findAGYExecutable(getenv, look); err != nil || got != first {
		t.Fatal("fixed local candidate not used")
	}
	look = func(string) (string, error) { return second, nil }
	if got, err := findAGYExecutable(getenv, look); err != nil || got != second {
		t.Fatal("allowlisted PATH result not used")
	}
	if _, err := findAGYExecutable(func(string) string { return "relative" }, look); !errors.Is(err, errCLIUnavailable) {
		t.Fatal("relative install root accepted")
	}
}

func TestCLIProbeRequiresDedicatedEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	dir, err := agyProbeDirectory(root)
	if err != nil || dir != filepath.Join(root, "QuotaDock", "agy-probe") {
		t.Fatal("incorrect probe directory")
	}
	if _, err := agyProbeDirectory(root); err != nil {
		t.Fatal("empty directory creation is not idempotent")
	}
	if err := os.WriteFile(filepath.Join(dir, "project-file"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := agyProbeDirectory(root); !errors.Is(err, errCLIUnavailable) {
		t.Fatal("nonempty probe accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "project-file")); err != nil {
		t.Fatal("probe cleanup deleted an unexpected file")
	}
	for _, invalid := range []string{"", ".", "relative"} {
		if _, err := agyProbeDirectory(invalid); !errors.Is(err, errCLIUnavailable) {
			t.Fatal("relative probe root accepted")
		}
	}
}

func TestCLIRunnerUsesBoundedTreeAndCancellation(t *testing.T) {
	tree := &agyProcessTree{}
	runner := agyRunner(nil, tree)
	if runner.Tree != tree || runner.Timeout != cliUsageTimeout || runner.MaxOutputBytes != cliOutputLimit || runner.MaxStderrBytes != cliOutputLimit {
		t.Fatal("runner budgets or tree missing")
	}
	cmd := &exec.Cmd{}
	if err := tree.Prepare(cmd); err != nil || cmd.Cancel == nil || cmd.WaitDelay <= 0 {
		t.Fatal("tree cancellation does not bound output-pipe waits")
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatal(err)
	}
	if err := tree.Terminate(cmd); err != nil || !tree.stopped {
		t.Fatal("tree termination is not idempotent")
	}
	var _ process.TreeController = tree
	if NewLocalClient().cli != NewLocalClient().cli {
		t.Fatal("clients do not share the process-lifetime breaker")
	}
}
