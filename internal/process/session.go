package process

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jungdosa/QuotaDock/internal/diagnostics"
	"github.com/jungdosa/QuotaDock/internal/security"
)

var ErrRPCResponse = errors.New("RPC request failed")

type NotificationHandler func(method string, params json.RawMessage)

// JSONLSession owns one bounded, shell-free stdio JSON-RPC process.
type JSONLSession struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	tree       TreeController
	mux        *RPCMux
	maxLine    int
	log        LogFunc
	done       chan struct{}
	stderrDone chan struct{}
	stderr     *limitedBuffer

	writeMu   sync.Mutex
	handlerMu sync.RWMutex
	handler   NotificationHandler
	closeOnce sync.Once
	nextID    atomic.Uint64
}

func StartJSONLSession(spec CommandSpec, runner Runner) (*JSONLSession, error) {
	if err := ValidateCommand(spec); err != nil {
		return nil, err
	}
	cmd := exec.Command(spec.Name, spec.Args...)
	configureCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdin: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open stderr: %w", err)
	}
	lineLimit, _, stderrLimit := runner.limits()
	tree := runner.Tree
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

	session := &JSONLSession{
		cmd: cmd, stdin: stdin, tree: tree, mux: NewRPCMux(), maxLine: lineLimit,
		log: runner.Log, done: make(chan struct{}), stderrDone: make(chan struct{}),
		stderr: &limitedBuffer{limit: stderrLimit},
	}
	diagnostics.Go("session_stderr", func() {
		_, _ = io.Copy(session.stderr, stderrPipe)
		close(session.stderrDone)
	})
	diagnostics.Go("session_read", func() { session.readLoop(stdout) })
	diagnostics.Go("session_wait", func() {
		_ = cmd.Wait()
		<-session.stderrDone
		if session.stderr.Len() > 0 && session.log != nil {
			session.log(security.MaskSecrets(session.stderr.String()))
		}
		session.mux.CloseAll()
		close(session.done)
	})
	return session, nil
}

func (s *JSONLSession) SetNotificationHandler(handler NotificationHandler) {
	s.handlerMu.Lock()
	s.handler = handler
	s.handlerMu.Unlock()
}

func (s *JSONLSession) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, min(s.maxLine, 64<<10)), s.maxLine)
	for scanner.Scan() {
		line := append(json.RawMessage(nil), scanner.Bytes()...)
		if !json.Valid(line) {
			if s.log != nil {
				s.log("ignored invalid JSONL record")
			}
			continue
		}
		if s.mux.Deliver(line) {
			continue
		}
		var notification struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(line, &notification) != nil || notification.Method == "" {
			continue
		}
		s.handlerMu.RLock()
		handler := s.handler
		s.handlerMu.RUnlock()
		if handler != nil {
			handler(notification.Method, append(json.RawMessage(nil), notification.Params...))
		}
	}
}

func (s *JSONLSession) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := strconv.FormatUint(s.nextID.Add(1), 10)
	response, err := s.mux.Register(id)
	if err != nil {
		return nil, err
	}
	defer s.mux.Cancel(id)
	if err := s.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case line, ok := <-response:
		if !ok {
			return nil, ErrProcessExited
		}
		var envelope struct {
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &envelope) != nil {
			return nil, ErrRPCResponse
		}
		if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
			return nil, ErrRPCResponse
		}
		if len(envelope.Result) == 0 {
			return json.RawMessage(`null`), nil
		}
		return append(json.RawMessage(nil), envelope.Result...), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, ErrProcessExited
	}
}

func (s *JSONLSession) Notify(method string, params any) error {
	return s.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (s *JSONLSession) write(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.done:
		return ErrProcessExited
	default:
	}
	_, err = s.stdin.Write(data)
	if err != nil {
		return ErrProcessExited
	}
	return nil
}

func (s *JSONLSession) Done() <-chan struct{} { return s.done }

func (s *JSONLSession) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		closeErr = s.tree.Terminate(s.cmd)
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			if s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
			<-s.done
		}
	})
	return closeErr
}
