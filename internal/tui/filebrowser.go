package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/session"
	"github.com/dr4zz/nexus/internal/theme"
)

// ---------- Messages ----------

// fileBrowserOpenMsg is sent to open the file browser for a connection.
type fileBrowserOpenMsg struct {
	connName     string
	host         string
	port         int
	username     string
	password     string
	identityFile string
	proxyJump    string
}

// fileBrowserCloseMsg is sent when the file browser should close.
type fileBrowserCloseMsg struct{}

// fileBrowserReadyMsg is sent when the SFTP session is established.
type fileBrowserReadyMsg struct {
	sftp       *session.SFTPSession
	remotePath string
	localPath  string
}

// fileBrowserErrMsg is sent on SFTP connection errors.
type fileBrowserErrMsg struct{ err error }

// fileBrowserListDoneMsg carries a refreshed file listing for one pane.
type fileBrowserListDoneMsg struct {
	remote  bool   // true = remote pane, false = local pane
	path    string
	entries []session.FileEntry
	err     error
}

// fileBrowserOpDoneMsg signals completion of a file operation.
type fileBrowserOpDoneMsg struct{ err error }

// ---------- Model ----------

const (
	fbFocusLocal  = 0
	fbFocusRemote = 1
)

// fileBrowserState tracks what mode the browser is in.
type fileBrowserState int

const (
	fbStateNormal fileBrowserState = iota
	fbStateConnecting
	fbStateMkdir        // waiting for mkdir input
	fbStateConfirmDelete // waiting for delete confirmation
)

type fileBrowserModel struct {
	active bool
	sftp   *session.SFTPSession
	// owns the SSH conn (true when opened standalone, not via existing session)
	ownsConn bool

	localPath  string
	remotePath string
	localFiles []session.FileEntry
	remoteFiles []session.FileEntry

	focus        int // fbFocusLocal or fbFocusRemote
	localCursor  int
	remoteCursor int

	width  int
	height int

	state         fileBrowserState
	statusMsg     string
	statusIsError bool

	// Mkdir input
	mkdirInput string

	// Delete confirmation
	deleteTarget string
	deleteRemote bool

	// Yank buffer
	yankPath string

	// Connection info (for reconnect / display)
	connName string
}

func newFileBrowserModel() fileBrowserModel {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	return fileBrowserModel{
		localPath: home,
	}
}

// open activates the file browser and starts connecting.
func (m *fileBrowserModel) open(msg fileBrowserOpenMsg) tea.Cmd {
	m.active = true
	m.state = fbStateConnecting
	m.connName = msg.connName
	m.statusMsg = "Connecting..."
	m.statusIsError = false
	m.localCursor = 0
	m.remoteCursor = 0
	m.focus = fbFocusLocal

	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	m.localPath = home

	return func() tea.Msg {
		sftp, err := session.NewSFTPSessionFromParams(
			msg.host, msg.port,
			msg.username, msg.password,
			msg.identityFile, msg.proxyJump,
		)
		if err != nil {
			return fileBrowserErrMsg{err: err}
		}
		wd, err := sftp.Getwd()
		if err != nil {
			wd = "/"
		}
		return fileBrowserReadyMsg{
			sftp:       sftp,
			remotePath: wd,
			localPath:  home,
		}
	}
}

// listLocal returns a tea.Cmd that lists local directory entries.
func (m *fileBrowserModel) listLocal(path string) tea.Cmd {
	return func() tea.Msg {
		entries, err := localList(path)
		return fileBrowserListDoneMsg{remote: false, path: path, entries: entries, err: err}
	}
}

// listRemote returns a tea.Cmd that lists remote directory entries.
func (m *fileBrowserModel) listRemote(path string, sftp *session.SFTPSession) tea.Cmd {
	return func() tea.Msg {
		entries, err := sftp.List(path)
		return fileBrowserListDoneMsg{remote: true, path: path, entries: entries, err: err}
	}
}

