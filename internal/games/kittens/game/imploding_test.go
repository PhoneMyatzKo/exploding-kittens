package game

import (
	"boardgame/kittens/internal/prng"
	"fmt"
	"testing"
)

// The Imploding Kittens expansion. Its cards are the interesting ones to test:
// four of the six change something the base rules assumed was fixed — the
// direction of play, which end of the deck you draw from, whether a cat has to
// match, and whether a Defuse can save you.

// ------------------------------------------------------------------------ setup

func TestExpansionDeckHasTwentyExtraCards(t *testing.T) {
	base := len(fullDeck(Original, prng.New(uint64(1))))
	with := len(fullDeck(Imploding, prng.New(uint64(1))))
	if base != 56 {
		t.Fatalf("original deck = %d cards, want 56", base)
	}
	if with-base != 20 {
		t.Errorf("expansion adds %d cards, want 20", with-base)
	}
}

func TestOriginalDeckHasNoExpansionCards(t *testing.T) {
	deck := fullDeck(Original, prng.New(uint64(2)))
	for _, ct := range []CardType{
		ImplodingKitten, Reverse, DrawFromBottom, FeralCat, AlterTheFuture, TargetedAttack,
	} {
		if n := countType(deck, ct); n != 0 {
			t.Errorf("original deck contains %d %s", n, ct)
		}
	}
}

// The kitten arithmetic is the whole reason the expansion can seat six: one
// Imploding Kitten plus the Original's four Exploding Kittens is exactly the five
// a six-player game needs.
func TestExpansionSeedsOneImplodingKittenAndTheRestExploding(t *testing.T) {
	for n := MinPlayers; n <= MaxPlayersFor(Imploding); n++ {
		t.Run(fmt.Sprintf("%d players", n), func(t *testing.T) {
			var seats []Seat
			for i := 0; i < n; i++ {
				seats = append(seats, Seat{ID: fmt.Sprintf("p%d", i), Name: "P"})
			}
			s, err := NewGame(seats, Imploding, prng.New(uint64(n)))
			if err != nil {
				t.Fatal(err)
			}

			imploding := countType(s.Draw, ImplodingKitten)
			exploding := countType(s.Draw, ExplodingKitten)
			if imploding != 1 {
				t.Errorf("deck has %d Imploding Kittens, want exactly 1", imploding)
			}
			if want := n - 2; exploding != want {
				t.Errorf("deck has %d Exploding Kittens, want %d", exploding, want)
			}
			if total := imploding + exploding; total != n-1 {
				t.Errorf("deck has %d kittens for %d players, want %d", total, n, n-1)
			}

			// It starts face down: the first player to find it gets a warning, not
			// a funeral.
			for _, c := range s.Draw {
				if c.Type == ImplodingKitten && c.FaceUp {
					t.Error("the Imploding Kitten was dealt face up")
				}
			}
			if s.ImplodingArmed() {
				t.Error("ImplodingArmed at the deal, want false")
			}
		})
	}
}

func TestSixPlayersNeedTheExpansion(t *testing.T) {
	var seats []Seat
	for i := 0; i < 6; i++ {
		seats = append(seats, Seat{ID: fmt.Sprintf("p%d", i), Name: "P"})
	}
	if _, err := NewGame(seats, Original, prng.New(uint64(1))); err != ErrPlayerCount {
		t.Errorf("six players on the original deck: err = %v, want ErrPlayerCount", err)
	}
	if _, err := NewGame(seats, Imploding, prng.New(uint64(1))); err != nil {
		t.Errorf("six players with the expansion: %v", err)
	}
}

func TestEverybodyStillGetsExactlyOneDefuse(t *testing.T) {
	var seats []Seat
	for i := 0; i < 6; i++ {
		seats = append(seats, Seat{ID: fmt.Sprintf("p%d", i), Name: "P"})
	}
	s, err := NewGame(seats, Imploding, prng.New(uint64(5)))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range s.Players {
		if n := countType(p.Hand, Defuse); n != 1 {
			t.Errorf("%s holds %d Defuses at the deal, want 1", p.ID, n)
		}
		if len(p.Hand) != handSize+1 {
			t.Errorf("%s holds %d cards, want %d", p.ID, len(p.Hand), handSize+1)
		}
	}
	// Six players, six printed Defuses: none should be left for the deck.
	if n := countType(s.Draw, Defuse); n != 0 {
		t.Errorf("%d Defuses left in a six-player deck, want 0", n)
	}
}

