package game

import (
	"boardgame/kittens/internal/prng"
	"fmt"
	"testing"
)

// mkState builds a fully deterministic table. Hands and the draw pile are given
// as card types, top-first for the draw pile.
//
// Note that a player holding no Nope card can never open a Nope window, so tests
// that want an action to resolve instantly simply omit Nope from every hand.
func mkState(hands [][]CardType, draw []CardType) *State {
	s := &State{Phase: PhaseTurn, TurnsRemaining: 1, RNG: prng.New(uint64(1))}
	id := 0
	for i, h := range hands {
		p := &Player{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i), Alive: true}
		for _, t := range h {
			p.Hand = append(p.Hand, newCard(id, t))
			id++
		}
		s.Players = append(s.Players, p)
	}
	for _, t := range draw {
		s.Draw = append(s.Draw, newCard(id, t))
		id++
	}
	return s
}

// cardID finds the first card of the given type in a player's hand.
func cardID(t *testing.T, p *Player, ct CardType) int {
	t.Helper()
	for _, c := range p.Hand {
		if c.Type == ct {
			return c.ID
		}
	}
	t.Fatalf("player %s has no %s (hand: %v)", p.ID, ct, typesOf(p.Hand))
	return 0
}

func typesOf(cards []Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Type.String()
	}
	return out
}

func mustApply(t *testing.T, s *State, a Action) []Event {
	t.Helper()
	ev, err := Apply(s, a)
	if err != nil {
		t.Fatalf("Apply(%s by %s): unexpected error: %v", a.Kind, a.PlayerID, err)
	}
	return ev
}

func countType(cards []Card, ct CardType) int {
	n := 0
	for _, c := range cards {
		if c.Type == ct {
			n++
		}
	}
	return n
}

// ------------------------------------------------------------------------ setup

func TestNewGameSetup(t *testing.T) {
	for n := MinPlayers; n <= MaxPlayers; n++ {
		t.Run(fmt.Sprintf("%dplayers", n), func(t *testing.T) {
			seats := make([]Seat, n)
			for i := range seats {
				seats[i] = Seat{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i)}
			}
			s, err := NewGame(seats, Original, prng.New(uint64(n)))
			if err != nil {
				t.Fatal(err)
			}

			total := len(s.Draw)
			for _, p := range s.Players {
				if len(p.Hand) != handSize+1 {
					t.Errorf("hand size = %d, want %d", len(p.Hand), handSize+1)
				}
				if got := countType(p.Hand, Defuse); got != 1 {
					t.Errorf("player %s holds %d Defuses, want exactly 1", p.ID, got)
				}
				if got := countType(p.Hand, ExplodingKitten); got != 0 {
					t.Errorf("player %s was dealt %d Kittens, want 0", p.ID, got)
				}
				total += len(p.Hand)
			}
			// The 4-(n-1) unused Kittens are removed from the game at setup, so
			// only 51+n of the 56 cards are ever in play.
			if want := 51 + n; total != want {
				t.Errorf("total cards in play = %d, want %d", total, want)
			}

			// One fewer Kitten than players is what guarantees a single survivor.
			if got := countType(s.Draw, ExplodingKitten); got != n-1 {
				t.Errorf("Kittens in deck = %d, want %d", got, n-1)
			}
			if got := countType(s.Draw, Defuse); got != 6-n {
				t.Errorf("leftover Defuses in deck = %d, want %d", got, 6-n)
			}
		})
	}
}

func TestNewGameRejectsBadPlayerCount(t *testing.T) {
	for _, n := range []int{0, 1, 6} {
		seats := make([]Seat, n)
		for i := range seats {
			seats[i] = Seat{ID: fmt.Sprintf("p%d", i)}
		}
		if _, err := NewGame(seats, Original, prng.New(uint64(1))); err != ErrPlayerCount {
			t.Errorf("NewGame with %d players: err = %v, want ErrPlayerCount", n, err)
		}
	}
}

// -------------------------------------------------------------- turns & drawing

func TestDrawEndsTurnAndKeepsCard(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Skip}}, []CardType{Attack, Shuffle})

	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if s.Current != 1 {
		t.Errorf("current player = %d, want 1 — drawing is what passes the turn", s.Current)
	}
	if len(s.Players[0].Hand) != 2 {
		t.Errorf("p0 hand = %v, want the drawn card added", typesOf(s.Players[0].Hand))
	}
	if len(s.Draw) != 1 {
		t.Errorf("draw pile = %d, want 1", len(s.Draw))
	}
}

