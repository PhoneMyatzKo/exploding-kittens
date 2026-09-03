// Package game implements Monopoly Myanmar's rules.
//
// A pure reducer, like the other games here: Apply(state, action) mutates the
// state and returns what happened, and knows nothing about sockets, JSON or
// players' connections. The board itself is in board.go.
//
// Scope, deliberately: this is the first slice — dice, movement, buying an
// unowned square, paying rent, taxes, and going bankrupt. What the original has
// and this does not yet is listed at the bottom of engine.go so the gap is
// written down rather than discovered.
package game

import "boardgame/kittens/internal/prng"

// MinPlayers and MaxPlayers bound the table. The original takes up to eight
// tokens; six is what the seat strip shows without scrolling, and a six-player
// game is already long.
const (
	MinPlayers = 2
	MaxPlayers = 6
)

// Seat is one player being dealt in.
type Seat struct {
	ID   string
	Name string
}

// Phase is what the table is waiting for. Every phase names exactly one player
// who has to act, which is what lets the room's idle watchdog keep a table
// moving when somebody closes their laptop.
type Phase string

const (
	// PhaseRoll: the current player has not rolled yet.
	PhaseRoll Phase = "roll"
	// PhaseBuy: they landed on an unowned square and must buy or pass on it.
	PhaseBuy Phase = "buy"
	// PhaseGameOver: one player left standing.
	PhaseGameOver Phase = "gameOver"
)

// Player is one token on the board.
type Player struct {
	ID   string
	Name string
	Cash int
	// Pos is a square index, always 0..39.
	Pos int
	// Alive is false once bankrupt. A bankrupt player stays in the list so the
	// log and the seat strip can still name them.
	Alive bool
}

// State is one game.
type State struct {
	// RNG is the game's own dice and shuffles. Exported for one reason: it is
	// state, and a game that cannot write its randomness down cannot be saved and
	// resumed. Kept off the wire with json:"-" — gob, which is what snapshots use,
	// ignores the tag and saves it. Nothing outside this package should draw from
	// it; it is here to be persisted, not to be used.
	RNG *prng.Source `json:"-"`

	Players []Player
	// Current indexes Players. Play runs in seat order; there is no Reverse here.
	Current int
	Phase   Phase
	// Dice is the last roll, both faces, so the client can show what was thrown
	// rather than only the total.
	Dice [2]int
	// Doubles counts consecutive doubles this turn. Three sends you to jail,
	// which is what stops a lucky streak going round the board forever.
	Doubles int
	// Owner maps a square index to the player holding it, or "" for the bank.
	// Indexed by tile so a lookup during rent is an array read, and so the whole
	// thing marshals as a plain slice.
	Owner [BoardSize]string
	// Pending is the square awaiting a buy-or-pass decision during PhaseBuy.
	Pending int
	// WinnerID is set once, when the game ends.
	WinnerID string
}

// CurrentID is whoever the table is waiting for.
func (s *State) CurrentID() string {
	if s.Current < 0 || s.Current >= len(s.Players) {
		return ""
	}
	return s.Players[s.Current].ID
}

// Find is a player by id, or nil.
func (s *State) Find(playerID string) *Player {
	for i := range s.Players {
		if s.Players[i].ID == playerID {
			return &s.Players[i]
		}
	}
	return nil
}

// PendingTile is the square being offered, or -1 when nothing is.
func (s *State) PendingTile() int {
	if s.Phase != PhaseBuy {
		return -1
	}
	return s.Pending
}

// OwnerOf is who holds a square, or "" for the bank.
func (s *State) OwnerOf(pos int) string {
	if pos < 0 || pos >= BoardSize {
		return ""
	}
	return s.Owner[pos]
}

// Owned lists the squares a player holds, in board order.
func (s *State) Owned(playerID string) []int {
	var out []int
	for i, owner := range s.Owner {
		if owner == playerID && playerID != "" {
			out = append(out, i)
		}
	}
	return out
}

// aliveCount is how many players are still in.
func (s *State) aliveCount() int {
	n := 0
	for _, p := range s.Players {
		if p.Alive {
			n++
		}
	}
	return n
}

// advance passes play to the next player still in the game, and resets the
// per-turn state that belongs to whoever is leaving.
func (s *State) advance() {
	s.Doubles = 0
	n := len(s.Players)
	for k := 1; k <= n; k++ {
		next := (s.Current + k) % n
		if s.Players[next].Alive {
			s.Current = next
			s.Phase = PhaseRoll
			return
		}
	}
}

// ownsGroup reports whether a player holds every property in a colour set, which
// doubles the unimproved rent on all of them.
func (s *State) ownsGroup(playerID, group string) bool {
	if group == "" || playerID == "" {
		return false
	}
	for i, t := range board {
		if t.Group == group && s.Owner[i] != playerID {
			return false
		}
	}
	return true
}

// countKind is how many stations or utilities a player holds, which is what
// their rent is scaled by.
func (s *State) countKind(playerID string, kind TileKind) int {
	if playerID == "" {
		return 0
	}
	n := 0
	for i, t := range board {
		if t.Kind == kind && s.Owner[i] == playerID {
			n++
		}
	}
	return n
}
