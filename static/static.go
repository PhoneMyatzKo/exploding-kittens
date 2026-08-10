// Package static carries the media the client loads at runtime — scanned card
// faces and the intro theme — compiled into the server binary so the game stays
// a single executable with no asset paths to get wrong.
package static

import (
	"embed"
	"io/fs"
)

// Matched by extension rather than by directory so that working files living
// alongside the assets do not silently end up in the binary — src/ has held a
// 5 MB rules PDF and a 46 MB download of an audio source, neither of which is
// served. Adding a new media type means adding its pattern here.
//
// Each pattern must match at least one file or the build fails. That is a
// feature: removing the last mp3 is meant to be a decision, not a surprise.
//
//go:embed src/images/*.jpg src/audio/*.mp3
var assets embed.FS

// CardArt is the card-face image set, rooted so that a request for
// "Beard-Cat.jpg" resolves without the src/images prefix.
func CardArt() fs.FS { return sub("src/images") }

// Audio is the sound set, rooted the same way. It is expected to be sparse or
// empty: the client treats a missing intro.mp3 as "play nothing".
func Audio() fs.FS { return sub("src/audio") }

func sub(dir string) fs.FS {
	f, err := fs.Sub(assets, dir)
	if err != nil {
		panic(err) // impossible: dir is a compile-time constant under src
	}
	return f
}