func TestDrawnCardIsPrivate(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Skip}}, []CardType{Attack})
	ev := mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	for _, e := range ev {
		if e.Kind == EvDrew && len(e.Cards) > 0 && e.OnlyFor != "p0" {
			t.Errorf("drew event exposes %v to everyone; it must be OnlyFor p0", typesOf(e.Cards))
		}
	}
}

func TestSkipEndsTurnWithoutDrawing(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Attack}}, []CardType{Shuffle, Shuffle})
	id := cardID(t, s.Players[0], Skip)

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{id}})

	if s.Current != 1 {
		t.Errorf("current = %d, want 1", s.Current)
	}
	if len(s.Draw) != 2 {
		t.Errorf("draw pile = %d, want 2 — Skip must not draw", len(s.Draw))
	}
}

func TestAttackGivesNextPlayerTwoTurns(t *testing.T) {
	s := mkState([][]CardType{{Attack}, {Skip}, {Skip}}, []CardType{Shuffle, Shuffle, Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Attack)}})

	if s.Current != 1 || s.TurnsRemaining != 2 {
		t.Fatalf("after Attack: current=%d turns=%d, want current=1 turns=2", s.Current, s.TurnsRemaining)
	}

	// First of the two turns.
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p1"})
	if s.Current != 1 || s.TurnsRemaining != 1 {
		t.Fatalf("after first draw: current=%d turns=%d, want current=1 turns=1", s.Current, s.TurnsRemaining)
	}

	// Second turn completes the debt and passes play on.
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p1"})
	if s.Current != 2 {
		t.Errorf("after second draw: current=%d, want 2", s.Current)
	}
}

func TestAttackStacks(t *testing.T) {
	s := mkState([][]CardType{{Attack}, {Attack}, {Skip}}, []CardType{Shuffle, Shuffle, Shuffle})

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Attack)}})
	// p1 owes 2 turns and immediately re-attacks: p2 inherits the 1 remaining
	// turn plus 2 more.
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p1", CardIDs: []int{cardID(t, s.Players[1], Attack)}})

	if s.Current != 2 || s.TurnsRemaining != 3 {
		t.Errorf("current=%d turns=%d, want current=2 turns=3", s.Current, s.TurnsRemaining)
	}
}

func TestSkipUnderAttackBurnsOnlyOneTurn(t *testing.T) {
	s := mkState([][]CardType{{Attack}, {Skip}, {Skip}}, []CardType{Shuffle, Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Attack)}})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p1", CardIDs: []int{cardID(t, s.Players[1], Skip)}})

	if s.Current != 1 || s.TurnsRemaining != 1 {
		t.Errorf("current=%d turns=%d, want p1 still on turn with 1 remaining", s.Current, s.TurnsRemaining)
	}
}

func TestPlayOutOfTurnRejected(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Skip}}, []CardType{Shuffle})
	_, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p1", CardIDs: []int{cardID(t, s.Players[1], Skip)}})
	if err != ErrNotYourTurn {
		t.Errorf("err = %v, want ErrNotYourTurn", err)
	}
}

func TestUnplayableCardsRejected(t *testing.T) {
	s := mkState([][]CardType{{Defuse, CatTaco, Nope}, {Skip}}, []CardType{Shuffle})
	for _, ct := range []CardType{Defuse, CatTaco, Nope} {
		_, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], ct)}})
		if err != ErrNotPlayable {
			t.Errorf("playing a lone %s: err = %v, want ErrNotPlayable", ct, err)
		}
	}
	if len(s.Players[0].Hand) != 3 {
		t.Errorf("a rejected play must not discard anything; hand = %v", typesOf(s.Players[0].Hand))
	}
}

// --------------------------------------------------------- exploding & defusing

func TestExplodeWithoutDefuseEliminates(t *testing.T) {
	s := mkState([][]CardType{{Skip, Attack}, {Skip}, {Skip}}, []CardType{ExplodingKitten, Shuffle})

	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if s.Players[0].Alive {
		t.Fatal("p0 drew a Kitten with no Defuse and is still alive")
	}
	if len(s.Players[0].Hand) != 0 {
		t.Errorf("eliminated hand = %v, want discarded", typesOf(s.Players[0].Hand))
	}
	if countType(s.Discard, ExplodingKitten) != 1 {
		t.Error("the Kitten should be in the discard pile")
	}
	if s.Current != 1 {
		t.Errorf("current = %d, want play to move to p1", s.Current)
	}
}

