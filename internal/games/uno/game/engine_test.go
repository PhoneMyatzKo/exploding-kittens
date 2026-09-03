package game

import (
	"boardgame/kittens/internal/prng"
	"fmt"
	"testing"
)

// spec is a card written the way the tests talk about one: "red 7",
// "green skip", "wild-draw-four".
type spec struct {
	colour Colour
	rank   Rank
}

func c(colour Colour, rank Rank) spec { return spec{colour, rank} }

// mkState builds a fully deterministic table: hands in seat order, the draw pile
// top-first, and the given card face-up on the discard.
func mkState(hands [][]spec, draw []spec, top spec) *State {
	s := &State{Phase: PhaseTurn, Dir: 1, Round: 1, Colour: top.colour, RNG: prng.New(uint64(1))}
	id := 0
	next := func(sp spec) Card {
		card := newCard(id, sp.colour, sp.rank)
		id++
		return card
	}
	for i, h := range hands {
		p := &Player{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i)}
		for _, sp := range h {
			p.Hand = append(p.Hand, next(sp))
		}
		s.Players = append(s.Players, p)
		s.Seats = append(s.Seats, Seat{ID: p.ID, Name: p.Name})
	}
	for _, sp := range draw {
		s.Draw = append(s.Draw, next(sp))
	}
	s.Discard = []Card{next(top)}
	return s
}

// handID finds the first card in a hand matching the spec.
func handID(t *testing.T, p *Player, sp spec) int {
	t.Helper()
	for _, card := range p.Hand {
		if card.Colour == sp.colour && card.Rank == sp.rank {
			return card.ID
		}
	}
	t.Fatalf("player %s has no %s %s (hand: %v)", p.ID, sp.colour, sp.rank, namesOf(p.Hand))
	return 0
}

