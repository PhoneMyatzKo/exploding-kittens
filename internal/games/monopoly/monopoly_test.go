package monopoly

import (
	"bytes"
	"testing"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/monopoly/game"
	"boardgame/kittens/internal/prng"
)

// An internal test, which is unusual here and deliberate: setting up a complete
// colour set is not something a legal sequence of moves can be made to do on
// demand — it needs the dice to land one player on both browns — and reaching in
// is the only way to test building through the wire at all.
//
// What it covers is the seam rather than the rules, which are covered next door
// in ./game. Two things live only here: the build message carrying a square, and
// the snapshot carrying the buildings.

func dealt(t *testing.T) *Game {
	t.Helper()
	g, ok := New(prng.New(3)).(*Game)
	if !ok {
		t.Fatal("New did not return a *Game")
	}
	if _, err := g.Deal([]core.Seat{{ID: "p0", Name: "A"}, {ID: "p1", Name: "B"}}); err != nil {
		t.Fatal(err)
	}
	// A complete set, and the money to build on it.
	g.state.Owner[1], g.state.Owner[3] = "p0", "p0"
	g.state.Find("p0").Cash = 1_000_000
	return g
}

// The build message names a square, which is the only move here that does — so
// it is the only place a dropped field would show up as "you don't own that
// square" on a square you plainly do.
func TestABuildMessageCarriesItsSquare(t *testing.T) {
	g := dealt(t)

	entries, err := g.Submit("p0", core.ClientMsg{Type: "build", Tile: 3})
	if err != nil {
		t.Fatalf("building on square 3: %v", err)
	}
	if g.state.Houses[3] != 1 {
		t.Fatalf("square 3 stands at %d; square 1 at %d — the tile was lost on the way in",
			g.state.Houses[3], g.state.Houses[1])
	}

	if len(entries) != 1 {
		t.Fatalf("building reported %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != "built" {
		t.Errorf("the entry is %q", e.Kind)
	}
	if e.Tile == nil || *e.Tile != 3 {
		t.Errorf("the entry names square %v", e.Tile)
	}
	// The level travels with the line, so the log can still say "a house" rather
	// than "a hotel" long after the square has grown one.
	if e.Houses == nil || *e.Houses != 1 {
		t.Errorf("the entry says %v buildings", e.Houses)
	}
	if e.Count != game.TileAt(3).House {
		t.Errorf("the entry says it cost %d, want %d", e.Count, game.TileAt(3).House)
	}
}

func TestSellingSendsTheSquareBackDown(t *testing.T) {
	g := dealt(t)
	if _, err := g.Submit("p0", core.ClientMsg{Type: "build", Tile: 1}); err != nil {
		t.Fatal(err)
	}
	entries, err := g.Submit("p0", core.ClientMsg{Type: "sell", Tile: 1})
	if err != nil {
		t.Fatalf("selling: %v", err)
	}
	if g.state.Houses[1] != 0 {
		t.Errorf("square 1 stands at %d after a sale", g.state.Houses[1])
	}
	if len(entries) != 1 || entries[0].Kind != "sold" {
		t.Fatalf("selling reported %v", entries)
	}
	if h := entries[0].Houses; h == nil || *h != 0 {
		t.Errorf("the entry says %v buildings left, want 0", h)
	}
}

// The generic snapshot test in ../snapshot_test.go drives every game with
// AutoMove, and AutoMove never builds — so an all-zero Houses array round-trips
// there whatever happens to it. This is the case that would actually notice.
func TestBuildingsSurviveASnapshot(t *testing.T) {
	g := dealt(t)
	for _, tile := range []int{1, 3, 1, 3, 1, 3, 1, 3} {
		if _, err := g.Submit("p0", core.ClientMsg{Type: "build", Tile: tile}); err != nil {
			t.Fatalf("building on %d: %v", tile, err)
		}
	}
	// Four each: one more on either is a hotel, which is the state most worth
	// getting back intact.
	if g.state.Houses[1] != 4 || g.state.Houses[3] != 4 {
		t.Fatalf("the set stands at %d/%d, want 4/4", g.state.Houses[1], g.state.Houses[3])
	}
	if _, err := g.Submit("p0", core.ClientMsg{Type: "build", Tile: 1}); err != nil {
		t.Fatal(err)
	}

	blob, err := g.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, ok := New(nil).(*Game)
	if !ok {
		t.Fatal("New did not return a *Game")
	}
	if err := restored.Restore(blob); err != nil {
		t.Fatal(err)
	}

	if got := restored.state.Houses[1]; got != game.HotelLevel {
		t.Errorf("the hotel came back as %d buildings, want %d", got, game.HotelLevel)
	}
	if got := restored.state.Houses[3]; got != 4 {
		t.Errorf("Dawei came back with %d houses, want 4", got)
	}
	// The bank's stock is counted from the board, so it comes back only if the
	// board did — which is the point of counting it rather than storing it.
	if got, want := restored.state.HousesLeft(), g.state.HousesLeft(); got != want {
		t.Errorf("the bank came back holding %d houses, want %d", got, want)
	}
	if got, want := restored.state.HotelsLeft(), g.state.HotelsLeft(); got != want {
		t.Errorf("the bank came back holding %d hotels, want %d", got, want)
	}

	// And byte-identical, the assertion that catches a field the snapshot is
	// quietly dropping — see the note on gob in core.Game.
	again, err := restored.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blob, again) {
		t.Fatalf("the snapshot did not round-trip: %d bytes in, %d out", len(blob), len(again))
	}
}
