package game

import (
	"strings"
	"testing"

	"boardgame/kittens/internal/prng"
)

// The pack is fifty hand-typed cards in two languages. Most of what can go wrong
// with it is data rather than rules: a card with no effect, a missing
// translation, an amount with a zero too many. Those are all invisible until
// somebody draws it, which on a fifty-card deck can be a long way into a game.

func TestThePackIsFiftyCardsInTwoDecks(t *testing.T) {
	pack := Cards()
	if len(pack) != 50 {
		t.Fatalf("the pack has %d cards, want 50", len(pack))
	}
	counts := map[Deck]int{}
	for _, c := range pack {
		counts[c.Deck]++
	}
	if counts[DeckChance] != 25 || counts[DeckChest] != 25 {
		t.Errorf("%d chance and %d chest, want 25 each", counts[DeckChance], counts[DeckChest])
	}
}

// U+1000–U+109F is the Burmese block. Every card carries both languages, and a
// card that lost its translation would silently show English on a Burmese table.
func TestEveryCardIsWrittenInBothLanguages(t *testing.T) {
	hasBurmese := func(s string) bool {
		for _, r := range s {
			if r >= 0x1000 && r <= 0x109F {
				return true
			}
		}
		return false
	}

	for i, c := range Cards() {
		if strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.TitleMy) == "" {
			t.Errorf("card %d has no title: %q / %q", i, c.Title, c.TitleMy)
		}
		if strings.TrimSpace(c.Flavour) == "" || strings.TrimSpace(c.FlavourMy) == "" {
			t.Errorf("card %d (%s) has no flavour line", i, c.Title)
		}
		if c.Emoji == "" {
			t.Errorf("card %d (%s) has no emoji", i, c.Title)
		}
		if !hasBurmese(c.TitleMy) {
			// Two cards are titled in English on purpose — "Traffic Jam",
			// "Perfect Day" — because that is what people say. The flavour line
			// is where the Burmese has to be.
			if !hasBurmese(c.FlavourMy) {
				t.Errorf("card %d (%s) has no Burmese anywhere", i, c.Title)
			}
		}
	}
}

func TestEveryCardDoesSomething(t *testing.T) {
	for i, c := range Cards() {
		if len(c.Effects) == 0 {
			t.Errorf("card %d (%s) has no effects — it would draw and do nothing", i, c.Title)
			continue
		}
		for j, e := range c.Effects {
			switch e.Kind {
			case EffMoney:
				if e.Money == 0 {
					t.Errorf("%s effect %d moves no money", c.Title, j)
				}
			case EffMove:
				if e.Steps == 0 {
					t.Errorf("%s effect %d moves nowhere", c.Title, j)
				}
				if e.Steps > 6 || e.Steps < -6 {
					t.Errorf("%s moves %d squares — more than a throw, which reads as a bug", c.Title, e.Steps)
				}
			case EffSkip:
				if e.Turns < 1 || e.Turns > maxMissed {
					t.Errorf("%s misses %d turns; advance() only looks %d ahead", c.Title, e.Turns, maxMissed)
				}
			case EffJail, EffPardon, EffRelease:
			default:
				t.Errorf("%s effect %d is of unknown kind %q", c.Title, j, e.Kind)
			}
		}
	}
}

// The amounts were scaled up from the ideas by ten. What matters is that they
// stayed in a band that is felt without being fatal: a card should not cost more
// than a lap of the board pays, or one unlucky draw decides the game.
func TestCardAmountsAreInABandThatMatters(t *testing.T) {
	for _, c := range Cards() {
		for _, e := range c.Effects {
			if e.Kind != EffMoney {
				continue
			}
			amount := e.Money
			if amount < 0 {
				amount = -amount
			}
			if amount < 10*kyat {
				t.Errorf("%s moves %d, which is under a tenth of a percent of starting cash", c.Title, amount)
			}
			if amount > PassGo {
				t.Errorf("%s moves %d, more than a lap of the board pays (%d)", c.Title, amount, PassGo)
			}
		}
	}
}

