package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

// BulkOpType identifies the kind of bulk operation.
type BulkOpType int

const (
	BulkConnect     BulkOpType = iota
	BulkDelete
	BulkTag
	BulkMove
	BulkExport
	BulkHealthCheck
)

func (b BulkOpType) String() string {
	switch b {
	case BulkConnect:
		return "connect"
	case BulkDelete:
		return "delete"
	case BulkTag:
		return "tag"
	case BulkMove:
		return "move"
	case BulkExport:
		return "export"
	case BulkHealthCheck:
		return "health-check"
	default:
		return "unknown"
	}
}

// BulkOperation describes a bulk action to be performed on multiple connections.
type BulkOperation struct {
	Type      BulkOpType
	TargetIDs []string
}

// BulkProgress tracks the progress of a running bulk operation.
type BulkProgress struct {
	Total     int
	Completed int
	Failed    int
	Current   string // name of the connection currently being processed
}

// ProgressText returns a human-readable progress string.
func (bp BulkProgress) ProgressText() string {
	if bp.Total == 0 {
		return ""
	}
	pct := (bp.Completed * 100) / bp.Total
	if bp.Failed > 0 {
		return fmt.Sprintf("[%d/%d] %d%% (%d failed) %s", bp.Completed, bp.Total, pct, bp.Failed, bp.Current)
	}
	return fmt.Sprintf("[%d/%d] %d%% %s", bp.Completed, bp.Total, pct, bp.Current)
}

// IsComplete returns true when all items have been processed.
func (bp BulkProgress) IsComplete() bool {
	return bp.Completed+bp.Failed >= bp.Total
}

// BulkResultMsg is sent when a bulk operation completes.
type BulkResultMsg struct {
	Type      BulkOpType
	Completed int
	Failed    int
	Total     int
	Message   string
}

// ExecuteBulkHealthCheck runs health checks on the specified connections and
// returns a tea.Cmd that produces health.ResultMsg when finished.
func ExecuteBulkHealthCheck(checker *health.Checker, connections []config.Connection) tea.Cmd {
	targets := make(map[string]string, len(connections))
	for _, c := range connections {
		targets[c.ID] = c.HostPort()
	}
	return checker.CheckAll(targets)
}

// ExecuteBulkTag adds or removes a tag from all given connections.
// Returns the count of connections modified.
func ExecuteBulkTag(cfg *config.Config, ids []string, tag string, add bool) (modified int, err error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return 0, fmt.Errorf("tag cannot be empty")
	}

	tm := NewTagManager()
	for _, id := range ids {
		c := cfg.FindConnection(id)
		if c == nil {
			continue
		}
		var changed bool
		if add {
			changed = tm.AddTag(c, tag)
		} else {
			changed = tm.RemoveTag(c, tag)
		}
		if changed {
			if e := cfg.UpdateConnection(*c); e != nil {
				err = e // report last error
				continue
			}
			modified++
		}
	}
	return modified, err
}

// ExecuteBulkMove moves all specified connections to the given group/folder.
// Returns the count of connections moved.
func ExecuteBulkMove(cfg *config.Config, ids []string, folder string) (moved int, err error) {
	folder = strings.TrimSpace(folder)

	for _, id := range ids {
		c := cfg.FindConnection(id)
		if c == nil {
			continue
		}
		if c.Group == folder {
			continue // already in the target group
		}
		c.Group = folder
		if e := cfg.UpdateConnection(*c); e != nil {
			err = e
			continue
		}
		moved++
	}
	return moved, err
}

// ExecuteBulkDelete deletes all specified connections.
// Returns the count of connections deleted and any error.
func ExecuteBulkDelete(cfg *config.Config, ids []string) (deleted int, err error) {
	for _, id := range ids {
		if e := cfg.DeleteConnection(id); e != nil {
			err = e
			continue
		}
		deleted++
	}
	return deleted, err
}

// ResolveRangeIDs converts a range of row indices to connection IDs.
// rangeStart and rangeEnd are 0-based indices into the filtered connection list.
func ResolveRangeIDs(filtered []config.Connection, rangeStart, rangeEnd int) []string {
	if rangeStart < 0 {
		rangeStart = 0
	}
	if rangeEnd >= len(filtered) {
		rangeEnd = len(filtered) - 1
	}
	if rangeStart > rangeEnd {
		return nil
	}

	ids := make([]string, 0, rangeEnd-rangeStart+1)
	for i := rangeStart; i <= rangeEnd; i++ {
		ids = append(ids, filtered[i].ID)
	}
	return ids
}

// ResolveVisualIDs extracts connection IDs from the visual selection.
func ResolveVisualIDs(vs *VisualState, rows []Row) []string {
	if vs == nil || !vs.Active() {
		return nil
	}
	return vs.SelectedIDs(rows)
}

// ConnectionsByIDs returns connections matching the given IDs, preserving order.
func ConnectionsByIDs(cfg *config.Config, ids []string) []config.Connection {
	result := make([]config.Connection, 0, len(ids))
	for _, id := range ids {
		if c := cfg.FindConnection(id); c != nil {
			result = append(result, *c)
		}
	}
	return result
}
