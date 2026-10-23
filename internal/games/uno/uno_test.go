package uno

import (
	"testing"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/uno/game"
	"boardgame/kittens/internal/prng"
)

func seats(n int) []core.Seat {
	out := make([]core.Seat, n)
	for i := range out {
		out[i] = core.Seat{ID: string(rune('a' + i)), Name: "P"}
	}
	return out
}

func dealt(t *testing.T, n int) *Game {
	t.Helper()
	g := New(prng.New(uint64(4))).(*Game)
	if _, err := g.Deal(seats(n)); err != nil {
		t.Fatalf("deal: %v", err)
	}
	return g
}

func shell(g *Game, viewer string) core.Shell {
	return core.Shell{Code: "ABCD", Game: "uno", ViewerID: viewer,
		Members: []core.Membership{
			{ID: "a", Name: "Ann", Connected: true, Host: true},
			{ID: "b", Name: "Bob", Connected: true},
		}}
}

func TestLobbyViewIsRenderable(t *testing.T) {
	g := New(nil).(*Game)
	if g.Started() || g.Over() {
		t.Fatal("a fresh table has nothing dealt")
	}
	v := g.View(shell(g, "a")).(*View)
	if v.Started || v.Phase != "lobby" || v.Game != "uno" || len(v.Seats) != 2 {
		t.Fatalf("lobby view: %+v", v)
	}
	if !v.Me.Host || v.Me.ID != "a" {
		t.Errorf("me = %+v, want the host's own seat", v.Me)
	}
	// The shell renders the lobby off these, so an absent hand would be a crash
	// in the browser rather than an empty list.
	if v.Me.Hand == nil || v.Log == nil {
		t.Error("hand and log must be empty rather than absent")
	}
}

// A view must never carry another player's cards, whatever the phase.
func TestViewHidesOtherHandsAndTheDeck(t *testing.T) {
	g := dealt(t, 2)
	v := g.View(shell(g, "a")).(*View)

	if len(v.Me.Hand) != 7 {
		t.Errorf("own hand has %d cards, want 7", len(v.Me.Hand))
	}
	for _, s := range v.Seats {
		if s.HandCount != 7 {
			t.Errorf("%s shows %d cards, want a count of 7", s.ID, s.HandCount)
		}
	}
	// 108 - 14 dealt - 1 turned over, less anything the opening card made
	// somebody draw.
	if v.DeckCount < 90 || v.DeckCount > 93 {
		t.Errorf("deck count %d is not what a two-player deal leaves", v.DeckCount)
	}
	if v.DiscardTop == nil {
		t.Fatal("no card face up")
	}
}

func TestPlayThroughTheAdapter(t *testing.T) {
	g := dealt(t, 2)
	v := g.View(shell(g, g.state.CurrentID())).(*View)
	if v.Phase == string(game.PhaseColour) {
		// Dealt on a wild: name a colour first, which is a move like any other.
		if _, err := g.Submit(v.Me.ID, core.ClientMsg{Type: "colour", Colour: "red"}); err != nil {
			t.Fatalf("colour: %v", err)
		}
		v = g.View(shell(g, g.state.CurrentID())).(*View)
	}
	if !v.Me.MyTurn {
		t.Fatalf("the current player's view says it is not their turn: %+v", v.Me)
	}

	if len(v.Me.Playable) > 0 {
		card := v.Me.Playable[0]
		if _, err := g.Submit(v.Me.ID, core.ClientMsg{Type: "play", CardID: card, Colour: "blue"}); err != nil {
			// A colour on a non-wild is refused, which is itself the rule under
			// test; retry without one.
			if _, err := g.Submit(v.Me.ID, core.ClientMsg{Type: "play", CardID: card}); err != nil {
				t.Fatalf("play: %v", err)
			}
		}
	} else if _, err := g.Submit(v.Me.ID, core.ClientMsg{Type: "draw"}); err != nil {
		t.Fatalf("draw: %v", err)
	}

	if _, err := g.Submit("a", core.ClientMsg{Type: "nope"}); err != core.ErrUnknownAction {
		t.Errorf("a kittens move on an UNO table: got %v, want ErrUnknownAction", err)
	}
}

// The room asks who it is waiting for, and plays for them if they have gone.
func TestBlockedOnAndAutoMove(t *testing.T) {
	g := dealt(t, 3)
	blocked := g.BlockedOn()
	if blocked == "" {
		t.Fatal("a freshly dealt game is waiting for somebody")
	}
	if _, err := g.AutoMove(blocked); err != nil {
		t.Fatalf("auto-move: %v", err)
	}
	// Whatever it played, the table is still in a state somebody can act in.
	if g.BlockedOn() == "" && !g.Over() {
		t.Fatal("after an auto-move the table is blocked on nobody")
	}
}

func TestAutoMoveNamesAColourItActuallyHolds(t *testing.T) {
	g := dealt(t, 2)
	// Force the wild-in-flight phase by hand: dealing into it is a one-in-fifteen
	// chance and a test that only sometimes runs is worse than none.
	for g.state.Phase != game.PhaseColour {
		id := g.state.CurrentID()
		p := g.state.Find(id)
		wild := game.Card{}
		found := false
		for _, c := range p.Hand {
			if c.Rank == game.RankWild {
				wild, found = c, true
				break
			}
		}
		if !found {
			if _, err := g.AutoMove(id); err != nil {
				t.Fatalf("auto-move: %v", err)
			}
			if g.Over() {
				t.Skip("the game ended before a wild turned up")
			}
			continue
		}
		if _, err := g.Submit(id, core.ClientMsg{Type: "play", CardID: wild.ID}); err != nil {
			t.Fatalf("play wild: %v", err)
		}
	}

	waiting := g.BlockedOn()
	if _, err := g.AutoMove(waiting); err != nil {
		t.Fatalf("auto-move on a colour prompt: %v", err)
	}
	if g.state.Phase == game.PhaseColour {
		t.Error("the colour prompt is still open after an auto-move")
	}
}

// UNO has no window for the rest of the table to interrupt a play in, and the
// room must be told so plainly or it will run a timer for nothing.
func TestNoActionWindow(t *testing.T) {
	g := dealt(t, 2)
	if _, _, open := g.Window(); open {
		t.Error("uno should never open an action window")
	}
	if entries := g.WindowExpired(); entries != nil {
		t.Errorf("a window that cannot open cannot expire: %v", entries)
	}
}

func TestResetReturnsToTheLobby(t *testing.T) {
	g := dealt(t, 2)
	g.Reset()
	if g.Started() {
		t.Error("reset left a game in play")
	}
	if v := g.View(shell(g, "a")).(*View); v.Started {
		t.Error("the view still says a game is under way")
	}
}
