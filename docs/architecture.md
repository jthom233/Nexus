# Architecture

## Overview

Nexus is built with a layered architecture separating concerns across well-defined packages. The TUI uses [Bubbletea](https://github.com/charmbracelet/bubbletea) (Elm-architecture), and an optional GUI uses [Ebiten](https://ebitengine.org/) with IPC bridging.

```
┌─────────────────────────────────────────────────┐
│                   cmd/nexus                      │
│              (TUI entry point)                   │
├──────────┬──────────┬───────────┬───────────────┤
│   tui/   │ launcher/ │ session/  │   health/     │
│  (views) │(strategy) │(protocol) │  (probes)     │
├──────────┴──────────┴───────────┴───────────────┤
│                   config/                        │
│           (YAML, SSH import)                     │
└─────────────────────────────────────────────────┘
```

## Package Details

### `config/`

- `Config` struct: version, settings, groups, connections
- Reads/writes `~/.config/nexus/config.yaml` (XDG-compliant)
- `ImportSSHConfig()` parses `~/.ssh/config` into Connection structs
- `Connection` supports SSH, RDP, VNC, Telnet with protocol-specific options

### `health/`

- `Check()` performs TCP dial with 3s timeout, returns Online/Offline/Degraded + latency
- `CheckAll()` runs all checks concurrently via goroutines
- `ScheduleTick()` drives periodic re-checks (default 30s)
- Results flow as Bubbletea messages (`ResultMsg`, `TickMsg`)

### `launcher/`

Implements the Strategy pattern for launching connections:

```
Launcher interface
  ├── SSHLauncher      → session.ManagedSession (native, detachable)
  ├── TelnetLauncher   → session.TelnetSession (native)
  ├── RDPLauncher      → grdp library
  └── VNCLauncher      → vnc2video library
```

Each launcher returns a `tea.Cmd` using `tea.Exec()` which suspends the TUI and gives the session raw terminal access. When the session exits, a `LaunchFinishedMsg` is sent back.

### `session/`

Native protocol implementations that satisfy `tea.ExecCommand` (Run, SetStdin, SetStdout, SetStderr).

#### `SSHSession` (ssh.go)

Standard SSH session — connects, requests PTY, pipes I/O, watches for terminal resize. Used as a fallback for non-managed connections.

#### `ManagedSession` (managed.go)

SSH session with detach/reattach support:

```
First Run():
  connect() → startShell() → launch background goroutines → attachLoop()

Reattach Run():
  attachLoop()  (connection already established)

Background goroutines (lifetime of SSH connection):
  readOutput()  → reads SSH stdout → ringBuffer (1000 chunks)
  readStderr()  → reads SSH stderr → ringBuffer (100 chunks)
  waitDone()    → waits for session.Wait(), closes doneCh
```

**attachLoop** enters raw terminal mode and runs a select loop:
- Polls ringBuffer every 5ms, writes to terminal stdout
- Reads stdin, checks each byte for `0x1C` (Ctrl+\) → returns `ErrDetached`
- Watches `doneCh` → session ended naturally
- Resize watcher goroutine (stopped on detach)

**ringBuffer** is a thread-safe circular buffer. Write() appends copies, Drain() returns all data and clears. Old data is overwritten when full.

**Lifecycle states:** Connecting → Connected → Detached → Connected → ... → Closed

#### `ssh_auth.go`

Shared auth functions used by both SSHSession and ManagedSession:
- `buildSSHAuth()` — builds methods from explicit credentials (key file, password, keyboard-interactive)
- `defaultSSHAuth()` — falls back to `~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`
- `loadSSHKey()` — reads and parses private keys, handles `~/` expansion

### `tui/`

Bubbletea TUI with view-stack navigation.

#### App Model (app.go)

Root model managing all sub-models and view routing:

```
viewStack: [viewList] → [viewList, viewDetail] → [viewList, viewSessions]
```

Views: List, Detail, Form, Log, Sessions

Message flow:
```
tea.KeyMsg → handleKey() → view-specific handler
health.ResultMsg → update status indicators
launcher.LaunchFinishedMsg → flash "session ended"
SessionDetachedMsg → flash + start watchSessionDone
SessionDiedMsg → remove session + update view
CommandMsg → handleCommand() → dispatch
ConfirmResultMsg → handleConfirmResult()
```

#### Session Manager (sessions.go)

Thread-safe manager for ManagedSession instances:
- `Add()` assigns auto-incrementing IDs (s1, s2, ...)
- `watchSessionDone()` polls IsAlive() every 500ms, sends SessionDiedMsg

#### Sessions View (sessions_view.go)

Table view showing: `#`, `NAME`, `HOST`, `PROTO`, `STATUS`, `UPTIME`

#### Other TUI Components

| File | Purpose |
|------|---------|
| `list.go` | Connection table with dynamic column widths |
| `detail.go` | Connection detail view |
| `form.go` | Add/edit connection form (huh library) |
| `filter.go` | Fuzzy search input bar |
| `command.go` | `:command` input with tab autocomplete |
| `confirm.go` | Yes/no confirmation dialog overlay |
| `header.go` | Logo + breadcrumbs + hint line |
| `statusbar.go` | Connection counts + session count + flash messages |
| `help.go` | Context-sensitive keybinding overlay |
| `keys.go` | Keybinding definitions |
| `styles.go` | Dracula-inspired color palette and Lipgloss styles |
| `log.go` | Event log with viewport scrolling |

### `gui/` and `ipc/`

Optional graphical frontend:
- `gui/` — Ebiten game loop rendering tabs for RDP/VNC
- `ipc/` — Unix socket protocol for TUI↔GUI communication
- `nexus-gui` binary runs independently, receives commands from `nexus` TUI

## Data Flow

### Connecting to SSH

```
User presses Enter on SSH connection
  → connectSelected()
  → connectManaged(conn)
  → NewManagedSession(...)
  → SessionManager.Add(managed)
  → tea.Exec(managed, callback)
  → TUI suspends, ManagedSession.Run() called
  → connect() + startShell() + background goroutines
  → attachLoop() — raw terminal I/O

User presses Ctrl+\
  → attachLoop returns ErrDetached
  → callback returns SessionDetachedMsg
  → TUI resumes, flash "Session detached"
  → watchSessionDone starts polling

User presses 's', selects session, Enter
  → reattachSession(sess)
  → tea.Exec(managed, callback)
  → ManagedSession.Run() — skips connect, enters attachLoop
  → Buffered output flushed, live I/O resumes
```

### Health Checking

```
Init() / TickMsg / manual 'r'
  → CheckAll(targets)  [concurrent goroutines]
  → ResultMsg{Results}
  → list.updateHealthResults()
  → statusBar counts updated
  → ScheduleTick(30s) → TickMsg → repeat
```

## Security Model

- Config stored at `~/.config/nexus/config.yaml` — never in the project directory
- Passwords stored in plaintext in config (user responsibility to secure)
- SSH keys referenced by path, not embedded
- `ssh.InsecureIgnoreHostKey()` used (no host key verification) — suitable for internal/lab networks
- Sessions are in-memory only, ring buffers never touch disk
- No telemetry, no network calls beyond user-configured connections
