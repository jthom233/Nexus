# Configuration

## Config File Location

Nexus stores its configuration at:

```
~/.config/nexus/config.yaml
```

This respects `XDG_CONFIG_HOME`. If set, the path becomes `$XDG_CONFIG_HOME/nexus/config.yaml`.

The config file is created automatically on first run with default settings.

## Config Structure

```yaml
version: 1

settings:
  health_check_interval: 30s        # How often to check connection health
  theme: tokyonight-storm            # UI color scheme
  vault: internal                    # Credential vault backend

groups:
  - name: Production
    color: "#ff5555"
  - name: Home Lab
    color: "#8be9fd"

templates:
  - name: my-ssh-template
    description: Standard SSH with key auth
    protocol: ssh
    port: 22
    username: admin
    group: Production
    tags: [linux]
    proxy_jump: bastion@10.0.0.1:22
    record_session: false

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
    notes: "Primary web server — runs nginx"
    custom_fields:
      environment: production
      owner: devops-team
    hooks:
      pre_connect:
        - command: echo "Connecting..."
          on_failure: warn
    port_forwards:
      - type: local
        local_addr: "127.0.0.1:8080"
        remote_addr: "10.0.0.10:80"
```

## Settings

| Field | Default | Description |
|-------|---------|-------------|
| `health_check_interval` | `30s` | Duration between automatic health checks |
| `theme` | `tokyonight-storm` | Color scheme: `tokyonight-storm`, `catppuccin-mocha`, `dracula`, `gruvbox-dark`, `nord` |
| `vault` | `internal` | Credential vault backend: `internal`, `pass`, `keyring` |

## Connection Fields

| Field | Required | Protocols | Description |
|-------|----------|-----------|-------------|
| `id` | Yes | All | Unique identifier |
| `name` | Yes | All | Display name |
| `protocol` | Yes | All | `ssh`, `rdp`, `vnc`, `telnet` |
| `host` | Yes | All | Hostname or IP address |
| `port` | No | All | Port number (defaults to protocol standard) |
| `username` | No | SSH, RDP, Telnet | Login username |
| `password` | No | SSH, RDP | Password for authentication |
| `identity_file` | No | SSH | Path to SSH private key |
| `proxy_jump` | No | SSH | Comma-separated jump hosts (`user@host:port`) |
| `proxy_command` | No | SSH | Custom proxy command |
| `port_forwards` | No | SSH | List of port forwarding rules |
| `domain` | No | RDP | Windows domain |
| `group` | No | All | Group name for organization |
| `tags` | No | All | List of tags for filtering |
| `vnc_password` | No | VNC | VNC authentication password |
| `hooks` | No | SSH | Lifecycle hooks (pre/post connect/disconnect) |
| `favorite` | No | All | Mark as favorite (`true`/`false`) |
| `notes` | No | All | Free-text notes (markdown supported in detail view) |
| `custom_fields` | No | All | Arbitrary key-value metadata |

Auto-managed fields (set by Nexus, not edited manually):

| Field | Description |
|-------|-------------|
| `last_connected_at` | Timestamp of last connection |
| `connect_count` | Number of times connected |

### Default Ports

| Protocol | Default Port |
|----------|-------------|
| SSH | 22 |
| RDP | 3389 |
| VNC | 5900 |
| Telnet | 23 |

## RDP Options

```yaml
connections:
  - id: my-rdp
    protocol: rdp
    host: 192.168.1.50
    rdp_options:
      resolution: 1920x1080      # Display resolution
      fullscreen: false           # Start in fullscreen
      dynamic_resolution: true    # Allow dynamic resize
```

## SSH Jump Hosts

Route connections through bastion hosts:

```yaml
connections:
  - id: internal-server
    protocol: ssh
    host: 10.10.0.5
    # Single hop
    proxy_jump: bastion@10.0.0.1:22

  - id: deep-server
    protocol: ssh
    host: 172.16.0.5
    # Multi-hop chain
    proxy_jump: bastion1@10.0.0.1:22,bastion2@10.10.0.1:22

  - id: custom-tunnel
    protocol: ssh
    host: 10.0.0.5
    # Custom proxy command
    proxy_command: ssh -W %h:%p bastion
```

## SSH Port Forwarding

```yaml
connections:
  - id: my-server
    protocol: ssh
    host: 10.0.0.10
    port_forwards:
      # Local forward (-L): access remote service on local port
      - type: local
        local_addr: "127.0.0.1:8080"
        remote_addr: "10.0.0.10:80"

      # Remote forward (-R): expose local service on remote port
      - type: remote
        local_addr: "127.0.0.1:3000"
        remote_addr: "0.0.0.0:3000"

      # Dynamic SOCKS5 proxy (-D)
      - type: dynamic
        local_addr: "127.0.0.1:1080"
```

Port forwards are started automatically on connect and run for the lifetime of the SSH session.

## Connection Lifecycle Hooks

Run shell commands at connection events:

```yaml
connections:
  - id: vpn-server
    protocol: ssh
    host: 10.0.0.5
    hooks:
      pre_connect:
        - command: wg-quick up vpn0
          on_failure: abort      # abort | warn | ignore
          timeout: 10s           # default: 30s
      post_connect:
        - command: notify-send "Connected to vpn-server"
          on_failure: ignore
      pre_disconnect:
        - command: echo "Disconnecting..."
          on_failure: ignore
      post_disconnect:
        - command: wg-quick down vpn0
          on_failure: warn
```

### Hook Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `command` | Yes | — | Shell command to execute |
| `on_failure` | No | `warn` | Failure policy: `abort` (stop), `warn` (log and continue), `ignore` (silent) |
| `timeout` | No | `30s` | Command timeout |

### Hook Environment Variables

Hooks receive these environment variables:

| Variable | Description |
|----------|-------------|
| `NEXUS_CONNECTION_ID` | Connection ID |
| `NEXUS_HOST` | Target hostname |
| `NEXUS_PORT` | Target port |
| `NEXUS_USER` | Username |
| `NEXUS_PROTOCOL` | Protocol name |

## Connection Templates

Define reusable presets for new connections:

```yaml
templates:
  - name: prod-ssh
    description: Production SSH server
    protocol: ssh
    port: 22
    username: deploy
    group: Production
    tags: [linux, production]
    proxy_jump: bastion@10.0.0.1:22
    record_session: true
```

### Template Fields

| Field | Description |
|-------|-------------|
| `name` | Template name (used in `:template` command and form) |
| `description` | Human-readable description |
| `protocol` | Default protocol |
| `port` | Default port |
| `username` | Default username |
| `group` | Default group |
| `tags` | Default tags |
| `proxy_jump` | Default jump host |
| `record_session` | Whether to record sessions by default |

### Built-in Templates

Nexus includes 5 built-in templates available without configuration:

| Template | Protocol | Description |
|----------|----------|-------------|
| `ssh-standard` | SSH | Standard SSH with key auth |
| `ssh-jumphost` | SSH | SSH through bastion host |
| `rdp-windows` | RDP | Windows Remote Desktop |
| `vnc-linux` | VNC | Linux VNC server |
| `telnet-network` | Telnet | Network device management |

## SSH Config Import

Import connections from your existing `~/.ssh/config`:

```
:import-ssh
```

This parses Host entries and creates connections with:
- `id` — `ssh-<hostname>` (sanitized)
- `name` — SSH config Host alias
- `host` — HostName value
- `port` — Port value (default 22)
- `username` — User value
- `identity_file` — IdentityFile value
- `proxy_jump` — ProxyJump value (if present)

Wildcard hosts (`Host *`) are skipped. Existing connections with the same ID are not duplicated.

## Import & Export

### Import