// ---------------------------------------------------------------------- Reverse

func TestReverseTurnsPlayAround(t *testing.T) {
	s := mkState([][]CardType{
		{Reverse}, {Skip}, {Skip},
	}, []CardType{Skip, Skip, Skip})
	s.Direction = 1
	s.Current = 0

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Reverse)}})

	if s.PlayDirection() != -1 {
		t.Errorf("direction = %d after a Reverse, want -1", s.PlayDirection())
	}
	// Play was 0 -> 1 -> 2; reversed it is 0 -> 2 -> 1, and Reverse ends the turn
	// without drawing.
	if got := s.CurrentID(); got != "p2" {
		t.Errorf("turn passed to %s, want p2", got)
	}
	if len(s.Draw) != 3 {
		t.Errorf("deck = %d cards, want 3 — Reverse must not draw", len(s.Draw))
	}
}

func TestReverseTwiceRestoresTheOrder(t *testing.T) {
	// p2 holds the second Reverse, because after the first one play has turned
	// around and p2 is who it reaches.
	s := mkState([][]CardType{
		{Reverse}, {Skip}, {Reverse},
	}, []CardType{Skip, Skip, Skip})
	s.Current = 0

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Reverse)}})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p2", CardIDs: []int{cardID(t, s.Players[2], Reverse)}})

	if s.PlayDirection() != 1 {
		t.Errorf("direction = %d after two Reverses, want 1", s.PlayDirection())
	}
	if got := s.CurrentID(); got != "p0" {
		t.Errorf("turn passed to %s, want p0", got)
	}
}

// Under Attack a Reverse spends one of the turns owed, exactly as a Skip does.
func TestReverseUnderAttackBurnsOneTurn(t *testing.T) {
	s := mkState([][]CardType{{Reverse}, {Skip}}, []CardType{Skip, Skip})
	s.Current = 0
	s.TurnsRemaining = 2

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], Reverse)}})

	if s.CurrentID() != "p0" {
		t.Errorf("turn = %s, want p0 still owing a turn", s.CurrentID())
	}
	if s.TurnsRemaining != 1 {
		t.Errorf("turnsRemaining = %d, want 1", s.TurnsRemaining)
	}
}

// ------------------------------------------------------------ Draw From the Bottom

func TestDrawFromBottomTakesTheBottomCard(t *testing.T) {
	s := mkState([][]CardType{{DrawFromBottom}, {Skip}}, []CardType{Skip, Attack, Favor})
	s.Current = 0
	bottom := s.Draw[len(s.Draw)-1].ID

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], DrawFromBottom)}})

	if len(s.Draw) != 2 {
		t.Fatalf("deck = %d cards, want 2", len(s.Draw))
	}
	if s.Draw[0].Type != Skip {
		t.Errorf("top of deck = %s, want the untouched Skip", s.Draw[0].Type)
	}
	var got bool
	for _, c := range s.Players[0].Hand {
		if c.ID == bottom {
			got = true
		}
	}
	if !got {
		t.Error("the bottom card did not reach the hand")
	}
	// It is still the turn-ending draw.
	if s.CurrentID() != "p1" {
		t.Errorf("turn = %s, want p1 — drawing ends the turn", s.CurrentID())
	}
}

func TestDrawFromBottomCanExplode(t *testing.T) {
	s := mkState([][]CardType{{DrawFromBottom}, {Skip}}, []CardType{Skip, Skip, ExplodingKitten})
	s.Current = 0

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], DrawFromBottom)}})

	if s.Players[0].Alive {
		t.Error("p0 drew the kitten off the bottom with no Defuse and survived")
	}
	if s.Phase != PhaseGameOver || s.WinnerID != "p1" {
		t.Errorf("phase = %s winner = %q, want game over won by p1", s.Phase, s.WinnerID)
	}
}

// --------------------------------------------------------------------- Feral Cat

