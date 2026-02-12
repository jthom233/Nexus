# Nexus

A terminal UI for managing and connecting to remote servers. Supports SSH, RDP, VNC, and Telnet with health monitoring, session management, and SSH config import.

## Features

- **Multi-protocol** — SSH (native Go), RDP, VNC, Telnet
- **Session management** — Detach from SSH sessions with `Ctrl+\`, reattach later, run multiple sessions in background
- **Health monitoring** — Real-time TCP health checks with latency display
- **SSH config import** — Import connections from `~/.ssh/config`
- **Connection groups** — Organize connections by group with color coding
- **Filter & search** — Fuzzy search across names, hosts, protocols, tags
- **Command mode** — Vim-style `:command` interface
- **Clipboard** — Copy connection commands to clipboard
- **GUI companion** — Optional Ebiten-based GUI (`nexus-gui`) with IPC bridge

## Install

Requires Go 1.25+.

```bash
go install github.com/jthom233/Nexus/cmd/nexus@latest
```

Or build from source:

```bash
git clone git@github.com:jthom233/Nexus.git
cd Nexus
go build ./cmd/nexus
./nexus
```

## Quick Start

1. **Run** — `./nexus` (starts with empty config on first run)
2. **Add connections** — Press `a` or `:add`
3. **Import SSH config** — `:import-ssh` to pull from `~/.ssh/config`
4. **Connect** — Select a connection and press `Enter`
5. **Detach** — Press `Ctrl+\` during an SSH session to return to the TUI
6. **Sessions** — Press `s` to view and manage background sessions

## Keybindings

### Connection List

| Key | Action |
|-----|--------|
| `j/k` `↑/↓` | Navigate |
| `Enter` | Connect to selected |
| `/` | Filter connections |
| `:` | Command mode |
| `a` | Add new connection |
| `e` | Edit selected |
| `d` | Delete selected |
| `D` | Show detail view |
| `g` | Cycle group filter |
| `r` | Refresh health checks |
| `y` | Copy command to clipboard |
| `s` | Active sessions |
| `L` | Event log |
| `?` | Help |
| `q` | Quit |

### SSH Sessions

| Key | Action |
|-----|--------|
| `Ctrl+\` | Detach (session keeps running in background) |

### Sessions View

| Key | Action |
|-----|--------|
| `Enter` | Reattach to session |
| `d` | Kill session |
| `Esc` | Back to list |

### Commands

| Command | Description |
|---------|-------------|
| `:connect <name>` | Connect by name |
| `:add` | Add new connection |
| `:edit <name>` | Edit connection |
| `:delete <name>` | Delete connection |
| `:group <name>` | Filter by group |
| `:all` | Show all connections |
| `:import-ssh` | Import from SSH config |
| `:sessions` | View active sessions |
| `:logs` | Event log |
| `:help` | Toggle help |
| `:q` | Quit |

## Configuration

Config is stored at `~/.config/nexus/config.yaml` (respects `XDG_CONFIG_HOME`).

```yaml
version: 1
settings:
  health_check_interval: 30s
  theme: default

groups:
  - name: Production
    color: "#ff5555"
  - name: Home Lab
    color: "#8be9fd"

connections:
  - id: web-server
    name: Web Server
    protocol: ssh
    host: 10.0.0.10
    port: 22
    username: admin
    identity_file: ~/.ssh/id_ed25519
    group: Production
    tags: [web, linux]
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

See [configs/example.yaml](configs/example.yaml) for a full example.

## Security

**No credentials or session data are stored in this repository.**

- All connection config (including passwords, identity file paths) lives in `~/.config/nexus/config.yaml` on your local machine
- SSH sessions are in-memory only — they are never written to disk
- The ring buffers used for background session output exist only in process memory
- The example config in `configs/` uses placeholder data with no real credentials

## Architecture

See [docs/architecture.md](docs/architecture.md) for detailed architecture documentation.

```
cmd/
  nexus/          TUI application entry point
  nexus-gui/      GUI application entry point (Ebiten)
internal/
  config/         YAML config management, SSH config import
  health/         TCP health checking with concurrent probes
  launcher/       Protocol-specific launch strategies
  session/        Native SSH and Telnet session implementations
  tui/            Bubbletea TUI components
  gui/            Ebiten GUI with IPC bridge
  ipc/            Unix socket IPC between TUI and GUI
```

## License

MIT
