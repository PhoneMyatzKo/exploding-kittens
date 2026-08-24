// Package games is the catalogue the menu is built from: which games this
// server hosts, and which of them can actually be played yet.
//
// It is deliberately data and nothing else. A game's rules live under its own
// directory (internal/games/kittens/game); this package only knows enough to put
// a tile on the front page and to validate the slug a room is created with. That
// keeps the menu from being the thing every future game has to be wired into.
package games

import (
	kittensgame "boardgame/kittens/internal/games/kittens/game"
	unogame "boardgame/kittens/internal/games/uno/game"
)

// Slugs. Rooms default to Kittens, so an older client that knows nothing about
// games still gets a game.
const (
	Kittens = "kittens"
	UNO     = "uno"
)

// Info is one tile on the menu.
type Info struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	Emoji   string `json:"emoji"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
	// Playable is false for a game that is announced but not built. The menu
	// shows those greyed out rather than hiding them: an empty-looking hub reads
	// as broken, and "coming soon" is information.
	Playable bool `json:"playable"`
}

// catalogue is ordered as the menu shows it, playable first.
var catalogue = []Info{
	{
		Slug: Kittens, Name: "Exploding Kittens", Tagline: "Draw a kitten and you're out. Don't draw a kitten.",
		Emoji: "💥", Min: kittensgame.MinPlayers, Max: kittensgame.MaxPlayers, Playable: true,
	},
	// Taglines describe the game rather than its status: the tile already carries
	// a "soon" badge, and saying it twice wastes the only line there is to say
	// what the game actually is.
	{
		Slug: UNO, Name: "UNO", Tagline: "Match the colour or the number.",
		Emoji: "🎴", Min: unogame.MinPlayers, Max: unogame.MaxPlayers, Playable: true,
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
