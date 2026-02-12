package hooks

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSuccessfulHookExecution(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "echo hello", OnFailure: FailAbort, Timeout: 5 * time.Second},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("expected no error, got %v", results[0].Error)
	}
	if got := strings.TrimSpace(results[0].Output); got != "hello" {
		t.Fatalf("expected output 'hello', got %q", got)
	}
	if results[0].Duration <= 0 {
		t.Fatal("expected positive duration")
	}
}

func TestHookTimeout(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "sleep 10", OnFailure: FailAbort, Timeout: 100 * time.Millisecond},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err == nil {
		t.Fatal("expected error from timed-out hook")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Fatal("expected error in result")
	}
}

func TestFailAbortStopsExecution(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "exit 1", OnFailure: FailAbort, Timeout: 5 * time.Second},
		{Event: PreConnect, Command: "echo should-not-run", OnFailure: FailAbort, Timeout: 5 * time.Second},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err == nil {
		t.Fatal("expected error from FailAbort hook")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (second hook should not run), got %d", len(results))
	}
}

func TestFailWarnContinues(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "exit 1", OnFailure: FailWarn, Timeout: 5 * time.Second},
		{Event: PreConnect, Command: "echo continued", OnFailure: FailAbort, Timeout: 5 * time.Second},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Fatal("expected error in first result (warn)")
	}
	if results[1].Error != nil {
		t.Fatalf("expected no error in second result, got %v", results[1].Error)
	}
	if got := strings.TrimSpace(results[1].Output); got != "continued" {
		t.Fatalf("expected 'continued', got %q", got)
	}
}

func TestFailIgnoreContinues(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "exit 1", OnFailure: FailIgnore, Timeout: 5 * time.Second},
		{Event: PreConnect, Command: "echo ok", OnFailure: FailAbort, Timeout: 5 * time.Second},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Fatal("expected error in first result (ignore)")
	}
	if got := strings.TrimSpace(results[1].Output); got != "ok" {
		t.Fatalf("expected 'ok', got %q", got)
	}
}

func TestEnvironmentVariablesPassed(t *testing.T) {
	runner := NewHookRunner()
	env := ConnectionEnv("test-conn", "10.0.0.1", 22, "admin", "ssh")

	hks := []Hook{
		{
			Event:     PreConnect,
			Command:   "echo $NEXUS_HOST:$NEXUS_PORT:$NEXUS_USER:$NEXUS_PROTOCOL:$NEXUS_CONNECTION_ID",
			OnFailure: FailAbort,
			Timeout:   5 * time.Second,
		},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(results[0].Output)
	expected := "10.0.0.1:22:admin:ssh:test-conn"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestMultipleHooksRunInOrder(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "echo first", OnFailure: FailAbort, Timeout: 5 * time.Second},
		{Event: PreConnect, Command: "echo second", OnFailure: FailAbort, Timeout: 5 * time.Second},
		{Event: PreConnect, Command: "echo third", OnFailure: FailAbort, Timeout: 5 * time.Second},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := []string{"first", "second", "third"}
	for i, exp := range expected {
		got := strings.TrimSpace(results[i].Output)
		if got != exp {
			t.Fatalf("result[%d]: expected %q, got %q", i, exp, got)
		}
	}
}

func TestDefaultTimeout(t *testing.T) {
	runner := NewHookRunner()
	hks := []Hook{
		{Event: PreConnect, Command: "echo fast", OnFailure: FailAbort},
	}

	results, err := runner.RunHooks(context.Background(), PreConnect, hks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("expected no error, got %v", results[0].Error)
	}
}

func TestConnectionEnv(t *testing.T) {
	env := ConnectionEnv("my-server", "192.168.1.1", 2222, "root", "ssh")

	checks := map[string]string{
		"NEXUS_CONNECTION_ID": "my-server",
		"NEXUS_HOST":          "192.168.1.1",
		"NEXUS_PORT":          "2222",
		"NEXUS_USER":          "root",
		"NEXUS_PROTOCOL":      "ssh",
	}
	for k, want := range checks {
		got, ok := env[k]
		if !ok {
			t.Fatalf("missing env var %s", k)
		}
		if got != want {
			t.Fatalf("env %s: expected %q, got %q", k, want, got)
		}
	}
}

func TestHooksForEvent(t *testing.T) {
	h := Hooks{
		PreConnect:     []Hook{{Command: "pre-c"}},
		PostConnect:    []Hook{{Command: "post-c"}},
		PreDisconnect:  []Hook{{Command: "pre-d"}},
		PostDisconnect: []Hook{{Command: "post-d"}},
	}

	if got := h.ForEvent(PreConnect); len(got) != 1 || got[0].Command != "pre-c" {
		t.Fatalf("unexpected ForEvent(PreConnect): %v", got)
	}
	if got := h.ForEvent(PostConnect); len(got) != 1 || got[0].Command != "post-c" {
		t.Fatalf("unexpected ForEvent(PostConnect): %v", got)
	}
	if got := h.ForEvent(PreDisconnect); len(got) != 1 || got[0].Command != "pre-d" {
		t.Fatalf("unexpected ForEvent(PreDisconnect): %v", got)
	}
	if got := h.ForEvent(PostDisconnect); len(got) != 1 || got[0].Command != "post-d" {
		t.Fatalf("unexpected ForEvent(PostDisconnect): %v", got)
	}
	if got := h.ForEvent("unknown"); got != nil {
		t.Fatalf("expected nil for unknown event, got %v", got)
	}
}

func TestHooksIsEmpty(t *testing.T) {
	empty := Hooks{}
	if !empty.IsEmpty() {
		t.Fatal("expected empty hooks to be empty")
	}

	notEmpty := Hooks{PreConnect: []Hook{{Command: "echo hi"}}}
	if notEmpty.IsEmpty() {
		t.Fatal("expected non-empty hooks to not be empty")
	}
}