func namesOf(cards []Card) []string {
	out := make([]string, len(cards))
	for i, card := range cards {
		out[i] = card.Name
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

func wantErr(t *testing.T, s *State, a Action, want error) {
	t.Helper()
	if _, err := Apply(s, a); err != want {
		t.Fatalf("Apply(%s by %s): got %v, want %v", a.Kind, a.PlayerID, err, want)
	}
}

func has(events []Event, kind EventKind) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

// find returns the first event of a kind, for the cases that care about its
// payload rather than its presence.
func find(t *testing.T, events []Event, kind EventKind) Event {
	t.Helper()
	for _, e := range events {
		if e.Kind == kind {
			return e
		}
	}
	t.Fatalf("no %s event in %v", kind, kindsOf(events))
	return Event{}
}

func kindsOf(events []Event) []EventKind {
	out := make([]EventKind, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

// ------------------------------------------------------------------- the deck

func TestFullDeckComposition(t *testing.T) {
	deck := fullDeck()
	if len(deck) != DeckSize {
		t.Fatalf("deck has %d cards, want %d", len(deck), DeckSize)
	}

	byRank := map[Rank]int{}
	byColour := map[Colour]int{}
	ids := map[int]bool{}
	points := 0
	for _, card := range deck {
		if ids[card.ID] {
			t.Fatalf("duplicate card ID %d", card.ID)
		}
		ids[card.ID] = true
		byRank[card.Rank]++
		byColour[card.Colour]++
		points += card.Points()
	}

	for _, colour := range Colours() {
		if byColour[colour] != 25 {
			t.Errorf("%s has %d cards, want 25", colour, byColour[colour])
		}
	}
	if byColour[NoColour] != 8 {
		t.Errorf("%d colourless cards, want 8", byColour[NoColour])
	}
	if byRank[0] != 4 {
		t.Errorf("%d zeroes, want 4", byRank[0])
	}
	for n := 1; n <= 9; n++ {
		if byRank[Rank(n)] != 8 {
			t.Errorf("%d %ds, want 8", byRank[Rank(n)], n)
		}
	}
	for _, r := range []Rank{RankSkip, RankReverse, RankDrawTwo} {
		if byRank[r] != 8 {
			t.Errorf("%d %ss, want 8", byRank[r], r)
		}
	}
	for _, r := range []Rank{RankWild, RankWildDrawFour} {
		if byRank[r] != 4 {
			t.Errorf("%d %ss, want 4", byRank[r], r)
		}
	}
	// 1240 points are printed on the deck: 360 in numbers, 480 in coloured action
	// cards and 400 in wilds. Worth pinning down, because the 500 the game is
	// played to only means anything relative to it.
	if points != 1240 {
		t.Errorf("deck is worth %d points, want 1240", points)
	}
}

func TestCardNaming(t *testing.T) {
	cases := []struct {
		card       Card
		name       string
		colourSlug string
		rankSlug   string
	}{
		{newCard(0, Red, Rank(7)), "Red 7", "red", "7"},
		{newCard(0, Green, RankSkip), "Green Skip", "green", "skip"},
		{newCard(0, Blue, RankDrawTwo), "Blue Draw Two", "blue", "draw-two"},
		{newCard(0, NoColour, RankWild), "Wild", "wild", "wild"},
		{newCard(0, NoColour, RankWildDrawFour), "Wild Draw Four", "wild", "wild-draw-four"},
	}
	for _, tc := range cases {
		if tc.card.Name != tc.name {
			t.Errorf("name = %q, want %q", tc.card.Name, tc.name)
		}
		if tc.card.ColourSlug != tc.colourSlug || tc.card.RankSlug != tc.rankSlug {
			t.Errorf("%s slugs = %q/%q, want %q/%q",
				tc.name, tc.card.ColourSlug, tc.card.RankSlug, tc.colourSlug, tc.rankSlug)
		}
	}
}

// ---------------------------------------------------------------- what matches

func TestPlayable(t *testing.T) {
	s := mkState([][]spec{{c(Red, 5)}, {c(Red, 5)}}, nil, c(Red, Rank(7)))
	cases := []struct {
		card spec
		want bool
	}{
		{c(Red, Rank(3)), true},       // colour
		{c(Blue, Rank(7)), true},      // symbol
		{c(Blue, Rank(3)), false},     // neither
		{c(Red, RankSkip), true},      // colour, action card
		{c(Blue, RankSkip), false},    //
		{c(NoColour, RankWild), true}, // wilds go on anything
		{c(NoColour, RankWildDrawFour), true},
	}
	for _, tc := range cases {
		card := newCard(99, tc.card.colour, tc.card.rank)
		if got := s.playable(card); got != tc.want {
			t.Errorf("%s on Red 7: playable = %v, want %v", card.Name, got, tc.want)
		}
	}

	// On a wild, only the named colour counts — a second wild does not "match"
	// the first by symbol.
	s.Discard = append(s.Discard, newCard(98, NoColour, RankWild))
	s.Colour = Green
	if s.playable(newCard(97, Red, RankWild)) != true {
		t.Error("a wild should still be playable on a wild")
	}
	if s.playable(newCard(96, Red, Rank(3))) {
		t.Error("red should not play on a wild that named green")
	}
	if !s.playable(newCard(95, Green, Rank(3))) {
		t.Error("green should play on a wild that named green")
	}
}

func TestPlayRejectsUnmatchedCard(t *testing.T) {
	s := mkState([][]spec{{c(Blue, Rank(3))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	wantErr(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(Blue, Rank(3)))}, ErrNotPlayable)
	if len(s.Players[0].Hand) != 1 {
		t.Error("a refused play must leave the hand alone")
	}
}

func TestPlayRejectsOutOfTurnAndUnheldCards(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	wantErr(t, s, Action{Kind: ActPlay, PlayerID: "p1",
		CardID: handID(t, s.Players[1], c(Red, Rank(1)))}, ErrNotYourTurn)
	wantErr(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: 9999}, ErrNoSuchCard)
}

// ------------------------------------------------------------- symbol effects

func TestNumberCardPassesTheTurn(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3)), c(Blue, Rank(1))}, {c(Red, Rank(1))}, {c(Red, Rank(2))}},
		nil, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	if s.CurrentID() != "p1" {
		t.Errorf("turn is %s, want p1", s.CurrentID())
	}
	if s.Colour != Red || s.DiscardTop().Rank != 3 {
		t.Errorf("top is %s in %s, want Red 3", s.DiscardTop().Name, s.Colour)
	}
}

func TestSkipJumpsThePlayerAfter(t *testing.T) {
	s := mkState([][]spec{{c(Red, RankSkip), c(Blue, Rank(1))}, {c(Red, Rank(1))}, {c(Red, Rank(2))}},
		nil, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, RankSkip))})
	if got := find(t, ev, EvSkipped).ActorID; got != "p1" {
		t.Errorf("skipped %s, want p1", got)
	}
	if s.CurrentID() != "p2" {
		t.Errorf("turn is %s, want p2", s.CurrentID())
	}
}

