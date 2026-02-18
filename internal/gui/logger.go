package gui

import (
	"fmt"
	"log"
)

// ipcLogger is a thin logging wrapper that dual-writes to stderr (via the
// standard log package) and broadcasts structured log lines over IPC when an
// IPCManager is available.
type ipcLogger struct {
	mgr *IPCManager
}

// newIPCLogger creates a new ipcLogger backed by the given IPCManager.
// If mgr is nil, log lines are written to stderr only.
func newIPCLogger(mgr *IPCManager) *ipcLogger {
	return &ipcLogger{mgr: mgr}
}

// Info logs a formatted message at the "info" level.
func (l *ipcLogger) Info(format string, args ...interface{}) {
	l.write("info", format, args...)
}

// Warn logs a formatted message at the "warn" level.
func (l *ipcLogger) Warn(format string, args ...interface{}) {
	l.write("warn", format, args...)
}

// Error logs a formatted message at the "error" level.
func (l *ipcLogger) Error(format string, args ...interface{}) {
	l.write("error", format, args...)
}

func (l *ipcLogger) write(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", level, msg)
	if l.mgr != nil {
		l.mgr.SendLogLine(level, msg)
	}
}

// guiLog is the package-level logger for the gui package.
// It is initialised without an IPCManager (stderr-only) and upgraded to
// IPC-aware logging when SetIPCLogger is called from App.SetIPCManager.
var guiLog = &ipcLogger{}

// SetIPCLogger replaces the package-level logger with one backed by the given
// IPCManager. Call this once the IPC server is ready (i.e. from
// App.SetIPCManager).
func SetIPCLogger(mgr *IPCManager) {
	guiLog = newIPCLogger(mgr)
}
