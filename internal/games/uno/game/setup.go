package game

import (
	"boardgame/kittens/internal/prng"
	"errors"
)

const (
	// MinPlayers and MaxPlayers bound the printed rules: two to ten.
	MinPlayers = 2
	MaxPlayers = 10
	handSize   = 7
	// DefaultTarget is the official winning score.
	DefaultTarget = 500
)

// Seat identifies a player joining a new game.
type Seat struct {
	ID   string
	Name string
}

var ErrPlayerCount = errors.New("uno needs 2 to 10 players")

// NewGame deals a game to the target score.
func NewGame(seats []Seat, rng *prng.Source) (*State, []Event, error) {
	return NewGameTo(seats, rng, DefaultTarget)
}

// NewGameTo deals a game that ends when somebody reaches target points. A target
// of zero makes it a single round, which is how a table that only has ten
// minutes plays.
func NewGameTo(seats []Seat, rng *prng.Source, target int) (*State, []Event, error) {
	n := len(seats)
	if n < MinPlayers || n > MaxPlayers {
		return nil, nil, ErrPlayerCount
	}
	if rng == nil {
		rng = prng.NewSeeded()
	}
	if target < 0 {
		target = 0
	}

	s := &State{Target: target, Seats: append([]Seat(nil), seats...), RNG: rng}
	for _, seat := range seats {
		s.Players = append(s.Players, &Player{ID: seat.ID, Name: seat.Name})
	}
	// The first dealer is random so hosting isn't an advantage.
	events := s.deal(rng.Intn(n))
	return s, events, nil
}

// deal shuffles a fresh deck, hands out seven each and turns the starting card
// over. first is the seat the turn starts on, before the opening card has had
// its say about that.
//
// Scores survive; everything else is rebuilt, so this is also how the second and
// subsequent rounds begin.
func (s *State) deal(first int) []Event {
	deck := fullDeck()
	shuffle(deck, s.RNG)

	for _, p := range s.Players {
		p.Hand = make([]Card, handSize)
		copy(p.Hand, deck[:handSize])
		deck = deck[handSize:]
	}

	// A Wild Draw Four may not start the game: it goes back in and another card
	// comes off the top. Anything else is a legal starter, effects and all.
	var aside []Card
	var starter Card
	for len(deck) > 0 {
		c := deck[0]
		deck = deck[1:]
		if c.Rank == RankWildDrawFour {
			aside = append(aside, c)
			continue
		}
		starter = c
		break
	}
	if len(aside) > 0 {
		deck = append(deck, aside...)
		shuffle(deck, s.RNG)
	}

	s.Draw = deck
	s.Discard = []Card{starter}
	s.Colour = starter.Colour
	s.Current = first
	s.Dir = 1
	s.Turns = 0
	s.Phase = PhaseTurn
	s.Drawn = nil
	s.Wild = nil
	s.Uno = nil
	s.RoundWinnerID = ""
	s.Round++

	events := []Event{{
		Kind: EvStarted, Cards: []Card{starter},
		Colour: s.Colour.Slug(), Points: s.Round,
	}}
	return append(events, append(s.openingEffect(starter), s.turnEvents()...)...)
}

// openingEffect applies the starting card's symbol, which counts as if it had
// been played at the first player.
//
//	Wild      the first player names the colour
//	Skip      the first player loses their turn
//	Reverse   play runs the other way from the off
//	Draw Two  the first player takes two and loses their turn
func (s *State) openingEffect(starter Card) []Event {
	switch starter.Rank {
	case RankWild:
		// No colour is in force yet, so nothing is playable until one is named.
		s.Colour = NoColour
		s.Phase = PhaseColour
		s.Wild = &wildPlay{Card: starter, ActorID: s.current().ID, opening: true}
		return nil

	case RankSkip:
		p := s.current()
		s.advance(1)
		return []Event{{Kind: EvSkipped, ActorID: p.ID}}

	case RankReverse:
		// The turn stays put and the direction flips, so play moves away from the
		// first player the other way round rather than towards them.
		s.Dir = -1
		return []Event{{Kind: EvReversed, ActorID: s.current().ID}}

	case RankDrawTwo:
		p := s.current()
		drawn, _ := s.drawCards(p, 2)
		s.advance(1)
		return append(drawEvents(p, drawn, "opening draw two"),
			Event{Kind: EvSkipped, ActorID: p.ID})
	}
	return nil
}
