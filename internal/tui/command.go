package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandMsg is sent when a command is submitted from the command bar.
type CommandMsg struct {
	Name       string
	Args       string
	RangeStart int // -1 means no range
	RangeEnd   int // -1 means no range
}

// ---------- Command registry ----------

// CommandDef describes a registered command.
type CommandDef struct {
	Name        string
	Aliases     []string
	Description string
	ArgSpec     string // human-readable argument description (e.g. "<name>", "-t <tag>")
}

// defaultCommands returns the built-in command definitions.
func defaultCommands() []CommandDef {
	return []CommandDef{
		{Name: "connect", Aliases: nil, Description: "Connect to a host", ArgSpec: "<name|user@host>"},
		{Name: "disconnect", Aliases: []string{"dc"}, Description: "Disconnect active session", ArgSpec: "[session-id]"},
		{Name: "quit", Aliases: []string{"q"}, Description: "Quit Nexus", ArgSpec: ""},
		{Name: "add", Aliases: nil, Description: "Add a new connection", ArgSpec: ""},
		{Name: "edit", Aliases: nil, Description: "Edit a connection", ArgSpec: "<name>"},
		{Name: "delete", Aliases: []string{"del", "rm"}, Description: "Delete a connection", ArgSpec: "<name>"},
		{Name: "sort", Aliases: nil, Description: "Sort connections", ArgSpec: "<field>"},
		{Name: "filter", Aliases: nil, Description: "Filter connections", ArgSpec: "[-t tag] [-g group] [pattern]"},
		{Name: "tag", Aliases: nil, Description: "Manage tags", ArgSpec: "<add|remove> <tag>"},
		{Name: "group", Aliases: nil, Description: "Filter by group", ArgSpec: "[name]"},
		{Name: "theme", Aliases: nil, Description: "Switch theme", ArgSpec: "<name>"},
		{Name: "help", Aliases: nil, Description: "Show help", ArgSpec: ""},
		{Name: "export", Aliases: nil, Description: "Export connections", ArgSpec: "[file]"},
		{Name: "import", Aliases: nil, Description: "Import connections", ArgSpec: "[file]"},
		{Name: "import-ssh", Aliases: nil, Description: "Import SSH config", ArgSpec: ""},
		{Name: "favorites", Aliases: []string{"fav"}, Description: "Show favorites", ArgSpec: ""},
		{Name: "recent", Aliases: nil, Description: "Show recent connections", ArgSpec: ""},
		{Name: "frequent", Aliases: nil, Description: "Show frequently used", ArgSpec: ""},
		{Name: "pulse", Aliases: nil, Description: "Health pulse check", ArgSpec: ""},
		{Name: "log", Aliases: []string{"logs", "audit"}, Description: "Show audit log", ArgSpec: ""},
		{Name: "sessions", Aliases: nil, Description: "Manage sessions", ArgSpec: ""},
		{Name: "version", Aliases: []string{"ver"}, Description: "Show version", ArgSpec: ""},
		{Name: "marks", Aliases: nil, Description: "Show marks", ArgSpec: ""},
		{Name: "template", Aliases: []string{"tpl"}, Description: "Manage templates", ArgSpec: "<name>"},
		{Name: "settings", Aliases: []string{"set"}, Description: "Open settings", ArgSpec: ""},
		{Name: "recordings", Aliases: []string{"rec"}, Description: "Session recordings", ArgSpec: ""},
		{Name: "mkdir", Aliases: nil, Description: "Create group", ArgSpec: "<name>"},
		{Name: "rmdir", Aliases: nil, Description: "Remove group", ArgSpec: "<name>"},
		{Name: "mv", Aliases: nil, Description: "Move connection to group", ArgSpec: "<conn> <group>"},
		{Name: "note", Aliases: nil, Description: "Set note on current connection", ArgSpec: "<text>"},
		{Name: "field", Aliases: nil, Description: "Manage custom fields", ArgSpec: "set <key> <value> | remove <key>"},
		{Name: "all", Aliases: nil, Description: "Show all connections", ArgSpec: ""},
		{Name: "health", Aliases: nil, Description: "Health check connections", ArgSpec: ""},
		{Name: "move", Aliases: nil, Description: "Move to group", ArgSpec: "<group>"},
		{Name: "vault", Aliases: nil, Description: "List vault credentials", ArgSpec: ""},
	}
}