func TestReverseTurnsThePlayAround(t *testing.T) {
	s := mkState([][]spec{{c(Red, RankReverse), c(Blue, Rank(1))}, {c(Red, Rank(1))}, {c(Red, Rank(2))}},
		nil, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, RankReverse))})
	if !has(ev, EvReversed) {
		t.Fatalf("no reverse event: %v", kindsOf(ev))
	}
	if s.Dir != -1 {
		t.Errorf("direction is %d, want -1", s.Dir)
	}
	if s.CurrentID() != "p2" {
		t.Errorf("turn is %s, want p2 (the other way round)", s.CurrentID())
	}
}

// With two players there is nobody to reverse past, so the rules make it a Skip.
func TestReverseIsASkipWithTwoPlayers(t *testing.T) {
	s := mkState([][]spec{{c(Red, RankReverse), c(Blue, Rank(1))}, {c(Red, Rank(1))}},
		nil, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, RankReverse))})
	if !has(ev, EvSkipped) {
		t.Fatalf("two-player reverse should skip: %v", kindsOf(ev))
	}
	if s.CurrentID() != "p0" {
		t.Errorf("turn is %s, want p0 again", s.CurrentID())
	}
}

func TestDrawTwoHitsTheNextPlayerAndSkipsThem(t *testing.T) {
	s := mkState([][]spec{{c(Red, RankDrawTwo), c(Blue, Rank(1))}, {c(Red, Rank(1))}, {c(Red, Rank(2))}},
		[]spec{c(Green, Rank(4)), c(Blue, Rank(9))}, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, RankDrawTwo))})

	if n := len(s.Players[1].Hand); n != 3 {
		t.Errorf("p1 holds %d cards, want 3", n)
	}
	if s.CurrentID() != "p2" {
		t.Errorf("turn is %s, want p2", s.CurrentID())
	}
	// The count is public, the cards are not.
	drew := find(t, ev, EvDrew)
	if drew.Count != 2 || len(drew.Cards) != 0 {
		t.Errorf("public draw event = %d cards, %v, want count 2 and no cards", drew.Count, namesOf(drew.Cards))
	}
	var private []Event
	for _, e := range ev {
		if e.Kind == EvDrew && e.OnlyFor != "" {
			private = append(private, e)
		}
	}
	if len(private) != 1 || private[0].OnlyFor != "p1" || len(private[0].Cards) != 2 {
		t.Errorf("private draw event = %v, want two cards for p1 only", private)
	}
}

// ---------------------------------------------------------------------- wilds

func TestWildNamesAColour(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWild), c(Blue, Rank(1))}, {c(Red, Rank(1))}},
		nil, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWild)), Colour: "green"})
	if s.Colour != Green {
		t.Errorf("colour is %s, want Green", s.Colour)
	}
	if find(t, ev, EvColour).Colour != "green" {
		t.Error("the colour event should carry the chosen colour")
	}
	if s.CurrentID() != "p1" {
		t.Errorf("turn is %s, want p1", s.CurrentID())
	}
}

// A client may lay the card down first and pick the colour from a picker
// afterwards, which is what the extra phase is for.
func TestWildWithoutAColourWaitsForOne(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWild), c(Blue, Rank(1))}, {c(Red, Rank(1))}},
		nil, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWild))})
	if s.Phase != PhaseColour || s.MustName() != "p0" {
		t.Fatalf("phase %s, waiting on %q; want colour/p0", s.Phase, s.MustName())
	}
	// Nothing is in force meanwhile, so nobody can sneak a card in.
	if s.ActiveColour() != "" {
		t.Errorf("active colour is %q, want none", s.ActiveColour())
	}
	wantErr(t, s, Action{Kind: ActColour, PlayerID: "p1", Colour: "green"}, ErrNotYourTurn)
	wantErr(t, s, Action{Kind: ActColour, PlayerID: "p0", Colour: "purple"}, ErrNeedColour)

	mustApply(t, s, Action{Kind: ActColour, PlayerID: "p0", Colour: "blue"})
	if s.Colour != Blue || s.CurrentID() != "p1" {
		t.Errorf("after naming: colour %s, turn %s; want Blue/p1", s.Colour, s.CurrentID())
	}
}

func TestColourOnANumberCardIsRefused(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	wantErr(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(Red, Rank(3))), Colour: "green"}, ErrNoColourYet)
}

