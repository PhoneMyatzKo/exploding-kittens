// Package game holds the rules of UNO, and nothing else.
//
// It has no I/O, no timers and no idea that a browser exists: Apply is a pure
// reducer over State, exactly like its Exploding Kittens counterpart, so the
// whole rulebook is testable without a server.
//
// The rules implemented are the official ones as published at unorules.com:
// a 108-card deck, seven cards each, match colour or symbol, and scoring to 500.
// House rules that the internet treats as official (stacking draws, drawing
// until you can play, jumping in) are deliberately absent — those are UNO No
// Mercy's business, and mixing the two would make neither of them right.
package game

import "fmt"

// Colour is one of UNO's four suits. NoColour is what the Wild cards carry: they
// have no printed colour of their own and borrow one when they are played.
type Colour int

const (
	NoColour Colour = iota
	Red
	Yellow
	Green
	Blue
)

// Colours lists the four playable colours, which is also the set a Wild may name.
func Colours() []Colour { return []Colour{Red, Yellow, Green, Blue} }

var colourSlugs = map[Colour]string{
	NoColour: "wild",
	Red:      "red",
	Yellow:   "yellow",
	Green:    "green",
	Blue:     "blue",
}

var colourNames = map[Colour]string{
	NoColour: "Wild",
	Red:      "Red",
	Yellow:   "Yellow",
	Green:    "Green",
	Blue:     "Blue",
}

// Slug is the stable identifier the browser draws the card from.
func (c Colour) Slug() string { return colourSlugs[c] }

func (c Colour) String() string {
	if n, ok := colourNames[c]; ok {
		return n
	}
	return fmt.Sprintf("Colour(%d)", int(c))
}

// ColourFromSlug resolves the identifier a client sends back when it names a
// colour for a Wild.
func ColourFromSlug(slug string) (Colour, bool) {
	for c, s := range colourSlugs {
		if c != NoColour && s == slug {
			return c, true
		}
	}
	return NoColour, false
}

// Rank is what is printed on the face. The digits are their own values, so
// Rank(7) is a seven and arithmetic on numbers needs no lookup table; the action
// cards continue past nine.
type Rank int

const (
	RankSkip Rank = iota + 10
	RankReverse
	RankDrawTwo
	RankWild
	RankWildDrawFour
)

// IsNumber reports whether the rank is one of the ten digits.
func (r Rank) IsNumber() bool { return r >= 0 && r <= 9 }

// IsWild reports whether the rank is one of the two colourless cards. Those are
// the only cards playable on anything, and the only ones whose colour is chosen
// rather than printed.
func (r Rank) IsWild() bool { return r == RankWild || r == RankWildDrawFour }

var rankSlugs = map[Rank]string{
	RankSkip:         "skip",
	RankReverse:      "reverse",
	RankDrawTwo:      "draw-two",
	RankWild:         "wild",
	RankWildDrawFour: "wild-draw-four",
}

var rankNames = map[Rank]string{
	RankSkip:         "Skip",
	RankReverse:      "Reverse",
	RankDrawTwo:      "Draw Two",
	RankWild:         "Wild",
	RankWildDrawFour: "Wild Draw Four",
}

// Slug is the stable identifier for the face. Digits are their own slug.
func (r Rank) Slug() string {
	if r.IsNumber() {
		return fmt.Sprintf("%d", int(r))
	}
	return rankSlugs[r]
}

func (r Rank) String() string {
	if r.IsNumber() {
		return fmt.Sprintf("%d", int(r))
	}
	if n, ok := rankNames[r]; ok {
		return n
	}
	return fmt.Sprintf("Rank(%d)", int(r))
}

// Points is what the card is worth to the winner of a round: digits score their
// face value, the coloured action cards twenty each, and either Wild fifty.
func (r Rank) Points() int {
	switch {
	case r.IsNumber():
		return int(r)
	case r == RankSkip, r == RankReverse, r == RankDrawTwo:
		return 20
	case r.IsWild():
		return 50
	}
	return 0
}

// Card is a single physical card. ID is unique within a game so a client can
// point at one specific card in its hand without ambiguity.
//
// Unlike Exploding Kittens there is no Art field, because there is no art: a
// coloured rounded rectangle with a numeral or a glyph on it is fully described
// by Colour and Rank, so the client draws every card from those two strings and
// the server ships no images at all.
type Card struct {
	ID     int    `json:"id"`
	Colour Colour `json:"-"`
	Rank   Rank   `json:"-"`

	// Denormalised so a redacted view is directly renderable: the client never
	// has to map an enum it cannot see.
	ColourSlug string `json:"colour"`
	RankSlug   string `json:"rank"`
	Name       string `json:"name"`
}

func newCard(id int, colour Colour, rank Rank) Card {
	c := Card{ID: id, Colour: colour, Rank: rank,
		ColourSlug: colour.Slug(), RankSlug: rank.Slug()}
	if colour == NoColour {
		c.Name = rank.String()
	} else {
		c.Name = colour.String() + " " + rank.String()
	}
	return c
}

// Points is what this card adds to the winner's score if it is caught in a hand.
func (c Card) Points() int { return c.Rank.Points() }

// DeckSize is the size of a standard UNO deck. Asserted by the tests rather than
// derived, because a deck that is one card out is the kind of bug that only
// shows up as a slightly wrong game weeks later.
const DeckSize = 108

// fullDeck builds all 108 cards with sequential IDs, unshuffled:
//
//	per colour: one 0, two each of 1-9, two Skip, two Reverse, two Draw Two (25)
//	then four Wild and four Wild Draw Four
func fullDeck() []Card {
	out := make([]Card, 0, DeckSize)
	id := 0
	add := func(colour Colour, rank Rank) {
		out = append(out, newCard(id, colour, rank))
		id++
	}
	for _, colour := range Colours() {
		add(colour, Rank(0))
		for n := 1; n <= 9; n++ {
			add(colour, Rank(n))
			add(colour, Rank(n))
		}
		for _, r := range []Rank{RankSkip, RankReverse, RankDrawTwo} {
			add(colour, r)
			add(colour, r)
		}
	}
	for i := 0; i < 4; i++ {
		add(NoColour, RankWild)
		add(NoColour, RankWildDrawFour)
	}
	return out
}
