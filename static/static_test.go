package static

import (
	"strings"
	"testing"

	"boardgame/kittens/internal/games/kittens/game"
)

// Every card the engine deals must have somewhere to get a face from. Without
// this, renaming a directory costs those cards their art silently — the client
// falls back to a glyph and the game plays on, so nothing else would complain.
// Both decks, because the expansion brings six card types and six directories of
// its own — checking only the Original Edition would leave exactly the new art
// unguarded.
func TestCardArtVariantsCoverEveryDealtCard(t *testing.T) {
	art := CardArtVariants()

	for _, v := range []game.Variant{game.Original, game.Imploding} {
		for _, ct := range game.AllTypes(v) {
			if len(art[ct.Slug()]) == 0 {
				t.Errorf("%s deck: no card art for %q: check cardArtDirs/cardArtFiles against src/games/kittens/images",
					v, ct.Slug())
			}
		}
	}
}

// The paths go straight into an <img src> the browser resolves against /cards/,
// which is CardArt() — so they have to be relative to it and actually be there.
func TestCardArtVariantsResolveAgainstCardArt(t *testing.T) {
	art := CardArt()
	for slug, files := range CardArtVariants() {
		for _, f := range files {
			if strings.HasPrefix(f, "/") || strings.HasPrefix(f, "src/") {
				t.Errorf("%s: %q is not relative to the card-art root", slug, f)
				continue
			}
			if _, err := art.Open(f); err != nil {
				t.Errorf("%s: %q is not served: %v", slug, f, err)
			}
		}
	}
}

// The card back is loaded by the stylesheet rather than by any Go code, so the
// narrowed embed pattern is the only thing keeping it in the binary.
func TestCardBackIsServed(t *testing.T) {
	if _, err := CardArt().Open("background.jpeg"); err != nil {
		t.Fatalf("card back missing from the binary: %v", err)
	}
}
