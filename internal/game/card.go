package game

import "fmt"

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
// have no effect alone and are only useful in matching pairs.
func (t CardType) IsCat() bool { return t >= CatTaco && t <= CatBeard }

// Card is a single physical card. ID is unique within a game so that clients can
// unambiguously refer to one specific card in their hand.
type Card struct {
	ID   int      `json:"id"`
	Type CardType `json:"-"`
	// Name and Slug are denormalised so the redacted view is directly renderable.
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func newCard(id int, t CardType) Card {
	return Card{ID: id, Type: t, Name: t.String(), Slug: t.Slug()}
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
func fullDeck() []Card {
	var out []Card
	id := 0
	for _, c := range composition {
		for i := 0; i < c.Count; i++ {
			out = append(out, newCard(id, c.Type))
			id++
		}
	}
	return out
}
