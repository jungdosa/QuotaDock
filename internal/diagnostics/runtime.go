package diagnostics

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type Runtime struct {
	directory string
	version   string
	run       string
	now       func() time.Time
	normal    *FileLogger

	crashMu sync.Mutex
	crash   *FileLogger

	sessionMu sync.Mutex
	started   time.Time
	active    bool
}

type RuntimeOptions struct {
	Directory string
	Version   string
	RunID     string
	Level     slog.Level
	Now       func() time.Time
}

func LocalDataDirectory() (string, error) {
	if runtime.GOOS == "windows" {
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			return "", fmt.Errorf("LOCALAPPDATA is unavailable")
		}
		return filepath.Join(root, "QuotaDock"), nil
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate local application data: %w", err)
	}
	return filepath.Join(root, "QuotaDock"), nil
}

func NewDefault(version string) (*Runtime, error) {
	directory, err := LocalDataDirectory()
	if err != nil {
		return nil, err
	}
	run, err := NewRunID()
	if err != nil {
		return nil, err
	}
	return NewRuntime(RuntimeOptions{
		Directory: directory,
		Version:   version,
		RunID:     run,
		Level:     LevelFromEnvironment(os.Getenv),
	})
}

func NewRuntime(options RuntimeOptions) (*Runtime, error) {
	if options.Directory == "" || options.Version == "" || options.RunID == "" {
		return nil, fmt.Errorf("diagnostic directory, version, and run id are required")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	normal, err := newFileLogger(filepath.Join(options.Directory, NormalLogName), options.Version, options.RunID, NormalMaxBytes, options.Level, appEvents)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		directory: options.Directory,
		version:   options.Version,
		run:       options.RunID,
		now:       options.Now,
		normal:    normal,
	}, nil
}

func (r *Runtime) Logger() *slog.Logger { return r.normal.Logger() }
func (r *Runtime) RunID() string        { return r.run }

var activeRuntime atomic.Pointer[Runtime]

// Install routes slog to the bounded local file and makes panic recovery
// available to every application goroutine.
func (r *Runtime) Install() func() {
	previousLogger := slog.Default()
	previousRuntime := activeRuntime.Swap(r)
	slog.SetDefault(r.Logger())
	return func() {
		activeRuntime.Store(previousRuntime)
		slog.SetDefault(previousLogger)
	}
}

// BeginSession creates the empty marker. A marker left by the prior process is
// evidence of an unclean exit and is recorded before the marker is replaced.
func (r *Runtime) BeginSession() error {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	if r.active {
		return nil
	}
	marker := filepath.Join(r.directory, SessionFileName)
	if _, err := os.Stat(marker); err == nil {
		if recordErr := r.recordCrash("previous_unclean", "session", "", nil); recordErr != nil {
			return recordErr
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect diagnostic session marker: %w", err)
	}
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create diagnostic session marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close diagnostic session marker: %w", err)
	}
	r.started = r.now()
	r.active = true
	return nil
}

func (r *Runtime) LogStart(attrs ...any) {
	r.Logger().Info("app.start", attrs...)
}

// EndSession is idempotent so update and tray shutdown paths can safely share
// it. The marker is removed before the normal exit event is emitted.
func (r *Runtime) EndSession(reason string) error {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	if !r.active {
		return nil
	}
	if err := os.Remove(filepath.Join(r.directory, SessionFileName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove diagnostic session marker: %w", err)
	}
	uptime := int64(r.now().Sub(r.started) / time.Second)
	if uptime < 0 {
		uptime = 0
	}
	r.Logger().Info("app.exit", "reason", reason, "uptime_s", uptime)
	r.active = false
	return nil
}

func (r *Runtime) crashLogger() (*slog.Logger, error) {
	r.crashMu.Lock()
	defer r.crashMu.Unlock()
	if r.crash != nil {
		return r.crash.Logger(), nil
	}
	logger, err := newFileLogger(filepath.Join(r.directory, CrashLogName), r.version, r.run, CrashMaxBytes, slog.LevelDebug, []string{"crash"})
	if err != nil {
		return nil, err
	}
	r.crash = logger
	return logger.Logger(), nil
}

func (r *Runtime) recordCrash(kind, component, value string, stack []byte) error {
	logger, err := r.crashLogger()
	if err != nil {
		return err
	}
	attrs := []any{"kind", kind, "component", component}
	if value != "" {
		attrs = append(attrs, "panic", value)
	}
	if len(stack) > 0 {
		attrs = append(attrs, "stack", string(stack))
	}
	logger.Error("crash", attrs...)
	r.crashMu.Lock()
	defer r.crashMu.Unlock()
	if r.crash == nil {
		return nil
	}
	return r.crash.Sync()
}

func (r *Runtime) RecordFailure(component string, value any) error {
	return r.recordCrash("fatal_error", component, fmt.Sprint(value), debug.Stack())
}

func (r *Runtime) RecordPanic(component string, value any, stack []byte) error {
	return r.recordCrash("panic", component, fmt.Sprint(value), stack)
}

func (r *Runtime) Close() error {
	var crashErr error
	r.crashMu.Lock()
	if r.crash != nil {
		crashErr = r.crash.Close()
		r.crash = nil
	}
	r.crashMu.Unlock()
	return firstError(r.normal.Close(), crashErr)
}

func firstError(left, right error) error {
	if left != nil {
		return left
	}
	return right
}

// Recover records and rethrows a panic so the process still terminates as an
// abnormal run and the session marker remains for the next launch.
func Recover(component string) {
	value := recover()
	if value == nil {
		return
	}
	if runtime := activeRuntime.Load(); runtime != nil {
		_ = runtime.RecordPanic(component, value, debug.Stack())
	}
	panic(value)
}

// Go is the only application-owned goroutine entry point. Its top-level
// recovery records evidence before preserving Go's fatal panic behavior.
func Go(component string, fn func()) {
	go func() {
		defer Recover(component)
		fn()
	}()
}

func AfterFunc(delay time.Duration, component string, fn func()) *time.Timer {
	return time.AfterFunc(delay, func() {
		defer Recover(component)
		fn()
	})
}