func TestAcceptedWildDrawFour(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWildDrawFour), c(Blue, Rank(1))}, {c(Red, Rank(1))}, {c(Red, Rank(2))}},
		[]spec{c(Green, 1), c(Green, 2), c(Green, 3), c(Green, 4)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWildDrawFour)), Colour: "green"})

	target, actor := s.Challenged()
	if target != "p1" || actor != "p0" {
		t.Fatalf("challenge is %q vs %q, want p1 vs p0", target, actor)
	}
	wantErr(t, s, Action{Kind: ActAcceptDraw, PlayerID: "p2"}, ErrNotYourTurn)

	mustApply(t, s, Action{Kind: ActAcceptDraw, PlayerID: "p1"})
	if n := len(s.Players[1].Hand); n != 5 {
		t.Errorf("p1 holds %d cards, want 5", n)
	}
	if s.CurrentID() != "p2" {
		t.Errorf("turn is %s, want p2 (p1 is skipped)", s.CurrentID())
	}
}

// Challenging a bluff: the player who tried it draws the four instead, and the
// challenger's turn goes ahead.
func TestChallengedBluffBackfires(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWildDrawFour), c(Red, Rank(1))}, {c(Blue, Rank(1))}, {c(Blue, Rank(2))}},
		[]spec{c(Green, 1), c(Green, 2), c(Green, 3), c(Green, 4)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWildDrawFour)), Colour: "green"})

	ev := mustApply(t, s, Action{Kind: ActChallenge, PlayerID: "p1"})
	if !has(ev, EvBluffed) {
		t.Fatalf("holding a red, p0 was bluffing: %v", kindsOf(ev))
	}
	if n := len(s.Players[0].Hand); n != 5 {
		t.Errorf("p0 holds %d cards, want 5", n)
	}
	if n := len(s.Players[1].Hand); n != 1 {
		t.Errorf("p1 holds %d cards, want 1 — the challenger draws nothing", n)
	}
	if s.CurrentID() != "p1" {
		t.Errorf("turn is %s, want p1 — a won challenge keeps your turn", s.CurrentID())
	}
	// The hand is shown to the challenger and to nobody else.
	shown := find(t, ev, EvRevealed)
	if shown.OnlyFor != "p1" || len(shown.Cards) == 0 {
		t.Errorf("revealed hand = %v for %q, want p1 only", namesOf(shown.Cards), shown.OnlyFor)
	}
}

func TestFailedChallengeCostsSix(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWildDrawFour), c(Blue, Rank(1))}, {c(Blue, Rank(1))}, {c(Blue, Rank(2))}},
		[]spec{c(Green, 1), c(Green, 2), c(Green, 3), c(Green, 4), c(Green, 5), c(Green, 6)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWildDrawFour)), Colour: "green"})

	ev := mustApply(t, s, Action{Kind: ActChallenge, PlayerID: "p1"})
	if !has(ev, EvBluffFailed) {
		t.Fatalf("p0 held no red, so the play was honest: %v", kindsOf(ev))
	}
	if n := len(s.Players[1].Hand); n != 7 {
		t.Errorf("p1 holds %d cards, want 7 (1 + 6)", n)
	}
	if s.CurrentID() != "p2" {
		t.Errorf("turn is %s, want p2 — a lost challenge still loses the turn", s.CurrentID())
	}
}

// A wild does not make the holder immune: holding one is not holding the colour.
func TestHoldingOnlyWildsIsAnHonestDrawFour(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWildDrawFour), c(NoColour, RankWild)}, {c(Blue, Rank(1))}},
		[]spec{c(Green, 1), c(Green, 2), c(Green, 3), c(Green, 4), c(Green, 5), c(Green, 6)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWildDrawFour)), Colour: "green"})
	if ev := mustApply(t, s, Action{Kind: ActChallenge, PlayerID: "p1"}); !has(ev, EvBluffFailed) {
		t.Fatalf("a hand of wilds is an honest draw four: %v", kindsOf(ev))
	}
}

// ------------------------------------------------------------------- drawing

func TestDrawingAPlayableCardLeavesTheTurnOpen(t *testing.T) {
	s := mkState([][]spec{{c(Blue, Rank(3))}, {c(Red, Rank(1))}},
		[]spec{c(Red, Rank(5))}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	if s.Phase != PhaseDrawn {
		t.Fatalf("phase is %s, want drawn", s.Phase)
	}
	if s.CurrentID() != "p0" {
		t.Errorf("turn is %s, want p0", s.CurrentID())
	}
	// Only the drawn card may be played, not the rest of the hand.
	wantErr(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(Blue, Rank(3)))}, ErrCantPlayDrawn)

	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: s.Drawn.ID})
	if s.CurrentID() != "p1" || s.DiscardTop().Rank != 5 {
		t.Errorf("after playing the draw: turn %s, top %s", s.CurrentID(), s.DiscardTop().Name)
	}
}

