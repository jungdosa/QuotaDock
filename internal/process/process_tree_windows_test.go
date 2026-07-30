//go:build windows

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsCommandConfigurationHidesConsole(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	configureCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("Windows process attributes were not configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("Windows child process window is not hidden")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CreationFlags=%#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}

func TestWindowsGrandchildProcessIsTerminated(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := (Runner{}).RunJSONL(ctx, CommandSpec{Name: os.Args[0], Args: []string{"-test.run=TestProcessHelper", "--", "tree-parent", pidFile}})
		result <- err
	}()

	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		t.Fatal("grandchild PID was not reported")
	}
	cancel()
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("process runner did not stop")
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		active, err := processIsActive(uint32(pid))
		if err != nil || !active {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("grandchild process %d remained active", pid)
}

func processIsActive(pid uint32) (bool, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false, err
	}
	return exitCode == 259, nil
}
