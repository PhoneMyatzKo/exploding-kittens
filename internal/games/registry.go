package games

import (
	"fmt"
	"math/rand"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/kittens"
	"boardgame/kittens/internal/games/uno"
)

// New builds the rules for one table.
//
// This is the only place a slug becomes code. The catalogue above says which
// games exist; this says what to do about it, and the two are checked against
// each other by the tests so a game cannot be advertised as playable without
// something here to deal it.
func New(slug string, rng *rand.Rand) (core.Game, error) {
	switch slug {
	case Kittens:
		return kittens.New(rng), nil
	case UNO:
		return uno.New(rng), nil
	}
	return nil, fmt.Errorf("no rules for game %q", slug)
}