```
:import file.csv       # CSV import
:import file.json      # JSON import
:import-ssh            # SSH config import
```

Or via leader key: `<leader>is` (SSH), `<leader>ic` (CSV), `<leader>ij` (JSON).

**CSV format** — required columns: `name`, `host`. Optional: `port`, `protocol`, `username`, `password`, `group`, `tags` (semicolons separate multiple tags).

**JSON format** — array of connection objects with the same fields.

**Merge strategies** — when duplicates are found (matching by name):
- `skip` — keep existing, only add new ones
- `overwrite` — replace existing connections
- `rename` — append numeric suffix to duplicates

### Export

```
:export file.csv       # CSV export
:export file.json      # JSON export
:export file.yaml      # YAML export
```

Or via leader key: `<leader>ea` (all), `<leader>es` (selection), `<leader>eg` (group).

Credentials are masked by default. Use Visual mode to export a subset of connections.

## Password Encryption

Nexus encrypts passwords at rest using AES-256-GCM with scrypt key derivation from a master password.

### How It Works

1. **First run with passwords** — Nexus prompts you to set a master password. All plaintext passwords are encrypted and saved immediately.
2. **Subsequent runs** — Nexus prompts for your master password to decrypt credentials in memory.
3. **No passwords** — If no connections have passwords, no prompt is shown.

Encrypted passwords are stored in the config file as `ENC:<base64-encoded data>`:

```yaml
connections:
  - id: my-server
    name: My Server
    protocol: ssh
    host: 10.0.0.1
    password: "ENC:dGhpcyBpcyBhbiBleGFtcGxl..."
```

Each password uses its own random salt and nonce, so identical passwords produce different ciphertexts.

### Viewing Passwords

In the detail view (`D`), passwords are masked as `****` by default. Press `p` to toggle password visibility.

### Migration

Existing configs with plaintext passwords are automatically migrated to encrypted on the first run after setting a master password.

### Wrong Password

If the wrong master password is entered, decryption fails and Nexus exits with an error. No data is modified.

## Credential Vault

For more advanced credential management, Nexus supports pluggable vault backends configured in settings:

```yaml
settings:
  vault: internal    # internal | pass | keyring
```

### Backends

| Backend | Setting | Description |
|---------|---------|-------------|
| Internal | `internal` | AES-256-GCM encrypted JSON file (default). Master password required. |
| Pass | `pass` | Integration with [pass](https://www.passwordstore.org/) (password-store). Credentials stored under `nexus/` prefix. |
| Keyring | `keyring` | System keyring via `secret-tool` (Linux) or `security` (macOS). |

Manage vault entries with `:vault` command or `<leader>p` group.

## Storage Backend

For large deployments (1000+ connections), switch to SQLite:

```
:migrate sqlite    # Migrate from YAML to SQLite
:migrate yaml      # Migrate back to YAML
```

SQLite provides FTS5 full-text search and WAL mode for better performance with large connection databases.

## Data Locations

| Data | Path | Description |
|------|------|-------------|
| Config | `~/.config/nexus/config.yaml` | Main configuration |
| Audit log | `~/.local/share/nexus/audit.log` | Connection event log (JSON lines) |
| Recordings | `~/.local/share/nexus/recordings/` | Session recordings (asciicast v2) |
| Vault | `~/.local/share/nexus/vault.json` | Internal vault (encrypted) |
| Command history | `~/.local/share/nexus/cmd_history` | Command palette history |

## Security Considerations

- Passwords are encrypted at rest using AES-256-GCM with per-password salts and scrypt key derivation.
- The config file is written with `0600` permissions (owner read/write only).
- Identity files are referenced by path, never copied or embedded.
- The config file is local only — it is not part of the Nexus repository and should never be committed to version control.
- Session recordings may contain sensitive terminal output. Review retention policies.
- Audit logs track credential access events for compliance.
- Consider using SSH keys or vault-backed credentials instead of passwords in config where possible.