func TestFeralCatStandsInForAnyCat(t *testing.T) {
	cases := []struct {
		name string
		set  []CardType
		ok   bool
	}{
		{"two matching cats", []CardType{CatTaco, CatTaco}, true},
		{"feral plus a cat", []CardType{FeralCat, CatTaco}, true},
		{"cat plus a feral", []CardType{CatMelon, FeralCat}, true},
		{"two ferals", []CardType{FeralCat, FeralCat}, true},
		{"feral and two matching", []CardType{FeralCat, CatBeard, CatBeard}, true},
		{"two ferals and a cat", []CardType{FeralCat, FeralCat, CatBeard}, true},
		{"two different cats", []CardType{CatTaco, CatMelon}, false},
		{"feral and two different cats", []CardType{FeralCat, CatTaco, CatMelon}, false},
		{"feral and a non-cat", []CardType{FeralCat, Skip}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mkState([][]CardType{c.set, {Skip}}, []CardType{Skip, Skip, Skip})
			s.Current = 0
			ids := make([]int, len(s.Players[0].Hand))
			for i, card := range s.Players[0].Hand {
				ids[i] = card.ID
			}

			a := Action{Kind: ActPlay, PlayerID: "p0", CardIDs: ids, TargetID: "p1"}
			if len(ids) == 3 {
				a.Named = "skip"
			}
			_, err := Apply(s, a)

			if c.ok && err != nil {
				t.Errorf("set was rejected: %v", err)
			}
			if !c.ok && err == nil {
				t.Error("an invalid set was accepted")
			}
		})
	}
}

// A Feral Cat is a card somebody can hold, so a Three of a Kind must be able to
// ask for one; the kittens are never in a hand, so it must not be able to ask for
// those.
func TestDemandableTypesFollowTheDeck(t *testing.T) {
	has := func(list []CardType, want CardType) bool {
		for _, t := range list {
			if t == want {
				return true
			}
		}
		return false
	}

	original := DemandableTypes(Original)
	expanded := DemandableTypes(Imploding)

	for _, ct := range []CardType{ExplodingKitten, ImplodingKitten} {
		if has(original, ct) || has(expanded, ct) {
			t.Errorf("%s is demandable, but is never in a hand", ct)
		}
	}
	for _, ct := range []CardType{FeralCat, Reverse, DrawFromBottom, AlterTheFuture, TargetedAttack} {
		if has(original, ct) {
			t.Errorf("%s is demandable without the expansion, but is not in that deck", ct)
		}
		if !has(expanded, ct) {
			t.Errorf("%s is in the expansion deck but cannot be demanded", ct)
		}
	}
}

// --------------------------------------------------------------- Targeted Attack

func TestTargetedAttackPointsTheTurnsAtSomebody(t *testing.T) {
	s := mkState([][]CardType{{TargetedAttack}, {Skip}, {Skip}}, []CardType{Skip, Skip, Skip})
	s.Current = 0

	mustApply(t, s, Action{
		Kind: ActPlay, PlayerID: "p0",
		CardIDs: []int{cardID(t, s.Players[0], TargetedAttack)}, TargetID: "p2",
	})

	// p1 would have been next; the card is for skipping over them.
	if got := s.CurrentID(); got != "p2" {
		t.Errorf("turn = %s, want p2", got)
	}
	if s.TurnsRemaining != 2 {
		t.Errorf("turnsRemaining = %d, want 2", s.TurnsRemaining)
	}
	if len(s.Draw) != 3 {
		t.Error("Targeted Attack must not draw")
	}
}

func TestTargetedAttackStacks(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {TargetedAttack}, {Skip}}, []CardType{Skip, Skip, Skip})
	s.Current = 1
	s.TurnsRemaining = 2 // p1 is already under attack

	mustApply(t, s, Action{
		Kind: ActPlay, PlayerID: "p1",
		CardIDs: []int{cardID(t, s.Players[1], TargetedAttack)}, TargetID: "p0",
	})

	if got := s.CurrentID(); got != "p0" {
		t.Errorf("turn = %s, want p0", got)
	}
	// One turn owed, passed on, plus two more.
	if s.TurnsRemaining != 3 {
		t.Errorf("turnsRemaining = %d, want 3", s.TurnsRemaining)
	}
}

