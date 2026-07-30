package process

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInvalidJSONIsIgnored(t *testing.T) {
	var logs []string
	records, err := ParseJSONLines(strings.NewReader("not-json\n{\"id\":1}\n"), 1024, 4096, func(value string) { logs = append(logs, value) })
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(logs) != 1 {
		t.Fatalf("records=%d logs=%v", len(records), logs)
	}
}

func TestRequestTimeout(t *testing.T) {
	runner := Runner{Timeout: 40 * time.Millisecond, Tree: noOpTree{}}
	_, err := runner.RunJSONL(context.Background(), helperSpec("sleep"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestAbnormalExitAndRestartBackoff(t *testing.T) {
	runner := Runner{Timeout: time.Second, Tree: noOpTree{}}
	_, err := runner.RunJSONL(context.Background(), helperSpec("exit"))
	if !errors.Is(err, ErrProcessExited) {
		t.Fatalf("exit error = %v", err)
	}
	backoff := Backoff{Base: 100 * time.Millisecond, Maximum: 450 * time.Millisecond}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 450 * time.Millisecond}
	for attempt, expected := range want {
		if got := backoff.Delay(attempt); got != expected {
			t.Errorf("attempt %d = %v, want %v", attempt, got, expected)
		}
	}
}

func TestChildProcessTreeCleanup(t *testing.T) {
	tree := &recordingTree{}
	runner := Runner{Timeout: time.Second, Tree: tree}
	if _, err := runner.RunJSONL(context.Background(), helperSpec("jsonl")); err != nil {
		t.Fatal(err)
	}
	if tree.prepared.Load() != 1 || tree.attached.Load() != 1 || tree.terminated.Load() == 0 {
		t.Fatalf("prepare=%d attach=%d terminate=%d", tree.prepared.Load(), tree.attached.Load(), tree.terminated.Load())
	}
}

func TestStderrIsMaskedBeforeLogging(t *testing.T) {
	var log string
	runner := Runner{Timeout: time.Second, Tree: noOpTree{}, Log: func(value string) { log += value }}
	if _, err := runner.RunJSONL(context.Background(), helperSpec("stderr")); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-value", "person@example.invalid"} {
		if strings.Contains(log, secret) {
			t.Fatalf("log retained %q: %s", secret, log)
		}
	}
}

func TestRPCRequestResponseMatching(t *testing.T) {
	mux := NewRPCMux()
	one, _ := mux.Register("1")
	two, _ := mux.Register("2")
	if !mux.Deliver(json.RawMessage(`{"id":2,"result":"second"}`)) || !mux.Deliver(json.RawMessage(`{"id":1,"result":"first"}`)) {
		t.Fatal("response not delivered")
	}
	if !strings.Contains(string(<-one), "first") || !strings.Contains(string(<-two), "second") {
		t.Fatal("IDs were mismatched")
	}
}

func TestInflightRequestsAreCoalesced(t *testing.T) {
	var group Group[int]
	var calls atomic.Int32
	start := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := group.Do(context.Background(), func() (int, error) { calls.Add(1); close(start); <-release; return 7, nil })
			if err != nil {
				t.Error(err)
			}
			results <- value
		}()
	}
	<-start
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)
	if calls.Load() != 1 {
		t.Fatalf("underlying calls = %d", calls.Load())
	}
	for value := range results {
		if value != 7 {
			t.Fatalf("result = %d", value)
		}
	}
}

func TestJSONLSessionRequestAndCloseCleansProcessTree(t *testing.T) {
	tree := &killingTree{}
	session, err := StartJSONLSession(helperSpec("jsonl-session"), Runner{Tree: tree, MaxLineBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := session.Request(ctx, "initialize", map[string]any{"safe": true})
	if err != nil || !strings.Contains(string(response), "ok") {
		t.Fatalf("response=%s error=%v", response, err)
	}
	if err := session.Notify("initialized", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if tree.terminated.Load() == 0 {
		t.Fatal("session close did not terminate its process tree")
	}
}

func TestSensitiveCommandArgumentsRejected(t *testing.T) {
	for _, arg := range []string{"--api-key", "access_token=fake", "Authorization: Bearer fake"} {
		if err := ValidateCommand(CommandSpec{Name: "tool.exe", Args: []string{arg}}); err == nil {
			t.Errorf("accepted sensitive arg %q", arg)
		}
	}
	if err := ValidateCommand(CommandSpec{Name: "cmd.exe", Args: []string{"/c", "echo"}}); err == nil {
		t.Fatal("accepted shell executable")
	}
}

type noOpTree struct{}

func (noOpTree) Prepare(*exec.Cmd) error   { return nil }
func (noOpTree) Attach(*exec.Cmd) error    { return nil }
func (noOpTree) Terminate(*exec.Cmd) error { return nil }

type killingTree struct {
	terminated atomic.Int32
}

func (t *killingTree) Prepare(*exec.Cmd) error { return nil }
func (t *killingTree) Attach(*exec.Cmd) error  { return nil }
func (t *killingTree) Terminate(cmd *exec.Cmd) error {
	t.terminated.Add(1)
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

type recordingTree struct {
	prepared   atomic.Int32
	attached   atomic.Int32
	terminated atomic.Int32
}

func (t *recordingTree) Prepare(*exec.Cmd) error   { t.prepared.Add(1); return nil }
func (t *recordingTree) Attach(*exec.Cmd) error    { t.attached.Add(1); return nil }
func (t *recordingTree) Terminate(*exec.Cmd) error { t.terminated.Add(1); return nil }

func helperSpec(mode string) CommandSpec {
	return CommandSpec{Name: os.Args[0], Args: []string{"-test.run=TestProcessHelper", "--", mode}}
}
func TestProcessHelper(t *testing.T) {
	isHelper := false
	for _, argument := range os.Args {
		if argument == "-test.run=TestProcessHelper" {
			isHelper = true
			break
		}
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if !isHelper || separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	mode := os.Args[separator+1]
	switch mode {
	case "sleep":
		time.Sleep(time.Second)
	case "exit":
		os.Exit(7)
	case "jsonl":
		fmt.Println(`{"id":1,"result":"ok"}`)
	case "stderr":
		fmt.Fprintln(os.Stderr, "access_token=fake-value person@example.invalid")
	case "jsonl-session":
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) == nil && len(request.ID) > 0 {
				fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"status":"ok"}}`+"\n", request.ID)
			}
		}
		time.Sleep(10 * time.Second)
	case "tree-parent":
		if separator+2 >= len(os.Args) {
			os.Exit(2)
		}
		child := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestProcessHelper", "--", "tree-child")
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Args[separator+2], []byte(fmt.Sprint(child.Process.Pid)), 0o600); err != nil {
			os.Exit(4)
		}
		fmt.Println("{\"ready\":true}")
		time.Sleep(10 * time.Second)
	case "tree-child":
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}
