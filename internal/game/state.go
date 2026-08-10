package game

import "math/rand"

// Phase drives what each client is allowed to do. Every transition is decided by
// the engine; clients only render the affordances a phase implies.
type Phase string

const (
	PhaseLobby    Phase = "lobby"
	PhaseTurn     Phase = "turn"      // current player is choosing what to do
	PhaseNope     Phase = "nope"      // an action is on the table, awaiting Nopes
	PhaseFavor    Phase = "favor"     // Favor target is choosing a card to give
	PhaseDefuse   Phase = "defuse"    // a player is reinserting a defused Kitten
	PhaseGameOver Phase = "game_over" //
)

// PendingKind identifies the effect waiting behind an open Nope window.
type PendingKind string

const (
	PendSkip    PendingKind = "skip"
	PendAttack  PendingKind = "attack"
	PendFavor   PendingKind = "favor"
	PendShuffle PendingKind = "shuffle"
	PendFuture  PendingKind = "future"
	PendCatPair PendingKind = "cat_pair"
)

// PendingAction is an effect that has been played but not yet resolved, because
// any player may still Nope it.
type PendingAction struct {
	Kind     PendingKind
	ActorID  string
	TargetID string // Favor and cat-pair only
	Cards    []Card // the cards that were played to trigger this

	// Nopes counts the Nope cards stacked on top. An odd count cancels the action.
	Nopes int
	// LastPlayerID is whoever most recently added a card to the stack. They may
	// not close their own window by passing, and are not asked to.
	LastPlayerID string
	// Passed records which eligible players have declined to Nope.
	Passed map[string]bool
}

// Cancelled reports whether the accumulated Nopes negate the action.
func (p *PendingAction) Cancelled() bool { return p.Nopes%2 == 1 }

// Player is one seat at the table.
type Player struct {
	ID    string
	Name  string
	Hand  []Card
	Alive bool
}

func (p *Player) hasType(t CardType) bool {
	for _, c := range p.Hand {
		if c.Type == t {
			return true
		}
	}
	return false
}

func (p *Player) findType(t CardType) (Card, bool) {
	for _, c := range p.Hand {
		if c.Type == t {
			return c, true
		}
	}
	return Card{}, false
}

// State is the complete, unredacted game. It contains the deck order and every
// hand, so it must never be serialised to a client — see internal/view.
type State struct {
	Players []*Player
	Draw    []Card // top-first
	Discard []Card // last element is the visible top

	Current        int // index into Players
	TurnsRemaining int // turns the current player still owes, including this one
	Phase          Phase
	Pending        *PendingAction

	// PendingKitten is the Exploding Kitten being reinserted during PhaseDefuse.
	PendingKitten *Card
	// FavorRequester is who gets the card during PhaseFavor.
	FavorRequester string

	WinnerID string

	rng *rand.Rand
}

func (s *State) playerByID(id string) *Player {
	for _, p := range s.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *State) current() *Player { return s.Players[s.Current] }

func (s *State) aliveCount() int {
	n := 0
	for _, p := range s.Players {
		if p.Alive {
			n++
		}
	}
	return n
}

// advance moves the turn to the next living player and resets their turn debt to
// a single turn. Callers that need to hand over extra turns (Attack) set
// TurnsRemaining afterwards.
func (s *State) advance() {
	for i := 1; i <= len(s.Players); i++ {
		next := (s.Current + i) % len(s.Players)
		if s.Players[next].Alive {
			s.Current = next
			s.TurnsRemaining = 1
			return
		}
	}
}

// nextAliveAfter returns the index of the living player following i, or i itself
// if nobody else is left.
func (s *State) nextAliveAfter(i int) int {
	for k := 1; k <= len(s.Players); k++ {
		n := (i + k) % len(s.Players)
		if s.Players[n].Alive {
			return n
		}
	}
	return i
}

// endTurn burns one of the current player's turns. If they still owe more (they
// were Attacked), they go again; otherwise play passes on.
func (s *State) endTurn() {
	s.TurnsRemaining--
	if s.TurnsRemaining > 0 {
		return
	}
	s.advance()
}

// checkWin promotes the game to game over once a single player is left standing.
func (s *State) checkWin() bool {
	if s.aliveCount() > 1 {
		return false
	}
	s.Phase = PhaseGameOver
	for _, p := range s.Players {
		if p.Alive {
			s.WinnerID = p.ID
		}
	}
	return true
}