func TestTargetedAttackNeedsSomebodyElse(t *testing.T) {
	s := mkState([][]CardType{{TargetedAttack}, {Skip}}, []CardType{Skip, Skip})
	s.Current = 0
	id := cardID(t, s.Players[0], TargetedAttack)

	if _, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{id}, TargetID: "p0"}); err != ErrBadTarget {
		t.Errorf("targeting yourself: err = %v, want ErrBadTarget", err)
	}
	if _, err := Apply(s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{id}}); err != ErrBadTarget {
		t.Errorf("targeting nobody: err = %v, want ErrBadTarget", err)
	}
}

// ------------------------------------------------------------ Alter the Future

func TestAlterTheFutureReordersTheTop(t *testing.T) {
	s := mkState([][]CardType{{AlterTheFuture}, {Skip}}, []CardType{Skip, Attack, Favor, Shuffle})
	s.Current = 0
	before := []int{s.Draw[0].ID, s.Draw[1].ID, s.Draw[2].ID}
	fourth := s.Draw[3].ID

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], AlterTheFuture)}})

	if s.Phase != PhaseAlter {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseAlter)
	}
	if s.AlteringID() != "p0" {
		t.Errorf("altering = %q, want p0", s.AlteringID())
	}
	if n := len(s.AlterFaces()); n != 3 {
		t.Fatalf("exposed %d cards, want 3", n)
	}

	// Put the third card on top and the first at the back of the three.
	mustApply(t, s, Action{Kind: ActAlterFuture, PlayerID: "p0", Order: []int{2, 1, 0}})

	if s.Phase != PhaseTurn {
		t.Errorf("phase = %s after altering, want %s", s.Phase, PhaseTurn)
	}
	want := []int{before[2], before[1], before[0]}
	for i, id := range want {
		if s.Draw[i].ID != id {
			t.Errorf("deck[%d] = %d, want %d", i, s.Draw[i].ID, id)
		}
	}
	if s.Draw[3].ID != fourth {
		t.Error("altering disturbed the fourth card")
	}
	// Looking at the future is not drawing, so the turn continues.
	if s.CurrentID() != "p0" {
		t.Errorf("turn = %s, want p0 — Alter the Future does not end a turn", s.CurrentID())
	}
}

func TestAlterTheFutureRejectsRubbishOrders(t *testing.T) {
	setup := func(t *testing.T) *State {
		t.Helper()
		s := mkState([][]CardType{{AlterTheFuture}, {Skip}}, []CardType{Skip, Attack, Favor})
		s.Current = 0
		mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], AlterTheFuture)}})
		return s
	}

	for name, order := range map[string][]int{
		"too short":      {0, 1},
		"too long":       {0, 1, 2, 0},
		"a repeat":       {0, 0, 1},
		"out of range":   {0, 1, 3},
		"negative":       {-1, 1, 2},
		"nothing at all": {},
	} {
		t.Run(name, func(t *testing.T) {
			s := setup(t)
			if _, err := Apply(s, Action{Kind: ActAlterFuture, PlayerID: "p0", Order: order}); err != ErrBadOrder {
				t.Errorf("order %v: err = %v, want ErrBadOrder", order, err)
			}
			if s.Phase != PhaseAlter {
				t.Errorf("a rejected order left the phase as %s", s.Phase)
			}
		})
	}
}

func TestOnlyTheAltererMayReorder(t *testing.T) {
	s := mkState([][]CardType{{AlterTheFuture}, {Skip}}, []CardType{Skip, Attack, Favor})
	s.Current = 0
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], AlterTheFuture)}})

	if _, err := Apply(s, Action{Kind: ActAlterFuture, PlayerID: "p1", Order: []int{0, 1, 2}}); err != ErrWrongPhase {
		t.Errorf("somebody else reordering: err = %v, want ErrWrongPhase", err)
	}
}

func TestAlterTheFutureCopesWithAShortDeck(t *testing.T) {
	s := mkState([][]CardType{{AlterTheFuture}, {Skip}}, []CardType{Skip, Attack})
	s.Current = 0
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardIDs: []int{cardID(t, s.Players[0], AlterTheFuture)}})

	if n := len(s.AlterFaces()); n != 2 {
		t.Fatalf("exposed %d cards from a two-card deck, want 2", n)
	}
	mustApply(t, s, Action{Kind: ActAlterFuture, PlayerID: "p0", Order: []int{1, 0}})
	if s.Phase != PhaseTurn {
		t.Errorf("phase = %s, want %s", s.Phase, PhaseTurn)
	}
}

