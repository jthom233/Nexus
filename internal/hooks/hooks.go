// Package hooks provides connection lifecycle hook execution for Nexus.
//
// Hooks run shell commands before/after connect and disconnect events,
// with configurable failure handling (abort, warn, or ignore).
package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// HookEvent identifies when a hook fires in the connection lifecycle.
type HookEvent string

const (
	PreConnect      HookEvent = "pre_connect"
	PostConnect     HookEvent = "post_connect"
	PreDisconnect   HookEvent = "pre_disconnect"
	PostDisconnect  HookEvent = "post_disconnect"
)

// OnFailure determines what happens when a hook command fails.
type OnFailure string

const (
	FailAbort  OnFailure = "abort"
	FailWarn   OnFailure = "warn"
	FailIgnore OnFailure = "ignore"
)

// Hook defines a single lifecycle hook.
type Hook struct {
	Event     HookEvent     `yaml:"event"`
	Command   string        `yaml:"command"`
	OnFailure OnFailure     `yaml:"on_failure"`
	Timeout   time.Duration `yaml:"timeout"`
}

// DefaultTimeout is used when a hook does not specify a timeout.
const DefaultTimeout = 30 * time.Second

// HookResult captures the outcome of running a single hook.
type HookResult struct {
	Hook     Hook
	Output   string
	Error    error
	Duration time.Duration
}

// Hooks groups lifecycle hooks by event.
type Hooks struct {
	PreConnect     []Hook `yaml:"pre_connect,omitempty"`
	PostConnect    []Hook `yaml:"post_connect,omitempty"`
	PreDisconnect  []Hook `yaml:"pre_disconnect,omitempty"`
	PostDisconnect []Hook `yaml:"post_disconnect,omitempty"`
}

// ForEvent returns the hook list for the given event.
func (h Hooks) ForEvent(event HookEvent) []Hook {
	switch event {
	case PreConnect:
		return h.PreConnect
	case PostConnect:
		return h.PostConnect
	case PreDisconnect:
		return h.PreDisconnect
	case PostDisconnect:
		return h.PostDisconnect
	}
	return nil
}

// IsEmpty returns true if no hooks are configured.
func (h Hooks) IsEmpty() bool {
	return len(h.PreConnect) == 0 &&
		len(h.PostConnect) == 0 &&
		len(h.PreDisconnect) == 0 &&
		len(h.PostDisconnect) == 0
}

// HookRunner executes lifecycle hooks.
type HookRunner struct{}

// NewHookRunner creates a new HookRunner.
func NewHookRunner() *HookRunner {
	return &HookRunner{}
}

// RunHooks executes a list of hooks sequentially, respecting each hook's
// on-failure policy. Environment variables are merged with the process env
// and passed to each command.
//
// If any hook with FailAbort fails, execution stops and an error is returned.
// FailWarn hooks produce a non-nil HookResult.Error but do not stop execution.
// FailIgnore hooks silently continue on failure.
func (r *HookRunner) RunHooks(ctx context.Context, event HookEvent, hks []Hook, env map[string]string) ([]HookResult, error) {
	results := make([]HookResult, 0, len(hks))

	for _, h := range hks {
		result := r.runOne(ctx, h, env)
		results = append(results, result)

		if result.Error != nil {
			switch h.OnFailure {
			case FailAbort:
				return results, fmt.Errorf("hook %q (event %s) failed: %w", h.Command, event, result.Error)
			case FailWarn:
				// Caller can inspect result.Error for logging; we continue.
				continue
			case FailIgnore:
				continue
			default:
				// Unknown on_failure — treat as warn for safety.
				continue
			}
		}
	}

	return results, nil
}

// runOne executes a single hook command via "sh -c" with timeout.
func (r *HookRunner) runOne(ctx context.Context, h Hook, env map[string]string) HookResult {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command)

	// Build environment
	envSlice := make([]string, 0, len(env))
	for k, v := range env {
		envSlice = append(envSlice, k+"="+v)
	}
	cmd.Env = append(cmd.Environ(), envSlice...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := stdout.String()
	if se := stderr.String(); se != "" {
		if output != "" {
			output += "\n"
		}
		output += se
	}

	return HookResult{
		Hook:     h,
		Output:   output,
		Error:    err,
		Duration: duration,
	}
}

// ConnectionEnv builds the standard environment variables for hook execution.
func ConnectionEnv(connID, host string, port int, user, protocol string) map[string]string {
	return map[string]string{
		"NEXUS_CONNECTION_ID": connID,
		"NEXUS_HOST":          host,
		"NEXUS_PORT":          fmt.Sprintf("%d", port),
		"NEXUS_USER":          user,
		"NEXUS_PROTOCOL":      protocol,
	}
}
