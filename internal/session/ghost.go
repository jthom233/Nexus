package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/dr4zz/nexus/internal/config"
)

const (
	ghostReconnectInitial = 1 * time.Second
	ghostReconnectMax     = 60 * time.Second
)

// GhostManager manages persistent auto-connecting background sessions.
// Sessions marked with AutoConnect:true are started on startup and
// auto-reconnected (with exponential backoff) when AutoReconnect:true.
type GhostManager struct {
	sessions map[string]*ManagedSession // keyed by connection ID
	mu       sync.Mutex
	cfg      *config.Config
	done     chan struct{}
	nextID   int
}

// NewGhostManager creates a GhostManager backed by the provided config.
func NewGhostManager(cfg *config.Config) *GhostManager {
	return &GhostManager{
		sessions: make(map[string]*ManagedSession),
		cfg:      cfg,
		done:     make(chan struct{}),
	}
}

// Start connects all connections with AutoConnect:true as background sessions.
// Each session gets a monitor goroutine that auto-reconnects when AutoReconnect:true.
func (g *GhostManager) Start() {
	for i := range g.cfg.Connections {
		conn := g.cfg.Connections[i]
		if !conn.AutoConnect {
			continue
		}
		if conn.Protocol != config.ProtoSSH {
			// Ghost sessions are SSH-only (port forwards, SOCKS proxies).
			continue
		}
		g.startGhost(conn)
	}
}

// Stop disconnects all managed ghost sessions and signals the manager to shut down.
func (g *GhostManager) Stop() {
	close(g.done)

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, sess := range g.sessions {
		sess.Kill()
	}
	g.sessions = make(map[string]*ManagedSession)
}

// Sessions returns a snapshot of all currently active ghost sessions.
func (g *GhostManager) Sessions() []*ManagedSession {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]*ManagedSession, 0, len(g.sessions))
	for _, s := range g.sessions {
		out = append(out, s)
	}
	return out
}

// GetByConnID returns the ghost session for the given connection ID, or nil.
func (g *GhostManager) GetByConnID(connID string) *ManagedSession {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessions[connID]
}

// StartConn starts a ghost session for a single connection.
// It is safe to call for connections already managed — in that case it is a no-op.
func (g *GhostManager) StartConn(conn config.Connection) {
	if conn.Protocol != config.ProtoSSH {
		return
	}
	g.mu.Lock()
	_, exists := g.sessions[conn.ID]
	g.mu.Unlock()
	if exists {
		return
	}
	g.startGhost(conn)
}

// StopConn kills and removes the ghost session for the given connection ID.
func (g *GhostManager) StopConn(connID string) {
	g.mu.Lock()
	sess, ok := g.sessions[connID]
	if ok {
		delete(g.sessions, connID)
	}
	g.mu.Unlock()

	if ok && sess != nil {
		sess.Kill()
	}
}

// startGhost creates and starts a background session for conn, then monitors
// it for reconnection if conn.AutoReconnect is true.
func (g *GhostManager) startGhost(conn config.Connection) {
	sess := g.newSession(conn)
	g.mu.Lock()
	g.sessions[conn.ID] = sess
	g.mu.Unlock()

	go func() {
		_ = sess.StartBackground()
		g.monitorAndReconnect(conn, sess)
	}()
}

// monitorAndReconnect waits for sess to end, then re-connects with exponential
// backoff if conn.AutoReconnect is true and the manager has not been stopped.
func (g *GhostManager) monitorAndReconnect(conn config.Connection, sess *ManagedSession) {
	// Wait for the session's done channel.
	select {
	case <-g.done:
		return
	case <-sess.doneCh:
	}

	if !conn.AutoReconnect {
		g.mu.Lock()
		delete(g.sessions, conn.ID)
		g.mu.Unlock()
		return
	}

	backoff := ghostReconnectInitial
	for {
		// Check if we've been stopped.
		select {
		case <-g.done:
			return
		default:
		}

		// Wait before reconnecting.
		select {
		case <-g.done:
			return
		case <-time.After(backoff):
		}

		// Double the backoff for next attempt, capped at max.
		backoff *= 2
		if backoff > ghostReconnectMax {
			backoff = ghostReconnectMax
		}

		// Create a fresh session.
		newSess := g.newSession(conn)
		g.mu.Lock()
		g.sessions[conn.ID] = newSess
		g.mu.Unlock()

		if err := newSess.StartBackground(); err != nil {
			// Connection failed; loop again after backoff.
			// Replace doneCh so the wait below works correctly.
			sess = newSess
			select {
			case <-g.done:
				return
			case <-sess.doneCh:
			}
			continue
		}

		// Connected — reset backoff and wait for next disconnect.
		backoff = ghostReconnectInitial
		sess = newSess
		select {
		case <-g.done:
			return
		case <-sess.doneCh:
		}

		if !conn.AutoReconnect {
			break
		}
	}

	g.mu.Lock()
	delete(g.sessions, conn.ID)
	g.mu.Unlock()
}

// newSession allocates a ManagedSession for the given connection.
func (g *GhostManager) newSession(conn config.Connection) *ManagedSession {
	g.mu.Lock()
	g.nextID++
	id := fmt.Sprintf("g%d", g.nextID)
	g.mu.Unlock()

	sess := NewManagedSession(ManagedSessionOptions{
		ID:           id,
		Name:         conn.Name,
		ConnID:       conn.ID,
		Protocol:     string(conn.Protocol),
		Host:         conn.Host,
		Port:         conn.EffectivePort(),
		Username:     conn.Username,
		Password:     conn.Password,
		IdentityFile: conn.IdentityFile,
		ProxyJump:    conn.ProxyJump,
		ProxyCommand: conn.ProxyCommand,
	})
	sess.PortForwards = conn.PortForwards
	sess.IsGhost = true
	if !conn.Hooks.IsEmpty() {
		sess.SetHooks(conn.Hooks)
	}
	return sess
}