func TestExplodeEndsGameWithTwoPlayers(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Skip}}, []CardType{ExplodingKitten})
	ev := mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if s.Phase != PhaseGameOver {
		t.Fatalf("phase = %s, want game_over", s.Phase)
	}
	if s.WinnerID != "p1" {
		t.Errorf("winner = %q, want p1", s.WinnerID)
	}
	var sawGameOver bool
	for _, e := range ev {
		if e.Kind == EvGameOver {
			sawGameOver = true
		}
	}
	if !sawGameOver {
		t.Error("no game_over event emitted")
	}
	if _, err := Apply(s, Action{Kind: ActDraw, PlayerID: "p1"}); err != ErrGameOver {
		t.Errorf("acting after the win: err = %v, want ErrGameOver", err)
	}
}

func TestDefuseReinsertsAtChosenIndex(t *testing.T) {
	s := mkState([][]CardType{{Defuse, Skip}, {Skip}},
		[]CardType{ExplodingKitten, Shuffle, Attack, Favor})

	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	if s.Phase != PhaseDefuse {
		t.Fatalf("phase = %s, want defuse", s.Phase)
	}
	if s.Players[0].hasType(Defuse) {
		t.Error("the Defuse should have been spent")
	}
	if s.Current != 0 {
		t.Error("the turn must not pass until the Kitten is placed")
	}

	mustApply(t, s, Action{Kind: ActPlaceKitten, PlayerID: "p0", Index: 2})

	if s.Draw[2].Type != ExplodingKitten {
		t.Errorf("draw pile = %v, want the Kitten at index 2", typesOf(s.Draw))
	}
	if len(s.Draw) != 4 {
		t.Errorf("draw pile size = %d, want 4", len(s.Draw))
	}
	if s.Current != 1 {
		t.Errorf("current = %d, want the turn to end after defusing", s.Current)
	}
}

func TestDefusePlacementIndexIsClamped(t *testing.T) {
	s := mkState([][]CardType{{Defuse}, {Skip}}, []CardType{ExplodingKitten, Shuffle})
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	mustApply(t, s, Action{Kind: ActPlaceKitten, PlayerID: "p0", Index: 999})

	if s.Draw[len(s.Draw)-1].Type != ExplodingKitten {
		t.Errorf("draw = %v, want an out-of-range index clamped to the bottom", typesOf(s.Draw))
	}
}

// ----------------------------------------------------------------------- Favor

func TestFavorTargetChoosesAndTurnContinues(t *testing.T) {
	s := mkState([][]CardType{{Favor}, {Skip, Attack}}, []CardType{Shuffle})

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Favor)}, TargetID: "p1"})
	if s.Phase != PhaseFavor {
		t.Fatalf("phase = %s, want favor", s.Phase)
	}

	// The target picks — not the requester.
	if _, err := Apply(s, Action{Kind: ActGiveCard, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[1], Skip)}}); err == nil {
		t.Error("the requester must not be able to choose the card themselves")
	}

	give := cardID(t, s.Players[1], Attack)
	mustApply(t, s, Action{Kind: ActGiveCard, PlayerID: "p1", CardIDs: []int{give}})

	if !s.Players[0].hasType(Attack) || s.Players[1].hasType(Attack) {
		t.Errorf("card did not move: p0=%v p1=%v", typesOf(s.Players[0].Hand), typesOf(s.Players[1].Hand))
	}
	if s.Current != 0 || s.Phase != PhaseTurn {
		t.Errorf("current=%d phase=%s, want p0 still on turn — Favor does not end it", s.Current, s.Phase)
	}
}

func TestFavorAgainstEmptyHandFizzles(t *testing.T) {
	s := mkState([][]CardType{{Favor}, {}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Favor)}, TargetID: "p1"})

	if s.Phase != PhaseTurn {
		t.Errorf("phase = %s, want the game to carry on when there is nothing to give", s.Phase)
	}
}

func TestFavorRejectsBadTargets(t *testing.T) {
	s := mkState([][]CardType{{Favor}, {Skip}}, []CardType{Shuffle})
	id := cardID(t, s.Players[0], Favor)
	for _, target := range []string{"", "p0", "nobody"} {
		if _, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{id}, TargetID: target}); err != ErrBadTarget {
			t.Errorf("Favor targeting %q: err = %v, want ErrBadTarget", target, err)
		}
	}
}