func TestDrawingAnUnplayableCardEndsTheTurn(t *testing.T) {
	s := mkState([][]spec{{c(Blue, Rank(3))}, {c(Red, Rank(1))}},
		[]spec{c(Green, Rank(5))}, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	if !has(ev, EvPassed) {
		t.Fatalf("an unusable draw ends the turn: %v", kindsOf(ev))
	}
	if s.Phase != PhaseTurn || s.CurrentID() != "p1" {
		t.Errorf("phase %s, turn %s; want turn/p1", s.Phase, s.CurrentID())
	}
}

func TestPassingOnADrawnCard(t *testing.T) {
	s := mkState([][]spec{{c(Blue, Rank(3))}, {c(Red, Rank(1))}},
		[]spec{c(Red, Rank(5))}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	mustApply(t, s, Action{Kind: ActPass, PlayerID: "p0"})
	if s.CurrentID() != "p1" || len(s.Players[0].Hand) != 2 {
		t.Errorf("turn %s, hand %v; want p1 and both cards kept", s.CurrentID(), namesOf(s.Players[0].Hand))
	}
}

func TestEmptyDeckIsRefilledFromTheDiscard(t *testing.T) {
	s := mkState([][]spec{{c(Blue, Rank(3))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	// A pile of played cards under the face-up top.
	s.Discard = append([]Card{
		newCard(500, Green, Rank(4)), newCard(501, Yellow, Rank(8)), newCard(502, Blue, Rank(2)),
	}, s.Discard...)

	ev := mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	if !has(ev, EvReshuffled) {
		t.Fatalf("the discard should have become the deck: %v", kindsOf(ev))
	}
	if len(s.Discard) != 1 || s.DiscardTop().Rank != 7 {
		t.Errorf("discard is %v, want just the face-up Red 7", namesOf(s.Discard))
	}
	if len(s.Draw) != 2 {
		t.Errorf("deck has %d cards, want 2 (3 recycled, 1 drawn)", len(s.Draw))
	}
}

// Nothing left anywhere. Rare to the point of theoretical, but it must pass the
// turn rather than wedge the table.
func TestDrawingWithNothingLeftPassesTheTurn(t *testing.T) {
	s := mkState([][]spec{{c(Blue, Rank(3))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActDraw, PlayerID: "p0"})
	if s.CurrentID() != "p1" || len(s.Players[0].Hand) != 1 {
		t.Errorf("turn %s, hand size %d; want p1 and an unchanged hand", s.CurrentID(), len(s.Players[0].Hand))
	}
}

// ---------------------------------------------------------------------- "UNO!"

func TestSayingUnoWithThePlay(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3)), c(Blue, Rank(1))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(Red, Rank(3))), SayUno: true})
	if !has(ev, EvUnoCalled) {
		t.Fatalf("no uno call: %v", kindsOf(ev))
	}
	if _, called, open := s.UnoWindow(); !open || !called {
		t.Errorf("window open=%v called=%v, want both true", open, called)
	}
	if s.CanCatch("p1") {
		t.Error("p1 should have nothing to catch")
	}
	wantErr(t, s, Action{Kind: ActCatchUno, PlayerID: "p1"}, ErrAlreadyCalled)
}

func TestCatchingASilentPlayerCostsThemTwo(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3)), c(Blue, Rank(1))}, {c(Red, Rank(1))}},
		[]spec{c(Green, 1), c(Green, 2)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	if !s.CanCatch("p1") {
		t.Fatal("p0 said nothing on one card; p1 should be able to catch them")
	}
	// You cannot catch yourself out of the penalty.
	wantErr(t, s, Action{Kind: ActCatchUno, PlayerID: "p0"}, ErrNoCatch)

	before := s.CurrentID()
	ev := mustApply(t, s, Action{Kind: ActCatchUno, PlayerID: "p1"})
	if !has(ev, EvUnoCaught) {
		t.Fatalf("no catch event: %v", kindsOf(ev))
	}
	if n := len(s.Players[0].Hand); n != 3 {
		t.Errorf("p0 holds %d cards, want 3", n)
	}
	if s.CurrentID() != before {
		t.Error("a penalty is not a move: the turn must not change")
	}
	if _, _, open := s.UnoWindow(); open {
		t.Error("the window should be shut once it has been used")
	}
}

