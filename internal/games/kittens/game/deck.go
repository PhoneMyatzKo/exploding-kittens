package game

import "math/rand"

// Throughout this package a []Card used as a draw pile is ordered top-first:
// index 0 is the next card a player would draw.

// shuffle randomises a pile in place.
func shuffle(pile []Card, rng *rand.Rand) {
	rng.Shuffle(len(pile), func(i, j int) {
		pile[i], pile[j] = pile[j], pile[i]
	})
}

// extract removes every card of the given type from pile, returning the pared-down
// pile and the removed cards. Used at setup to pull Kittens and Defuses out before
// the initial deal.
func extract(pile []Card, t CardType) (rest, taken []Card) {
	for _, c := range pile {
		if c.Type == t {
			taken = append(taken, c)
		} else {
			rest = append(rest, c)
		}
	}
	return rest, taken
}

// insertAt places card into pile at index i, where 0 puts it on top and
// len(pile) puts it on the bottom. Out-of-range indices are clamped.
func insertAt(pile []Card, i int, card Card) []Card {
	if i < 0 {
		i = 0
	}
	if i > len(pile) {
		i = len(pile)
	}
	out := make([]Card, 0, len(pile)+1)
	out = append(out, pile[:i]...)
	out = append(out, card)
	out = append(out, pile[i:]...)
	return out
}

// peekTop returns up to n cards from the top of pile without removing them.
func peekTop(pile []Card, n int) []Card {
	if n > len(pile) {
		n = len(pile)
	}
	out := make([]Card, n)
	copy(out, pile[:n])
	return out
}

// removeCard pulls the card with the given ID out of pile. The bool reports
// whether it was there at all.
func removeCard(pile []Card, id int) ([]Card, Card, bool) {
	for i, c := range pile {
		if c.ID == id {
			out := make([]Card, 0, len(pile)-1)
			out = append(out, pile[:i]...)
			out = append(out, pile[i+1:]...)
			return out, c, true
		}
	}
	return pile, Card{}, false
}