// ---------- CommandEngine ----------

const maxHistoryEntries = 500

// CommandEngine provides history, tab completion, aliases, and a command registry.
type CommandEngine struct {
	history      []string          // past commands (most recent first)
	historyIndex int               // -1 = current input, 0+ = browsing
	historyFile  string            // persistent history file path
	savedInput   string            // saved input while browsing history
	registry     []CommandDef      // registered commands
	aliases      map[string]string // user-defined aliases (alias -> expansion)
	// Tab completion state
	completionCandidates []string // current completion candidates
	completionIndex      int      // position in candidate list
	completionPrefix     string   // the partial text being completed
}

// NewCommandEngine creates and initializes a CommandEngine.
func NewCommandEngine() *CommandEngine {
	ce := &CommandEngine{
		historyIndex: -1,
		aliases:      make(map[string]string),
		registry:     defaultCommands(),
	}
	ce.historyFile = ce.defaultHistoryPath()
	ce.LoadHistory()
	return ce
}

func (ce *CommandEngine) defaultHistoryPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "nexus", "history")
}

// CommandNames returns all canonical command names.
func (ce *CommandEngine) CommandNames() []string {
	var names []string
	for _, def := range ce.registry {
		names = append(names, def.Name)
	}
	sort.Strings(names)
	return names
}

// AllNames returns all names and aliases for completion purposes.
func (ce *CommandEngine) AllNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, def := range ce.registry {
		if !seen[def.Name] {
			names = append(names, def.Name)
			seen[def.Name] = true
		}
		for _, alias := range def.Aliases {
			if !seen[alias] {
				names = append(names, alias)
				seen[alias] = true
			}
		}
	}
	for alias := range ce.aliases {
		if !seen[alias] {
			names = append(names, alias)
			seen[alias] = true
		}
	}
	sort.Strings(names)
	return names
}

// --- History ---

// AddToHistory records a command (deduplicates, caps at maxHistoryEntries).
func (ce *CommandEngine) AddToHistory(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	// Remove duplicate
	for i, h := range ce.history {
		if h == cmd {
			ce.history = append(ce.history[:i], ce.history[i+1:]...)
			break
		}
	}
	// Prepend
	ce.history = append([]string{cmd}, ce.history...)
	if len(ce.history) > maxHistoryEntries {
		ce.history = ce.history[:maxHistoryEntries]
	}
}

// SaveHistory writes the history to disk.
func (ce *CommandEngine) SaveHistory() {
	if ce.historyFile == "" {
		return
	}
	dir := filepath.Dir(ce.historyFile)
	_ = os.MkdirAll(dir, 0o755)
	data := strings.Join(ce.history, "\n") + "\n"
	_ = os.WriteFile(ce.historyFile, []byte(data), 0o644)
}

// LoadHistory reads the history from disk.
func (ce *CommandEngine) LoadHistory() {
	if ce.historyFile == "" {
		return
	}
	data, err := os.ReadFile(ce.historyFile)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	ce.history = nil
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			ce.history = append(ce.history, line)
		}
	}
	if len(ce.history) > maxHistoryEntries {
		ce.history = ce.history[:maxHistoryEntries]
	}
}

// HistoryPrev returns the previous (older) history entry.
// On first call, saves the current input so it can be restored.
func (ce *CommandEngine) HistoryPrev(current string) string {
	if len(ce.history) == 0 {
		return current
	}
	if ce.historyIndex == -1 {
		ce.savedInput = current
	}
	if ce.historyIndex+1 < len(ce.history) {
		ce.historyIndex++
		return ce.history[ce.historyIndex]
	}
	// Already at oldest entry
	return ce.history[ce.historyIndex]
}

