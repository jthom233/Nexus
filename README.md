# Nexus

A production-grade terminal UI for managing and connecting to remote servers. Neovim-native modal controls, k9s-inspired visual design, and deep feature set rivaling Devolutions Remote Desktop Manager — all in a single Go binary.

## Features

### Neovim-Native Controls
- **Modal system** — Normal / Insert / Visual / Command modes with color-coded statusbar
- **Vim motion engine** — `j/k`, `gg/G`, `Ctrl+d/u`, `Ctrl+f/b`, `H/M/L`, count prefixes (`5j`), operator+motion (`dG`, `yy`)
- **Leader key (Space)** — with which-key popup showing all key groups after 500ms
- **Visual mode** — `V` range select, `v` toggle, `d/y` operators on selections
- **Marks** — `m{a-z}` set mark, `'{a-z}` jump to mark
- **Jump list** — `Ctrl+o` back, `Ctrl+i` forward
- **Bracket navigation** — `]c/[c` connections, `]g/[g` groups, `]e/[e` errors, `]s/[s` sessions, `]f/[f` favorites
- **Undo/Redo** — `u` undo, `Ctrl+r` redo, 50-deep stack

### k9s-Style Visual Design
- **Theme system** — TokyoNight Storm default + Catppuccin Mocha, Dracula, Gruvbox Dark, Nord
- **k9s layout** — logo header, breadcrumb bar, context-sensitive hotkey bar, information-dense table, color-coded statusbar
- **Custom table** — sort indicators, column auto-sizing, row striping, cursor bar, wide mode (`Ctrl+W`)
- **Status indicators** — colored status dots, latency color gradient