// Exactly one card is held rather than resolved. More than one and the pardon
// bookkeeping — which returns "the" card to its deck — would put back the wrong
// one.
func TestExactlyOneCardIsHeld(t *testing.T) {
	held := 0
	for _, c := range Cards() {
		if c.Held() {
			held++
		}
	}
	if held != 1 {
		t.Errorf("%d cards are held in hand, want exactly 1", held)
	}
}

// ─────────────────────────────────────────────────────────── drawing

func TestBothDecksAreShuffledAtTheDeal(t *testing.T) {
	s := deal(t, 3)
	if len(s.ChanceDraw) != 25 || len(s.ChestDraw) != 25 {
		t.Fatalf("piles are %d and %d deep", len(s.ChanceDraw), len(s.ChestDraw))
	}
	if s.Drawn != -1 {
		t.Errorf("a card is face up before anyone has played: %d", s.Drawn)
	}

	// Two games on different seeds deal the piles in a different order, or the
	// shuffle is not happening.
	a, _ := NewGame(seats(2), prng.New(1))
	b, _ := NewGame(seats(2), prng.New(2))
	same := 0
	for i := range a.ChanceDraw {
		if a.ChanceDraw[i] == b.ChanceDraw[i] {
			same++
		}
	}
	if same == len(a.ChanceDraw) {
		t.Error("two seeds produced the same order — the deck is not shuffled")
	}
}

// Landing on a Chance square turns a card face up and stops. The stop is the
// point: a card that resolved instantly would leave the log as the only place
// anybody found out what it said.
func TestLandingOnChanceTurnsACardFaceUp(t *testing.T) {
	s := deal(t, 2)
	// Square 7 is the first Chance. Put a known non-pardon card on top so this
	// does not depend on the shuffle.
	s.ChanceDraw = append([]int{indexOfEffect(t, EffMove)}, s.ChanceDraw...)
	s.Players[0].Pos = 0

	events := moveBy(s, &s.Players[0], 7)

	if s.Phase != PhaseCard {
		t.Fatalf("phase is %s after landing on Chance, want a card to read", s.Phase)
	}
	if s.DrawnCard() == nil {
		t.Fatal("no card is face up")
	}
	if !hasKind(events, EvCard) {
		t.Errorf("drawing was not reported: %v", kinds(events))
	}
	// And it is the current player's to acknowledge, nobody else's.
	if _, err := Apply(s, Action{Kind: ActReadCard, PlayerID: "p1"}); err == nil {
		t.Error("somebody else read the card")
	}
	if _, err := Apply(s, Action{Kind: ActReadCard, PlayerID: "p0"}); err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	if s.Phase == PhaseCard {
		t.Error("the card is still face up after being read")
	}
	if s.Drawn != -1 {
		t.Error("the card was not cleared")
	}
}

