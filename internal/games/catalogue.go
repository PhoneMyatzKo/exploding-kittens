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
	monopolygame "boardgame/kittens/internal/games/monopoly/game"
	unogame "boardgame/kittens/internal/games/uno/game"
)

// Slugs. Rooms default to Kittens, so an older client that knows nothing about
// games still gets a game.
const (
	Kittens          = "kittens"
	KittensImploding = "kittens-imploding"
	UNO              = "uno"
	Monopoly         = "monopoly"
)

// Info is one tile on the menu.
type Info struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	Emoji   string `json:"emoji"`
	Cover   string `json:"cover,omitempty"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
	// Playable is false for a game that is announced but not built. The menu
	// shows those greyed out rather than hiding them: an empty-looking hub reads
	// as broken, and "coming soon" is information.
	Playable bool `json:"playable"`
	// Preview is a game that is finished enough to play but not finished enough
	// to show: it is hidden from the menu entirely until somebody unlocks it with
	// the party key, and rooms cannot be created for it meanwhile.
	//
	// Distinct from Playable on purpose, and the distinction is load-bearing.
	// Playable=false means "there are no rules behind this slug" and is enforced
	// against the registry by TestEveryPlayableGameHasRules; a preview game has
	// rules and passes that test. Flipping Playable to hide something would
	// break the invariant and make the server refuse a game it can in fact deal.
	//
	// Not sent to the client: once a tile has arrived, it has been unlocked, and
	// which games were withheld is not the browser's business.
	Preview bool `json:"-"`
}

// catalogue is ordered as the menu shows it, playable first.
var catalogue = []Info{
	{
		Slug: Kittens, Name: "Exploding Kittens", Tagline: "Draw a kitten and you're out. Don't draw a kitten.",
		Emoji: "💥", Cover: "/cards/background.jpeg",
		Min: kittensgame.MinPlayers, Max: kittensgame.MaxPlayers, Playable: true,
	},
	{
		Slug: KittensImploding, Name: "Imploding Kittens",
		Tagline: "The expansion: reversals, wildcards, and one kitten no Defuse can stop.",
		Emoji:   "🌀", Cover: "/cards/Imploding_Kitten.jpg",
		Min: kittensgame.MinPlayers, Max: kittensgame.MaxPlayersFor(kittensgame.Imploding), Playable: true,
	},
	{
		Slug: UNO, Name: "UNO", Tagline: "Match the colour or the number.",
		Emoji: "🎴", Min: unogame.MinPlayers, Max: unogame.MaxPlayers, Playable: true,
	},
	{
		Slug: Monopoly, Name: "Monopoly Myanmar",
		Tagline: "Buy up Myanmar, bankrupt your friends. Rent in kyat.",
		Emoji:   "🎩",
		Min:     monopolygame.MinPlayers, Max: monopolygame.MaxPlayers, Playable: true,
		// Playable, and deliberately not on the menu yet: houses are in but
		// auctions, trading and mortgaging are not, and this ships on main ahead
		// of them. The party key is what lets it be tested with real people
		// without putting an unfinished game in front of everybody who visits.
		Preview: true,
	},
	{
		Slug: "uno-no-mercy", Name: "UNO No Mercy", Tagline: "Stacking draws, no way out.",
		Emoji: "🃏", Min: 2, Max: 10, Playable: false,
	},
}

// All returns the catalogue. A copy, so a caller cannot rewrite the menu.
func All() []Info { return append([]Info(nil), catalogue...) }

// Visible is the menu as one visitor should see it: everything, or everything
// except the preview games.
//
// The filtering happens here rather than in the browser because a menu filtered
// in JavaScript is a menu anybody can un-filter with dev tools — and, more to
// the point, the server would still have opened a room for the game. See
// Preview below for the other half.
func Visible(unlocked bool) []Info {
	out := make([]Info, 0, len(catalogue))
	for _, g := range catalogue {
		if g.Preview && !unlocked {
			continue
		}
		out = append(out, g)
	}
	return out
}

// Preview reports whether a slug is gated behind the party key. Rooms for it are
// refused unless the request carries the key, which is what stops an unlocked
// link or a hand-written POST reaching a game that is not ready to be shown.
func Preview(slug string) bool {
	for _, g := range catalogue {
		if g.Slug == slug {
			return g.Preview
		}
	}
	return false
}

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
func VariantFor(slug string) (kittensgame.Variant, bool) {
	switch slug {
	case Kittens:
		return kittensgame.Original, true
	case KittensImploding:
		return kittensgame.Imploding, true
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
