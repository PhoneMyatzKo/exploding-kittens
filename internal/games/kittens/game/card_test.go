package game

import (
	"boardgame/kittens/internal/prng"
	"testing"
)

// withCardArt registers a catalogue for one test and puts the empty one back
// afterwards, so the rest of the package keeps dealing art-free decks.
func withCardArt(t *testing.T, variants map[string][]string) {
	t.Helper()
	SetCardArt(variants)
	t.Cleanup(func() { SetCardArt(nil) })
}

// The point of the whole feature: eighteen Defuses are printed and six are in
// the deck, so those six must be six different scans rather than one repeated.
func TestFullDeckGivesCopiesDistinctFaces(t *testing.T) {
	deep := make([]string, 18)
	for i := range deep {
		deep[i] = "defuse/" + string(rune('a'+i)) + ".jpg"
	}
	withCardArt(t, map[string][]string{"defuse": deep})

	deck := fullDeck(Original, prng.New(uint64(3)))

	seen := map[string]bool{}
	n := 0
	for _, c := range deck {
		if c.Type != Defuse {
			continue
		}
		n++
		if c.Art == "" {
			t.Fatalf("defuse %d has no face", c.ID)
		}
		if seen[c.Art] {
			t.Errorf("face %q used twice", c.Art)
		}
		seen[c.Art] = true
	}
	if n != 6 {
		t.Fatalf("expected 6 Defuses, got %d", n)
	}
}

// A card printed in a single design — every cat — has to be dealt anyway.
func TestFullDeckWrapsAPoolSmallerThanTheCount(t *testing.T) {
	withCardArt(t, map[string][]string{"cat-taco": {"cat-card/Tacocat.jpg"}})

	deck := fullDeck(Original, prng.New(uint64(4)))
	for _, c := range deck {
		if c.Type == CatTaco && c.Art != "cat-card/Tacocat.jpg" {
			t.Fatalf("taco cat %d got %q", c.ID, c.Art)
		}
	}
}

// Cards with nothing in the catalogue still deal; the client falls back to a
// glyph. This is also what every other test in the package relies on.
func TestFullDeckWithoutArtLeavesFacesEmpty(t *testing.T) {
	for _, c := range fullDeck(Original, prng.New(uint64(5))) {
		if c.Art != "" {
			t.Fatalf("card %d got a face from an empty catalogue: %q", c.ID, c.Art)
		}
	}
}

// The deck order must not depend on whether art happens to be registered:
// picking faces off the same rng would make adding a scan reshuffle every game,
// and would quietly invalidate the seeded decks the rest of these tests build.
func TestFullDeckOrderIsUnchangedByAnEmptyCatalogue(t *testing.T) {
	order := func() []string {
		rng := prng.New(uint64(9))
		deck := fullDeck(Original, rng)
		shuffle(deck, rng)
		return typesOf(deck)
	}

	before := order()
	SetCardArt(nil)
	after := order()

	if len(before) != len(after) {
		t.Fatalf("deck sizes differ: %d vs %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("deck order changed at %d: %v vs %v", i, before[i], after[i])
		}
	}
}

// Faces travel with the card, so a Defuse in a hand carries the same scan the
// whole table saw when it was played.
func TestNewGameDealsFacesIntoHands(t *testing.T) {
	withCardArt(t, map[string][]string{
		"defuse": {"defuse/a.jpg", "defuse/b.jpg", "defuse/c.jpg", "defuse/d.jpg", "defuse/e.jpg", "defuse/f.jpg"},
	})

	s, err := NewGame([]Seat{{ID: "p0", Name: "A"}, {ID: "p1", Name: "B"}}, Original, prng.New(uint64(11)))
	if err != nil {
		t.Fatal(err)
	}

	faces := map[string]bool{}
	for _, p := range s.Players {
		for _, c := range p.Hand {
			if c.Type != Defuse {
				continue
			}
			if c.Art == "" {
				t.Fatalf("dealt defuse %d has no face", c.ID)
			}
			if faces[c.Art] {
				t.Errorf("two players hold the same Defuse face %q", c.Art)
			}
			faces[c.Art] = true
		}
	}
	if len(faces) < 2 {
		t.Fatalf("expected each player's guaranteed Defuse to differ, got %d faces", len(faces))
	}
}
