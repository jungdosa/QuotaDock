package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jungdosa/QuotaDock/internal/process"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

const MinimumCLIVersion = "2.1.215"

var ErrRateLimitsUnavailable = errors.New("Claude CLI rate limits are unavailable")

// CLIClient uses only documented, non-interactive Claude CLI commands.
type CLIClient struct {
	executable string
	runner     process.Runner
	find       func() (string, error)
}

func NewCLIClient(log process.LogFunc) *CLIClient {
	client := &CLIClient{runner: process.Runner{Timeout: 5 * time.Second, MaxOutputBytes: 256 << 10, MaxStderrBytes: 64 << 10, Log: log}}
	client.find = findClaudeExecutable
	return client
}

func (c *CLIClient) executablePath() (string, error) {
	if c.executable != "" {
		return c.executable, nil
	}
	path, err := c.find()
	if err != nil {
		return "", err
	}
	c.executable = path
	return path, nil
}

// ExecutablePath exposes the detected official CLI path for connection
// diagnostics. It never returns command arguments or authentication data.
func (c *CLIClient) ExecutablePath() (string, error) {
	return c.executablePath()
}

func findClaudeExecutable() (string, error) {
	if path, err := exec.LookPath("claude"); err == nil && acceptableExecutable(path) {
		return path, nil
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", executableName("claude")))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		candidates = append(candidates,
			filepath.Join(local, "Programs", "Claude", executableName("claude")),
			filepath.Join(local, "AnthropicClaude", executableName("claude")),
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
	return base == "claude" || base == "claude.exe"
}

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+){1,2}`)

func (c *CLIClient) Version(ctx context.Context) (string, error) {
	path, err := c.executablePath()
	if err != nil {
		return "", err
	}
	output, err := c.runner.RunOutput(ctx, process.CommandSpec{Name: path, Args: []string{"--version"}})
	if err != nil {
		return "", err
	}
	version := versionPattern.FindString(string(output))
	if version == "" {
		return "", errors.New("unrecognized Claude CLI version")
	}
	return version, nil
}

func (c *CLIClient) AuthStatus(ctx context.Context) (json.RawMessage, error) {
	path, err := c.executablePath()
	if err != nil {
		return nil, err
	}
	output, err := c.runner.RunOutput(ctx, process.CommandSpec{Name: path, Args: []string{"auth", "status", "--json"}})
	if err != nil {
		return nil, err
	}
	output = []byte(strings.TrimSpace(string(output)))
	if !json.Valid(output) {
		return nil, errors.New("invalid Claude auth response")
	}
	return json.RawMessage(output), nil
}

func (c *CLIClient) RateLimits(ctx context.Context) (json.RawMessage, error) {
	raw, err := c.AuthStatus(ctx)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		RateLimits json.RawMessage `json:"rate_limits"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.RateLimits) == 0 || string(envelope.RateLimits) == "null" {
		return nil, ErrRateLimitsUnavailable
	}
	filtered, err := json.Marshal(struct {
		RateLimits json.RawMessage `json:"rate_limits"`
	}{RateLimits: envelope.RateLimits})
	if err != nil {
		return nil, ErrRateLimitsUnavailable
	}
	return filtered, nil
}

func (c *CLIClient) Close() error { return nil }

var _ Client = (*CLIClient)(nil)
