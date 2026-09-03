package game

import "boardgame/kittens/internal/prng"

// Phase drives what each client is allowed to do. Every transition is decided by
// the engine; clients only render the affordances a phase implies.
type Phase string

const (
	PhaseTurn      Phase = "turn"       // current player plays a card or draws
	PhaseDrawn     Phase = "drawn"      // they drew a playable card: use it or pass
	PhaseColour    Phase = "colour"     // a wild is down and wants a colour
	PhaseChallenge Phase = "challenge"  // a Wild Draw Four is aimed at you: take it or call it
	PhaseRoundOver Phase = "round_over" // a hand was scored; deal the next one
	PhaseGameOver  Phase = "game_over"  // somebody reached the target score
)

// Player is one seat at the table. Nobody is ever eliminated in UNO — a bad
// round costs points, not your place — so there is no Alive flag to check.
type Player struct {
	ID   string
	Name string
	Hand []Card
	// Score is the running total across rounds, not this round's points.
	Score int
}

// hasColour reports whether the hand holds a card of a printed colour. This is
// the question a Wild Draw Four challenge turns on, so it deliberately ignores
// the wilds: holding one is never an excuse.
func (p *Player) hasColour(c Colour) bool {
	for _, card := range p.Hand {
		if card.Colour == c {
			return true
		}
	}
	return false
}

func (p *Player) findCard(id int) (Card, bool) {
	for _, c := range p.Hand {
		if c.ID == id {
			return c, true
		}
	}
	return Card{}, false
}

// unoWindow is the interval in which a player who is down to one card can be
// caught not having said so.
//
// The official rule is "before the next player begins their turn", which on a
// physical table is about a second and a half of shouting. Online there is
// latency between every pair of players, so the window here lasts until the next
// player's turn has finished — long enough that a fast connection isn't the
// thing that wins the game.
type unoWindow struct {
	PlayerID string
	Called   bool
	// openedOn is State.Turns when the window opened.
	openedOn int
}

// wildPlay is a wild card that has been laid down but hasn't finished resolving:
// it may still be waiting for a colour, and if it is a Draw Four, for the target
// to take the cards or call the bluff.
type wildPlay struct {
	Card     Card
	ActorID  string
	TargetID string
	// Illegal records whether the actor was actually out of the colour in force
	// when they played a Draw Four. Computed at play time from the hand as it was
	// then, because that is the only moment at which the question has an answer.
	Illegal bool
	// opening marks the wild the game was dealt on. Nobody played it, so naming
	// its colour must not cost the first player the turn they haven't had yet.
	opening bool
}

// State is the complete, unredacted game. It contains the deck order and every
// hand, so it must never be serialised to a client — see internal/view.
type State struct {
	Players []*Player
	Draw    []Card // top-first
	Discard []Card // last element is the visible top

	Current int
	// Dir is +1 for the direction the game was dealt in and -1 after an odd
	// number of Reverses.
	Dir int
	// Turns counts completed turns for the life of the game. Only the UNO window
	// reads it, and only as a clock.
	Turns int

	Phase Phase
	// Colour is the colour in force. It is not always the top card's own colour:
	// a wild names one, and that name outlives the card it was made on.
	Colour Colour

	// Drawn is the card the current player has just taken and may still play. Set
	// only during PhaseDrawn, and it is already in their hand.
	Drawn *Card
	// Wild is a wild card mid-resolution, during PhaseColour or PhaseChallenge.
	Wild *wildPlay
	// Uno is the open catch-them-out window, if any.
	Uno *unoWindow

	// Target is the score that wins the game. Zero means the game is a single
	// round, which is how a quick table plays it.
	Target int
	Round  int
	// RoundWinnerID is who went out last round. Kept through PhaseRoundOver so
	// the scoreboard can say who earned the points.
	RoundWinnerID string
	WinnerID      string

	// Seats is kept so a second round can be dealt without the room having to
	// remember who was playing. Seats do not change mid-game: somebody who
	// disconnects is still holding a hand and still owes points.
	//
	// Seats and RNG are exported for one reason: they are state, and a game that
	// cannot write them down cannot be saved and resumed. Both are kept off the
	// wire with json:"-" — gob, which is what snapshots use, ignores the tag and
	// saves them. Nothing outside this package should touch either.
	Seats []Seat       `json:"-"`
	RNG   *prng.Source `json:"-"`
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

// step returns the index one seat along in the current direction.
func (s *State) step(i int) int {
	n := len(s.Players)
	return ((i+s.Dir)%n + n) % n
}

// top is the face-up card of the discard pile, or nil before the first flip.
func (s *State) top() *Card {
	if len(s.Discard) == 0 {
		return nil
	}
	return &s.Discard[len(s.Discard)-1]
}

// playable reports whether a card may legally be laid on the current top: the
// colour in force, or the same symbol, or any wild.
//
// The rank comparison skips wild tops on purpose. Once a Wild has been played
// the colour it named is the only thing that matters; "my wild matches your
// wild" is not a rule, and treating it as one would let two wilds chain forever.
func (s *State) playable(c Card) bool {
	if c.Rank.IsWild() {
		return true
	}
	if c.Colour == s.Colour {
		return true
	}
	t := s.top()
	return t != nil && !t.Rank.IsWild() && c.Rank == t.Rank
}

// advance hands the turn on, n seats along. n is 2 for a Skip.
func (s *State) advance(n int) {
	s.Turns++
	for i := 0; i < n; i++ {
		s.Current = s.step(s.Current)
	}
	s.closeStaleUno()
}

// closeStaleUno shuts the catch window once its time is up, or once the player
// it was about is no longer on one card — drawing back up makes the whole
// question moot.
func (s *State) closeStaleUno() {
	if s.Uno == nil {
		return
	}
	p := s.playerByID(s.Uno.PlayerID)
	if p == nil || len(p.Hand) != 1 || s.Turns > s.Uno.openedOn+1 {
		s.Uno = nil
	}
}

// turnEvents emits a turn marker whenever the game is back in a state where the
// active player is the one expected to act.
func (s *State) turnEvents() []Event {
	switch s.Phase {
	case PhaseTurn, PhaseColour:
		return []Event{{Kind: EvTurn, ActorID: s.current().ID, Colour: s.Colour.Slug()}}
	}
	return nil
}