// ------------------------------------------------------------ Imploding Kitten

// The first sighting is a warning: it goes back face up, no Defuse is spent, and
// the turn ends.
func TestImplodingKittenGoesBackFaceUpTheFirstTime(t *testing.T) {
	s := mkState([][]CardType{{Defuse, Skip}, {Skip}}, []CardType{ImplodingKitten, Skip, Skip})
	s.Current = 0

	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if !s.Players[0].Alive {
		t.Fatal("the first sighting of the Imploding Kitten killed somebody")
	}
	if s.Phase != PhaseDefuse {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseDefuse)
	}
	if countType(s.Players[0].Hand, Defuse) != 1 {
		t.Error("putting the Imploding Kitten back cost a Defuse; it should not")
	}
	if s.Placing() != "imploding" {
		t.Errorf("placing = %q, want imploding", s.Placing())
	}

	mustApply(t, s, Action{Kind: ActPlaceKitten, PlayerID: "p0", Index: 2})

	if !s.ImplodingArmed() {
		t.Error("the kitten is back in the deck but not armed")
	}
	if s.Draw[2].Type != ImplodingKitten || !s.Draw[2].FaceUp {
		t.Errorf("deck[2] = %s faceUp=%v, want a face-up Imploding Kitten",
			s.Draw[2].Type, s.Draw[2].FaceUp)
	}
	if s.CurrentID() != "p1" {
		t.Errorf("turn = %s, want p1 — the kitten was drawn, so the turn ends", s.CurrentID())
	}
}

// The second time it comes up, a Defuse is no help. That is the card's whole
// point, and getting it wrong would silently make the expansion pointless.
func TestArmedImplodingKittenIgnoresDefuse(t *testing.T) {
	s := mkState([][]CardType{{Defuse, Defuse, Skip}, {Skip}}, []CardType{ImplodingKitten, Skip})
	s.Current = 0
	s.Draw[0].FaceUp = true

	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if s.Players[0].Alive {
		t.Error("a Defuse saved a player from the armed Imploding Kitten")
	}
	if s.Phase != PhaseGameOver || s.WinnerID != "p1" {
		t.Errorf("phase = %s winner = %q, want game over won by p1", s.Phase, s.WinnerID)
	}
}

func TestArmedImplodingKittenReportsItself(t *testing.T) {
	s := mkState([][]CardType{{Skip}, {Skip}}, []CardType{Skip, ImplodingKitten})
	if s.ImplodingArmed() {
		t.Error("armed while the kitten is still face down")
	}
	s.Draw[1].FaceUp = true
	if !s.ImplodingArmed() {
		t.Error("not armed once the kitten is face up")
	}
}

// An Exploding Kitten still behaves exactly as it did, expansion or not.
func TestExplodingKittenStillDefusable(t *testing.T) {
	s := mkState([][]CardType{{Defuse, Skip}, {Skip}}, []CardType{ExplodingKitten, Skip})
	s.Current = 0

	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})

	if !s.Players[0].Alive {
		t.Fatal("a Defuse failed to stop an Exploding Kitten")
	}
	if s.Placing() != "exploding" {
		t.Errorf("placing = %q, want exploding", s.Placing())
	}
	if countType(s.Players[0].Hand, Defuse) != 0 {
		t.Error("the Defuse was not spent")
	}
}

// ------------------------------------------------------------------- full games

