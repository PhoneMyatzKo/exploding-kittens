package game

import (
	"errors"
	"math/rand"
)

// MinPlayers and MaxPlayers bound the Original Edition.
const (
	MinPlayers = 2
	MaxPlayers = 5
	handSize   = 7 // dealt cards, before the guaranteed Defuse is added
)

// Seat identifies a player joining a new game.
type Seat struct {
	ID   string
	Name string
}

var ErrPlayerCount = errors.New("exploding kittens needs 2 to 5 players")

// NewGame performs the official setup:
//
//	remove all Kittens and Defuses -> shuffle -> deal 7 each -> give everyone one
//	Defuse -> return the leftover Defuses to the deck -> insert (n-1) Kittens ->
//	shuffle.
//
// The resulting deck therefore always has exactly one fewer Kitten than there are
// players, which is what guarantees the game ends.
func NewGame(seats []Seat, rng *rand.Rand) (*State, error) {
	n := len(seats)
	if n < MinPlayers || n > MaxPlayers {
		return nil, ErrPlayerCount
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}

	deck := fullDeck(rng)
	deck, kittens := extract(deck, ExplodingKitten)
	deck, defuses := extract(deck, Defuse)
	shuffle(deck, rng)

	s := &State{
		Phase:          PhaseTurn,
		TurnsRemaining: 1,
		rng:            rng,
	}

	for _, seat := range seats {
		hand := make([]Card, handSize, handSize+1)
		copy(hand, deck[:handSize])
		deck = deck[handSize:]

		// One guaranteed Defuse per player.
		hand = append(hand, defuses[0])
		defuses = defuses[1:]

		s.Players = append(s.Players, &Player{
			ID:    seat.ID,
			Name:  seat.Name,
			Hand:  hand,
			Alive: true,
		})
	}

	// Leftover Defuses (6 - n of them) go back into the deck, along with one
	// fewer Kitten than there are players.
	deck = append(deck, defuses...)
	deck = append(deck, kittens[:n-1]...)
	shuffle(deck, rng)
	s.Draw = deck

	// The first player is random so hosting isn't an advantage.
	s.Current = rng.Intn(n)
	return s, nil
}