func TestAReadCardGoesToTheDiscards(t *testing.T) {
	s := deal(t, 2)
	idx := indexOfEffect(t, EffMoney)
	s.ChanceDraw = append([]int{idx}, s.ChanceDraw...)
	s.Players[0].Pos = 0
	moveBy(s, &s.Players[0], 7)
	if _, err := Apply(s, Action{Kind: ActReadCard, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if len(s.ChanceDiscard) != 1 || s.ChanceDiscard[0] != idx {
		t.Errorf("discards are %v, want just card %d", s.ChanceDiscard, idx)
	}
}

// An empty pile is refilled from its own discards and reshuffled, the way a table
// does. Without this the fourth lap of a long game draws nothing.
func TestAnEmptyPileIsRefilledFromItsDiscards(t *testing.T) {
	s := deal(t, 2)
	s.ChanceDiscard = s.ChanceDraw
	s.ChanceDraw = nil

	got := s.drawFrom(DeckChance)
	if got < 0 {
		t.Fatal("an empty pile with a full discard drew nothing")
	}
	if len(s.ChanceDraw) != 24 {
		t.Errorf("the refilled pile is %d deep, want 24 after one draw", len(s.ChanceDraw))
	}
	if len(s.ChanceDiscard) != 0 {
		t.Errorf("the discards were not taken up: %d left", len(s.ChanceDiscard))
	}
}

// Every card in the pack has to be reachable. A shuffle that never surfaced the
// last card, or a refill that dropped it, would hide a card nobody could ever
// report as missing.
func TestEveryCardCanBeDrawn(t *testing.T) {
	for _, deck := range []Deck{DeckChance, DeckChest} {
		s := deal(t, 2)
		seen := map[int]bool{}
		// Three times round the deck, so the refill path is walked too.
		for i := 0; i < 90; i++ {
			idx := s.drawFrom(deck)
			if idx < 0 {
				t.Fatalf("%s ran dry at draw %d", deck, i)
			}
			seen[idx] = true
			s.discardTo(deck, idx)
		}
		if len(seen) != 25 {
			t.Errorf("%s: only %d of 25 cards were ever drawn", deck, len(seen))
		}
	}
}

// The pardon is kept, not read. It never reaches PhaseCard and it does not go to
// the discards until it is spent.
func TestThePardonIsKeptRatherThanRead(t *testing.T) {
	s := deal(t, 2)
	idx := indexOfHeld(t)
	s.ChanceDraw = append([]int{idx}, s.ChanceDraw...)
	s.Players[0].Pos = 0

	moveBy(s, &s.Players[0], 7)

	if s.Phase == PhaseCard {
		t.Error("the pardon stopped the table to be read")
	}
	if s.Players[0].Pardons != 1 {
		t.Errorf("holding %d pardons, want 1", s.Players[0].Pardons)
	}
	for _, d := range s.ChanceDiscard {
		if d == idx {
			t.Error("a held card went to the discards")
		}
	}
}

// ─────────────────────────────────────────────────────────── card effects

func TestACardCanWalkYouBackwards(t *testing.T) {
	s := deal(t, 2)
	s.Players[0].Pos = 5
	before := s.Players[0].Cash

	applyEffect(s, &s.Players[0], back(3))

	if s.Players[0].Pos != 2 {
		t.Errorf("walked back 3 from square 5 and ended on %d", s.Players[0].Pos)
	}
	if s.Players[0].Cash > before {
		t.Error("going backwards paid a salary")
	}
}

// Backwards past GO must not pay, and must not land on a negative square. Go's %
// keeps the sign of its left operand, so this is where that bites.
func TestWalkingBackwardsPastGoNeitherPaysNorBreaks(t *testing.T) {
	s := deal(t, 2)
	s.Players[0].Pos = 2
	before := s.Players[0].Cash

	applyEffect(s, &s.Players[0], back(5))

	if got := s.Players[0].Pos; got != 37 {
		t.Errorf("walked back 5 from square 2 and ended on %d, want 37", got)
	}
	if s.Players[0].Cash != before {
		t.Errorf("cash changed by %d going backwards past GO", s.Players[0].Cash-before)
	}
}

func TestACardCanCostYouATurn(t *testing.T) {
	s := deal(t, 3)
	applyEffect(s, &s.Players[0], miss(1))
	if s.Players[0].Missing != 1 {
		t.Fatalf("owing %d turns, want 1", s.Players[0].Missing)
	}

	// Round the table: p0 → p1 → p2 → and then p0's turn comes up owed, so it is
	// spent sitting out and play carries on to p1.
	s.Current = 0
	s.advance()
	if s.CurrentID() != "p1" {
		t.Fatalf("play went to %s, want p1", s.CurrentID())
	}
	s.advance()
	if s.CurrentID() != "p2" {
		t.Fatalf("play went to %s, want p2", s.CurrentID())
	}
	events := s.advance()
	if s.CurrentID() != "p1" {
		t.Errorf("play went to %s, want p1 — p0's turn should have been stepped over", s.CurrentID())
	}
	if s.Players[0].Missing != 0 {
		t.Errorf("p0 still owes %d turns", s.Players[0].Missing)
	}
	// And it is reported: a turn silently skipped reads as the table forgetting
	// whose go it is.
	if !hasKind(events, EvMissTurn) {
		t.Errorf("the skipped turn was not reported: %v", kinds(events))
	}
}

// The flat tyre costs money *and* a square. It is the only shape of card with two
// effects, and it was broken: paying used to end the turn, so the move never
// happened.
func TestACardWithTwoEffectsDoesBoth(t *testing.T) {
	s := deal(t, 2)
	idx := -1
	for i, c := range Cards() {
		if len(c.Effects) == 2 && c.Effects[0].Kind == EffMoney && c.Effects[1].Kind == EffMove {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("no pay-and-move card in the pack")
	}
	card := CardAt(idx)

	s.Players[0].Pos = 7
	s.Drawn = idx
	s.Phase = PhaseCard
	cashBefore := s.Players[0].Cash

	if _, err := Apply(s, Action{Kind: ActReadCard, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}

	wantCash := cashBefore + card.Effects[0].Money
	if s.Players[0].Cash != wantCash {
		t.Errorf("cash is %d, want %d — the payment did not happen", s.Players[0].Cash, wantCash)
	}
	wantPos := ((7+card.Effects[1].Steps)%BoardSize + BoardSize) % BoardSize
	if s.Players[0].Pos != wantPos {
		t.Errorf("ended on square %d, want %d — the move did not happen", s.Players[0].Pos, wantPos)
	}
}

// ─────────────────────────────────────────────────────────── jail

func TestGoToJailHoldsYouThere(t *testing.T) {
	s := deal(t, 2)
	moveBy(s, &s.Players[0], GoToJailTile)

	if !s.Players[0].Jailed {
		t.Error("landed on Go To Jail and was not held")
	}
	if s.Players[0].Pos != JailTile {
		t.Errorf("standing on %d, want the jail corner", s.Players[0].Pos)
	}
	// And when it comes round again, the choice is how to get out.
	s.Current = 0
	s.Phase = PhaseRoll
	s.advance()
	s.Current = 0
	if p := s.Find("p0"); p.Jailed {
		s.Phase = PhaseJail
	}
	if _, err := Apply(s, Action{Kind: ActBuy, PlayerID: "p0"}); err == nil {
		t.Error("bought something from inside jail")
	}
}

func TestJustVisitingIsNotBeingHeld(t *testing.T) {
	s := deal(t, 2)
	moveBy(s, &s.Players[0], JailTile) // landed on the corner the ordinary way
	if s.Players[0].Jailed {
		t.Error("landing on the jail corner held a visitor")
	}
}

func TestPayingTheFineLetsYouRollThisTurn(t *testing.T) {
	s := jailed(t)
	before := s.Players[0].Cash

	events, err := Apply(s, Action{Kind: ActPayFine, PlayerID: "p0"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Jailed {
		t.Error("paid the fine and is still being held")
	}
	if got, want := s.Players[0].Cash, before-JailFine; got != want {
		t.Errorf("cash is %d, want %d", got, want)
	}
	// The turn you just bought is yours to use. This was the bug: paying used to
	// end it.
	if s.Phase != PhaseRoll {
		t.Errorf("phase is %s after paying, want to be able to roll", s.Phase)
	}
	if s.CurrentID() != "p0" {
		t.Errorf("paying the fine passed the turn to %s", s.CurrentID())
	}
	if !hasKind(events, EvFine) {
		t.Errorf("the fine was not reported: %v", kinds(events))
	}
}

func TestYouCannotPayAFineYouCannotAfford(t *testing.T) {
	s := jailed(t)
	s.Players[0].Cash = JailFine - 1
	if _, err := Apply(s, Action{Kind: ActPayFine, PlayerID: "p0"}); err == nil {
		t.Error("paid a fine with too little money")
	}
	if !s.Players[0].Jailed {
		t.Error("a refused payment let them out anyway")
	}
}

func TestAPardonOpensTheDoorAndGoesBackToTheDeck(t *testing.T) {
	s := jailed(t)
	s.Players[0].Pardons = 1
	before := s.Players[0].Cash
	discards := len(s.ChanceDiscard) + len(s.ChestDiscard)

	if _, err := Apply(s, Action{Kind: ActUsePardon, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Jailed {
		t.Error("used a pardon and is still being held")
	}
	if s.Players[0].Cash != before {
		t.Error("a pardon cost money")
	}
	if s.Players[0].Pardons != 0 {
		t.Error("the pardon was not spent")
	}
	if got := len(s.ChanceDiscard) + len(s.ChestDiscard); got != discards+1 {
		t.Error("the spent pardon did not go back into a deck")
	}
}

func TestWithoutAPardonYouCannotUseOne(t *testing.T) {
	s := jailed(t)
	if _, err := Apply(s, Action{Kind: ActUsePardon, PlayerID: "p0"}); err == nil {
		t.Error("used a pardon nobody was holding")
	}
}

func TestThrowingADoubleGetsYouOut(t *testing.T) {
	s := jailed(t)
	s.RNG = doublesRoller(t)

	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Jailed {
		t.Error("threw a double and is still being held")
	}
	if s.Players[0].Pos == JailTile {
		t.Error("came out of jail and did not move")
	}
}

// Three failed attempts and the fine is taken anyway, which is what stops a bad
// run costing somebody the rest of the game.
func TestThreeFailedAttemptsPayTheFineAnyway(t *testing.T) {
	s := jailed(t)
	s.Players[0].Tries = JailAttempts - 1
	s.RNG = noDoublesRoller(t)
	before := s.Players[0].Cash

	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Jailed {
		t.Error("still being held after the last attempt")
	}
	if got, want := s.Players[0].Cash, before-JailFine; got > want {
		t.Errorf("cash is %d, want at most %d — the fine was not taken", got, want)
	}
}

func TestAFailedAttemptPassesTheTurn(t *testing.T) {
	s := jailed(t)
	s.RNG = noDoublesRoller(t)

	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if !s.Players[0].Jailed {
		t.Error("a failed attempt let them out")
	}
	if s.Players[0].Tries != 1 {
		t.Errorf("attempts is %d, want 1", s.Players[0].Tries)
	}
	if s.CurrentID() == "p0" {
		t.Error("a failed attempt did not pass the turn")
	}
}

// ─────────────────────────────────────────────────────────────────── helpers

// jailed is a two-player game with p0 being held and the table waiting on them.
func jailed(t *testing.T) *State {
	t.Helper()
	s := deal(t, 2)
	p := &s.Players[0]
	p.Jailed = true
	p.Pos = JailTile
	p.Tries = 0
	s.Current = 0
	s.Phase = PhaseJail
	return s
}

func indexOfEffect(t *testing.T, kind EffectKind) int {
	t.Helper()
	for i, c := range Cards() {
		if c.Deck == DeckChance && len(c.Effects) == 1 && c.Effects[0].Kind == kind {
			return i
		}
	}
	t.Fatalf("no single-effect %s card in the chance deck", kind)
	return -1
}

func indexOfHeld(t *testing.T) int {
	t.Helper()
	for i, c := range Cards() {
		if c.Held() {
			return i
		}
	}
	t.Fatal("no held card in the pack")
	return -1
}

// noDoublesRoller finds a seed whose first throw is not a double, rather than
// assuming one.
func noDoublesRoller(t *testing.T) *prng.Source {
	t.Helper()
	for seed := uint64(0); seed < 500; seed++ {
		r := prng.New(seed)
		if r.Intn(6) != r.Intn(6) {
			return prng.New(seed)
		}
	}
	t.Fatal("no seed in 500 threw anything but a double")
	return nil
}