func TestCallingUnoLateButInTime(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3)), c(Blue, Rank(1))}, {c(Red, Rank(1))}},
		[]spec{c(Green, 1)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	mustApply(t, s, Action{Kind: ActCallUno, PlayerID: "p0"})
	wantErr(t, s, Action{Kind: ActCallUno, PlayerID: "p0"}, ErrAlreadyCalled)
	wantErr(t, s, Action{Kind: ActCatchUno, PlayerID: "p1"}, ErrAlreadyCalled)
}

// The window shuts once the next player has had their turn, so a catch three
// turns later is not a catch.
func TestTheCatchWindowExpires(t *testing.T) {
	// p1 keeps three cards on purpose: going down to one themselves would open a
	// window of their own and hide the one under test.
	s := mkState([][]spec{
		{c(Red, Rank(3)), c(Blue, Rank(1))},
		{c(Red, Rank(1)), c(Blue, Rank(2)), c(Blue, Rank(4))},
		{c(Red, Rank(2)), c(Blue, Rank(5))},
	}, []spec{c(Green, 1), c(Green, 2)}, c(Red, Rank(7)))
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p1", CardID: handID(t, s.Players[1], c(Red, Rank(1)))})
	if s.CanCatch("p2") {
		t.Error("the window should have closed when p1 finished their turn")
	}
	wantErr(t, s, Action{Kind: ActCatchUno, PlayerID: "p2"}, ErrNoCatch)
}

func TestCallingUnoOnAFullHandIsRefused(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3)), c(Blue, Rank(1))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	wantErr(t, s, Action{Kind: ActCallUno, PlayerID: "p0"}, ErrNothingToCall)
}

// ------------------------------------------------------------ ending a round

func TestGoingOutScoresEveryOtherHand(t *testing.T) {
	s := mkState([][]spec{
		{c(Red, Rank(3))},
		{c(Blue, Rank(9)), c(Green, RankSkip)},      // 9 + 20
		{c(NoColour, RankWild), c(Yellow, Rank(0))}, // 50 + 0
	}, nil, c(Red, Rank(7)))
	s.Target = 500

	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(Red, Rank(3))), SayUno: false})
	round := find(t, ev, EvRoundOver)
	if round.ActorID != "p0" || round.Points != 79 {
		t.Errorf("round over: %s scored %d, want p0 with 79", round.ActorID, round.Points)
	}
	if s.Phase != PhaseRoundOver {
		t.Errorf("phase is %s, want round_over", s.Phase)
	}
	if s.Players[0].Score != 79 {
		t.Errorf("p0's total is %d, want 79", s.Players[0].Score)
	}
}

func TestReachingTheTargetEndsTheGame(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3))}, {c(Blue, Rank(9))}}, nil, c(Red, Rank(7)))
	s.Target = 5
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	if !has(ev, EvGameOver) || s.Phase != PhaseGameOver || s.WinnerID != "p0" {
		t.Fatalf("phase %s winner %q events %v", s.Phase, s.WinnerID, kindsOf(ev))
	}
	wantErr(t, s, Action{Kind: ActDraw, PlayerID: "p1"}, ErrGameOver)
}

// A target of zero is a one-round game, which is how a table with ten minutes
// plays it.
func TestSingleRoundGame(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3))}, {c(Blue, Rank(9))}}, nil, c(Red, Rank(7)))
	s.Target = 0
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	if s.Phase != PhaseGameOver {
		t.Errorf("phase is %s, want game_over", s.Phase)
	}
}

// Going out on a Draw Two still makes the next player draw: those cards count
// towards the score.
func TestGoingOutOnADrawTwoStillDeals(t *testing.T) {
	s := mkState([][]spec{{c(Red, RankDrawTwo)}, {c(Blue, Rank(9))}},
		[]spec{c(Green, RankSkip), c(Green, Rank(1))}, c(Red, Rank(7)))
	s.Target = 500
	ev := mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, RankDrawTwo))})
	if n := len(s.Players[1].Hand); n != 3 {
		t.Fatalf("p1 holds %d cards, want 3", n)
	}
	// 9 + 20 + 1
	if got := find(t, ev, EvRoundOver).Points; got != 30 {
		t.Errorf("scored %d, want 30 — the drawn cards count", got)
	}
}