// Update processes messages for the file browser.
func (m fileBrowserModel) Update(msg tea.Msg) (fileBrowserModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case fileBrowserReadyMsg:
		m.sftp = msg.sftp
		m.ownsConn = true
		m.remotePath = msg.remotePath
		m.localPath = msg.localPath
		m.state = fbStateNormal
		m.statusMsg = "Connected"
		m.statusIsError = false
		// Load both panes
		return m, tea.Batch(
			m.listLocal(m.localPath),
			m.listRemote(m.remotePath, m.sftp),
		)

	case fileBrowserErrMsg:
		m.state = fbStateNormal
		m.statusMsg = "Error: " + msg.err.Error()
		m.statusIsError = true
		return m, nil

	case fileBrowserListDoneMsg:
		if msg.err != nil {
			m.statusMsg = "List error: " + msg.err.Error()
			m.statusIsError = true
			return m, nil
		}
		if msg.remote {
			m.remotePath = msg.path
			m.remoteFiles = msg.entries
			m.remoteCursor = clampCursor(m.remoteCursor, len(m.remoteFiles))
		} else {
			m.localPath = msg.path
			m.localFiles = msg.entries
			m.localCursor = clampCursor(m.localCursor, len(m.localFiles))
		}
		m.statusMsg = ""
		m.statusIsError = false
		return m, nil

	case fileBrowserOpDoneMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
			m.statusIsError = true
			return m, nil
		}
		m.statusMsg = "Done"
		m.statusIsError = false
		// Refresh the remote listing after any operation
		if m.sftp != nil {
			return m, m.listRemote(m.remotePath, m.sftp)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m fileBrowserModel) handleKey(msg tea.KeyMsg) (fileBrowserModel, tea.Cmd) {
	k := msg.String()

	// --- State-specific handling ---
	switch m.state {
	case fbStateConnecting:
		if k == "q" || k == "esc" {
			return m.close()
		}
		return m, nil

	case fbStateMkdir:
		switch k {
		case "enter":
			if m.mkdirInput != "" {
				return m.executeMkdir()
			}
			m.state = fbStateNormal
		case "esc":
			m.state = fbStateNormal
			m.mkdirInput = ""
		case "backspace":
			if len(m.mkdirInput) > 0 {
				m.mkdirInput = m.mkdirInput[:len(m.mkdirInput)-1]
			}
		default:
			if len(k) == 1 {
				m.mkdirInput += k
			}
		}
		return m, nil

	case fbStateConfirmDelete:
		switch k {
		case "y", "Y":
			return m.executeDelete()
		default:
			m.state = fbStateNormal
			m.deleteTarget = ""
			m.statusMsg = "Delete cancelled"
		}
		return m, nil
	}

	// --- Normal state ---
	switch k {
	case "tab":
		if m.focus == fbFocusLocal {
			m.focus = fbFocusRemote
		} else {
			m.focus = fbFocusLocal
		}

	case "j", "down":
		if m.focus == fbFocusLocal {
			if m.localCursor < len(m.localFiles)-1 {
				m.localCursor++
			}
		} else {
			if m.remoteCursor < len(m.remoteFiles)-1 {
				m.remoteCursor++
			}
		}

	case "k", "up":
		if m.focus == fbFocusLocal {
			if m.localCursor > 0 {
				m.localCursor--
			}
		} else {
			if m.remoteCursor > 0 {
				m.remoteCursor--
			}
		}

	case "g":
		if m.focus == fbFocusLocal {
			m.localCursor = 0
		} else {
			m.remoteCursor = 0
		}

	case "G":
		if m.focus == fbFocusLocal {
			if len(m.localFiles) > 0 {
				m.localCursor = len(m.localFiles) - 1
			}
		} else {
			if len(m.remoteFiles) > 0 {
				m.remoteCursor = len(m.remoteFiles) - 1
			}
		}

	case "enter":
		return m.handleEnter()

	case "d":
		return m.promptDelete()

	case "m":
		m.state = fbStateMkdir
		m.mkdirInput = ""
		m.statusMsg = "mkdir: "

	case "r":
		return m.refresh()

	case "q", "esc":
		return m.close()

	case "y":
		return m.yankSelected()
	}

	return m, nil
}