// -------------------------------------------------------------------- cat pairs

func TestCatPairStealsOneCard(t *testing.T) {
	s := mkState([][]CardType{{CatTaco, CatTaco}, {Skip}}, []CardType{Shuffle})
	p0 := s.Players[0]

	mustApply(t, s, Action{
		Kind: ActPlay, PlayerID: "p0",
		CardIDs:  []int{p0.Hand[0].ID, p0.Hand[1].ID},
		TargetID: "p1",
	})

	if len(s.Players[1].Hand) != 0 {
		t.Errorf("p1 hand = %v, want emptied", typesOf(s.Players[1].Hand))
	}
	if len(p0.Hand) != 1 || p0.Hand[0].Type != Skip {
		t.Errorf("p0 hand = %v, want just the stolen Skip", typesOf(p0.Hand))
	}
	if s.Current != 0 {
		t.Error("stealing must not end the turn")
	}
}

func TestMismatchedCatsRejected(t *testing.T) {
	s := mkState([][]CardType{{CatTaco, CatBeard}, {Skip}}, []CardType{Shuffle})
	p0 := s.Players[0]

	_, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{p0.Hand[0].ID, p0.Hand[1].ID}, TargetID: "p1"})
	if err != ErrBadCatSet {
		t.Errorf("err = %v, want ErrBadCatSet", err)
	}
	if len(p0.Hand) != 2 {
		t.Error("a rejected pair must stay in hand")
	}
}

// ------------------------------------------------------------- three of a kind

// threeCats plays a trio of matching cats naming the given card.
func threeCats(t *testing.T, s *State, named string) ([]Event, error) {
	t.Helper()
	p0 := s.Players[0]
	return Apply(s, Action{
		Kind: ActPlay, PlayerID: "p0",
		CardIDs:  []int{p0.Hand[0].ID, p0.Hand[1].ID, p0.Hand[2].ID},
		TargetID: "p1", Named: named,
	})
}

func TestThreeOfAKindTakesTheNamedCard(t *testing.T) {
	s := mkState([][]CardType{
		{CatTaco, CatTaco, CatTaco},
		{Skip, Defuse, Attack},
	}, []CardType{Shuffle})

	ev, err := threeCats(t, s, "defuse")
	if err != nil {
		t.Fatal(err)
	}

	if !s.Players[0].hasType(Defuse) {
		t.Errorf("p0 hand = %v, want the demanded Defuse", typesOf(s.Players[0].Hand))
	}
	if s.Players[1].hasType(Defuse) {
		t.Errorf("p1 hand = %v, want the Defuse handed over", typesOf(s.Players[1].Hand))
	}
	if len(s.Players[1].Hand) != 2 {
		t.Errorf("p1 lost %d cards, want exactly 1", 3-len(s.Players[1].Hand))
	}
	if s.Current != 0 {
		t.Error("a demand must not end the turn")
	}
	// All three cats are spent.
	if countType(s.Discard, CatTaco) != 3 {
		t.Errorf("discard = %v, want three Taco Cats spent", typesOf(s.Discard))
	}

	var demanded, stole bool
	for _, e := range ev {
		if e.Kind == EvDemanded && e.Text == "Defuse" {
			demanded = true
		}
		if e.Kind == EvStole && e.Text == "Defuse" {
			stole = true
		}
		// The whole table hears this, so nothing here may be private.
		if (e.Kind == EvDemanded || e.Kind == EvStole) && e.OnlyFor != "" {
			t.Errorf("%s event was private; the demand is public", e.Kind)
		}
	}
	if !demanded || !stole {
		t.Errorf("events = %+v, want a public demand and a public transfer", ev)
	}
}

func TestThreeOfAKindMissesWhenTargetLacksTheCard(t *testing.T) {
	s := mkState([][]CardType{
		{CatTaco, CatTaco, CatTaco},
		{Skip, Attack},
	}, []CardType{Shuffle})

	ev, err := threeCats(t, s, "defuse")
	if err != nil {
		t.Fatal(err)
	}

	if len(s.Players[1].Hand) != 2 {
		t.Errorf("p1 hand = %v, want it untouched", typesOf(s.Players[1].Hand))
	}
	if len(s.Players[0].Hand) != 0 {
		t.Errorf("p0 hand = %v, want the three cats spent for nothing", typesOf(s.Players[0].Hand))
	}
	var missed bool
	for _, e := range ev {
		if e.Kind == EvMissed {
			missed = true
		}
	}
	if !missed {
		t.Error("no missed event; the table must hear that the demand failed")
	}
}

