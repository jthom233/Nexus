# Session Management

Nexus supports tmux-like SSH session management — detach from a running session, return to the TUI, open more sessions, and switch between them freely.

## How It Works

When you connect to an SSH server, Nexus creates a **ManagedSession** that wraps the SSH connection. Unlike a plain session, a managed session can be detached and reattached:

1. **Connect** — SSH connection established, PTY negotiated, shell started
2. **Background goroutines** — continuously read SSH stdout/stderr into ring buffers
3. **Attach loop** — pipes ring buffer → terminal and terminal → SSH stdin
4. **Detach** — `Ctrl+\` returns you to the TUI; SSH connection stays alive
5. **Reattach** — buffered output is flushed, then live I/O resumes

## Usage

### Detaching

While in an SSH session, press `Ctrl+\` (backslash). The TUI resumes and shows a flash message: "Session detached [s:sessions]".

The SSH connection continues running in the background. Any output from the remote server is captured in a ring buffer (up to 1000 chunks for stdout, 100 for stderr).

### Viewing Sessions

Press `s` from the connection list or detail view, or use `:sessions` in command mode.

The sessions view shows:

| Column | Description |
|--------|-------------|
| # | Session ID (s1, s2, ...) |
| NAME | Connection name |
| HOST | Host:port |
| PROTO | Protocol (SSH) |
| STATUS | connected, detached, or closed |
| UPTIME | Time since connection was established |

### Reattaching

In the sessions view, select a session and press `Enter`. Any output that arrived while detached is immediately displayed, then live I/O resumes.

### Killing a Session

In the sessions view, press `d` on a session. A confirmation dialog appears. Confirming kills the SSH connection and removes the session.

Sessions that end naturally (e.g., typing `exit` in the shell) are automatically detected and removed from the list.

## Session Indicators

When sessions are active:

- **Status bar** shows `~ N sessions` alongside connection counts
- **Header** shows `s:sessions(N)` in the hint area

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

## Ring Buffer

While detached, output is stored in a circular buffer:
- **Stdout**: 1000 chunks (each chunk up to 32KB)
- **Stderr**: 100 chunks (each chunk up to 4KB)

When the buffer is full, oldest data is overwritten. On reattach, all buffered data is flushed to the terminal before live I/O resumes.

## Limitations

- Only SSH sessions support detach/reattach. Telnet, RDP, and VNC sessions use the standard launch flow.
- Sessions are in-memory only — they do not survive a Nexus restart.
- The ring buffer has a fixed capacity. Very long-running detached sessions producing large amounts of output will lose the oldest data.
- Host key verification is disabled (`InsecureIgnoreHostKey`). This is suitable for trusted/internal networks but not for connections over the public internet.