// handleEnter navigates into a directory or transfers a file.
func (m fileBrowserModel) handleEnter() (fileBrowserModel, tea.Cmd) {
	if m.focus == fbFocusLocal {
		if len(m.localFiles) == 0 || m.localCursor >= len(m.localFiles) {
			return m, nil
		}
		entry := m.localFiles[m.localCursor]
		if entry.IsDir {
			newPath := filepath.Join(m.localPath, entry.Name)
			m.localCursor = 0
			return m, m.listLocal(newPath)
		}
		// Upload local file to remote
		if m.sftp == nil {
			return m, nil
		}
		localFile := filepath.Join(m.localPath, entry.Name)
		remoteFile := m.remotePath + "/" + entry.Name
		sftp := m.sftp
		m.statusMsg = "Uploading " + entry.Name + "..."
		return m, func() tea.Msg {
			err := sftp.Upload(localFile, remoteFile)
			return fileBrowserOpDoneMsg{err: err}
		}
	}

	// Remote pane
	if len(m.remoteFiles) == 0 || m.remoteCursor >= len(m.remoteFiles) {
		return m, nil
	}
	entry := m.remoteFiles[m.remoteCursor]
	if entry.IsDir {
		newPath := m.remotePath + "/" + entry.Name
		m.remoteCursor = 0
		if m.sftp != nil {
			return m, m.listRemote(newPath, m.sftp)
		}
		return m, nil
	}
	// Download remote file to local
	if m.sftp == nil {
		return m, nil
	}
	remoteFile := m.remotePath + "/" + entry.Name
	localFile := filepath.Join(m.localPath, entry.Name)
	sftp := m.sftp
	m.statusMsg = "Downloading " + entry.Name + "..."
	return m, func() tea.Msg {
		err := sftp.Download(remoteFile, localFile)
		return fileBrowserOpDoneMsg{err: err}
	}
}

// promptDelete sets up delete confirmation state.
func (m fileBrowserModel) promptDelete() (fileBrowserModel, tea.Cmd) {
	if m.focus == fbFocusLocal {
		// Don't support deleting local files from this view for safety
		m.statusMsg = "Local delete not supported in this view"
		m.statusIsError = true
		return m, nil
	}
	if len(m.remoteFiles) == 0 || m.remoteCursor >= len(m.remoteFiles) {
		return m, nil
	}
	entry := m.remoteFiles[m.remoteCursor]
	m.deleteTarget = m.remotePath + "/" + entry.Name
	m.deleteRemote = true
	m.state = fbStateConfirmDelete
	m.statusMsg = "Delete " + entry.Name + "? (y/n)"
	return m, nil
}

// executeDelete performs the actual delete.
func (m fileBrowserModel) executeDelete() (fileBrowserModel, tea.Cmd) {
	target := m.deleteTarget
	sftp := m.sftp
	m.state = fbStateNormal
	m.deleteTarget = ""
	if sftp == nil {
		return m, nil
	}
	m.statusMsg = "Deleting..."
	return m, func() tea.Msg {
		err := sftp.Remove(target)
		return fileBrowserOpDoneMsg{err: err}
	}
}

// executeMkdir creates a remote directory.
func (m fileBrowserModel) executeMkdir() (fileBrowserModel, tea.Cmd) {
	name := m.mkdirInput
	m.mkdirInput = ""
	m.state = fbStateNormal
	if m.sftp == nil || name == "" {
		return m, nil
	}
	newPath := m.remotePath + "/" + name
	sftp := m.sftp
	m.statusMsg = "Creating directory..."
	return m, func() tea.Msg {
		err := sftp.Mkdir(newPath)
		return fileBrowserOpDoneMsg{err: err}
	}
}

// refresh reloads both panes.
func (m fileBrowserModel) refresh() (fileBrowserModel, tea.Cmd) {
	m.statusMsg = "Refreshing..."
	cmds := []tea.Cmd{m.listLocal(m.localPath)}
	if m.sftp != nil {
		cmds = append(cmds, m.listRemote(m.remotePath, m.sftp))
	}
	return m, tea.Batch(cmds...)
}