// HistoryNext returns the next (newer) history entry, or the saved input.
func (ce *CommandEngine) HistoryNext() string {
	if ce.historyIndex <= -1 {
		ce.historyIndex = -1
		return ce.savedInput
	}
	ce.historyIndex--
	if ce.historyIndex < 0 {
		return ce.savedInput
	}
	return ce.history[ce.historyIndex]
}

// ResetHistoryNavigation resets the history browsing state.
func (ce *CommandEngine) ResetHistoryNavigation() {
	ce.historyIndex = -1
	ce.savedInput = ""
}

// --- Tab completion ---

// Complete returns command names matching the given partial input.
func (ce *CommandEngine) Complete(partial string) []string {
	partial = strings.ToLower(strings.TrimSpace(partial))
	if partial == "" {
		return nil
	}
	var matches []string
	for _, name := range ce.AllNames() {
		if strings.HasPrefix(strings.ToLower(name), partial) {
			matches = append(matches, name)
		}
	}
	return matches
}

// NextCompletion cycles through completion candidates, returning the next one.
// If the partial has changed since last call, candidates are recomputed.
// When the current input matches an active candidate, it cycles to the next one.
func (ce *CommandEngine) NextCompletion(currentInput string) string {
	// Extract the command word (first word only for completion)
	parts := strings.Fields(currentInput)
	if len(parts) == 0 {
		return currentInput
	}
	partial := parts[0]
	argsSuffix := ""
	if len(parts) > 1 {
		argsSuffix = " " + strings.Join(parts[1:], " ")
	}

	// Check if the current input word is one of the active candidates (cycling).
	// This happens when a previous Tab replaced the input with a candidate.
	isCycling := false
	if len(ce.completionCandidates) > 0 && ce.completionPrefix != "" {
		for _, c := range ce.completionCandidates {
			if c == partial {
				isCycling = true
				break
			}
		}
	}

	if isCycling {
		// Advance to next candidate
		ce.completionIndex = (ce.completionIndex + 1) % len(ce.completionCandidates)
	} else {
		// New prefix — recompute candidates
		ce.completionPrefix = partial
		ce.completionCandidates = ce.Complete(partial)
		ce.completionIndex = 0
	}

	if len(ce.completionCandidates) == 0 {
		return currentInput
	}

	candidate := ce.completionCandidates[ce.completionIndex]
	return candidate + argsSuffix
}

// ResetCompletion clears the completion cycling state.
func (ce *CommandEngine) ResetCompletion() {
	ce.completionCandidates = nil
	ce.completionIndex = 0
	ce.completionPrefix = ""
}

// --- Aliases ---

// SetAlias registers a user alias.
func (ce *CommandEngine) SetAlias(alias, expansion string) {
	ce.aliases[alias] = expansion
}

// ResolveAlias expands an alias if one exists, otherwise returns input unchanged.
// Only the first word (command name) is checked for alias resolution.
func (ce *CommandEngine) ResolveAlias(input string) string {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return input
	}
	if expansion, ok := ce.aliases[parts[0]]; ok {
		if len(parts) > 1 {
			return expansion + " " + strings.Join(parts[1:], " ")
		}
		return expansion
	}
	return input
}

// ResolveBuiltinAlias resolves built-in command aliases from the registry.
func (ce *CommandEngine) ResolveBuiltinAlias(name string) string {
	for _, def := range ce.registry {
		for _, a := range def.Aliases {
			if a == name {
				return def.Name
			}
		}
	}
	return name
}

// --- Range parsing ---

// rangePattern matches vim-style range prefixes like "1,10 ", "%", ".", "$".
var rangePattern = regexp.MustCompile(`^(\d+)\s*,\s*(\d+)\s+(.+)$`)

