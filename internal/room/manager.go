package room

import (
	"sort"
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

// Create allocates a room with a fresh, unused code. A public room is listed by
// List; a private one is reachable only by its code.
func (m *Manager) Create(public bool) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		code := NewCode()
		if _, taken := m.rooms[code]; taken {
			continue
		}
		r := newRoom(code, public)
		m.rooms[code] = r
		return r
	}
}

// List returns the public rooms somebody could actually walk into: still in the
// lobby, with a host present and a seat free.
//
// Each room describes itself through its own command channel, so this never
// reads room-owned state. Rooms that don't answer promptly are left out rather
// than allowed to hold up the listing.
func (m *Manager) List() []Summary {
	m.mu.RLock()
	candidates := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		if r.Public {
			candidates = append(candidates, r)
		}
	}
	m.mu.RUnlock()

	out := make([]Summary, 0, len(candidates))
	for _, r := range candidates {
		s, ok := r.Summarize(250 * time.Millisecond)
		if ok && s.Joinable {
			out = append(out, s)
		}
	}
	// Stable order so the list doesn't shuffle under a tapping finger between
	// polls; Go randomises map iteration.
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// SummarizeAll is List without the joinable filter, for tests and diagnostics.
func (m *Manager) SummarizeAll() []Summary {
	m.mu.RLock()
	all := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		all = append(all, r)
	}
	m.mu.RUnlock()

	out := make([]Summary, 0, len(all))
	for _, r := range all {
		if s, ok := r.Summarize(250 * time.Millisecond); ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
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