func TestThreeOfAKindNeedsANamedCard(t *testing.T) {
	for _, named := range []string{"", "not-a-card"} {
		s := mkState([][]CardType{{CatTaco, CatTaco, CatTaco}, {Defuse}}, []CardType{Shuffle})
		if _, err := threeCats(t, s, named); err != ErrNoNamedCard {
			t.Errorf("naming %q: err = %v, want ErrNoNamedCard", named, err)
		}
		if len(s.Players[0].Hand) != 3 {
			t.Error("a rejected demand must leave the cats in hand")
		}
	}
}

func TestThreeMismatchedCatsRejected(t *testing.T) {
	s := mkState([][]CardType{{CatTaco, CatTaco, CatBeard}, {Defuse}}, []CardType{Shuffle})
	if _, err := threeCats(t, s, "defuse"); err != ErrBadCatSet {
		t.Errorf("err = %v, want ErrBadCatSet", err)
	}
	if len(s.Players[0].Hand) != 3 {
		t.Error("a rejected trio must stay in hand")
	}
}

func TestThreeNonCatsRejected(t *testing.T) {
	s := mkState([][]CardType{{Skip, Skip, Skip}, {Defuse}}, []CardType{Shuffle})
	if _, err := threeCats(t, s, "defuse"); err != ErrBadCatSet {
		t.Errorf("err = %v, want ErrBadCatSet — only cats form sets", err)
	}
}

func TestFourCardsRejected(t *testing.T) {
	s := mkState([][]CardType{{CatTaco, CatTaco, CatTaco, CatTaco}, {Defuse}}, []CardType{Shuffle})
	p0 := s.Players[0]
	_, err := Apply(s, Action{
		Kind: ActPlay, PlayerID: "p0", TargetID: "p1", Named: "defuse",
		CardIDs: []int{p0.Hand[0].ID, p0.Hand[1].ID, p0.Hand[2].ID, p0.Hand[3].ID},
	})
	if err != ErrNotPlayable {
		t.Errorf("err = %v, want ErrNotPlayable", err)
	}
}

func TestThreeOfAKindCanBeNoped(t *testing.T) {
	s := mkState([][]CardType{
		{CatTaco, CatTaco, CatTaco},
		{Defuse, Nope},
	}, []CardType{Shuffle})

	if _, err := threeCats(t, s, "defuse"); err != nil {
		t.Fatal(err)
	}
	if s.Phase != PhaseNope {
		t.Fatalf("phase = %s, want a nope window", s.Phase)
	}
	mustApply(t, s, Action{Kind: ActNope, PlayerID: "p1"})

	if !s.Players[1].hasType(Defuse) {
		t.Error("the noped demand still took the Defuse")
	}
	if s.Current != 0 {
		t.Error("p0 should still be on turn")
	}
}

// A demand is public, so the named card must reach the view — but as a kind, not
// as a specific card carrying an ID.
func TestThreeOfAKindExposesTheNameNotACard(t *testing.T) {
	s := mkState([][]CardType{
		{CatTaco, CatTaco, CatTaco},
		{Defuse, Nope},
	}, []CardType{Shuffle})

	if _, err := threeCats(t, s, "defuse"); err != nil {
		t.Fatal(err)
	}
	if s.Pending == nil || !s.Pending.HasNamed {
		t.Fatal("pending action did not record the named card")
	}
	if s.Pending.Named != Defuse {
		t.Errorf("named = %s, want Defuse", s.Pending.Named)
	}
}

func TestSameCardTwiceRejected(t *testing.T) {
	s := mkState([][]CardType{{CatTaco}, {Skip}}, []CardType{Shuffle})
	id := s.Players[0].Hand[0].ID

	if _, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{id, id}, TargetID: "p1"}); err == nil {
		t.Error("playing the same physical card twice must be rejected")
	}
}

func TestCatPairAgainstEmptyHandFizzles(t *testing.T) {
	s := mkState([][]CardType{{CatTaco, CatTaco}, {}}, []CardType{Shuffle})
	p0 := s.Players[0]
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{p0.Hand[0].ID, p0.Hand[1].ID}, TargetID: "p1"})

	if s.Phase != PhaseTurn || s.Current != 0 {
		t.Errorf("phase=%s current=%d, want p0 to simply carry on", s.Phase, s.Current)
	}
}

