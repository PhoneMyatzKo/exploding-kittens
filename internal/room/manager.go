package room

import (
	"sync"
	"time"
)

// Manager is the registry of live rooms. It is the only shared mutable state in
// the server, and its lock never covers game logic — just the map.
type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room
	stop  chan struct{}
}

func NewManager() *Manager {
	m := &Manager{rooms: map[string]*Room{}, stop: make(chan struct{})}
	go m.reap()
	return m
}

// Create allocates a room with a fresh, unused code.
func (m *Manager) Create() *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		code := NewCode()
		if _, taken := m.rooms[code]; taken {
			continue
		}
		r := newRoom(code)
		m.rooms[code] = r
		return r
	}
}

// Get looks up a room by a possibly-messy user-typed code.
func (m *Manager) Get(code string) (*Room, error) {
	code = NormalizeCode(code)
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rooms[code]
	if !ok {
		return nil, ErrUnknownRoom
	}
	return r, nil
}

// Count is the number of live rooms, for the health endpoint.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

// Shutdown closes every room.
func (m *Manager) Shutdown() {
	close(m.stop)
	m.mu.Lock()
	defer m.mu.Unlock()
	for code, r := range m.rooms {
		r.close()
		delete(m.rooms, code)
	}
}

// reap discards rooms that have sat with nobody connected. The question is asked
// through each room's own command channel so the answer is computed by the
// goroutine that owns the data.
func (m *Manager) reap() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.mu.RLock()
			snapshot := make([]*Room, 0, len(m.rooms))
			for _, r := range m.rooms {
				snapshot = append(snapshot, r)
			}
			m.mu.RUnlock()

			for _, r := range snapshot {
				reply := make(chan bool, 1)
				select {
				case r.cmds <- cmdIdleCheck{reply: reply}:
				case <-r.done:
					continue
				case <-time.After(time.Second):
					continue
				}
				if <-reply {
					m.mu.Lock()
					delete(m.rooms, r.Code)
					m.mu.Unlock()
					r.close()
				}
			}
		}
	}
}
