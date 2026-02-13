package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/dr4zz/nexus/internal/health"
)

// StateChange records a transition in health status for a single connection.
type StateChange struct {
	ConnectionID   string
	ConnectionName string
	OldStatus      health.Status
	NewStatus      health.Status
}

// NotifyManager tracks connection health states and detects transitions.
type NotifyManager struct {
	// previousStatuses stores the last known health status per connection ID.
	previousStatuses map[string]health.Status

	// NotifyEnabled controls whether desktop notifications are sent.
	NotifyEnabled bool
}

// NewNotifyManager creates a NotifyManager ready to track state changes.
func NewNotifyManager() *NotifyManager {
	return &NotifyManager{
		previousStatuses: make(map[string]health.Status),
		NotifyEnabled:    false,
	}
}

// DetectStateChanges compares old and new health results and returns all
// connections whose status has changed. Connections appearing for the first
// time (transitioning from Unknown) are only reported if their new status
// is Online or Offline — initial "Unknown -> Unknown" is suppressed.
func (nm *NotifyManager) DetectStateChanges(oldStatuses, newStatuses map[string]health.Status) []StateChange {
	var changes []StateChange

	for id, newStatus := range newStatuses {
		oldStatus, existed := oldStatuses[id]
		if !existed {
			oldStatus = health.Unknown
		}
		if oldStatus != newStatus {
			// Skip transitions from Unknown to Unknown (no real change).
			if oldStatus == health.Unknown && newStatus == health.Unknown {
				continue
			}
			changes = append(changes, StateChange{
				ConnectionID: id,
				OldStatus:    oldStatus,
				NewStatus:    newStatus,
			})
		}
	}

	return changes
}

// RecordStatuses saves the current set of health statuses as the baseline
// for the next round of change detection.
func (nm *NotifyManager) RecordStatuses(statuses map[string]health.Status) {
	nm.previousStatuses = make(map[string]health.Status, len(statuses))
	for id, s := range statuses {
		nm.previousStatuses[id] = s
	}
}

// PreviousStatuses returns a copy of the previously recorded status map.
func (nm *NotifyManager) PreviousStatuses() map[string]health.Status {
	out := make(map[string]health.Status, len(nm.previousStatuses))
	for k, v := range nm.previousStatuses {
		out[k] = v
	}
	return out
}

// FormatNotification returns a human-friendly message describing a state change.
// Examples:
//
//	"web-server came online"
//	"db-server went offline"
//	"api-server became degraded"
func FormatNotification(change StateChange) string {
	name := change.ConnectionName
	if name == "" {
		name = change.ConnectionID
	}

	switch change.NewStatus {
	case health.Online:
		return fmt.Sprintf("%s came online", name)
	case health.Offline:
		return fmt.Sprintf("%s went offline", name)
	case health.Degraded:
		return fmt.Sprintf("%s became degraded", name)
	default:
		return fmt.Sprintf("%s status changed to %s", name, change.NewStatus.String())
	}
}

// FormatNotificationWithTransition includes the old status for additional context.
// Example: "web-server: offline -> online"
func FormatNotificationWithTransition(change StateChange) string {
	name := change.ConnectionName
	if name == "" {
		name = change.ConnectionID
	}
	return fmt.Sprintf("%s: %s \u2192 %s", name, change.OldStatus.String(), change.NewStatus.String())
}

// DesktopNotify sends a desktop notification using the platform's notification
// system. On Linux it calls notify-send; on other platforms it is a no-op.
// Returns nil if the notification was sent (or skipped on unsupported platforms).
func DesktopNotify(title, body string) error {
	if runtime.GOOS != "linux" {
		return nil // no-op on non-Linux platforms
	}

	path, err := exec.LookPath("notify-send")
	if err != nil {
		// notify-send not available — silent no-op
		return nil
	}

	cmd := exec.Command(path, title, body)
	return cmd.Run()
}

// NotifyAll sends desktop notifications for multiple state changes.
// Each change generates a separate notification with the app title.
func (nm *NotifyManager) NotifyAll(changes []StateChange) {
	if !nm.NotifyEnabled {
		return
	}

	for _, ch := range changes {
		msg := FormatNotification(ch)
		_ = DesktopNotify("Nexus", msg)
	}
}

// PopulateNames enriches StateChange entries with connection names from a
// name lookup map (id -> name). Changes with empty ConnectionName are filled in.
func PopulateNames(changes []StateChange, names map[string]string) {
	for i := range changes {
		if changes[i].ConnectionName == "" {
			if name, ok := names[changes[i].ConnectionID]; ok {
				changes[i].ConnectionName = name
			}
		}
	}
}

// SummarizeChanges returns a compact summary string for multiple changes.
// Example: "2 connections changed: web-server came online, db-server went offline"
func SummarizeChanges(changes []StateChange) string {
	if len(changes) == 0 {
		return ""
	}
	if len(changes) == 1 {
		return FormatNotification(changes[0])
	}

	var parts []string
	for _, ch := range changes {
		parts = append(parts, FormatNotification(ch))
	}
	return fmt.Sprintf("%d connections changed: %s", len(changes), strings.Join(parts, ", "))
}
