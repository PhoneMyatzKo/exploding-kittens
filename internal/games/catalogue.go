// Package games is the catalogue the menu is built from: which games this
// server hosts, and which of them can actually be played yet.
//
// It is deliberately data and nothing else. A game's rules live under its own
// directory (internal/games/kittens/game); this package only knows enough to put
// a tile on the front page and to validate the slug a room is created with. That
// keeps the menu from being the thing every future game has to be wired into.
package games

import "boardgame/kittens/internal/games/kittens/game"

// Kittens is the slug rooms default to, so an older client that knows nothing
// about games still gets a game. KittensImploding is the same engine dealing the
// Original Edition plus the Imploding Kittens pack — the expansion has no
// Defuses and no deck of its own, so it is only ever playable combined.
const (
	Kittens          = "kittens"
	KittensImploding = "kittens-imploding"
)

// Info is one tile on the menu.
type Info struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	Emoji   string `json:"emoji"`
	// Cover is the box art for the tile, or empty for a game with none — the
	// menu falls back to the emoji on a plain field, so an unillustrated game
	// still gets a tile rather than a hole.
	Cover string `json:"cover,omitempty"`
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	// Playable is false for a game that is announced but not built. The menu
	// shows those greyed out rather than hiding them: an empty-looking hub reads
	// as broken, and "coming soon" is information.
	Playable bool `json:"playable"`
}

// catalogue is ordered as the menu shows it, playable first.
var catalogue = []Info{
	{
		Slug: Kittens, Name: "Exploding Kittens", Tagline: "Draw a kitten and you're out. Don't draw a kitten.",
		Emoji: "💥", Cover: "/cards/background.jpeg",
		Min: game.MinPlayers, Max: game.MaxPlayersFor(game.Original), Playable: true,
	},
	{
		Slug: KittensImploding, Name: "Imploding Kittens",
		Tagline: "The expansion: reversals, wildcards, and one kitten no Defuse can stop.",
		Emoji:   "🌀", Cover: "/cards/imploding/Imploding-Kitten.jpg",
		Min: game.MinPlayers, Max: game.MaxPlayersFor(game.Imploding), Playable: true,
	},
	// Taglines describe the game rather than its status: the tile already carries
	// a "soon" badge, and saying it twice wastes the only line there is to say
	// what the game actually is.
	{
		Slug: "uno", Name: "UNO", Tagline: "Match the colour or the number.",
		Emoji: "🎴", Min: 2, Max: 10, Playable: false,
	},
	{
		Slug: "uno-no-mercy", Name: "UNO No Mercy", Tagline: "Stacking draws, no way out.",
		Emoji: "🃏", Min: 2, Max: 10, Playable: false,
	},
}

// All returns the catalogue. A copy, so a caller cannot rewrite the menu.
func All() []Info { return append([]Info(nil), catalogue...) }

// Playable reports whether rooms may be created for this slug. Unknown and
// not-yet-built games are both refused, so a hand-written POST cannot open a
// room nothing knows how to deal.
func Playable(slug string) bool {
	for _, g := range catalogue {
		if g.Slug == slug {
			return g.Playable
		}
	}
	return false
}

// VariantFor maps a catalogue slug onto the card sets it deals. This is the only
// place the mapping lives: the room carries a variant it does not interpret, and
// the engine has never heard of a slug.
func VariantFor(slug string) (game.Variant, bool) {
	switch slug {
	case Kittens:
		return game.Original, true
	case KittensImploding:
		return game.Imploding, true
	}
	return "", false
}

// Name is the display name for a slug, falling back to the slug itself so an
// unknown one shows up as odd rather than as blank.
func Name(slug string) string {
	for _, g := range catalogue {
		if g.Slug == slug {
			return g.Name
		}
	}
	return slug
}
