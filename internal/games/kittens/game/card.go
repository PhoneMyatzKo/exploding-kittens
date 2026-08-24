package game

import (
	"fmt"
	"math/rand"
)

// CardType enumerates every card this engine knows: the Exploding Kittens
// Original Edition, then the Imploding Kittens expansion.
//
// The five collectible cats are kept contiguous because IsCat is a range check.
// New kinds go on the end.
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

	// Imploding Kittens expansion.
	ImplodingKitten
	Reverse
	DrawFromBottom
	FeralCat
	AlterTheFuture
	TargetedAttack
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

	ImplodingKitten: "Imploding Kitten",
	Reverse:         "Reverse",
	DrawFromBottom:  "Draw From the Bottom",
	FeralCat:        "Feral Cat",
	AlterTheFuture:  "Alter the Future",
	TargetedAttack:  "Targeted Attack",
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

	ImplodingKitten: "imploding",
	Reverse:         "reverse",
	DrawFromBottom:  "bottom",
	FeralCat:        "feral-cat",
	AlterTheFuture:  "alter",
	TargetedAttack:  "targeted-attack",
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
//
// Deliberately excludes the Feral Cat: it plays *as* a cat but is not one of the
// five kinds, and treating it as one would let a Three of a Kind demand "a Feral
// Cat or anything like it". Use IsCatLike for the combo rules.
func (t CardType) IsCat() bool { return t >= CatTaco && t <= CatBeard }

// IsCatLike reports whether the card may take part in a cat combo. The Feral Cat
// is a wildcard: it stands in for whichever cat the rest of the set is.
func (t CardType) IsCatLike() bool { return t.IsCat() || t == FeralCat }

// Kills reports whether drawing this card takes a player out of the game. Both
// kittens do; only one of them can be defused.
func (t CardType) Kills() bool { return t == ExplodingKitten || t == ImplodingKitten }

// AllTypes lists every kind of card in a variant's deck, in the printed order —
// the kittens first, then the actions, then the cats. Used by the how-to-play
// sheet, which shows one of each and so must follow the deck rather than carry
// its own list.
func AllTypes(v Variant) []CardType {
	src := compositionFor(v)
	out := make([]CardType, 0, len(src))
	for _, c := range src {
		out = append(out, c.Type)
	}
	return out
}

// DemandableTypes lists the card kinds a Three of a Kind may name, for the given
// variant. Both kittens are left out: they are only ever in the deck or the
// discard, never in a hand, so demanding one could not possibly succeed.
func DemandableTypes(v Variant) []CardType {
	out := []CardType{
		Defuse, Nope, Attack, Skip, Favor, Shuffle, SeeTheFuture,
		CatTaco, CatRainbow, CatMelon, CatPotato, CatBeard,
	}
	if v == Imploding {
		out = append(out,
			Reverse, DrawFromBottom, FeralCat, AlterTheFuture, TargetedAttack)
	}
	return out
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
	// FaceUp is only ever true of the Imploding Kitten after somebody has drawn
	// it once and put it back. Never sent to a client: the draw pile is not
	// serialised at all, and whether the kitten is armed reaches the table as a
	// single flag on the view rather than as a property of a card nobody can see.
	FaceUp bool `json:"-"`
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

type tally struct {
	Type  CardType
	Count int
}

// composition is the Original Edition's 56-card breakdown.
var composition = []tally{
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

// implodingPack is the Imploding Kittens expansion: 20 cards shuffled into the
// Original Edition. It is never a deck of its own — there are no Defuses in it,
// and the box says so.
var implodingPack = []tally{
	{ImplodingKitten, 1},
	{Reverse, 4},
	{DrawFromBottom, 4},
	{FeralCat, 4},
	{AlterTheFuture, 4},
	{TargetedAttack, 3},
}

// compositionFor is the full card list for a variant, before any is removed.
func compositionFor(v Variant) []tally {
	if v != Imploding {
		return composition
	}
	return append(append([]tally(nil), composition...), implodingPack...)
}

// fullDeck builds every card of the variant with sequential IDs, unshuffled.
//
// Copies of the same card get different faces wherever the art pool is deep
// enough to give them one: eighteen Defuses are printed and six are dealt, so a
// table sees six different ways of defusing rather than the same one six times.
func fullDeck(v Variant, rng *rand.Rand) []Card {
	var out []Card
	id := 0
	for _, c := range compositionFor(v) {
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
