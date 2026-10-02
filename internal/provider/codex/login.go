package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/process"
)

type loginEvent struct {
	LoginID string `json:"loginId"`
	Success bool   `json:"success"`
}

// NewAccountTransport isolates added accounts from the user's CLI profile.
// The official CLI owns credentials; QuotaDock never reads or copies tokens.
func NewAccountTransport(home string, log process.LogFunc) *AppServerTransport {
	t := NewAppServerTransport(log)
	t.home = home
	return t
}

func (t *AppServerTransport) command(path string, args ...string) process.CommandSpec {
	spec := process.CommandSpec{Name: path, Args: args}
	if t.home == "" {
		return spec
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "CODEX_HOME", "OPENAI_API_KEY", "CODEX_API_KEY", "OPENAI_BASE_URL", "CODEX_SQLITE_HOME":
			continue
		}
		spec.Env = append(spec.Env, entry)
	}
	spec.Env = append(spec.Env, "CODEX_HOME="+t.home)
	spec.Dir = t.home
	spec.Args = append([]string{"-c", `cli_auth_credentials_store="file"`, "-c", `forced_login_method="chatgpt"`}, args...)
	return spec
}

func (t *AppServerTransport) deliverLoginEvent(raw json.RawMessage) {
	var event loginEvent
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loginEvents != nil {
		select {
		case t.loginEvents <- event:
		default:
		}
	}
}

func allowedLoginURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil &&
		(u.Port() == "" || u.Port() == "443") &&
		(u.Hostname() == "auth.openai.com" || u.Hostname() == "chatgpt.com" || u.Hostname() == "auth.chatgpt.com")
}

// SignIn uses the official managed browser flow, only for an isolated home.
// Callers serialize account operations; no inference method is ever invoked.
func (t *AppServerTransport) SignIn(ctx context.Context, open func(string) error) error {
	if t.home == "" || open == nil {
		return errors.New("isolated Codex sign-in required")
	}
	if _, err := t.Request(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "QuotaDock", "version": "1"}}); err != nil {
		return safeTransportError(err)
	}
	if err := t.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return safeTransportError(err)
	}
	events := make(chan loginEvent, 4)
	t.mu.Lock()
	t.loginEvents = events
	t.mu.Unlock()
	defer func() { t.mu.Lock(); t.loginEvents = nil; t.mu.Unlock() }()
	raw, err := t.Request(ctx, "account/login/start", map[string]string{"type": "chatgpt"})
	if err != nil {
		return safeTransportError(err)
	}
	var result struct {
		LoginID string `json:"loginId"`
		AuthURL string `json:"authUrl"`
	}
	if json.Unmarshal(raw, &result) != nil || result.LoginID == "" {
		return model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	completed := false
	defer func() {
		if !completed {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = t.Request(cancelCtx, "account/login/cancel", map[string]string{"loginId": result.LoginID})
		}
	}()
	if !allowedLoginURL(result.AuthURL) {
		return model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	if err := open(result.AuthURL); err != nil {
		return model.SafeError{Code: model.ErrUnavailable, Key: "error.unavailable"}
	}
	t.mu.Lock()
	session := t.session
	t.mu.Unlock()
	if session == nil {
		return safeTransportError(process.ErrProcessExited)
	}
	for {
		select {
		case <-ctx.Done():
			return model.SafeError{Code: model.ErrTimeout, Key: "error.timeout"}
		case <-session.Done():
			return safeTransportError(process.ErrProcessExited)
		case event := <-events:
			if event.LoginID != result.LoginID {
				continue
			}
			completed = true
			if !event.Success {
				return model.SafeError{Code: model.ErrNotLoggedIn, Key: "error.not_logged_in"}
			}
			return nil
		}
	}
}
