package kittens

import (
	"testing"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/kittens/game"
	"boardgame/kittens/internal/prng"
)

// These tests guard the seam, not the rules — the rules have their own suite in
// ./game. What broke here once, and cost a table a permanent deadlock, was the
// wiring either side of it: a wire message with no case in toAction, a field the
// client sends that the shared ClientMsg has nowhere to put, and a phase the
// idle watchdog cannot see. All three are invisible to a rules test and to the
// compiler, and only turned up as "no winner after 400 steps" in a browser run.

// Every message the client can send has to reach an action. Listed literally
// rather than derived, so deleting a case fails here instead of somewhere a
// player is waiting.
func TestEveryWireTypeMapsToAnAction(t *testing.T) {
	want := map[string]game.ActionKind{
		"play":  game.ActPlay,
		"draw":  game.ActDraw,
		"nope":  game.ActNope,
		"pass":  game.ActPass,
		"give":  game.ActGiveCard,
		"place": game.ActPlaceKitten,
		"alter": game.ActAlterFuture,
	}
	for typ, kind := range want {
		a, ok := toAction("p1", core.ClientMsg{Type: typ})
		if !ok {
			t.Errorf("%q is not mapped to any action", typ)
			continue
		}
		if a.Kind != kind {
			t.Errorf("%q became %v, want %v", typ, a.Kind, kind)
		}
	}

	// The room's timer owns this one; a client must never be able to fire it.
	if _, ok := toAction("p1", core.ClientMsg{Type: "nope-expired"}); ok {
		t.Error("a client can end the Nope window itself")
	}
	if _, ok := toAction("p1", core.ClientMsg{Type: "nonsense"}); ok {
		t.Error("an unknown message was accepted")
	}
}

// Alter the Future is submitted as positions, and positions travel in their own
// field. A ClientMsg that drops them leaves the engine with an empty order,
// which it rejects — so the table sits in PhaseAlter until somebody reloads.
func TestAlterOrderSurvivesTheWire(t *testing.T) {
	a, ok := toAction("p1", core.ClientMsg{Type: "alter", Order: []int{2, 0, 1}})
	if !ok {
		t.Fatal("alter is not mapped")
	}
	if len(a.Order) != 3 || a.Order[0] != 2 || a.Order[1] != 0 || a.Order[2] != 1 {
		t.Fatalf("the order reached the engine as %v", a.Order)
	}
}

// End to end through Submit, because the two halves above can both be right
// while the message still fails to change anything.
func TestSubmitReordersTheTopOfTheDeck(t *testing.T) {
	g := altering(t)
	st := g.(*Game).state

	before := []int{st.Draw[0].ID, st.Draw[1].ID, st.Draw[2].ID}
	if _, err := g.Submit("p1", core.ClientMsg{Type: "alter", Order: []int{2, 1, 0}}); err != nil {
		t.Fatalf("submitting a reorder: %v", err)
	}
	after := []int{st.Draw[0].ID, st.Draw[1].ID, st.Draw[2].ID}

	for i := range after {
		if after[i] != before[2-i] {
			t.Fatalf("the top of the deck is %v, want %v reversed", after, before)
		}
	}
	if st.Phase != game.PhaseTurn {
		t.Fatalf("the table is still in %v after the reorder", st.Phase)
	}
}

// The watchdog has to be able to name whoever the table is waiting for, or a
// player who closes their laptop mid-reorder strands everybody else.
func TestTheTableIsBlockedOnTheAlterer(t *testing.T) {
	g := altering(t)
	if got := g.BlockedOn(); got != "p1" {
		t.Fatalf("blocked on %q, want the alterer", got)
	}
}

// And AutoMove has to have something to play for them. Identity: whoever
// dropped off learns nothing and changes nothing.
func TestAutoMoveLeavesTheDeckAsItWas(t *testing.T) {
	g := altering(t)
	st := g.(*Game).state
	before := []int{st.Draw[0].ID, st.Draw[1].ID, st.Draw[2].ID}

	if _, err := g.AutoMove("p1"); err != nil {
		t.Fatalf("nothing to play for an absent alterer: %v", err)
	}
	if st.Phase != game.PhaseTurn {
		t.Fatalf("the table is still in %v after the auto-move", st.Phase)
	}
	for i, id := range before {
		if st.Draw[i].ID != id {
			t.Fatalf("the auto-move rearranged the deck: %v", st.Draw[:3])
		}
	}
}

// altering deals an expansion table and puts it in the reorder phase. The state
// is reached directly rather than by playing an Alter the Future card, because a
// deal that happens to hand p1 one is a deal this test would have to fish for.
func altering(t *testing.T) core.Game {
	t.Helper()
	g := New(prng.New(uint64(1)), game.Imploding)
	if _, err := g.Deal([]core.Seat{{ID: "p1", Name: "Alex"}, {ID: "p2", Name: "Sam"}}); err != nil {
		t.Fatalf("dealing: %v", err)
	}
	st := g.(*Game).state
	st.Phase = game.PhaseAlter
	st.Altering = "p1"
	return g
}
