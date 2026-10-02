package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/process"
)

func helperTransport(t *testing.T, home string) *AppServerTransport {
	t.Helper()
	transport := NewAccountTransport(home, nil)
	transport.find = func() (string, error) { return os.Executable() }
	transport.startSession = func(spec process.CommandSpec, runner process.Runner) (*process.JSONLSession, error) {
		spec.Args = []string{"-test.run=^TestCodexLoginProtocolHelper$"}
		spec.Env = append(spec.Env, "QD_CODEX_TEST_HELPER=1")
		return process.StartJSONLSession(spec, runner)
	}
	t.Cleanup(func() { _ = transport.Close() })
	return transport
}

func TestCodexLoginProtocolHelper(t *testing.T) {
	if os.Getenv("QD_CODEX_TEST_HELPER") != "1" {
		return
	}
	home := os.Getenv("CODEX_HOME")
	mode := filepath.Base(home)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		result := `{}`
		switch request.Method {
		case "account/read":
			cwd, _ := os.Getwd()
			raw, _ := json.Marshal(map[string]any{"account": nil, "home": home, "cwd": cwd})
			result = string(raw)
		case "account/login/start":
			loginURL := "https://auth.openai.com/authorize?state=synthetic"
			if mode == "unsafe" {
				loginURL = "https://auth.openai.com.evil.invalid/"
			}
			raw, _ := json.Marshal(map[string]string{"type": "chatgpt", "loginId": mode, "authUrl": loginURL})
			result = string(raw)
			// Notifications may arrive before the request response. An unrelated
			// login must not complete this attempt.
			fmt.Println(`{"method":"account/login/completed","params":{"loginId":"unrelated","success":true}}`)
			if mode != "cancel" && mode != "unsafe" {
				fmt.Printf("{\"method\":\"account/login/completed\",\"params\":{\"loginId\":%q,\"success\":%t}}\n", mode, mode != "failure")
			}
		case "account/login/cancel":
			_ = os.WriteFile(filepath.Join(home, "cancelled"), []byte("yes"), 0o600)
		}
		fmt.Printf("{\"id\":%s,\"result\":%s}\n", request.ID, result)
	}
	os.Exit(0)
}

func TestIsolatedCodexProcessesAndBrowserLoginProtocol(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, name := range []string{"two", "three"} {
		home := filepath.Join(root, name)
		transport := helperTransport(t, home)
		opened := false
		if err := transport.SignIn(ctx, func(raw string) error {
			opened = true
			if !allowedLoginURL(raw) {
				t.Fatal("unsafe login URL")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !opened {
			t.Fatal("browser callback not invoked")
		}
		raw, err := transport.Request(ctx, "account/read", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		var got struct{ Home, Cwd string }
		if json.Unmarshal(raw, &got) != nil || !strings.EqualFold(filepath.Clean(got.Home), filepath.Clean(home)) || !strings.EqualFold(filepath.Clean(got.Cwd), filepath.Clean(home)) {
			t.Fatal("child home or working directory was not isolated")
		}
	}
}

func TestCodexLoginRejectsUnsafeURLsAndCancels(t *testing.T) {
	for _, mode := range []string{"unsafe", "cancel", "failure"} {
		t.Run(mode, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), mode)
			transport := helperTransport(t, home)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			opened := false
			err := transport.SignIn(ctx, func(string) error {
				opened = true
				if mode == "cancel" {
					cancel()
				}
				return nil
			})
			if err == nil {
				t.Fatal("unexpected login success")
			}
			if mode == "unsafe" && opened {
				t.Fatal("untrusted URL opened")
			}
			if mode != "failure" {
				if _, err := os.Stat(filepath.Join(home, "cancelled")); err != nil {
					t.Fatal("pending login not cancelled")
				}
			}
		})
	}
}

func TestLiveCodexIsolatedHomesStartLoggedOut(t *testing.T) {
	if os.Getenv("QD_CODEX_ISOLATION_PROBE") != "1" {
		t.Skip("explicit local CLI isolation probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, name := range []string{"two", "three"} {
		transport := NewAccountTransport(filepath.Join(t.TempDir(), name), nil)
		t.Cleanup(func() { _ = transport.Close() })
		if _, err := transport.Request(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "quotadock-isolation-test", "version": "1"}}); err != nil {
			t.Fatal(err)
		}
		if err := transport.Notify(ctx, "initialized", map[string]any{}); err != nil {
			t.Fatal(err)
		}
		raw, err := transport.Request(ctx, "account/read", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Account json.RawMessage `json:"account"`
		}
		if json.Unmarshal(raw, &response) != nil || string(response.Account) != "null" {
			t.Fatal("fresh isolated home inherited an account")
		}
		t.Log("isolated official CLI home starts logged out; no login or inference invoked")
	}
}
