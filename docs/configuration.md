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
  health_check_interval: 30s   # How often to check connection health
  theme: default                # UI theme (currently only "default")

groups:
  - name: Production
    color: "#ff5555"            # Hex color for group label
  - name: Home Lab
    color: "#8be9fd"

connections:
  - id: unique-id               # Unique identifier (auto-generated on import)
    name: Display Name           # Human-readable name shown in list
    protocol: ssh                # ssh, rdp, vnc, or telnet
    host: 10.0.0.10             # Hostname or IP
    port: 22                     # Port (defaults to protocol standard)
    username: admin              # Login username
    password: ""                 # Password (optional, stored in plaintext)
    identity_file: ~/.ssh/key    # SSH private key path
    group: Production            # Group name (must match a defined group)
    tags: [web, linux]           # Tags for filtering
```

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
| `domain` | No | RDP | Windows domain |
| `group` | No | All | Group name for organization |
| `tags` | No | All | List of tags for filtering |
| `vnc_password` | No | VNC | VNC authentication password |

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

Wildcard hosts (`Host *`) are skipped. Existing connections with the same ID are not duplicated.

## Security Considerations

- **Passwords are stored in plaintext** in the config file. Secure the file with appropriate permissions (`chmod 600 ~/.config/nexus/config.yaml`).
- **Identity files** are referenced by path, never copied or embedded.
- The config file is **local only** — it is not part of the Nexus repository and should never be committed to version control.
- Consider using SSH keys instead of passwords where possible.
