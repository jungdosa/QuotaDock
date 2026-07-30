// Package process provides bounded, shell-free process and JSONL primitives.
package process

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/security"
)

var (
	ErrTimeout       = errors.New("process request timed out")
	ErrProcessExited = errors.New("process exited abnormally")
	ErrOutputLimit   = errors.New("process output limit exceeded")
)

type CommandSpec struct {
	Name string
	Args []string
}
type LogFunc func(string)

// TreeController abstracts platform process-tree preparation and cleanup.
type TreeController interface {
	Prepare(*exec.Cmd) error
	Attach(*exec.Cmd) error
	Terminate(*exec.Cmd) error
}

type Runner struct {
	Timeout        time.Duration
	MaxLineBytes   int
	MaxOutputBytes int
	MaxStderrBytes int
	Log            LogFunc
	Tree           TreeController
}

func (r Runner) limits() (int, int, int) {
	line := r.MaxLineBytes
	if line <= 0 {
		line = 256 << 10
	}
	output := r.MaxOutputBytes
	if output <= 0 {
		output = 1 << 20
	}
	stderr := r.MaxStderrBytes
	if stderr <= 0 {
		stderr = 64 << 10
	}
	return line, output, stderr
}

func ValidateCommand(spec CommandSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("executable name is required")
	}
	switch strings.ToLower(filepath.Base(spec.Name)) {
	case "cmd", "cmd.exe", "sh", "sh.exe", "bash", "bash.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return errors.New("shell executables are not allowed")
	}
	for _, arg := range spec.Args {
		lower := strings.ToLower(arg)
		for _, marker := range []string{"--api-key", "api_key=", "access_token=", "refresh_token=", "authorization:", "bearer "} {
			if strings.Contains(lower, marker) {
				return fmt.Errorf("sensitive command argument is not allowed")
			}
		}
	}
	return nil
}

func (r Runner) RunJSONL(ctx context.Context, spec CommandSpec) ([]json.RawMessage, error) {
	if err := ValidateCommand(spec); err != nil {
		return nil, err
	}
	runCtx := ctx
	cancel := func() {}
	if r.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.Timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, spec.Name, spec.Args...)
	configureCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout: %w", err)
	}
	_, _, stderrLimit := r.limits()
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stderr = stderr
	tree := r.Tree
	if tree == nil {
		tree = &platformTreeController{}
	}
	if err := tree.Prepare(cmd); err != nil {
		return nil, fmt.Errorf("prepare process tree: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}
	if err := tree.Attach(cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("attach process tree: %w", err)
	}
	finished := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			_ = tree.Terminate(cmd)
		case <-finished:
		}
	}()
	records, scanErr := r.parse(stdout)
	if scanErr != nil {
		_ = tree.Terminate(cmd)
	}
	waitErr := cmd.Wait()
	close(finished)
	_ = tree.Terminate(cmd)
	if stderr.Len() > 0 && r.Log != nil {
		r.Log(security.MaskSecrets(stderr.String()))
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return records, ErrTimeout
	}
	if scanErr != nil {
		return records, scanErr
	}
	if waitErr != nil {
		return records, fmt.Errorf("%w", ErrProcessExited)
	}
	return records, nil
}

func (r Runner) parse(reader io.Reader) ([]json.RawMessage, error) {
	lineLimit, outputLimit, _ := r.limits()
	return ParseJSONLines(reader, lineLimit, outputLimit, r.Log)
}

func ParseJSONLines(reader io.Reader, maxLineBytes, maxOutputBytes int, log LogFunc) ([]json.RawMessage, error) {
	if maxLineBytes <= 0 || maxOutputBytes <= 0 {
		return nil, errors.New("positive JSONL limits are required")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, min(maxLineBytes, 64<<10)), maxLineBytes)
	var output []json.RawMessage
	total := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		total += len(line)
		if total > maxOutputBytes {
			return output, ErrOutputLimit
		}
		if !json.Valid(line) {
			if log != nil {
				log("ignored invalid JSONL record")
			}
			continue
		}
		copyLine := append(json.RawMessage(nil), line...)
		output = append(output, copyLine)
	}
	if err := scanner.Err(); err != nil {
		return output, fmt.Errorf("scan JSONL: %w", err)
	}
	return output, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - b.Len()
	if original > remaining {
		b.exceeded = true
	}
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return original, nil
}

type Backoff struct {
	Base    time.Duration
	Maximum time.Duration
}

func (b Backoff) Delay(attempt int) time.Duration {
	base := b.Base
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	maximum := b.Maximum
	if maximum <= 0 {
		maximum = 30 * time.Second
	}
	if attempt < 0 {
		attempt = 0
	}
	delay := base
	for i := 0; i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

// Group coalesces concurrent calls into one in-flight operation.
type Group[T any] struct {
	mu      sync.Mutex
	current *groupCall[T]
}
type groupCall[T any] struct {
	done  chan struct{}
	value T
	err   error
}

func (g *Group[T]) Do(ctx context.Context, fn func() (T, error)) (T, error) {
	g.mu.Lock()
	call := g.current
	if call == nil {
		call = &groupCall[T]{done: make(chan struct{})}
		g.current = call
		g.mu.Unlock()
		call.value, call.err = fn()
		close(call.done)
		g.mu.Lock()
		if g.current == call {
			g.current = nil
		}
		g.mu.Unlock()
	} else {
		g.mu.Unlock()
	}
	select {
	case <-call.done:
		return call.value, call.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// RPCMux matches JSON-RPC response IDs without retaining unrelated payloads.
type RPCMux struct {
	mu      sync.Mutex
	pending map[string]chan json.RawMessage
}

func NewRPCMux() *RPCMux { return &RPCMux{pending: make(map[string]chan json.RawMessage)} }
func (m *RPCMux) Register(id string) (<-chan json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.pending[id]; exists {
		return nil, errors.New("duplicate request ID")
	}
	ch := make(chan json.RawMessage, 1)
	m.pending[id] = ch
	return ch, nil
}
func (m *RPCMux) Deliver(line json.RawMessage) bool {
	var envelope struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(line, &envelope) != nil || len(envelope.ID) == 0 {
		return false
	}
	id := strings.Trim(string(envelope.ID), `"`)
	m.mu.Lock()
	ch := m.pending[id]
	if ch != nil {
		delete(m.pending, id)
	}
	m.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- append(json.RawMessage(nil), line...)
	close(ch)
	return true
}
func (m *RPCMux) CloseAll() {
	m.mu.Lock()
	for id, ch := range m.pending {
		delete(m.pending, id)
		close(ch)
	}
	m.mu.Unlock()
}
func (m *RPCMux) Cancel(id string) {
	m.mu.Lock()
	if ch := m.pending[id]; ch != nil {
		delete(m.pending, id)
		close(ch)
	}
	m.mu.Unlock()
}

// PIDArgument is kept here so every platform tree implementation uses fixed args.
func pidArgument(cmd *exec.Cmd) string {
	if cmd == nil || cmd.Process == nil {
		return ""
	}
	return strconv.Itoa(cmd.Process.Pid)
}
