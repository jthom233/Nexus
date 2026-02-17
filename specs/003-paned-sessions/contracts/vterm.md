# Contract: VTermBuffer

**Module**: `internal/session/vterm.go`
**Role**: Interprets ANSI escape sequences from session output into a renderable cell grid.

## Interface

```go
// NewVTermBuffer creates a buffer with the given dimensions
func NewVTermBuffer(width, height int) *VTermBuffer

// Write processes raw terminal output bytes, interpreting ANSI sequences
// and updating the cell grid accordingly
func (v *VTermBuffer) Write(data []byte) (int, error)

// Resize changes the buffer dimensions, preserving content where possible
func (v *VTermBuffer) Resize(width, height int)

// Render produces a lipgloss-compatible string representation of the cell grid
func (v *VTermBuffer) Render() string

// RenderRegion renders a subregion of the buffer (for scrollback)
func (v *VTermBuffer) RenderRegion(startRow, endRow int) string

// Clear resets the buffer to empty state
func (v *VTermBuffer) Clear()
```

## Supported ANSI Sequences (MVP)

| Category | Sequences | Description |
|----------|-----------|-------------|
| Cursor movement | CUU, CUD, CUF, CUB, CUP, HVP | Move cursor up/down/left/right, absolute position |
| Erase | ED, EL | Erase display (below/above/all), erase line |
| SGR (colors) | SGR 0-49, 256-color, RGB | Reset, bold, underline, fg/bg colors |
| Scroll | SU, SD, DECSTBM | Scroll up/down, set scroll region |
| Mode | DECSET/DECRST 25 | Show/hide cursor |
| Character | Printable, newline, carriage return, tab, backspace | Standard character rendering |

## Behavioral Contracts

1. **Write implements io.Writer**: Can be used as a drop-in output destination.
2. **Render is pure**: Calling Render() does not modify buffer state.
3. **Resize preserves visible content**: When shrinking, bottom/right content is clipped. When growing, new cells are filled with spaces.
4. **Unknown sequences are ignored**: Unrecognized escape sequences are silently dropped, not rendered as raw characters.
5. **Thread-safe writes**: `Write()` can be called from a goroutine while `Render()` is called from the main Bubbletea goroutine. Internal mutex protects the cell grid.