// ------------------------------------------------------------------ See the Future

func TestSeeTheFutureIsPrivateAndNonDestructive(t *testing.T) {
	s := mkState([][]CardType{{SeeTheFuture}, {Skip}}, []CardType{Attack, Favor, Shuffle, Skip})

	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], SeeTheFuture)}})

	var future *Event
	for i := range ev {
		if ev[i].Kind == EvFuture {
			future = &ev[i]
		}
	}
	if future == nil {
		t.Fatal("no future event emitted")
	}
	if future.OnlyFor != "p0" {
		t.Errorf("future.OnlyFor = %q, want p0 — the top of the deck must not leak", future.OnlyFor)
	}
	if len(future.Cards) != 3 {
		t.Fatalf("saw %d cards, want 3", len(future.Cards))
	}
	for i, want := range []CardType{Attack, Favor, Shuffle} {
		if future.Cards[i].Type != want {
			t.Errorf("card %d = %s, want %s", i, future.Cards[i].Type, want)
		}
	}
	if len(s.Draw) != 4 {
		t.Errorf("draw pile = %d, want 4 — peeking must not consume cards", len(s.Draw))
	}
}

func TestSeeTheFutureNearEmptyDeck(t *testing.T) {
	s := mkState([][]CardType{{SeeTheFuture}, {Skip}}, []CardType{Attack})
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], SeeTheFuture)}})

	for _, e := range ev {
		if e.Kind == EvFuture && len(e.Cards) != 1 {
			t.Errorf("saw %d cards, want 1 when only one remains", len(e.Cards))
		}
	}
}

// ------------------------------------------------------------------------- Nope

func TestNopeWindowOpensOnlyWhenSomeoneCanNope(t *testing.T) {
	// Nobody holds a Nope, so the action resolves without stalling the table.
	s := mkState([][]CardType{{Skip}, {Attack}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	if s.Phase != PhaseTurn || s.Current != 1 {
		t.Errorf("phase=%s current=%d, want the Skip to resolve immediately", s.Phase, s.Current)
	}

	// With a Nope in the room, the window stays open.
	s = mkState([][]CardType{{Skip}, {Nope}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	if s.Phase != PhaseNope {
		t.Errorf("phase = %s, want a nope window", s.Phase)
	}
}

func TestNopeCancelsTheAction(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Nope}}, []CardType{Shuffle, Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	mustApply(t, s, Action{Kind: ActNope, PlayerID: "p1"})

	if s.Phase != PhaseTurn {
		t.Fatalf("phase = %s, want turn", s.Phase)
	}
	if s.Current != 0 {
		t.Errorf("current = %d, want p0 still on turn — the Skip was noped", s.Current)
	}
	if countType(s.Discard, Nope) != 1 || countType(s.Discard, Skip) != 1 {
		t.Errorf("discard = %v, want both the Skip and the Nope spent", typesOf(s.Discard))
	}
}

func TestNopeTheNopeRestoresTheAction(t *testing.T) {
	s := mkState([][]CardType{{Skip, Nope}, {Nope}}, []CardType{Shuffle, Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	mustApply(t, s, Action{Kind: ActNope, PlayerID: "p1"}) // nope
	mustApply(t, s, Action{Kind: ActNope, PlayerID: "p0"}) // yup

	if s.Phase != PhaseTurn {
		t.Fatalf("phase = %s, want turn", s.Phase)
	}
	if s.Current != 1 {
		t.Errorf("current = %d, want the Skip to take effect after the Yup", s.Current)
	}
}

func TestPlayerCannotNopeTheirOwnLastCard(t *testing.T) {
	s := mkState([][]CardType{{Skip, Nope}, {Nope}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})

	if _, err := Apply(s, Action{Kind: ActNope, PlayerID: "p0"}); err == nil {
		t.Error("a player must not be able to Nope the card they just played")
	}
}

func TestNopeRequiresANopeCard(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Nope}, {Attack}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})

	if _, err := Apply(s, Action{Kind: ActNope, PlayerID: "p2"}); err != ErrNoNopeCard {
		t.Errorf("err = %v, want ErrNoNopeCard", err)
	}
}

func TestPassingClosesTheWindow(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Nope}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	mustApply(t, s, Action{Kind: ActPass, PlayerID: "p1"})

	if s.Phase != PhaseTurn || s.Current != 1 {
		t.Errorf("phase=%s current=%d, want the Skip to resolve once everyone passed", s.Phase, s.Current)
	}
}

func TestWindowExpiryResolvesTheAction(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Nope}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	mustApply(t, s, Action{Kind: ActNopeExpired})

	if s.Phase != PhaseTurn || s.Current != 1 {
		t.Errorf("phase=%s current=%d, want the timer to let the Skip through", s.Phase, s.Current)
	}
}

