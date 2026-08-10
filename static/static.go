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
// 5 MB rules PDF and a 46 MB download of an audio source, neither of which is
// served. Adding a new media type means adding its pattern here.
//
// Each pattern must match at least one file or the build fails. That is a
// feature: removing the last mp3 is meant to be a decision, not a surprise.
//
//go:embed src/images/*.jpg src/images/*.jpeg src/avatars/*.png src/audio/*.mp3
var assets embed.FS

// CardArt is the card-face image set, rooted so that a request for
// "Beard-Cat.jpg" resolves without the src/images prefix.
func CardArt() fs.FS { return sub("src/images") }

// Avatars is the set of player portraits, rooted the same way.
func Avatars() fs.FS { return sub("src/avatars") }

// Audio is the sound set, rooted the same way. It is expected to be sparse or
// empty: the client treats a missing intro.mp3 as "play nothing".
func Audio() fs.FS { return sub("src/audio") }

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
