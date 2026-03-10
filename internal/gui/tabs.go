package gui

import "sync"

// Tab represents a single graphical session tab.
type Tab struct {
	ConnID    string
	Protocol  string
	Label     string
	Session   Session
	Status    string // "connecting", "connected", "error", "closed"
	Error     error
	ReplyFunc func(string, interface{}) error
}

// TabManager manages a collection of session tabs.
type TabManager struct {
	tabs     []*Tab
	activeID string
	mu       sync.RWMutex
}

// NewTabManager creates a new tab manager.
func NewTabManager() *TabManager {
	return &TabManager{}
}

// Add adds a tab and makes it active.
func (tm *TabManager) Add(tab *Tab) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tabs = append(tm.tabs, tab)
	tm.activeID = tab.ConnID
}

// Remove removes a tab by connection ID.
func (tm *TabManager) Remove(connID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, tab := range tm.tabs {
		if tab.ConnID == connID {
			tm.tabs = append(tm.tabs[:i], tm.tabs[i+1:]...)
			// If we removed the active tab, activate the nearest
			if tm.activeID == connID {
				if len(tm.tabs) > 0 {
					idx := i
					if idx >= len(tm.tabs) {
						idx = len(tm.tabs) - 1
					}
					tm.activeID = tm.tabs[idx].ConnID
				} else {
					tm.activeID = ""
				}
			}
			return
		}
	}
}

// Get returns a tab by connection ID.
func (tm *TabManager) Get(connID string) *Tab {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, tab := range tm.tabs {
		if tab.ConnID == connID {
			return tab
		}
	}
	return nil
}

// Active returns the currently active tab.
func (tm *TabManager) Active() *Tab {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, tab := range tm.tabs {
		if tab.ConnID == tm.activeID {
			return tab
		}
	}
	return nil
}

// Focus sets the active tab.
func (tm *TabManager) Focus(connID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, tab := range tm.tabs {
		if tab.ConnID == connID {
			tm.activeID = connID
			return
		}
	}
}

// All returns all tabs.
func (tm *TabManager) All() []*Tab {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]*Tab, len(tm.tabs))
	copy(result, tm.tabs)
	return result
}

// Count returns the number of tabs.
func (tm *TabManager) Count() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.tabs)
}