func TestNopeResetsPassesSoEveryoneCanYup(t *testing.T) {
	s := mkState([][]CardType{{Skip, Nope}, {Nope}, {Nope}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Skip)}})
	mustApply(t, s, Action{Kind: ActPass, PlayerID: "p1"})
	mustApply(t, s, Action{Kind: ActNope, PlayerID: "p2"})

	// p1 passed before the Nope landed; they must get another say.
	if s.Phase != PhaseNope {
		t.Fatalf("phase = %s, want the window reopened after a Nope", s.Phase)
	}
	if s.Pending.Passed["p1"] {
		t.Error("p1's earlier pass should have been cleared by the new Nope")
	}
}

func TestDrawIsNotNopeable(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Nope}}, []CardType{Shuffle, Shuffle})
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if s.Phase != PhaseTurn {
		t.Errorf("phase = %s, want drawing to bypass the Nope window entirely", s.Phase)
	}
	if _, err := Apply(s, Action{Kind: ActNope, PlayerID: "p1"}); err != ErrWrongPhase {
		t.Errorf("err = %v, want ErrWrongPhase", err)
	}
}

func TestExplodingKittenIsNotNopeable(t *testing.T) {
	s := mkState([][]CardType{{Defuse}, {Nope}}, []CardType{ExplodingKitten, Shuffle})
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if s.Phase != PhaseDefuse {
		t.Fatalf("phase = %s, want defuse", s.Phase)
	}
	if _, err := Apply(s, Action{Kind: ActNope, PlayerID: "p1"}); err != ErrWrongPhase {
		t.Errorf("err = %v, want a Defuse to be un-nopeable", err)
	}
}

func TestNopedAttackLeavesTurnAlone(t *testing.T) {
	s := mkState([][]CardType{{Attack}, {Nope}, {Skip}}, []CardType{Shuffle})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Attack)}})
	mustApply(t, s, Action{Kind: ActNope, PlayerID: "p1"})

	if s.Current != 0 || s.TurnsRemaining != 1 {
		t.Errorf("current=%d turns=%d, want p0 unchanged on turn", s.Current, s.TurnsRemaining)
	}
}

// ------------------------------------------------------------- full-game sanity

// TestRandomGamesAlwaysTerminate plays many games with random legal moves. It is
// the backstop for rule interactions no single unit test thought to cover: the
// deck must never be exhausted, the invariants must hold every step, and exactly
// one player must be left standing.
func TestRandomGamesAlwaysTerminate(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		rng := prng.New(uint64(seed))
		n := 2 + int(seed)%4
		seats := make([]Seat, n)
		for i := range seats {
			seats[i] = Seat{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i)}
		}
		s, err := NewGame(seats, Original, rng)
		if err != nil {
			t.Fatal(err)
		}

		for step := 0; s.Phase != PhaseGameOver; step++ {
			if step > 5000 {
				t.Fatalf("seed %d: game did not terminate after 5000 steps (phase=%s)", seed, s.Phase)
			}
			if _, err := Apply(s, randomMove(s, rng)); err != nil {
				t.Fatalf("seed %d step %d: legal move rejected: %v", seed, step, err)
			}
			checkInvariants(t, s, seed, step)
		}
		if s.WinnerID == "" {
			t.Fatalf("seed %d: game over with no winner", seed)
		}
		if s.aliveCount() != 1 {
			t.Fatalf("seed %d: %d players alive at game over, want 1", seed, s.aliveCount())
		}
	}
}

