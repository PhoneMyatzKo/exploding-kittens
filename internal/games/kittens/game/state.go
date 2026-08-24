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
	PhaseDefuse   Phase = "defuse"    // a player is putting a drawn kitten back
	PhaseAlter    Phase = "alter"     // a player is reordering the top of the deck
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
	// PendCatTriple is three matching cats: the actor names a card and takes it
	// from the target if they hold one.
	PendCatTriple PendingKind = "cat_triple"

	// Imploding Kittens expansion.
	PendReverse  PendingKind = "reverse"
	PendBottom   PendingKind = "bottom"
	PendAlter    PendingKind = "alter"
	PendTargeted PendingKind = "targeted_attack"
)

// PendingAction is an effect that has been played but not yet resolved, because
// any player may still Nope it.
type PendingAction struct {
	Kind     PendingKind
	ActorID  string
	TargetID string // Favor and cat sets only
	Cards    []Card // the cards that were played to trigger this

	// Named is the card a Three of a Kind is demanding. Public: you say it out
	// loud, and everybody hears whether it landed.
	Named    CardType
	HasNamed bool

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
	// Variant is which printed sets are in this deck. Fixed at the deal.
	Variant Variant

	Players []*Player
	Draw    []Card // top-first
	Discard []Card // last element is the visible top

	Current        int // index into Players
	TurnsRemaining int // turns the current player still owes, including this one
	// Direction is +1 for normal play and -1 once a Reverse has been played. It
	// is a field rather than a bool so advance() can just add it.
	Direction int
	Phase     Phase
	Pending   *PendingAction

	// PendingKitten is the kitten being put back during PhaseDefuse — either an
	// Exploding Kitten that was defused, or the Imploding Kitten on its first
	// appearance, which is put back face up and costs no Defuse.
	PendingKitten *Card
	// FavorRequester is who gets the card during PhaseFavor.
	FavorRequester string
	// Altering is who must reorder the top of the deck during PhaseAlter.
	Altering string

	WinnerID string

	rng *rand.Rand
}

// alterCount is how many cards Alter the Future exposes, capped by the deck.
func (s *State) alterCount() int {
	if len(s.Draw) < 3 {
		return len(s.Draw)
	}
	return 3
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
	s.Current = s.nextAliveAfter(s.Current)
	s.TurnsRemaining = 1
}

// nextAliveAfter returns the index of the living player following i in the
// current direction of play, or i itself if nobody else is left.
func (s *State) nextAliveAfter(i int) int {
	step := s.step()
	n := len(s.Players)
	for k := 1; k <= n; k++ {
		// +n keeps the modulo positive when play has been reversed.
		next := ((i+k*step)%n + n) % n
		if s.Players[next].Alive {
			return next
		}
	}
	return i
}

// step is the direction of play, defaulting to forwards so a State built without
// going through NewGame (the rules tests do this) still turns over.
func (s *State) step() int {
	if s.Direction < 0 {
		return -1
	}
	return 1
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