// A random driver over the expansion deck, checking the invariants that must hold
// on every path: cards are conserved, nobody dead ever holds the turn, and the
// game ends with exactly one survivor.
func TestRandomExpansionGamesStayConsistent(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := prng.New(uint64(seed))
		n := MinPlayers + int(seed)%(MaxPlayersFor(Imploding)-MinPlayers+1)

		var seats []Seat
		for i := 0; i < n; i++ {
			seats = append(seats, Seat{ID: fmt.Sprintf("p%d", i), Name: "P"})
		}
		s, err := NewGame(seats, Imploding, rng)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}

		// Unused kittens leave the game, so the count in play is the full deck
		// less the kittens that were never seeded.
		want := countCards(s)

		for step := 0; step < 1500 && s.Phase != PhaseGameOver; step++ {
			if _, err := Apply(s, expansionMove(s, rng)); err != nil {
				t.Fatalf("seed %d step %d: %v", seed, step, err)
			}
			if got := countCards(s); got != want {
				t.Fatalf("seed %d step %d: %d cards in play, want %d", seed, step, got, want)
			}
			if s.Phase != PhaseGameOver && !s.current().Alive {
				t.Fatalf("seed %d step %d: turn belongs to eliminated %s", seed, step, s.CurrentID())
			}
		}

		if s.Phase != PhaseGameOver {
			t.Fatalf("seed %d: no winner after 1500 steps", seed)
		}
		alive := 0
		for _, p := range s.Players {
			if p.Alive {
				alive++
			}
		}
		if alive != 1 {
			t.Fatalf("seed %d: %d survivors, want 1", seed, alive)
		}
	}
}

// countCards totals every card the game is holding. Deliberately does not count
// Pending.Cards: an open Nope window holds copies of cards that were already
// discarded when they were played, so adding them would double-count.
func countCards(s *State) int {
	n := len(s.Draw) + len(s.Discard)
	for _, p := range s.Players {
		n += len(p.Hand)
	}
	if s.PendingKitten != nil {
		n++
	}
	return n
}

// expansionMove picks a legal move for whatever the state is waiting on, biased
// towards actually playing the expansion's cards.
func expansionMove(s *State, rng *prng.Source) Action {
	switch s.Phase {
	case PhaseDefuse:
		return Action{Kind: ActPlaceKitten, PlayerID: s.CurrentID(), Index: rng.Intn(len(s.Draw) + 1)}
	case PhaseFavor:
		id := s.AwaitingGiftFrom()
		p := s.Find(id)
		return Action{Kind: ActGiveCard, PlayerID: id, CardIDs: []int{p.Hand[rng.Intn(len(p.Hand))].ID}}
	case PhaseAlter:
		k := len(s.AlterFaces())
		order := rng.Perm(k)
		return Action{Kind: ActAlterFuture, PlayerID: s.AlteringID(), Order: order}
	case PhaseNope:
		return Action{Kind: ActNopeExpired}
	}

	p := s.current()
	if rng.Intn(3) > 0 {
		if a, ok := expansionPlay(s, p, rng); ok {
			return a
		}
	}
	return Action{Kind: ActDraw, PlayerID: p.ID}
}

func expansionPlay(s *State, p *Player, rng *prng.Source) (Action, bool) {
	var other string
	for _, q := range s.Players {
		if q.Alive && q.ID != p.ID {
			other = q.ID
			break
		}
	}

	// A cat set first, Feral Cats included, since that is the combo rule the
	// expansion changes.
	groups := map[CardType][]int{}
	var ferals []int
	for _, c := range p.Hand {
		switch {
		case c.Type == FeralCat:
			ferals = append(ferals, c.ID)
		case c.Type.IsCat():
			groups[c.Type] = append(groups[c.Type], c.ID)
		}
	}
	for _, ids := range groups {
		set := append(append([]int(nil), ids...), ferals...)
		if len(set) >= 2 && other != "" {
			take := 2
			if len(set) >= 3 && rng.Intn(2) == 0 {
				take = 3
			}
			a := Action{Kind: ActPlay, PlayerID: p.ID, CardIDs: set[:take], TargetID: other}
			if take == 3 {
				a.Named = "defuse"
			}
			return a, true
		}
	}
	if len(ferals) >= 2 && other != "" {
		return Action{Kind: ActPlay, PlayerID: p.ID, CardIDs: ferals[:2], TargetID: other}, true
	}

	for _, c := range p.Hand {
		switch c.Type {
		case Skip, Attack, Shuffle, SeeTheFuture, Reverse, DrawFromBottom, AlterTheFuture:
			return Action{Kind: ActPlay, PlayerID: p.ID, CardIDs: []int{c.ID}}, true
		case Favor, TargetedAttack:
			if other != "" {
				return Action{Kind: ActPlay, PlayerID: p.ID, CardIDs: []int{c.ID}, TargetID: other}, true
			}
		}
	}
	return Action{}, false
}