func TestGoingOutOnAWildDrawFourSkipsTheChallenge(t *testing.T) {
	s := mkState([][]spec{{c(NoColour, RankWildDrawFour)}, {c(Blue, Rank(9))}},
		[]spec{c(Green, 1), c(Green, 2), c(Green, 3), c(Green, 4)}, c(Red, Rank(7)))
	s.Target = 500
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0",
		CardID: handID(t, s.Players[0], c(NoColour, RankWildDrawFour))})
	if s.Phase != PhaseRoundOver {
		t.Fatalf("phase is %s, want round_over — there is nothing left to challenge", s.Phase)
	}
	if n := len(s.Players[1].Hand); n != 5 {
		t.Errorf("p1 holds %d cards, want 5", n)
	}
}

func TestNextRoundKeepsScoresAndDealsAfresh(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3))}, {c(Blue, Rank(9))}}, nil, c(Red, Rank(7)))
	s.Target = 500
	mustApply(t, s, Action{Kind: ActPlay, PlayerID: "p0", CardID: handID(t, s.Players[0], c(Red, Rank(3)))})
	wantErr(t, s, Action{Kind: ActDraw, PlayerID: "p0"}, ErrWrongPhase)

	ev := mustApply(t, s, Action{Kind: ActNextRound, PlayerID: "p0"})
	if !has(ev, EvStarted) {
		t.Fatalf("no fresh deal: %v", kindsOf(ev))
	}
	if s.Players[0].Score != 9 {
		t.Errorf("p0's score is %d, want 9 — scores survive the round", s.Players[0].Score)
	}
	for _, p := range s.Players {
		if len(p.Hand) != handSize {
			t.Errorf("%s was dealt %d cards, want %d", p.ID, len(p.Hand), handSize)
		}
	}
	if s.Round != 2 {
		t.Errorf("round is %d, want 2", s.Round)
	}
	if s.RoundWinnerID != "" {
		t.Errorf("round winner is %q, want cleared", s.RoundWinnerID)
	}
	wantErr(t, s, Action{Kind: ActNextRound, PlayerID: "p0"}, ErrRoundLive)
}

// ---------------------------------------------------------------------- setup

func TestNewGamePlayerCount(t *testing.T) {
	one := []Seat{{ID: "p0"}}
	if _, _, err := NewGame(one, prng.New(uint64(1))); err != ErrPlayerCount {
		t.Errorf("one player: got %v, want ErrPlayerCount", err)
	}
	eleven := make([]Seat, 11)
	if _, _, err := NewGame(eleven, prng.New(uint64(1))); err != ErrPlayerCount {
		t.Errorf("eleven players: got %v, want ErrPlayerCount", err)
	}
}

// Whatever the seed, a fresh game is a legal one: seven cards each, every card
// accounted for, a face-up starter that is never a Wild Draw Four, and somebody
// able to act.
func TestNewGameIsAlwaysLegal(t *testing.T) {
	for seed := int64(0); seed < 300; seed++ {
		for n := MinPlayers; n <= MaxPlayers; n++ {
			seats := make([]Seat, n)
			for i := range seats {
				seats[i] = Seat{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i)}
			}
			s, ev, err := NewGame(seats, prng.New(uint64(seed)))
			if err != nil {
				t.Fatalf("seed %d, %d players: %v", seed, n, err)
			}

			total := len(s.Draw) + len(s.Discard)
			for _, p := range s.Players {
				total += len(p.Hand)
			}
			if total != DeckSize {
				t.Fatalf("seed %d, %d players: %d cards in play, want %d", seed, n, total, DeckSize)
			}
			if top := s.DiscardTop(); top == nil || top.Rank == RankWildDrawFour {
				t.Fatalf("seed %d: illegal starting card %v", seed, top)
			}
			if !has(ev, EvStarted) {
				t.Fatalf("seed %d: no start event", seed)
			}
			switch s.Phase {
			case PhaseTurn:
				if s.Colour == NoColour {
					t.Fatalf("seed %d: no colour in force on a live turn", seed)
				}
			case PhaseColour:
				// Dealt on a Wild: the first player names the colour and keeps
				// their turn.
				if s.MustName() != s.CurrentID() {
					t.Fatalf("seed %d: %q must name a colour but %q has the turn",
						seed, s.MustName(), s.CurrentID())
				}
			default:
				t.Fatalf("seed %d: fresh game is in phase %s", seed, s.Phase)
			}
		}
	}
}