// yankSelected copies the selected file path.
func (m fileBrowserModel) yankSelected() (fileBrowserModel, tea.Cmd) {
	if m.focus == fbFocusLocal {
		if len(m.localFiles) > 0 && m.localCursor < len(m.localFiles) {
			m.yankPath = filepath.Join(m.localPath, m.localFiles[m.localCursor].Name)
			m.statusMsg = "Yanked: " + m.yankPath
		}
	} else {
		if len(m.remoteFiles) > 0 && m.remoteCursor < len(m.remoteFiles) {
			m.yankPath = m.remotePath + "/" + m.remoteFiles[m.remoteCursor].Name
			m.statusMsg = "Yanked: " + m.yankPath
		}
	}
	return m, nil
}

// close closes the file browser and cleans up.
func (m fileBrowserModel) close() (fileBrowserModel, tea.Cmd) {
	sftp := m.sftp
	ownsConn := m.ownsConn
	m.active = false
	m.sftp = nil
	m.ownsConn = false
	m.state = fbStateNormal
	m.statusMsg = ""
	return m, func() tea.Msg {
		if sftp != nil {
			if ownsConn {
				sftp.CloseAll()
			} else {
				sftp.Close()
			}
		}
		return fileBrowserCloseMsg{}
	}
}

// View renders the two-pane file browser.
func (m fileBrowserModel) View() string {
	if !m.active {
		return ""
	}

	th := theme.Current()

	// Styles
	focusedBorderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Accent)
	unfocusedBorderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Subtle)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	selectedStyle := lipgloss.NewStyle().Foreground(th.Cursor).Background(th.Selection).Bold(true)
	dirStyle := lipgloss.NewStyle().Foreground(th.Info).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(th.Fg)
	mutedStyle := lipgloss.NewStyle().Foreground(th.Muted)
	errorStyle := lipgloss.NewStyle().Foreground(th.Error)
	hintStyle := lipgloss.NewStyle().Foreground(th.Subtle)

	// Outer border takes 2 cols each side, separator takes 1 col.
	// Each pane gets (width/2 - 2) usable content width.
	paneW := m.width/2 - 2
	if paneW < 10 {
		paneW = 10
	}
	// Inner height: total - header(1) - status(1) - hint(1) - top/bottom border(2) = height-5
	innerH := m.height - 5
	if innerH < 3 {
		innerH = 3
	}

	buildPane := func(title, path string, files []session.FileEntry, cursor, focus int, isFocused bool) string {
		var sb strings.Builder

		// Path header
		truncPath := truncatePath(path, paneW-2)
		hdr := titleStyle.Render(truncPath)
		sb.WriteString(hdr)
		sb.WriteString("\n")
		sep := mutedStyle.Render(strings.Repeat("─", paneW))
		sb.WriteString(sep)
		sb.WriteString("\n")

		headerLines := 2
		listH := innerH - headerLines
		if listH < 1 {
			listH = 1
		}

		if m.state == fbStateConnecting {
			loading := lipgloss.NewStyle().Foreground(th.Muted).Render("  Connecting...")
			sb.WriteString(loading)
		} else if len(files) == 0 {
			empty := lipgloss.NewStyle().Foreground(th.Muted).Render("  (empty)")
			sb.WriteString(empty)
		} else {
			offset := 0
			if cursor >= listH {
				offset = cursor - listH + 1
			}
			end := offset + listH
			if end > len(files) {
				end = len(files)
			}

			for i := offset; i < end; i++ {
				f := files[i]
				isSelected := i == cursor && isFocused

				icon := fileIcon(f)
				name := truncateToWidth(f.Name, paneW-20)
				sizeStr := formatSize(f.Size)
				if f.IsDir {
					sizeStr = "     "
				}
				dateStr := f.ModTime.Format("Jan 02 15:04")

				line := fmt.Sprintf(" %s %-*s  %5s  %s",
					icon, paneW-20, name, sizeStr, dateStr)

				// Pad to pane width
				lw := lipgloss.Width(line)
				if lw < paneW {
					line += strings.Repeat(" ", paneW-lw)
				}

				if isSelected {
					sb.WriteString(selectedStyle.Render(line))
				} else if f.IsDir {
					sb.WriteString(dirStyle.Render(line))
				} else {
					sb.WriteString(fileStyle.Render(line))
				}
				sb.WriteString("\n")
			}
		}

		content := sb.String()

		// Wrap in a border
		borderStyle := unfocusedBorderStyle
		if isFocused {
			borderStyle = focusedBorderStyle
		}
		borderStyle = borderStyle.Width(paneW).Height(innerH)
		titleLabel := " " + title + " "
		_ = titleLabel

		return borderStyle.Render(content)
	}

	localTitle := "Local"
	remoteTitle := "Remote (" + m.connName + ")"

	localPane := buildPane(localTitle, m.localPath, m.localFiles,
		m.localCursor, m.focus, m.focus == fbFocusLocal)
	remotePane := buildPane(remoteTitle, m.remotePath, m.remoteFiles,
		m.remoteCursor, m.focus, m.focus == fbFocusRemote)

	// Join panes side by side
	panesRow := lipgloss.JoinHorizontal(lipgloss.Top, localPane, remotePane)

	// Status line
	var statusLine string
	if m.state == fbStateMkdir {
		statusLine = lipgloss.NewStyle().Foreground(th.Info).Render("mkdir: " + m.mkdirInput + "_")
	} else if m.statusIsError {
		statusLine = errorStyle.Render(m.statusMsg)
	} else if m.statusMsg != "" {
		statusLine = mutedStyle.Render(m.statusMsg)
	}

	// Hint bar
	hints := "Tab:switch  j/k:nav  Enter:open/transfer  d:delete  m:mkdir  r:refresh  y:yank  q:close"
	hintBar := hintStyle.Render(hints)

	return lipgloss.JoinVertical(lipgloss.Left,
		panesRow,
		statusLine,
		hintBar,
	)
}

