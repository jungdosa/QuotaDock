package codex

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/process"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

const MinimumCLIVersion = "0.145.0"

type AppServerTransport struct {
	mu         sync.Mutex
	executable string
	find       func() (string, error)
	runner     process.Runner
	session    *process.JSONLSession
	handler    func(json.RawMessage)
}

func NewAppServerTransport(log process.LogFunc) *AppServerTransport {
	transport := &AppServerTransport{runner: process.Runner{Timeout: 5 * time.Second, MaxLineBytes: 512 << 10, MaxOutputBytes: 1 << 20, MaxStderrBytes: 64 << 10, Log: log}}
	transport.find = findCodexExecutable
	return transport
}

func findCodexExecutable() (string, error) {
	if path, err := exec.LookPath("codex"); err == nil && acceptableExecutable(path) {
		return path, nil
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".codex", "bin", executableName("codex")),
			filepath.Join(home, "AppData", "Local", "Programs", "OpenAI", "Codex", "bin", executableName("codex")),
		)
	}
	for _, candidate := range candidates {
		if acceptableExecutable(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", shared.ErrNotInstalled
}

func executableName(base string) string {
	if filepath.Separator == '\\' {
		return base + ".exe"
	}
	return base
}

func acceptableExecutable(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "codex" || base == "codex.exe"
}

var codexVersionPattern = regexp.MustCompile(`\d+(?:\.\d+){1,2}`)

func (t *AppServerTransport) executablePath() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.executable != "" {
		return t.executable, nil
	}
	path, err := t.find()
	if err != nil {
		return "", err
	}
	t.executable = path
	return path, nil
}

// ExecutablePath exposes the detected official CLI path for connection
// diagnostics. It never returns command arguments or authentication data.
func (t *AppServerTransport) ExecutablePath() (string, error) {
	return t.executablePath()
}

func (t *AppServerTransport) Version(ctx context.Context) (string, error) {
	path, err := t.executablePath()
	if err != nil {
		return "", err
	}
	output, err := t.runner.RunOutput(ctx, process.CommandSpec{Name: path, Args: []string{"--version"}})
	if err != nil {
		return "", err
	}
	version := codexVersionPattern.FindString(string(output))
	if version == "" {
		return "", errors.New("unrecognized Codex CLI version")
	}
	return version, nil
}

func (t *AppServerTransport) ensureSession() (*process.JSONLSession, error) {
	path, err := t.executablePath()
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.session != nil {
		select {
		case <-t.session.Done():
			t.session = nil
		default:
			return t.session, nil
		}
	}
	session, err := process.StartJSONLSession(process.CommandSpec{Name: path, Args: []string{"app-server"}}, t.runner)
	if err != nil {
		return nil, err
	}
	session.SetNotificationHandler(func(method string, params json.RawMessage) {
		if method != "account/rateLimits/updated" {
			return
		}
		t.mu.Lock()
		handler := t.handler
		t.mu.Unlock()
		if handler != nil {
			handler(params)
		}
	})
	t.session = session
	return session, nil
}

func (t *AppServerTransport) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	session, err := t.ensureSession()
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return session.Request(requestCtx, method, params)
}

func (t *AppServerTransport) Notify(ctx context.Context, method string, params any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := t.ensureSession()
	if err != nil {
		return err
	}
	return session.Notify(method, params)
}

func (t *AppServerTransport) SetRateLimitsUpdatedHandler(handler func(json.RawMessage)) {
	t.mu.Lock()
	t.handler = handler
	t.mu.Unlock()
}

func (t *AppServerTransport) takeSession() *process.JSONLSession {
	t.mu.Lock()
	session := t.session
	t.session = nil
	t.mu.Unlock()
	return session
}

func (t *AppServerTransport) Invalidate() {
	session := t.takeSession()
	if session == nil {
		return
	}
	slog.Debug("Codex app-server session invalidated")
	if err := session.Close(); err != nil {
		slog.Debug("Codex app-server session close failed after invalidation", "error", err)
	}
}

func (t *AppServerTransport) Close() error {
	session := t.takeSession()
	if session == nil {
		return nil
	}
	return session.Close()
}

var _ Transport = (*AppServerTransport)(nil)
