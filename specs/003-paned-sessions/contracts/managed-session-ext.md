# Contract: ManagedSession Extensions

**Module**: `internal/session/managed.go`
**Role**: Extends ManagedSession with channel-based I/O for pane mode.

## New Interface

```go
// StartBackground connects the session and starts I/O goroutines
// without entering attachLoop or taking terminal control.
// Output is sent to OutputCh. Input is read from InputCh.
func (m *ManagedSession) StartBackground() error

// SetPaneSize updates the session's terminal dimensions and sends
// an SSH WindowChange request. Called by PaneLayoutModel on resize.
func (m *ManagedSession) SetPaneSize(width, height int)

// WriteInput sends raw bytes to the session's stdin.
// Thread-safe. Used by PaneLayoutModel.RouteInput().
func (m *ManagedSession) WriteInput(data []byte) error

// OutputChan returns a read-only channel that emits session output.
// Consumer (pane renderer) drains this channel.
func (m *ManagedSession) OutputChan() <-chan []byte
```

## Behavioral Contracts

1. **StartBackground is idempotent**: Calling it when already connected is a no-op.
2. **OutputCh is buffered**: Channel has buffer size of 256 to avoid blocking session I/O goroutines.
3. **WriteInput is thread-safe**: Multiple callers (broadcast) can write concurrently.
4. **SetPaneSize is debounced**: Rapid resize events are coalesced (max 1 SSH WindowChange per 100ms).
5. **Run() still works**: The existing `tea.Exec` path (`Run()` → `attachLoop()`) remains functional for non-pane single-session connections. `StartBackground()` and `Run()` are mutually exclusive — calling one after the other panics.
6. **Cleanup on close**: When the SSH session ends, `OutputCh` is closed and `doneCh` is closed (existing behavior).