// ---------- Helpers ----------

func clampCursor(cursor, length int) int {
	if length == 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	if cursor < 0 {
		return 0
	}
	return cursor
}

func localList(path string) ([]session.FileEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("local list %q: %w", path, err)
	}

	result := make([]session.FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, session.FileEntry{
			Name:    e.Name(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			IsDir:   e.IsDir(),
		})
	}

	// Sort: dirs first, then alphabetically
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func fileIcon(f session.FileEntry) string {
	if f.IsDir {
		return "d"
	}
	// Simple icon based on extension
	ext := strings.ToLower(filepath.Ext(f.Name))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".rs", ".c", ".cpp", ".h":
		return "s"
	case ".sh", ".bash", ".zsh":
		return "x"
	case ".txt", ".md", ".rst":
		return "t"
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".conf":
		return "c"
	case ".tar", ".gz", ".zip", ".bz2", ".xz":
		return "a"
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp":
		return "i"
	default:
		return "f"
	}
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case size < KB:
		return fmt.Sprintf("%dB", size)
	case size < MB:
		return fmt.Sprintf("%.1fK", float64(size)/KB)
	case size < GB:
		return fmt.Sprintf("%.1fM", float64(size)/MB)
	default:
		return fmt.Sprintf("%.1fG", float64(size)/GB)
	}
}

func truncatePath(path string, maxW int) string {
	if len(path) <= maxW {
		return path
	}
	if maxW < 4 {
		return path[:maxW]
	}
	return "..." + path[len(path)-(maxW-3):]
}

// formatModTime is kept for potential future use.
func formatModTime(t time.Time) string {
	return t.Format("Jan 02 15:04")
}

// handleFileBrowserKey forwards key events to the file browser model.
func (a App) handleFileBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.fileBrowser, cmd = a.fileBrowser.Update(msg)
	return a, cmd
}

// openFileBrowser opens the SFTP file browser for the given connection.
func (a App) openFileBrowser(c *config.Connection) (tea.Model, tea.Cmd) {
	if c == nil {
		a.statusBar.setFlash("No SSH connection selected", flashError)
		return a, scheduleFlashClear()
	}
	if c.Protocol != config.ProtoSSH {
		a.statusBar.setFlash("SFTP requires an SSH connection", flashError)
		return a, scheduleFlashClear()
	}
	a.resolveProfileCredentials(c)
	return a, func() tea.Msg {
		return fileBrowserOpenMsg{
			connName:     c.Name,
			host:         c.Host,
			port:         c.EffectivePort(),
			username:     c.Username,
			password:     c.Password,
			identityFile: c.IdentityFile,
			proxyJump:    c.ProxyJump,
		}
	}
}
