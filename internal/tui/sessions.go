package tui

import (
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/session"
)

// SessionDetachedMsg is sent when a user detaches from a managed session.
type SessionDetachedMsg struct {
	SessionID string
	ConnID    string
	ConnName  string
}

// SessionDiedMsg is sent when a managed session's SSH connection ends.
type SessionDiedMsg struct {
	SessionID string
	ConnID    string
	ConnName  string
	Err       error
}

// SessionManager tracks all active managed sessions.
type SessionManager struct {
	sessions []*session.ManagedSession
	mu       sync.RWMutex
	nextID   int
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// Add adds a managed session with an auto-incrementing ID like s1, s2, etc.
func (sm *SessionManager) Add(sess *session.ManagedSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.nextID++
	sess.ID = fmt.Sprintf("s%d", sm.nextID)
	sm.sessions = append(sm.sessions, sess)
}

// Remove removes a session by ID.
func (sm *SessionManager) Remove(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for i, s := range sm.sessions {
		if s.ID == id {
			sm.sessions = append(sm.sessions[:i], sm.sessions[i+1:]...)
			return
		}
	}
}

// Get returns a session by ID.
func (sm *SessionManager) Get(id string) *session.ManagedSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, s := range sm.sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// All returns a copy of all sessions.
func (sm *SessionManager) All() []*session.ManagedSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]*session.ManagedSession, len(sm.sessions))
	copy(result, sm.sessions)
	return result
}

// Count returns the number of active sessions.
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// CleanDead removes sessions that are no longer alive.
func (sm *SessionManager) CleanDead() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	alive := sm.sessions[:0]
	for _, s := range sm.sessions {
		if s.IsAlive() {
			alive = append(alive, s)
		}
	}
	sm.sessions = alive
}

// watchSessionDone polls a session's liveness and returns SessionDiedMsg when it ends.
func watchSessionDone(sess *session.ManagedSession) tea.Cmd {
	id := sess.ID
	connID := sess.ConnID
	name := sess.Name
	return func() tea.Msg {
		for {
			time.Sleep(500 * time.Millisecond)
			if !sess.IsAlive() {
				return SessionDiedMsg{
					SessionID: id,
					ConnID:    connID,
					ConnName:  name,
				}
			}
		}
	}
}
