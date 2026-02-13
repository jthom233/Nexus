# Architecture

## Overview

Nexus is built with a layered architecture separating concerns across well-defined packages. The TUI uses [Bubbletea](https://github.com/charmbracelet/bubbletea) (Elm-architecture) with neovim-style modal input, and an optional GUI uses [Ebiten](https://ebitengine.org/) with IPC bridging for RDP/VNC sessions.

```
┌─────────────────────────────────────────────────────────────────┐
│                         cmd/nexus                               │
│                     (TUI entry point)                           │
├───────┬──────────┬──────────┬──────────┬──────────┬────────────┤
│ tui/  │launcher/ │ session/ │ health/  │  store/  │   audit/   │
│(views)│(strategy)│(protocol)│ (probes) │(storage) │  (events)  │
├───────┴──────────┴──────────┴──────────┴──────────┴────────────┤
│  theme/  │  hooks/  │ vault/  │ template/ │ importexport/      │
│ (colors) │  (hooks) │ (creds) │ (presets) │  (CSV/JSON/YAML)   │
├──────────┴──────────┴─────────┴───────────┴────────────────────┤
│                        config/                                  │
│                (YAML, SSH import, types)                        │
└─────────────────────────────────────────────────────────────────┘
```

## Package Details

### `config/`

- `Config` struct: version, settings, groups, connections, templates
- Reads/writes `~/.config/nexus/config.yaml` (XDG-compliant)
- `ImportSSHConfig()` parses `~/.ssh/config` into Connection structs (handles ProxyJump, IdentityFile, Port, User)
- `Connection` supports SSH, RDP, VNC, Telnet with protocol-specific options
- Connection fields include: jump hosts, port forwards, lifecycle hooks, favorites, notes, custom fields, connect tracking

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
  ├── RDPLauncher      → GUI via IPC
  └── VNCLauncher      → GUI via IPC
```

Each launcher returns a `tea.Cmd` using `tea.Exec()` which suspends the TUI and gives the session raw terminal access. When the session exits, a `LaunchFinishedMsg` is sent back. The SSH launcher also runs lifecycle hooks (pre/post connect) and starts port forwards.

### `session/`

Native protocol implementations that satisfy `tea.ExecCommand` (Run, SetStdin, SetStdout, SetStderr).

#### `SSHSession` (ssh.go)

Standard SSH session — connects, requests PTY, pipes I/O, watches for terminal resize. Used as a fallback for non-managed connections.

#### `ManagedSession` (managed.go)

SSH session with detach/reattach support:

```
First Run():
  connect() → startShell() → launch reader goroutines → attachLoop()

Reattach Run():
  clear terminal → replay session history → attachLoop()

Reader goroutines (lifetime of SSH connection — never stopped on detach):
  readOutput()  → reads SSH stdout → ringBuffer + replayBuffer
  readStderr()  → reads SSH stderr → ringBuffer
  waitDone()    → waits for session.Wait(), closes doneCh
```

**attachLoop** enters raw terminal mode and runs a select loop:
- Polls ringBuffer every 5ms, writes to terminal stdout
- Reads stdin, checks each byte for `0x1C` (Ctrl+\) → returns `ErrDetached`
- Watches `doneCh` → session ended naturally
- Resize watcher goroutine (stopped on detach)
- On detach, interrupts stdin goroutine via `SetReadDeadline` and waits for clean exit

**ringBuffer** is a thread-safe circular buffer. Write() appends copies, Drain() returns all data and clears. Used as a passthrough pipe — drained by the ticker every 5ms while attached.

**replayBuffer** is a 256KB circular byte buffer that stores all session output and is never drained. On reattach, its snapshot is written to the terminal to restore the screen state (prompts, output, colors).

**Lifecycle states:** Connecting → Connected → Detached → Connected → ... → Closed

#### `PortForwardManager` (portforward.go)

Manages SSH port forwards:
- Local forward (`-L`): `net.Listen` → `ssh.Dial` → proxy goroutine
- Remote forward (`-R`): `client.Listen` → `net.Dial` → proxy goroutine
- Dynamic SOCKS5 (`-D`): `net.Listen` → SOCKS5 handshake → `ssh.Dial` → proxy

#### `ssh_auth.go`

Shared auth functions:
- `buildSSHAuth()` — builds methods from explicit credentials (key file, password, keyboard-interactive)
- `defaultSSHAuth()` — falls back to `~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`
- `loadSSHKey()` — reads and parses private keys, handles `~/` expansion

### `theme/`

Semantic color system with 5 built-in themes:

| Theme | Description |
|-------|-------------|
| `tokyonight-storm` | Default — cool blue-purple (popular neovim theme) |
| `catppuccin-mocha` | Warm pastels on dark background |
| `dracula` | Classic purple-accent dark theme |
| `gruvbox-dark` | Warm retro earth tones |
| `nord` | Arctic blue color palette |

Theme struct defines semantic colors: bg, fg, accent, error, warning, success, info, subtle, highlight, cursor, selection, border, header, muted. Plus mode colors (normal/insert/visual/command), protocol colors (ssh/rdp/vnc/telnet), and status colors (online/offline/degraded/unknown).

`styles.go` generates all Lipgloss styles from a Theme via factory functions.

### `hooks/`

Connection lifecycle hooks:

| Event | When |
|-------|------|
| `pre_connect` | Before SSH connection established |
| `post_connect` | After successful connection |
| `pre_disconnect` | Before disconnecting |
| `post_disconnect` | After disconnection |

Each hook specifies a shell command, an `on_failure` policy (`abort`, `warn`, `ignore`), and an optional timeout (default 30s). Hooks receive environment variables: `NEXUS_CONNECTION_ID`, `NEXUS_HOST`, `NEXUS_PORT`, `NEXUS_USER`, `NEXUS_PROTOCOL`.

### `recording/`

Session recording in asciicast v2 format:
- Header: JSON with version, width, height, timestamp, env
- Events: JSON arrays `[time, type, data]` where type is `"o"` (output) or `"i"` (input)
- Metadata sidecar: `.meta.json` with connection ID, host, user, duration
- Storage: `~/.local/share/nexus/recordings/`
- Filename format: `{connID}_{YYYYMMDD_HHMMSS}.cast`

### `store/`

Storage abstraction with two backends:

| Backend | Use Case |
|---------|----------|
| YAML | Default — human-editable `config.yaml` for simple setups |
| SQLite | Large deployments (1000+ connections), FTS5 full-text search, WAL mode |

Store interface: `ListConnections`, `GetConnection`, `AddConnection`, `UpdateConnection`, `DeleteConnection`, `InsertConnectionAt`, `SearchConnections`, `Close`.

Migration between backends via `:migrate sqlite` / `:migrate yaml`.

### `vault/`

Credential vault with pluggable backends:

| Backend | Config Value | Description |
|---------|-------------|-------------|
| Internal | `internal` | AES-256-GCM encrypted JSON file (default) |
| Pass | `pass` | Integration with `pass` (password-store) |
| Keyring | `keyring` | System keyring via `secret-tool` / `security` CLI |

Vault interface: `Get`, `Set`, `Delete`, `List`, `Close`.

### `audit/`

Persistent audit log stored as JSON lines at `~/.local/share/nexus/audit.log`.

Event types: `connect`, `disconnect`, `error`, `credential_use`, `config_change`.

Each event records: timestamp, connection ID/name, event type, duration, exit code, details. Queryable with date range, event type, and connection filters.

### `importexport/`

| Format | Import | Export |
|--------|--------|--------|
| CSV | Fields: name, host, port, protocol, username, password, group, tags (semicolon-separated) | Same fields, optional credential masking |
| JSON | Array of connection objects | Same structure |
| YAML | — | YAML list of connections |

Import merge strategies: `skip` (keep existing), `overwrite` (replace by name), `rename` (add numeric suffix).

### `template/`

Connection templates with 5 built-in presets:
- `ssh-standard` — SSH with key auth
- `ssh-jumphost` — SSH through bastion
- `rdp-windows` — Windows RDP
- `vnc-linux` — Linux VNC
- `telnet-network` — Network device telnet

Templates define default protocol, port, username, group, tags, proxy jump, and recording settings. Selectable when adding new connections.

### `tui/`

Bubbletea TUI with 48 source files organized into:

#### Core

| File | Purpose |
|------|---------|
| `app.go` | Root model — view stack, message routing, key dispatch |
| `mode.go` | Modal system — Normal, Insert, Visual, Command modes |
| `motion.go` | Vim motion engine — count + operator + motion composition |
| `keys.go` | Keybinding definitions |
| `styles.go` | Lipgloss style integration with theme system |

#### Views

| File | Purpose |
|------|---------|
| `list.go` | Connection list (row generation from config) |
| `table.go` | k9s-style table renderer with sort, striping, cursor bar |
| `detail.go` | Connection detail view with notes and custom fields |
| `form.go` | Add/edit connection form (huh library) |
| `sessions_view.go` | Sessions table view |
| `tree.go` | Folder tree view with vim fold keys |
| `pulse.go` | Pulse health dashboard |
| `logview.go` | Audit log viewer |
| `finder.go` | Telescope-style fuzzy finder with preview |

#### Interaction

| File | Purpose |
|------|---------|
| `leader.go` | Leader key (Space) dispatch with groups |
| `whichkey.go` | Which-key popup overlay |
| `search.go` | Incremental search with regex/fuzzy/tag/group modes |
| `command.go` | Command palette with history, tab completion, aliases |
| `visual.go` | Visual mode — range/toggle selection, bulk operators |
| `marks.go` | Vim marks (`m{a-z}`, `'{a-z}`) |
| `bracket.go` | Bracket navigation (`]c`, `[g`, `]e`, etc.) |
| `quickconnect.go` | Quick connect URI parsing |
| `undo.go` | Undo/redo stack for destructive operations |

#### UI Components

| File | Purpose |
|------|---------|
| `header.go` | Logo + breadcrumbs + context hotkey hints |
| `statusbar.go` | Mode indicator + counts + flash messages + position |
| `filter.go` | Fuzzy search input bar |
| `confirm.go` | Yes/no confirmation dialog overlay |
| `help.go` | Context-sensitive keybinding help overlay |
| `log.go` | Event log with viewport scrolling |
| `tags.go` | Tag management and boolean filtering |
| `favorites.go` | Favorites, recent, and frequent tracking |
| `notify.go` | State change notifications |
| `bulk.go` | Bulk operation execution and progress |

#### Message Flow

```
tea.KeyMsg
  → mode check (Normal/Insert/Visual/Command)
  → leader key check (Space + timeout)
  → motion engine (count + operator + motion)
  → view-specific handler
  → session/command dispatch

health.ResultMsg → update status indicators
launcher.LaunchFinishedMsg → flash "session ended"
SessionDetachedMsg → flash + start watchSessionDone
SessionDiedMsg → remove session + update view
CommandMsg → handleCommand() → dispatch to 40+ commands
ConfirmResultMsg → handleConfirmResult()
BulkResultMsg → update bulk progress
```

### `gui/` and `ipc/`

Optional graphical frontend for RDP/VNC:
- `gui/` — Ebiten game loop rendering tabs, stretch-to-fill scaling, independent X/Y mouse mapping
- `ipc/` — Unix socket protocol for TUI↔GUI communication
- `nexus-gui` binary runs independently, receives commands from `nexus` TUI

## Data Flow

### Connecting to SSH

```
User presses Enter on SSH connection
  → connectSelected()
  → connectManaged(conn)
  → Run lifecycle hooks (pre_connect)
  → NewManagedSession(...)
  → SessionManager.Add(managed)
  → tea.Exec(managed, callback)
  → TUI suspends, ManagedSession.Run() called
  → connect() + startShell() + start port forwards
  → Launch reader goroutines (run until SSH closes)
  → attachLoop() — raw terminal I/O

User presses Ctrl+\
  → attachLoop returns ErrDetached
  → Stdin goroutine interrupted via SetReadDeadline
  → Wait for goroutine exit (≤50ms)
  → callback returns SessionDetachedMsg
  → TUI resumes instantly (no input lag)
  → watchSessionDone starts polling

User presses 's', selects session, Enter
  → reattachSession(sess)
  → tea.Exec(managed, callback)
  → ManagedSession.Run() — skips connect
  → Clear terminal + replay replayBuffer snapshot
  → attachLoop() — live I/O resumes with full history visible
```

### Health Checking

```
Init() / TickMsg / manual ':health'
  → CheckAll(targets)  [concurrent goroutines]
  → ResultMsg{Results}
  → list.updateHealthResults()
  → statusBar counts updated
  → ScheduleTick(30s) → TickMsg → repeat
```

### Import Flow

```
:import file.csv
  → detectFormat(file) → CSV/JSON
  → parseCSV/parseJSON → []Connection
  → merge strategy (skip/overwrite/rename)
  → config.Save()
  → ImportResultMsg{Created, Updated, Skipped}
```

## Security Model

- Config stored at `~/.config/nexus/config.yaml` — never in the project directory
- Passwords encrypted at rest with AES-256-GCM (scrypt key derivation, per-password salt+nonce)
- Credential vault with pluggable backends (internal encrypted file, pass, system keyring)
- SSH keys referenced by path, not embedded
- `ssh.InsecureIgnoreHostKey()` used (no host key verification) — suitable for internal/lab networks
- Sessions are in-memory only, ring buffers and replay buffers never touch disk
- Audit log tracks all connection events to `~/.local/share/nexus/audit.log`
- Session recordings stored locally, never transmitted
- Config file written with `0600` permissions (owner read/write only)
- No telemetry, no network calls beyond user-configured connections
