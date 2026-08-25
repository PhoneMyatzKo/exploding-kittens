package static

import (
	"strings"
	"testing"

	"boardgame/kittens/internal/game"
)

// Every card the engine deals must have somewhere to get a face from. Without
// this, renaming a directory costs those cards their art silently — the client
// falls back to a glyph and the game plays on, so nothing else would complain.
func TestCardArtVariantsCoverEveryDealtCard(t *testing.T) {
	variants := CardArtVariants()

	slugs := []string{game.ExplodingKitten.Slug()}
	for _, ct := range game.DemandableTypes() {
		slugs = append(slugs, ct.Slug())
	}

	for _, slug := range slugs {
		if len(variants[slug]) == 0 {
			t.Errorf("no card art for %q: check cardArtDirs/cardArtFiles against src/images", slug)
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