func TestOpeningCardEffects(t *testing.T) {
	// The dealt hands are irrelevant here; only the flipped card matters, so it
	// is planted directly and the opening effect applied on its own.
	mk := func(starter spec, players int) *State {
		hands := make([][]spec, players)
		for i := range hands {
			hands[i] = []spec{c(Yellow, Rank(1))}
		}
		s := mkState(hands, []spec{c(Green, 1), c(Green, 2)}, starter)
		s.Colour = starter.colour
		return s
	}

	t.Run("skip", func(t *testing.T) {
		s := mk(c(Red, RankSkip), 3)
		s.openingEffect(*s.DiscardTop())
		if s.CurrentID() != "p1" {
			t.Errorf("turn is %s, want p1", s.CurrentID())
		}
	})
	t.Run("reverse", func(t *testing.T) {
		s := mk(c(Red, RankReverse), 3)
		s.openingEffect(*s.DiscardTop())
		if s.Dir != -1 || s.CurrentID() != "p0" {
			t.Errorf("dir %d, turn %s; want -1 and p0", s.Dir, s.CurrentID())
		}
	})
	t.Run("draw two", func(t *testing.T) {
		s := mk(c(Red, RankDrawTwo), 3)
		s.openingEffect(*s.DiscardTop())
		if n := len(s.Players[0].Hand); n != 3 {
			t.Errorf("p0 holds %d cards, want 3", n)
		}
		if s.CurrentID() != "p1" {
			t.Errorf("turn is %s, want p1", s.CurrentID())
		}
	})
	t.Run("wild", func(t *testing.T) {
		s := mk(c(NoColour, RankWild), 3)
		s.openingEffect(*s.DiscardTop())
		if s.Phase != PhaseColour || s.MustName() != "p0" {
			t.Fatalf("phase %s, naming %q; want colour/p0", s.Phase, s.MustName())
		}
		mustApply(t, s, Action{Kind: ActColour, PlayerID: "p0", Colour: "blue"})
		if s.CurrentID() != "p0" {
			t.Errorf("turn is %s, want p0 — naming the opening colour is not a turn", s.CurrentID())
		}
		if s.Colour != Blue {
			t.Errorf("colour is %s, want Blue", s.Colour)
		}
	})
}

// ------------------------------------------------------------------ the view

func TestPlayableIDsMatchWhatPlayWillAccept(t *testing.T) {
	s := mkState([][]spec{
		{c(Red, Rank(3)), c(Blue, Rank(7)), c(Blue, Rank(2)), c(NoColour, RankWild)},
		{c(Red, Rank(1))},
	}, nil, c(Red, Rank(7)))

	ids := s.PlayableIDs("p0")
	if len(ids) != 3 {
		t.Errorf("playable = %v, want three cards (Red 3, Blue 7, Wild)", ids)
	}
	if got := s.PlayableIDs("p1"); got != nil {
		t.Errorf("p1 is not on turn but has %v playable", got)
	}
	// Every card the accessor allows must actually be accepted, and vice versa.
	for _, card := range s.Players[0].Hand {
		want := s.Playable("p0", card.ID)
		probe := *s
		probe.Players = []*Player{{ID: "p0", Hand: append([]Card(nil), s.Players[0].Hand...)}, {ID: "p1"}}
		probe.Discard = append([]Card(nil), s.Discard...)
		_, err := Apply(&probe, Action{Kind: ActPlay, PlayerID: "p0", CardID: card.ID, Colour: colourFor(card)})
		if got := err == nil; got != want {
			t.Errorf("%s: accessor says %v, engine says %v (%v)", card.Name, want, got, err)
		}
	}
}

func colourFor(card Card) string {
	if card.Rank.IsWild() {
		return "green"
	}
	return ""
}

func TestHandSizesAndScores(t *testing.T) {
	s := mkState([][]spec{{c(Red, Rank(3)), c(Blue, Rank(1))}, {c(Red, Rank(1))}}, nil, c(Red, Rank(7)))
	s.Players[1].Score = 42
	if got := s.HandSizes(); got["p0"] != 2 || got["p1"] != 1 {
		t.Errorf("hand sizes = %v", got)
	}
	if got := s.Scores(); got["p1"] != 42 {
		t.Errorf("scores = %v", got)
	}
}
