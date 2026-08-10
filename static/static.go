// Package static carries the scanned card faces, compiled into the server binary
// so the game stays a single executable with no asset paths to get wrong.
package static

import (
	"embed"
	"io/fs"
)

//go:embed src/images
var assets embed.FS

// CardArt is the card-face image set, rooted so that a request for
// "Beard-Cat.jpg" resolves without the src/images prefix.
func CardArt() fs.FS {
	sub, err := fs.Sub(assets, "src/images")
	if err != nil {
		panic(err) // impossible: the path is a compile-time constant
	}
	return sub
}
