# Session Management

Nexus supports tmux-like SSH session management — detach from a running session, return to the TUI, open more sessions, and switch between them freely. Each session is fully independent with its own terminal history.

## How It Works

When you connect to an SSH server, Nexus creates a **ManagedSession** that wraps the SSH connection. Unlike a plain session, a managed session can be detached and reattached:

1. **Connect** — SSH connection established, PTY negotiated, shell started
2. **Reader goroutines** — continuously read SSH stdout/stderr into ring buffers and a replay buffer (run for the lifetime of the SSH connection)
3. **Attach loop** — pipes ring buffer to terminal and terminal to SSH stdin
4. **Detach** — `Ctrl+\` cleanly stops stdin goroutine and returns to the TUI
5. **Reattach** — terminal is cleared, full session history replayed from replay buffer, then live I/O resumes

## Usage

### Detaching

While in an SSH session, press `Ctrl+\` (backslash). The TUI resumes instantly and shows a flash message.

The SSH connection continues running in the background. Any output from the remote server is captured in both:
- **Ring buffer** — incremental passthrough (stdout: 1000 chunks, stderr: 100 chunks)
- **Replay buffer** — 256KB circular history (never drained, used to restore terminal state)

### Viewing Sessions

Press `s` from the connection list, or use `:sessions` in command mode.

The sessions view shows:

| Column | Description |
|--------|-------------|
| # | Session ID (s1, s2, ...) |
| NAME | Connection name |
| HOST | Host:port |
| PROTO | Protocol (SSH) |
| STATUS | connected, detached, or closed |
| UPTIME | Time since connection was established |

Navigate with `j/k`, `gg/G`.

### Reattaching

In the sessions view, select a session with `j/k` and press `Enter`. The terminal clears and the full session history is replayed from the replay buffer — you see all your prompts, command output, and colors exactly as they were. Then live I/O resumes.

Multiple sessions to the same server are fully independent — each has its own SSH connection, PTY, and replay buffer.

### Killing a Session

In the sessions view, press `d` on a session. A confirmation dialog appears. Confirming kills the SSH connection and removes the session.

Sessions that end naturally (e.g., typing `exit` in the shell) are automatically detected and removed from the list.

## Session Features

### Port Forwarding

SSH sessions support port forwarding configured per-connection:

```yaml
port_forwards:
  - type: local       # -L equivalent
    local_addr: "127.0.0.1:8080"
    remote_addr: "10.0.0.10:80"
  - type: remote      # -R equivalent
    local_addr: "127.0.0.1:3000"
    remote_addr: "0.0.0.0:3000"
  - type: dynamic     # -D equivalent (SOCKS5 proxy)
    local_addr: "127.0.0.1:1080"
```

Port forwards are started automatically on connect and run for the lifetime of the SSH connection.

### Jump Hosts

Connections can be routed through one or more jump hosts:

```yaml
proxy_jump: bastion@10.0.0.1:22
# Or multi-hop:
proxy_jump: bastion1@10.0.0.1:22,bastion2@10.0.0.2:22
```

ProxyCommand is also supported for custom tunneling:

```yaml
proxy_command: ssh -W %h:%p bastion
```

### Lifecycle Hooks

Shell commands can be run at connection events:

```yaml
hooks:
  pre_connect:
    - command: wg-quick up vpn
      on_failure: abort    # abort | warn | ignore
      timeout: 10s
  post_connect:
    - command: notify-send "Connected to server"
      on_failure: ignore
  post_disconnect:
    - command: wg-quick down vpn
      on_failure: warn
```

Hooks receive environment variables: `NEXUS_CONNECTION_ID`, `NEXUS_HOST`, `NEXUS_PORT`, `NEXUS_USER`, `NEXUS_PROTOCOL`.

### Session Recording

Sessions can be recorded in asciicast v2 format (compatible with asciinema):

Recordings are stored at `~/.local/share/nexus/recordings/` with the format `{connID}_{timestamp}.cast`. A `.meta.json` sidecar file stores connection metadata.

Playback with: `asciinema play ~/.local/share/nexus/recordings/my-session.cast`

### Quick Connect

Connect without saving a connection first:

- `<leader>cc` — opens quick connect prompt
- `:connect user@host:port` — parse URI and connect
- `:connect ssh://admin@10.0.0.1:22` — full URI syntax

Protocol is auto-detected from port (22=SSH, 23=Telnet, 3389=RDP, 5900=VNC) or URI scheme.

## Session Indicators

When sessions are active:

- **Status bar** shows session count alongside connection counts
- **Bracket navigation** — `]s` / `[s` jumps between connections with active sessions

## Lifecycle

```
Connecting → Connected → Detached → Connected → ... → Closed
                            ↑            │
                            └────────────┘
                           (reattach cycle)
```

A session enters **Closed** when:
- The remote shell exits (user types `exit`)
- The SSH connection drops
- The user kills it from the sessions view

Closed sessions are automatically removed from the session manager.

## Ring Buffer & Replay Buffer

While attached, output flows through two paths:

- **Ring buffer** — drained every 5ms by the attach loop ticker and written to the terminal. This is the realtime display path. Acts as a passthrough pipe.
- **Replay buffer** — 256KB circular buffer that stores ALL session output. Never drained, only snapshotted on reattach to restore the terminal screen.

The replay buffer is what makes session switching work — when you switch between s1 and s2, each session's replay buffer contains its full terminal history. On reattach, the terminal is cleared and the replay buffer snapshot is written, restoring the exact screen state.

## Limitations

- Only SSH sessions support detach/reattach. Telnet, RDP, and VNC sessions use the standard launch flow.
- Sessions are in-memory only — they do not survive a Nexus restart.
- The replay buffer has a 256KB capacity. Very long sessions may lose the oldest terminal output from the replay (though the shell continues working fine).
- Host key verification is disabled (`InsecureIgnoreHostKey`). This is suitable for trusted/internal networks but not for connections over the public internet.
