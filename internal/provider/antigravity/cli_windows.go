//go:build windows

package antigravity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/jungdosa/QuotaDock/internal/process"
	"golang.org/x/sys/windows"
)

var sharedCLIClient = &cliClient{prepare: prepareAGY, run: runAGY, now: time.Now}

func defaultCLIClient() *cliClient { return sharedCLIClient }

func prepareAGY() (process.CommandSpec, error) {
	path, err := findAGYExecutable(os.Getenv, exec.LookPath)
	if err != nil {
		return process.CommandSpec{}, err
	}
	dir, err := agyProbeDirectory(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		return process.CommandSpec{}, err
	}
	return process.CommandSpec{Name: path, Dir: dir, Env: cliEnvironment(os.Getenv)}, nil
}

func findAGYExecutable(getenv func(string) string, lookPath func(string) (string, error)) (string, error) {
	var candidates []string
	if root := getenv("LOCALAPPDATA"); filepath.IsAbs(root) {
		candidates = append(candidates, filepath.Join(root, "agy", "bin", "agy.exe"))
	}
	if root := getenv("USERPROFILE"); filepath.IsAbs(root) {
		candidates = append(candidates, filepath.Join(root, ".local", "bin", "agy.exe"))
	}
	allowed := func(path string) bool {
		if !filepath.IsAbs(path) {
			return false
		}
		for _, candidate := range candidates {
			if strings.EqualFold(filepath.Clean(path), candidate) && regularUnredirectedFile(candidate) {
				return true
			}
		}
		return false
	}
	if path, err := lookPath("agy"); err == nil && allowed(path) {
		return filepath.Clean(path), nil
	}
	for _, candidate := range candidates {
		if allowed(candidate) {
			return candidate, nil
		}
	}
	return "", errCLIUnavailable
}

func regularUnredirectedFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return noAGYReparsePoints(path)
}

// Attribute queries avoid enumerating ancestor directories (which may be
// denied by Windows ACLs) while rejecting symlink/junction redirection.
func noAGYReparsePoints(path string) bool {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		name, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return false
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return false
		}
		if filepath.Dir(current) == current {
			return true
		}
	}
}

func agyProbeDirectory(localAppData string) (string, error) {
	if !filepath.IsAbs(localAppData) {
		return "", errCLIUnavailable
	}
	dir := filepath.Join(localAppData, "QuotaDock", "agy-probe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errCLIUnavailable
	}
	if err := emptyAGYProbe(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func emptyAGYProbe(dir string) error {
	if !noAGYReparsePoints(dir) {
		return errCLIUnavailable
	}
	entries, err := os.ReadDir(dir)
	// Never delete unexpected files: refuse to run in a contaminated directory.
	if err != nil || len(entries) != 0 {
		return errCLIUnavailable
	}
	return nil
}

func runAGY(ctx context.Context, spec process.CommandSpec, log process.LogFunc) ([]byte, error) {
	if err := emptyAGYProbe(spec.Dir); err != nil {
		return nil, err
	}
	return agyRunner(log, &agyProcessTree{}).RunOutput(ctx, spec)
}

func agyRunner(log process.LogFunc, tree process.TreeController) process.Runner {
	return process.Runner{Timeout: cliUsageTimeout, MaxOutputBytes: cliOutputLimit,
		MaxStderrBytes: cliOutputLimit, Log: log, Tree: tree}
}

// RunOutput waits for output pipes before its final Terminate call. Closing the
// job from cmd.Cancel kills descendants even when they retain those pipes.
type agyProcessTree struct {
	mu      sync.Mutex
	job     windows.Handle
	stopped bool
}

func (t *agyProcessTree) Prepare(cmd *exec.Cmd) error {
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Cancel = func() error { return t.Terminate(cmd) }
	return nil
}

func (t *agyProcessTree) Attach(cmd *exec.Cmd) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return context.Canceled
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	err = windows.AssignProcessToJobObject(job, handle)
	windows.CloseHandle(handle)
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	t.job = job
	return nil
}

func (t *agyProcessTree) Terminate(cmd *exec.Cmd) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return nil
	}
	t.stopped = true
	if t.job != 0 {
		err := windows.CloseHandle(t.job)
		t.job = 0
		return err
	}
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