// ParseRange extracts a range prefix from a command string.
// Returns (rangeStart, rangeEnd, remainingCommand).
// rangeStart and rangeEnd are -1 if no range was found.
// Ranges are 1-based and converted to 0-based indices.
func ParseRange(input string) (int, int, string) {
	input = strings.TrimSpace(input)
	m := rangePattern.FindStringSubmatch(input)
	if m == nil {
		return -1, -1, input
	}
	start, err1 := strconv.Atoi(m[1])
	end, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return -1, -1, input
	}
	// Convert from 1-based to 0-based
	return start - 1, end - 1, strings.TrimSpace(m[3])
}

// --- Lookup ---

// FindCommand looks up a command definition by name or alias.
func (ce *CommandEngine) FindCommand(name string) *CommandDef {
	lower := strings.ToLower(name)
	for i := range ce.registry {
		if strings.ToLower(ce.registry[i].Name) == lower {
			return &ce.registry[i]
		}
		for _, a := range ce.registry[i].Aliases {
			if strings.ToLower(a) == lower {
				return &ce.registry[i]
			}
		}
	}
	return nil
}

// HelpLines returns formatted help text for all commands.
func (ce *CommandEngine) HelpLines() []string {
	var lines []string
	for _, def := range ce.registry {
		aliases := ""
		if len(def.Aliases) > 0 {
			aliases = " (" + strings.Join(def.Aliases, ", ") + ")"
		}
		line := fmt.Sprintf("  :%s%s  %s  %s", def.Name, aliases, def.ArgSpec, def.Description)
		lines = append(lines, line)
	}
	return lines
}

// ---------- commandModel (UI layer) ----------

type commandModel struct {
	input  textinput.Model
	engine *CommandEngine
	active bool
	width  int
}

func newCommand() commandModel {
	ti := textinput.New()
	ti.Prompt = ": "
	ti.PromptStyle = CommandPromptStyle
	ti.Placeholder = ""
	ti.CharLimit = 200
	return commandModel{
		input:  ti,
		engine: NewCommandEngine(),
	}
}

func (c *commandModel) activate() {
	c.active = true
	c.input.Focus()
	c.input.SetValue("")
	c.engine.ResetHistoryNavigation()
	c.engine.ResetCompletion()
}

func (c *commandModel) deactivate() {
	c.active = false
	c.input.Blur()
	c.input.SetValue("")
	c.engine.ResetHistoryNavigation()
	c.engine.ResetCompletion()
}

func (c commandModel) Update(msg tea.Msg) (commandModel, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			raw := strings.TrimSpace(c.input.Value())
			if raw != "" {
				c.engine.AddToHistory(raw)
				c.engine.SaveHistory()
			}
			cmd := c.parseCommand()
			c.deactivate()
			return c, func() tea.Msg { return cmd }
		case "esc":
			c.deactivate()
			return c, nil
		case "tab":
			completed := c.engine.NextCompletion(c.input.Value())
			c.input.SetValue(completed)
			c.input.SetCursor(len(completed))
			return c, nil
		case "up":
			prev := c.engine.HistoryPrev(c.input.Value())
			c.input.SetValue(prev)
			c.input.SetCursor(len(prev))
			c.engine.ResetCompletion()
			return c, nil
		case "down":
			next := c.engine.HistoryNext()
			c.input.SetValue(next)
			c.input.SetCursor(len(next))
			c.engine.ResetCompletion()
			return c, nil
		default:
			// Any other key resets completion cycling
			c.engine.ResetCompletion()
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c commandModel) View() string {
	if !c.active {
		return ""
	}
	return lipgloss.NewStyle().Width(c.width).Padding(0, 1).Render(c.input.View())
}

func (c commandModel) parseCommand() CommandMsg {
	raw := strings.TrimSpace(c.input.Value())

	// Resolve user aliases first
	raw = c.engine.ResolveAlias(raw)

	// Parse range prefix
	rangeStart, rangeEnd, rest := ParseRange(raw)

	// Split into command name and args
	parts := strings.SplitN(rest, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	// Resolve built-in aliases (e.g. "q" -> "quit", "fav" -> "favorites")
	name = c.engine.ResolveBuiltinAlias(name)

	return CommandMsg{
		Name:       name,
		Args:       args,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
	}
}