// randomMove picks an arbitrary legal action for whoever the phase says must act.
func randomMove(s *State, rng *prng.Source) Action {
	switch s.Phase {
	case PhaseDefuse:
		return Action{Kind: ActPlaceKitten, PlayerID: s.current().ID, Index: rng.Intn(len(s.Draw) + 1)}

	case PhaseFavor:
		giver := s.playerByID(s.Pending.TargetID)
		return Action{Kind: ActGiveCard, PlayerID: giver.ID, CardIDs: []int{giver.Hand[rng.Intn(len(giver.Hand))].ID}}

	case PhaseNope:
		// Half the time somebody stacks another Nope; otherwise let it expire.
		for _, p := range s.Players {
			if p.Alive && p.ID != s.Pending.LastPlayerID && p.hasType(Nope) && rng.Intn(2) == 0 {
				return Action{Kind: ActNope, PlayerID: p.ID}
			}
		}
		return Action{Kind: ActNopeExpired}
	}

	// PhaseTurn: sometimes play a card, otherwise draw.
	p := s.current()
	if rng.Intn(3) > 0 {
		if a, ok := randomPlay(s, p, rng); ok {
			return a
		}
	}
	return Action{Kind: ActDraw, PlayerID: p.ID}
}

// someSlug names an arbitrary card for a demand, so the driver exercises both
// hitting and missing.
func someSlug(rng *prng.Source) string {
	all := []CardType{Defuse, Nope, Attack, Skip, Favor, Shuffle, SeeTheFuture, CatTaco}
	return all[rng.Intn(len(all))].Slug()
}

func randomPlay(s *State, p *Player, rng *prng.Source) (Action, bool) {
	var singles []Card
	cats := map[CardType][]Card{}
	for _, c := range p.Hand {
		switch {
		case c.Type.IsCat():
			cats[c.Type] = append(cats[c.Type], c)
		case c.Type == Skip, c.Type == Attack, c.Type == Shuffle, c.Type == SeeTheFuture, c.Type == Favor:
			singles = append(singles, c)
		}
	}

	var others []*Player
	for _, q := range s.Players {
		if q.Alive && q.ID != p.ID {
			others = append(others, q)
		}
	}

	// Cat sets: play three when three are held, so the demand path gets the same
	// invariant checking as everything else.
	for _, group := range cats {
		if len(group) < 2 || len(others) == 0 {
			continue
		}
		target := others[rng.Intn(len(others))]
		a := Action{Kind: ActPlay, PlayerID: p.ID, TargetID: target.ID}
		if len(group) >= 3 && rng.Intn(2) == 0 {
			a.CardIDs = []int{group[0].ID, group[1].ID, group[2].ID}
			a.Named = someSlug(rng)
		} else {
			a.CardIDs = []int{group[0].ID, group[1].ID}
		}
		return a, true
	}
	if len(singles) == 0 {
		return Action{}, false
	}
	c := singles[rng.Intn(len(singles))]
	a := Action{Kind: ActPlay, PlayerID: p.ID, CardIDs: []int{c.ID}}
	if c.Type == Favor {
		if len(others) == 0 {
			return Action{}, false
		}
		a.TargetID = others[rng.Intn(len(others))].ID
	}
	return a, true
}

func checkInvariants(t *testing.T, s *State, seed int64, step int) {
	t.Helper()

	total := len(s.Draw) + len(s.Discard)
	seen := map[int]bool{}
	for _, c := range append(append([]Card{}, s.Draw...), s.Discard...) {
		if seen[c.ID] {
			t.Fatalf("seed %d step %d: card %d exists twice", seed, step, c.ID)
		}
		seen[c.ID] = true
	}
	for _, p := range s.Players {
		total += len(p.Hand)
		for _, c := range p.Hand {
			if seen[c.ID] {
				t.Fatalf("seed %d step %d: card %d exists twice", seed, step, c.ID)
			}
			seen[c.ID] = true
		}
		if !p.Alive && len(p.Hand) != 0 {
			t.Fatalf("seed %d step %d: eliminated %s still holds cards", seed, step, p.ID)
		}
	}
	if s.PendingKitten != nil {
		total++
	}
	if want := 51 + len(s.Players); total != want {
		t.Fatalf("seed %d step %d: %d cards accounted for, want %d", seed, step, total, want)
	}

	if s.Phase != PhaseGameOver {
		if !s.current().Alive {
			t.Fatalf("seed %d step %d: turn belongs to eliminated %s", seed, step, s.current().ID)
		}
		if s.TurnsRemaining < 1 {
			t.Fatalf("seed %d step %d: TurnsRemaining = %d", seed, step, s.TurnsRemaining)
		}
	}
}
