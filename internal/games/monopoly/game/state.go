// Package game implements Monopoly Myanmar's rules.
//
// A pure reducer, like the other games here: Apply(state, action) mutates the
// state and returns what happened, and knows nothing about sockets, JSON or
// players' connections.
//
// The board is in board.go, the two decks in cards.go and draw.go, and building
// in build.go. What the original has and this does not yet is listed at the
// bottom of engine.go, so the gap is written down rather than discovered.
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
	// PhaseCard: a Chance or Community Chest card is face up in front of them
	// and they have to read it before the table moves on. A phase rather than an
	// instant resolution because the card *is* the moment — resolving it silently
	// would mean the only place a player learns what happened is the log.
	PhaseCard Phase = "card"
	// PhaseJail: they are being held, and choose how to get out.
	PhaseJail Phase = "jail"
	// PhaseGameOver: one player left standing.
	PhaseGameOver Phase = "gameOver"
)

// JailFine is what buying your way out costs.
const JailFine = 50 * kyat

// JailAttempts is how many turns you may spend trying to throw doubles before
// the fine is taken and you are let out anyway — the original's three.
const JailAttempts = 3

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

	// Jailed is true while they are being held, as opposed to standing on the
	// corner as a visitor. Tries counts the turns spent attempting doubles.
	Jailed bool
	Tries  int
	// Pardons is how many get-out-of-jail cards they are holding.
	Pardons int
	// Missing is turns still to be sat out. Decremented as their turn comes
	// round, so "miss a turn" costs exactly one.
	Missing int
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

	// Houses is how many buildings stand on each square: 0 to 4 houses, and
	// HotelLevel for a hotel. Only a property can carry any — a station or a
	// utility charges by how many of its kind you hold instead.
	//
	// Indexed by square like Owner, and for the same reason: rent is looked up on
	// every landing, and a flat array is what the client draws from too. The bank's
	// stock of houses is *not* stored beside it; it is counted from here, so the
	// two cannot drift apart. See build.go.
	Houses [BoardSize]int

	// The two card piles, as indices into the pack in cards.go. Held as indices
	// rather than as cards so a saved game is a list of small integers and the
	// text lives in exactly one place.
	//
	// Drawn from the front; the used ones go to the back of Discard and the pile
	// is refilled and reshuffled when it runs out, which is how a physical deck
	// behaves. A pardon somebody is holding is in neither pile — it comes back
	// only when it is spent.
	ChanceDraw    []int
	ChanceDiscard []int
	ChestDraw     []int
	ChestDiscard  []int
	// Drawn is the card face up in front of the current player during PhaseCard,
	// as an index into the pack, or -1.
	Drawn int

	// WinnerID is set once, when the game ends.
	WinnerID string
}

// DrawnCard is the card being read, or nil when none is.
func (s *State) DrawnCard() *Card {
	if s.Phase != PhaseCard || s.Drawn < 0 || s.Drawn >= len(cards) {
		return nil
	}
	c := cards[s.Drawn]
	return &c
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
//
// Two things happen on the way past a seat. A player owing missed turns loses one
// here and is stepped over — which is what makes "miss a turn" cost exactly one
// turn rather than however long the loop happens to take. And a player being held
// lands in PhaseJail rather than PhaseRoll, because their choice is how to get
// out, not where to move.
//
// Returns the events worth reporting: a turn silently skipped looks like the
// table forgetting whose go it is.
func (s *State) advance() []Event {
	s.Doubles = 0
	var events []Event
	n := len(s.Players)
	if n == 0 {
		return nil
	}
	// A local cursor, and s.Current written only when a seat actually takes its
	// turn. Walking with `s.Current + step` while also assigning to s.Current —
	// which is what this did first — advances the base and the offset together and
	// steps over twice as many seats as it should.
	//
	// Bounded so a table where everybody owes turns resolves rather than spinning.
	cursor := s.Current
	for guard := 0; guard < n*(maxMissed+1); guard++ {
		cursor = (cursor + 1) % n
		p := &s.Players[cursor]
		if !p.Alive {
			continue
		}
		if p.Missing > 0 {
			p.Missing--
			events = append(events, Event{Kind: EvMissTurn, ActorID: p.ID, Tile: -1})
			continue
		}
		s.Current = cursor
		if p.Jailed {
			s.Phase = PhaseJail
		} else {
			s.Phase = PhaseRoll
		}
		return events
	}
	return events
}

// maxMissed bounds how many turns a card may cost, and so how far advance() has
// to look. No card in the pack asks for more than one; the bound is here so a
// future card that asks for three cannot wedge the loop.
const maxMissed = 5

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
