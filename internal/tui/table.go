package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

// Column defines a table column with layout and sort metadata.
type Column struct {
	Title    string
	MinWidth int
	MaxWidth int // 0 = unlimited
	Flex     int // flex weight for distributing remaining space
	SortKey  string
	Align    int // 0=left, 1=right, 2=center
}

// Row represents a single table row.
type Row struct {
	Cells  []string // cell values matching column order
	ID     string   // row identifier (connection ID)
	Status string   // "online", "offline", "degraded", "unknown" -- for coloring
}

// SortDir indicates the direction of column sorting.
type SortDir int

const (
	SortNone SortDir = iota
	SortAsc
	SortDesc
)

// tableModel is a custom k9s-style table with sort indicators, row striping,
// cursor highlight, semantic column coloring, wide mode, and vim motion integration.
type tableModel struct {
	columns       []Column
	rows          []Row
	allRows       []Row         // unfiltered rows (for search/filter restore)
	cursor        int           // current row index in rows
	offset        int           // first visible row index
	width         int           // available width
	height        int           // available rows for content (excluding header)
	sortCol       int           // index of currently sorted column (-1 = none)
	sortDir       SortDir       // sort direction
	wideMode      bool          // show extra columns
	filterText    string        // active filter text
	highlightText string        // search pattern to highlight in cells
	motion        *MotionEngine // vim motion engine integration
	lastOperator  Operator      // tracks the operator that produced the last OpLine/OpRange result
	selectionSet  *SelectionSet  // selection state (nil when nothing is selected)
}

// newTableModel creates a tableModel with the given columns and an integrated motion engine.
func newTableModel(cols []Column) tableModel {
	return tableModel{
		columns: cols,
		sortCol: -1,
		sortDir: SortNone,
		motion:  NewMotionEngine(),
	}
}

// setSize updates the available width and content height.
func (t *tableModel) setSize(width, height int) {
	t.width = width
	t.height = height
	t.clampViewport()
}

// setRows replaces all rows, stores them as allRows, and resets the cursor.
func (t *tableModel) setRows(rows []Row) {
	t.allRows = rows
	t.rows = rows
	t.applySortToRows()
	t.cursor = 0
	t.offset = 0
	t.clampViewport()
}

