// Package static carries the media the client loads at runtime — scanned card
// faces, the player portraits and the intro theme — compiled into the server
// binary so the game stays a single executable with no asset paths to get wrong.
package static

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

// Matched by extension rather than by directory so that working files living
// alongside the assets do not silently end up in the binary — src/ has held a
// 5 MB rules PDF, a 46 MB download of an audio source and a 4 MB explosion plate
// the served clip was cut from, none of which is served. Adding a new media type
// means adding its pattern here.
//
// The card scans are named one directory at a time rather than with a wildcard
// across src/games/kittens/images. Each directory is one card's set of printed faces, and most
// of what sits in there is for cards this engine does not implement — the
// Zombie, Imploding and Barking expansions all shipped their own, and taking
// them along would be five megabytes of pictures nothing can ever ask for.
//
// So the list below has to stay in step with cardArtDirs further down. Being
// go:embed patterns, they earn their keep: each must match at least one file or
// the build fails, which makes a renamed directory a compile error rather than a
// deck of cards quietly wearing emoji.
//
// Only background.jpeg, the card back, is served from the top level — hence the
// narrower .jpeg pattern beside the per-card .jpg ones.
//
//go:embed src/games/kittens/images/background.jpeg
//go:embed src/games/kittens/images/explode/*.jpg
//go:embed src/games/kittens/images/defuse/*.jpg
//go:embed src/games/kittens/images/nope/*.jpg
//go:embed src/games/kittens/images/attack/*.jpg
//go:embed src/games/kittens/images/skip/*.jpg
//go:embed src/games/kittens/images/favor/*.jpg
//go:embed src/games/kittens/images/shuffle/*.jpg
//go:embed src/games/kittens/images/see-the-future/*.jpg
//go:embed src/games/kittens/images/cat-card/*.jpg
//go:embed src/avatars/*.png src/audio/*.mp3 src/games/kittens/video/*.mp4
var assets embed.FS

// CardArt is the card-face image set, rooted so that a request for
// "defuse/Defuse-Via-Crate.jpg" resolves without the src/games/kittens/images prefix.
func CardArt() fs.FS { return sub("src/games/kittens/images") }

// Avatars is the set of player portraits, rooted the same way.
func Avatars() fs.FS { return sub("src/avatars") }

// Audio is the sound set, rooted the same way. It is expected to be sparse or
// empty: the client treats a missing intro.mp3 as "play nothing".
func Audio() fs.FS { return sub("src/audio") }

// Video is the effects footage, rooted the same way. Optional in the same sense
// as the audio: a missing explosion.mp4 costs the bang its picture, nothing else.
func Video() fs.FS { return sub("src/games/kittens/video") }

// cardArtDirs maps a card's slug — the identifier the engine and the client both
// use — to the directory holding the faces printed for it.
//
// One card has many faces. The Original Edition prints eighteen different
// Defuses and only four to six of them are in any one game, so which ones a
// table sees is drawn per deal rather than pinned down here. Directories with no
// slug against them are deliberately unserved: they hold cards from expansions
// this engine does not implement.
var cardArtDirs = map[string]string{
	"exploding": "explode",
	"defuse":    "defuse",
	"nope":      "nope",
	"attack":    "attack",
	"skip":      "skip",
	"favor":     "favor",
	"shuffle":   "shuffle",
	"future":    "see-the-future",
}

// cardArtFiles names the faces that come one to a card. The five cat cards are
// printed in a single design each and share one directory, so there is nothing
// to sample and the filename is the whole catalogue.
var cardArtFiles = map[string]string{
	"cat-taco":    "cat-card/Tacocat.jpg",
	"cat-rainbow": "cat-card/Rainbow-Ralphing-Cat.jpg",
	"cat-melon":   "cat-card/Cattermelon.jpg",
	"cat-potato":  "cat-card/Hairy-Potato-Cat.jpg",
	"cat-beard":   "cat-card/Beard-Cat.jpg",
}

// CardArtVariants lists the faces available for each card slug, as paths
// relative to CardArt() — "defuse/Defuse-Via-Crate.jpg".
//
// The files are the catalogue, the same bargain AvatarIDs makes: dropping
// another scan into defuse/ widens the pool the next deal draws from, with no
// code to change. A slug whose directory is empty or gone is simply absent from
// the result, which leaves those cards rendering as the client's fallback glyph
// instead of failing the deal.
func CardArtVariants() map[string][]string {
	src := cardArtVariants()
	out := make(map[string][]string, len(src))
	for slug, files := range src {
		out[slug] = append([]string(nil), files...)
	}
	return out
}

var cardArtVariants = sync.OnceValue(func() map[string][]string {
	out := map[string][]string{}

	for slug, dir := range cardArtDirs {
		entries, err := fs.ReadDir(assets, "src/games/kittens/images/"+dir)
		if err != nil {
			continue // directory removed: those cards fall back to a glyph
		}
		var files []string
		for _, e := range entries {
			if name := e.Name(); strings.HasSuffix(name, ".jpg") {
				files = append(files, dir+"/"+name)
			}
		}
		if len(files) > 0 {
			sort.Strings(files)
			out[slug] = files
		}
	}

	for slug, path := range cardArtFiles {
		if _, err := fs.Stat(assets, "src/games/kittens/images/"+path); err == nil {
			out[slug] = []string{path}
		}
	}
	return out
})

// AvatarIDs lists the pickable portraits by basename — "ninja-nala" for
// src/avatars/ninja-nala.png — in sorted order.
//
// The files are the catalogue. The client asks the server for this list rather
// than carrying one of its own and derives each label from the name it gets
// back, so adding a portrait is one step: drop a PNG in and rebuild.
func AvatarIDs() []string { return append([]string(nil), avatarIDs()...) }

// HasAvatar reports whether id names a portrait we actually serve. The room
// layer asks before storing a player's choice, so that a hand-written socket
// message cannot leave a broken image on everybody else's screen.
func HasAvatar(id string) bool {
	for _, a := range avatarIDs() {
		if a == id {
			return true
		}
	}
	return false
}

var avatarIDs = sync.OnceValue(func() []string {
	entries, err := fs.ReadDir(assets, "src/avatars")
	if err != nil {
		panic(err) // impossible: the embed pattern above guarantees the directory
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if name := e.Name(); strings.HasSuffix(name, ".png") {
			ids = append(ids, strings.TrimSuffix(name, ".png"))
		}
	}
	sort.Strings(ids)
	return ids
})

func sub(dir string) fs.FS {
	f, err := fs.Sub(assets, dir)
	if err != nil {
		panic(err) // impossible: dir is a compile-time constant under src
	}
	return f
}
