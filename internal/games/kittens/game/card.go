package game

import (
	"fmt"
	"math/rand"
)

// CardType enumerates every card in the Exploding Kittens Original Edition.
type CardType int

const (
	ExplodingKitten CardType = iota
	Defuse
	Nope
	Attack
	Skip
	Favor
	Shuffle
	SeeTheFuture
	CatTaco
	CatRainbow
	CatMelon
	CatPotato
	CatBeard
)

var cardNames = map[CardType]string{
	ExplodingKitten: "Exploding Kitten",
	Defuse:          "Defuse",
	Nope:            "Nope",
	Attack:          "Attack",
	Skip:            "Skip",
	Favor:           "Favor",
	Shuffle:         "Shuffle",
	SeeTheFuture:    "See the Future",
	CatTaco:         "Taco Cat",
	CatRainbow:      "Rainbow-Ralphing Cat",
	CatMelon:        "Cattermelon",
	CatPotato:       "Hairy Potato Cat",
	CatBeard:        "Beard Cat",
}

// slug is the stable identifier the browser uses to pick card art and colours.
var cardSlugs = map[CardType]string{
	ExplodingKitten: "exploding",
	Defuse:          "defuse",
	Nope:            "nope",
	Attack:          "attack",
	Skip:            "skip",
	Favor:           "favor",
	Shuffle:         "shuffle",
	SeeTheFuture:    "future",
	CatTaco:         "cat-taco",
	CatRainbow:      "cat-rainbow",
	CatMelon:        "cat-melon",
	CatPotato:       "cat-potato",
	CatBeard:        "cat-beard",
}

func (t CardType) String() string {
	if n, ok := cardNames[t]; ok {
		return n
	}
	return fmt.Sprintf("CardType(%d)", int(t))
}

// Slug returns the stable lowercase identifier sent to clients.
func (t CardType) Slug() string { return cardSlugs[t] }

// IsCat reports whether the type is one of the five collectible cat cards, which
// have no effect alone and are only useful in matching sets.
func (t CardType) IsCat() bool { return t >= CatTaco && t <= CatBeard }

// DemandableTypes lists the card kinds a Three of a Kind may name. Exploding
// Kittens are left out: they are only ever in the deck or the discard, never in
// a hand, so demanding one could not possibly succeed.
func DemandableTypes() []CardType {
	return []CardType{
		Defuse, Nope, Attack, Skip, Favor, Shuffle, SeeTheFuture,
		CatTaco, CatRainbow, CatMelon, CatPotato, CatBeard,
	}
}

// TypeFromSlug resolves the identifier clients use back to a card type. Needed
// because a Three of a Kind names the card it is asking for.
func TypeFromSlug(slug string) (CardType, bool) {
	for t, s := range cardSlugs {
		if s == slug {
			return t, true
		}
	}
	return 0, false
}

// Card is a single physical card. ID is unique within a game so that clients can
// unambiguously refer to one specific card in their hand.
type Card struct {
	ID   int      `json:"id"`
	Type CardType `json:"-"`
	// Name and Slug are denormalised so the redacted view is directly renderable.
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Art is the face this particular copy wears, as a path the client resolves
	// against the card-art route. Chosen once when the deck is built so that it
	// travels with the card between deck, hand and discard, and so every player
	// looking at the same card sees the same picture. Empty when no art has been
	// registered, which the client renders as its fallback glyph.
	Art string `json:"art,omitempty"`
}

func newCard(id int, t CardType) Card {
	return Card{ID: id, Type: t, Name: t.String(), Slug: t.Slug()}
}

// artVariants holds the faces available per slug. Nil until something calls
// SetCardArt: the engine deals a perfectly legal deck with no art at all, which
// is what keeps the rules and their tests free of any knowledge of asset paths.
var artVariants map[string][]string

// SetCardArt registers the faces available for each card slug. Decks dealt after
// this call give every copy of a card its own face, drawn from that slug's pool.
//
// Process-wide and set once at startup, because the pool is the contents of the
// binary and cannot sensibly differ between rooms.
func SetCardArt(variants map[string][]string) {
	if len(variants) == 0 {
		artVariants = nil
		return
	}
	artVariants = make(map[string][]string, len(variants))
	for slug, files := range variants {
		artVariants[slug] = append([]string(nil), files...)
	}
}

// composition is the Original Edition's 56-card breakdown.
var composition = []struct {
	Type  CardType
	Count int
}{
	{ExplodingKitten, 4},
	{Defuse, 6},
	{Nope, 5},
	{Attack, 4},
	{Skip, 4},
	{Favor, 4},
	{Shuffle, 4},
	{SeeTheFuture, 5},
	{CatTaco, 4},
	{CatRainbow, 4},
	{CatMelon, 4},
	{CatPotato, 4},
	{CatBeard, 4},
}

// fullDeck builds all 56 cards with sequential IDs, unshuffled.
//
// Copies of the same card get different faces wherever the art pool is deep
// enough to give them one: eighteen Defuses are printed and six are dealt, so a
// table sees six different ways of defusing rather than the same one six times.
func fullDeck(rng *rand.Rand) []Card {
	var out []Card
	id := 0
	for _, c := range composition {
		faces := pickFaces(c.Type.Slug(), c.Count, rng)
		for i := 0; i < c.Count; i++ {
			card := newCard(id, c.Type)
			if i < len(faces) {
				card.Art = faces[i]
			}
			out = append(out, card)
			id++
		}
	}
	return out
}

// pickFaces chooses n faces for one kind of card: a sample without repetition
// while the pool lasts, wrapping around a fresh order once it runs out — a cat
// card has one printed design, so all four copies necessarily share it.
//
// Returns nil, spending no randomness at all, when the kind has no art. That
// matters: the rules tests seed the rng themselves, and their decks must not
// shift underneath them because somebody added a scan.
func pickFaces(slug string, n int, rng *rand.Rand) []string {
	pool := artVariants[slug]
	if len(pool) == 0 {
		return nil
	}
	order := append([]string(nil), pool...)
	shuffleStrings(order, rng)

	out := make([]string, n)
	for i := range out {
		out[i] = order[i%len(order)]
	}
	return out
}

func shuffleStrings(s []string, rng *rand.Rand) {
	rng.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
}
