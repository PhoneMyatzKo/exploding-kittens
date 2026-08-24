package game

import (
	"errors"
	"math/rand"
)

// Variant selects which printed sets are shuffled together. The expansion is
// never a deck of its own: Imploding means the Original Edition *plus* the
// Imploding Kittens pack, which is the only way it is playable.
type Variant string

const (
	Original  Variant = "original"
	Imploding Variant = "imploding"
)

// MinPlayers is the same for both variants. The expansion raises the ceiling to
// six, and the arithmetic is why: a six-player game needs five kittens, and the
// pack's one Imploding Kitten plus the Original's four Exploding Kittens is
// exactly five. Six Defuses also means exactly one each with none left over.
const (
	MinPlayers = 2
	MaxPlayers = 5 // Original Edition
	handSize   = 7 // dealt cards, before the guaranteed Defuse is added
)

// MaxPlayersFor is the seat count a variant supports.
func MaxPlayersFor(v Variant) int {
	if v == Imploding {
		return 6
	}
	return MaxPlayers
}

// Seat identifies a player joining a new game.
type Seat struct {
	ID   string
	Name string
}

var ErrPlayerCount = errors.New("that many players won't fit this game")

// NewGame performs the official setup:
//
//	remove all kittens and Defuses -> shuffle -> deal 7 each -> give everyone one
//	Defuse -> return the leftover Defuses to the deck -> insert (n-1) kittens ->
//	shuffle.
//
// The resulting deck therefore always has exactly one fewer kitten than there are
// players, which is what guarantees the game ends with a survivor.
//
// With the expansion in, one of those kittens is the Imploding Kitten and the
// rest are Exploding Kittens. It goes in face down: the first player to draw it
// puts it back face up, and whoever draws it after that is out with no Defuse to
// save them.
func NewGame(seats []Seat, v Variant, rng *rand.Rand) (*State, error) {
	n := len(seats)
	if n < MinPlayers || n > MaxPlayersFor(v) {
		return nil, ErrPlayerCount
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}

	deck := fullDeck(v, rng)
	deck, exploding := extract(deck, ExplodingKitten)
	deck, imploding := extract(deck, ImplodingKitten)
	deck, defuses := extract(deck, Defuse)
	shuffle(deck, rng)

	s := &State{
		Variant:        v,
		Phase:          PhaseTurn,
		TurnsRemaining: 1,
		Direction:      1,
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

	// Leftover Defuses go back into the deck, along with one fewer kitten than
	// there are players.
	deck = append(deck, defuses...)
	deck = append(deck, kittensFor(n, exploding, imploding)...)
	shuffle(deck, rng)
	s.Draw = deck

	// The first player is random so hosting isn't an advantage.
	s.Current = rng.Intn(n)
	return s, nil
}

// kittensFor picks the n-1 kittens that go back into the deck. Without the
// expansion they are all Exploding. With it, the single Imploding Kitten is one
// of them, so only n-2 Exploding Kittens are needed — which is what lets a sixth
// player fit into a deck that only ever had four.
func kittensFor(n int, exploding, imploding []Card) []Card {
	if len(imploding) == 0 {
		return exploding[:n-1]
	}
	out := append([]Card(nil), imploding[0])
	return append(out, exploding[:n-2]...)
}