// updateRows replaces the rows preserving the cursor position as best as possible.
func (t *tableModel) updateRows(rows []Row) {
	oldID := ""
	if t.cursor >= 0 && t.cursor < len(t.rows) {
		oldID = t.rows[t.cursor].ID
	}
	t.allRows = rows
	t.rows = rows
	t.applySortToRows()

	// Try to restore cursor to the same row by ID.
	if oldID != "" {
		for i, r := range t.rows {
			if r.ID == oldID {
				t.cursor = i
				t.clampViewport()
				return
			}
		}
	}
	// Fallback: clamp cursor.
	if t.cursor >= len(t.rows) {
		t.cursor = len(t.rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.clampViewport()
}

// setFilter filters allRows by fuzzy matching on the filter text and resets the cursor.
func (t *tableModel) setFilter(text string) {
	t.filterText = text
	if text == "" {
		t.rows = t.allRows
	} else {
		var filtered []Row
		for _, r := range t.allRows {
			searchable := strings.Join(r.Cells, " ")
			if fuzzyMatch(text, searchable) {
				filtered = append(filtered, r)
			}
		}
		t.rows = filtered
	}
	t.applySortToRows()
	t.cursor = 0
	t.offset = 0
	t.clampViewport()
}

// selectedRow returns the currently selected row, or nil if there are no rows.
func (t *tableModel) selectedRow() *Row {
	if len(t.rows) == 0 || t.cursor < 0 || t.cursor >= len(t.rows) {
		return nil
	}
	return &t.rows[t.cursor]
}

// Cursor returns the current cursor index.
func (t *tableModel) Cursor() int {
	return t.cursor
}

// SetCursor sets the cursor to the given index, clamping to bounds.
func (t *tableModel) SetCursor(idx int) {
	t.cursor = clampInt(idx, 0, max(len(t.rows)-1, 0))
	t.clampViewport()
}

// RowCount returns the number of visible rows.
func (t *tableModel) RowCount() int {
	return len(t.rows)
}

// ToggleWideMode toggles wide mode on/off.
func (t *tableModel) ToggleWideMode() {
	t.wideMode = !t.wideMode
}

// WideMode returns whether wide mode is enabled.
func (t *tableModel) WideMode() bool {
	return t.wideMode
}

// SetColumns updates the column definitions (used when toggling wide mode).
func (t *tableModel) SetColumns(cols []Column) {
	t.columns = cols
}

// --- Sorting ---

// CycleSort cycles the sort state on the given column index.
// First press = ascending, second = descending, third = no sort.
func (t *tableModel) CycleSort(colIdx int) {
	if colIdx < 0 || colIdx >= len(t.columns) {
		return
	}
	if t.sortCol == colIdx {
		switch t.sortDir {
		case SortAsc:
			t.sortDir = SortDesc
		case SortDesc:
			t.sortCol = -1
			t.sortDir = SortNone
		default:
			t.sortDir = SortAsc
		}
	} else {
		t.sortCol = colIdx
		t.sortDir = SortAsc
	}
	t.applySortToRows()
}

func (t *tableModel) applySortToRows() {
	if t.sortCol < 0 || t.sortDir == SortNone {
		return
	}
	col := t.sortCol
	dir := t.sortDir
	sort.SliceStable(t.rows, func(i, j int) bool {
		a := ""
		b := ""
		if col < len(t.rows[i].Cells) {
			a = t.rows[i].Cells[col]
		}
		if col < len(t.rows[j].Cells) {
			b = t.rows[j].Cells[col]
		}
		if dir == SortDesc {
			a, b = b, a
		}
		return strings.ToLower(a) < strings.ToLower(b)
	})
}

// --- Navigation ---

// MoveCursor moves the cursor to an absolute position.
func (t *tableModel) MoveCursor(target int) {
	t.cursor = clampInt(target, 0, max(len(t.rows)-1, 0))
	t.clampViewport()
}

// PageDown scrolls the cursor down by count rows.
func (t *tableModel) PageDown(count int) {
	t.MoveCursor(t.cursor + count)
}

// PageUp scrolls the cursor up by count rows.
func (t *tableModel) PageUp(count int) {
	t.MoveCursor(t.cursor - count)
}

// ResolveScreenTarget resolves H/M/L sentinel targets to absolute row indices.
func (t *tableModel) ResolveScreenTarget(sentinel int) int {
	visibleHeight := t.height
	if visibleHeight <= 0 {
		visibleHeight = 1
	}
	lastVisible := min(t.offset+visibleHeight-1, len(t.rows)-1)
	switch sentinel {
	case -2: // H — top of visible screen
		return t.offset
	case -3: // M — middle of visible screen
		return (t.offset + lastVisible) / 2
	case -4: // L — bottom of visible screen
		return lastVisible
	default:
		return sentinel
	}
}

func (t *tableModel) clampViewport() {
	total := len(t.rows)
	if total == 0 {
		t.cursor = 0
		t.offset = 0
		return
	}
	t.cursor = clampInt(t.cursor, 0, total-1)

	visibleHeight := t.height
	if visibleHeight <= 0 {
		visibleHeight = 1
	}

	// Ensure cursor is within the visible window.
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+visibleHeight {
		t.offset = t.cursor - visibleHeight + 1
	}
	// Clamp offset.
	maxOffset := total - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	t.offset = clampInt(t.offset, 0, maxOffset)
}

// --- Rendering ---

// View renders the table to a string.
func (t *tableModel) View() string {
	if t.width <= 0 || len(t.columns) == 0 {
		return ""
	}

	th := theme.Current()
	colWidths := t.computeColumnWidths()

	var sb strings.Builder

	// --- Header row ---
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	headerSepStyle := lipgloss.NewStyle().Foreground(th.Subtle)

	var headerCells []string
	for i, col := range t.columns {
		title := col.Title
		// Append sort indicator
		if i == t.sortCol {
			switch t.sortDir {
			case SortAsc:
				title += " \u25b2" // ▲
			case SortDesc:
				title += " \u25bc" // ▼
			}
		}
		headerCells = append(headerCells, t.renderCell(title, colWidths[i], col.Align, headerStyle))
	}
	headerLine := " " + strings.Join(headerCells, " ")
	// Pad to full width
	if lipgloss.Width(headerLine) < t.width {
		headerLine += strings.Repeat(" ", t.width-lipgloss.Width(headerLine))
	}
	sb.WriteString(headerStyle.Render(headerLine))
	sb.WriteString("\n")

	// Separator line
	sep := headerSepStyle.Render(strings.Repeat("─", t.width))
	sb.WriteString(sep)
	sb.WriteString("\n")

	// --- Data rows ---
	visibleHeight := t.height
	if visibleHeight <= 0 {
		visibleHeight = 1
	}
	endIdx := min(t.offset+visibleHeight, len(t.rows))

	for i := t.offset; i < endIdx; i++ {
		row := t.rows[i]
		isCursor := i == t.cursor
		isOddRow := (i-t.offset)%2 == 1
		isSelected := t.selectionSet != nil && t.selectionSet.IsSelected(i)

		line := t.renderRow(row, colWidths, isCursor, isOddRow, isSelected, th)
		sb.WriteString(line)
		if i < endIdx-1 {
			sb.WriteString("\n")
		}
	}

	// Fill remaining lines if table is shorter than height
	rendered := endIdx - t.offset
	for rendered < visibleHeight {
		sb.WriteString("\n")
		emptyLine := strings.Repeat(" ", t.width)
		sb.WriteString(emptyLine)
		rendered++
	}

	return sb.String()
}

func (t *tableModel) renderRow(row Row, colWidths []int, isCursor, isOddRow, isSelected bool, th *theme.Theme) string {
	checkStyle := lipgloss.NewStyle().Foreground(th.Success).Bold(true)
	var cellStrs []string
	for i, col := range t.columns {
		cellValue := ""
		if i < len(row.Cells) {
			cellValue = row.Cells[i]
		}

		// Determine style for this cell
		cellStyle := t.cellStyle(i, cellValue, row.Status, isCursor, isOddRow, th)

		// For selected rows, reduce the first column width by 2 to reserve space for the
		// checkmark prefix ("✓ "). This prevents the checkmark from widening the row and
		// shifting all subsequent columns right.
		colW := colWidths[i]
		if isSelected && i == 0 {
			colW -= 2
		}

		// Apply search highlight if there's an active highlight pattern
		if t.highlightText != "" && !isCursor {
			cellStrs = append(cellStrs, t.renderCellHighlighted(cellValue, colW, col.Align, cellStyle, th))
		} else {
			cellStrs = append(cellStrs, t.renderCell(cellValue, colW, col.Align, cellStyle))
		}
	}

	// Prepend checkmark to the first cell for selected rows. The cell was already rendered
	// to (colWidths[0] - 2) characters wide, so the total first-column width is preserved.
	if isSelected && len(cellStrs) > 0 {
		cellStrs[0] = checkStyle.Render("\u2713") + " " + cellStrs[0]
	}

	line := " " + strings.Join(cellStrs, " ")

	// Apply row-level background
	if isCursor {
		rowStyle := lipgloss.NewStyle().
			Foreground(th.Cursor).
			Background(th.Selection).
			Bold(true)
		// Pad to full width
		lineW := lipgloss.Width(line)
		if lineW < t.width {
			line += strings.Repeat(" ", t.width-lineW)
		}
		return rowStyle.Render(line)
	}
	if isSelected {
		rowStyle := lipgloss.NewStyle().
			Background(th.Selection).
			Foreground(th.Fg)
		lineW := lipgloss.Width(line)
		if lineW < t.width {
			line += strings.Repeat(" ", t.width-lineW)
		}
		return rowStyle.Render(line)
	}
	if isOddRow {
		rowStyle := lipgloss.NewStyle().Background(th.Highlight)
		lineW := lipgloss.Width(line)
		if lineW < t.width {
			line += strings.Repeat(" ", t.width-lineW)
		}
		return rowStyle.Render(line)
	}

	// Normal row — pad to width
	lineW := lipgloss.Width(line)
	if lineW < t.width {
		line += strings.Repeat(" ", t.width-lineW)
	}
	return line
}

func (t *tableModel) cellStyle(colIdx int, value, rowStatus string, isCursor, isOddRow bool, th *theme.Theme) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(th.Fg)

	// Status column (first column, index 0) — color the indicator
	if colIdx == 0 {
		switch rowStatus {
		case "online":
			return base.Foreground(th.StatusOnline)
		case "offline":
			return base.Foreground(th.StatusOffline)
		case "degraded":
			return base.Foreground(th.StatusDegraded)
		default:
			return base.Foreground(th.StatusUnknown)
		}
	}

	// Favorite column: yellow star
	if colIdx < len(t.columns) && t.columns[colIdx].SortKey == "fav" && value != "" {
		return base.Foreground(th.Warning)
	}

	// Latency column detection: check if column title is "LATENCY"
	if colIdx < len(t.columns) && t.columns[colIdx].Title == "LATENCY" {
		return t.latencyStyle(value, th)
	}

	// Protocol column: color by protocol
	if colIdx < len(t.columns) && t.columns[colIdx].Title == "PROTOCOL" {
		return base
	}

	return base
}

func (t *tableModel) latencyStyle(value string, th *theme.Theme) lipgloss.Style {
	base := lipgloss.NewStyle()
	if value == "" {
		return base.Foreground(th.Muted)
	}
	// Parse the numeric value from strings like "42ms", "150ms"
	var ms int
	_, _ = fmt.Sscanf(value, "%dms", &ms)
	switch {
	case ms < 100:
		return base.Foreground(th.Success)
	case ms < 500:
		return base.Foreground(th.Warning)
	default:
		return base.Foreground(th.Error)
	}
}

func (t *tableModel) renderCell(text string, width, align int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	// Truncate if needed
	textW := lipgloss.Width(text)
	if textW > width {
		if width <= 2 {
			text = text[:width]
		} else {
			// Truncate with ".."
			text = truncateToWidth(text, width-2) + ".."
		}
		textW = lipgloss.Width(text)
	}

	// Pad
	padding := width - textW
	if padding < 0 {
		padding = 0
	}

	switch align {
	case 1: // right
		return style.Render(strings.Repeat(" ", padding) + text)
	case 2: // center
		left := padding / 2
		right := padding - left
		return style.Render(strings.Repeat(" ", left) + text + strings.Repeat(" ", right))
	default: // left
		return style.Render(text + strings.Repeat(" ", padding))
	}
}

// renderCellHighlighted renders a cell with search term highlighting.
func (t *tableModel) renderCellHighlighted(text string, width, align int, baseStyle lipgloss.Style, th *theme.Theme) string {
	if width <= 0 {
		return ""
	}
	// Truncate if needed
	textW := lipgloss.Width(text)
	if textW > width {
		if width <= 2 {
			text = text[:width]
		} else {
			text = truncateToWidth(text, width-2) + ".."
		}
		textW = lipgloss.Width(text)
	}

	// Pad
	padding := width - textW
	if padding < 0 {
		padding = 0
	}

	highlighted := highlightMatch(text, t.highlightText, th)

	switch align {
	case 1: // right
		return baseStyle.Render(strings.Repeat(" ", padding)) + highlighted
	case 2: // center
		left := padding / 2
		right := padding - left
		return baseStyle.Render(strings.Repeat(" ", left)) + highlighted + baseStyle.Render(strings.Repeat(" ", right))
	default: // left
		return highlighted + baseStyle.Render(strings.Repeat(" ", padding))
	}
}

// highlightMatch applies a highlight style to the first occurrence of pattern in cell text.
func highlightMatch(cell string, pattern string, th *theme.Theme) string {
	if pattern == "" {
		return cell
	}
	idx := strings.Index(strings.ToLower(cell), strings.ToLower(pattern))
	if idx < 0 {
		return cell
	}

	highlightStyle := lipgloss.NewStyle().
		Background(th.Warning).
		Foreground(th.Bg).
		Bold(true)

	before := cell[:idx]
	match := cell[idx : idx+len(pattern)]
	after := cell[idx+len(pattern):]

	normalStyle := lipgloss.NewStyle().Foreground(th.Fg)

	return normalStyle.Render(before) + highlightStyle.Render(match) + normalStyle.Render(after)
}

func (t *tableModel) computeColumnWidths() []int {
	if len(t.columns) == 0 {
		return nil
	}

	widths := make([]int, len(t.columns))
	totalFlex := 0
	fixedTotal := 0

	// Separators and left margin: 1 leading space + (n-1) separators of 1 char each
	overhead := 1 + (len(t.columns) - 1)

	for i, col := range t.columns {
		if col.Flex > 0 {
			totalFlex += col.Flex
		} else {
			widths[i] = col.MinWidth
			fixedTotal += col.MinWidth
		}
	}

	remaining := t.width - fixedTotal - overhead
	if remaining < 0 {
		remaining = 0
	}

	// Distribute remaining space to flex columns
	if totalFlex > 0 {
		for i, col := range t.columns {
			if col.Flex > 0 {
				w := remaining * col.Flex / totalFlex
				if w < col.MinWidth {
					w = col.MinWidth
				}
				if col.MaxWidth > 0 && w > col.MaxWidth {
					w = col.MaxWidth
				}
				widths[i] = w
			}
		}
	}

	return widths
}

// truncateToWidth truncates a string to fit within maxWidth characters.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth])
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// truncate truncates a string to fit within max display width, appending "~" if truncated.
// This is a shared utility used by both the custom table and the sessions view.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	// Rough byte-level truncation for ASCII-heavy content
	if len(s) > max-1 {
		return s[:max-1] + "~"
	}
	return s
}