### Multi-Protocol Sessions
- **SSH** — native Go with detach/reattach (`Ctrl+\`), managed ring buffers, replay on reattach
- **RDP & VNC** — via Ebiten GUI with IPC bridge, stretch-to-fill scaling
- **Telnet** — native terminal sessions
- **SSH jump hosts** — ProxyJump / ProxyCommand with multi-hop chains
- **SSH port forwarding** — local (`-L`), remote (`-R`), dynamic SOCKS5 (`-D`)
- **Lifecycle hooks** — pre/post connect/disconnect shell commands with abort/warn/ignore policies
- **Session recording** — asciicast v2 format (asciinema-compatible)
- **Quick connect** — `<leader>cc` or `:connect user@host:port` with auto-detect

### Organization
- **Folder tree view** — hierarchical folders, vim fold keys (`za/zo/zc/zR/zM`)
- **Favorites** — `<leader>fv` toggle, starred indicator, `:favorites` view
- **Recent & Frequent** — `:recent` by last connected, `:frequent` by connect count
- **Connection templates** — 5 built-in + custom templates
- **Tag system** — multi-tag, boolean filtering (`:filter -t web AND production`), tag cloud
- **Telescope fuzzy finder** — `Ctrl+p`, three-panel layout with preview

### Search & Commands
- **Incremental search** — `/` forward, `?` reverse, regex, fuzzy (`-f`), tag (`-t`), group (`-g`)
- **Command palette** — `:` with 40+ commands, tab completion, history, aliases, range syntax
- **Bulk operations** — Visual select + command, range ops (`:1,10 delete`)

### Data Layer
- **YAML config** — human-editable, default for simple setups
- **SQLite backend** — FTS5 full-text search, WAL mode, for large deployments
- **Import** — CSV, JSON, SSH config (`~/.ssh/config`)
- **Export** — CSV, JSON, YAML with optional credential masking

### Security
- **AES-256-GCM encryption** — credentials encrypted at rest with master password
- **Credential vault** — internal encrypted vault, `pass` (password-store), system keyring
- **Audit log** — persistent JSON-lines log of all connection events

### Monitoring
- **Pulse dashboard** — `:pulse` with summary cards, group health, latency table
- **Health checking** — TCP connect with latency, configurable interval
- **Notifications** — flash messages + optional desktop `notify-send`
- **Connection notes** — per-connection markdown notes
- **Custom fields** — arbitrary key-value metadata

## Install

Requires Go 1.26+.

```bash
go install github.com/dr4zz/nexus/cmd/nexus@latest
```

Or build from source:

```bash
git clone git@github.com:dr4zz/nexus.git
cd nexus
go build ./cmd/nexus
./nexus
```

For the GUI companion (RDP/VNC):

```bash
go build ./cmd/nexus-gui
```

## Quick Start

1. **Run** — `./nexus` (creates default config on first run)
2. **Import SSH config** — `:import-ssh` to pull from `~/.ssh/config`
3. **Add connections** — press `a` or `:add`
4. **Connect** — select a connection and press `Enter`
5. **Detach** — press `Ctrl+\` during an SSH session to return to the TUI
6. **Sessions** — press `s` to view and manage background sessions

## Keybindings

### Normal Mode (Connection List)

| Key | Action |
|-----|--------|
| `j/k` | Navigate up/down |
| `gg` / `G` | Jump to first / last |
| `Ctrl+d/u` | Half-page scroll |
| `H/M/L` | Top / middle / bottom of screen |
| `Enter` | Connect to selected |
| `/` | Search / filter |
| `:` | Command mode |
| `Space` | Leader key (which-key popup) |
| `V` | Visual mode (range select) |
| `a` | Add new connection |
| `e` | Edit selected |
| `dd` | Delete selected |
| `D` | Detail view |
| `yy` | Copy command to clipboard |
| `s` | Active sessions |
| `u` | Undo |
| `Ctrl+r` | Redo |
| `Ctrl+p` | Telescope fuzzy finder |
| `Ctrl+W` | Toggle wide mode |
| `m{a-z}` | Set mark |
| `'{a-z}` | Jump to mark |
| `?` | Help |
| `q` | Quit |

### Leader Key Groups (Space + key)

| Key | Group | Actions |
|-----|-------|---------|
| `c` | Connect | connect selected, quick connect |
| `f` | Find | fuzzy find, sessions, by tag, by group, recent, toggle favorite |
| `s` | Sessions | list, kill session, kill all |
| `g` | Groups | list groups, filter by group |
| `i` | Import | SSH config, CSV, JSON |
| `e` | Export | all, selection, group |
| `t` | Tags | filter by tag, add tag, remove tag |
| `v` | View | tree view, table view, detail panel, wide mode |
| `h` | Health | check all, check selected, pulse dashboard |
| `p` | Password | show password, copy password |
| `o` | Options | theme, keybindings |
| `?` | Help | show help |

### SSH Sessions

| Key | Action |
|-----|--------|
| `Ctrl+\` | Detach (session keeps running) |

### Sessions View

| Key | Action |
|-----|--------|
| `j/k` | Navigate sessions |
| `Enter` | Reattach to session |
| `d` | Kill session |
| `Esc` | Back to list |

### Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `:connect <name\|user@host>` | | Connect to host |
| `:add` | | Add new connection |
| `:edit <name>` | | Edit connection |
| `:delete <name>` | `del`, `rm` | Delete connection |
| `:group [name]` | | Filter by group |
| `:all` | | Show all connections |
| `:import-ssh` | | Import SSH config |
| `:import [file]` | | Import CSV/JSON |
| `:export [file]` | | Export connections |
| `:sessions` | | Active sessions |
| `:favorites` | `fav` | Favorites view |
| `:recent` | | Recent connections |
| `:frequent` | | Most-used connections |
| `:pulse` | | Health dashboard |
| `:log` | `logs`, `audit` | Audit log viewer |
| `:theme <name>` | | Switch theme |
| `:sort <field>` | | Sort connections |
| `:filter [-t tag] [-g group] [pattern]` | | Filter connections |
| `:tag <add\|remove> <tag>` | | Manage tags |
| `:note <text>` | | Set connection note |
| `:field set <key> <value>` | | Set custom field |
| `:template <name>` | `tpl` | Apply template |
| `:marks` | | Show marks |
| `:vault` | | List vault credentials |
| `:help` | | Help overlay |
| `:q` | `quit` | Quit |

## Configuration

Config is stored at `~/.config/nexus/config.yaml` (respects `XDG_CONFIG_HOME`).

```yaml
version: 1

settings:
  health_check_interval: 30s
  theme: tokyonight-storm  # or: catppuccin-mocha, dracula, gruvbox-dark, nord
  vault: internal           # or: pass, keyring

templates:
  - name: my-ssh
    protocol: ssh
    username: admin
    group: Production
    tags: [linux]

connections:
  - id: web-server
    name: Web Server
    protocol: ssh
    host: 10.0.0.10
    port: 22
    username: admin
    identity_file: ~/.ssh/id_ed25519
    proxy_jump: bastion@10.0.0.1:22
    group: Production
    tags: [web, linux]
    favorite: true
    notes: "Primary web server"
    hooks:
      pre_connect:
        - command: echo "Connecting..."
          on_failure: warn
    port_forwards:
      - type: local
        local_addr: "127.0.0.1:8080"
        remote_addr: "10.0.0.10:80"

  - id: win-desktop
    name: Windows Desktop
    protocol: rdp
    host: 192.168.1.50
    port: 3389
    username: user
    domain: WORKGROUP
    group: Home Lab
    rdp_options:
      resolution: 1920x1080
      fullscreen: false
      dynamic_resolution: true
```

See [docs/configuration.md](docs/configuration.md) for full field reference.

## Security

**No credentials or session data are stored in this repository.**

- Passwords encrypted at rest using AES-256-GCM with scrypt key derivation
- Credential vault with pluggable backends (internal, `pass`, system keyring)
- Config file written with `0600` permissions
- SSH sessions in-memory only, ring buffers never touch disk
- Audit log tracks all connection events
- Session recordings stored locally in asciicast v2 format

## Architecture

See [docs/architecture.md](docs/architecture.md) for detailed architecture documentation.

```
cmd/
  nexus/            TUI application entry point
  nexus-gui/        GUI application entry point (Ebiten)
internal/
  audit/            Persistent audit event log (JSON lines)
  config/           YAML config, SSH config import, connection types
  crypto/           AES-256-GCM encryption utilities
  gui/              Ebiten GUI with tab management for RDP/VNC
  health/           TCP health checking with concurrent probes
  hooks/            Connection lifecycle hooks (pre/post connect/disconnect)
  importexport/     CSV, JSON, YAML import and export engines
  ipc/              Unix socket IPC between TUI and GUI
  launcher/         Protocol-specific launch strategies
  recording/        Session recording in asciicast v2 format
  session/          SSH/Telnet sessions, port forwarding, managed detach/reattach
  store/            Storage abstraction (YAML + SQLite backends)
  template/         Connection templates with built-in presets
  theme/            Theme system with 5 built-in color schemes
  tui/              Bubbletea TUI — 48 source files, modal system, views, motions
  vault/            Credential vault (internal encrypted, pass, keyring)
```

## License

MIT
