package game

import (
	"boardgame/kittens/internal/prng"
	"testing"
)

// The board is data, and data typed by hand is data with mistakes in it. These
// first tests are about the table rather than the rules: the original board's
// shape is what makes the game work, so a square in the wrong place or a price
// off by a zero is worth catching here rather than in a playtest.

func TestBoardHasTheOriginalShape(t *testing.T) {
	b := Board()
	if len(b) != 40 {
		t.Fatalf("the board has %d squares, want 40", len(b))
	}

	counts := map[TileKind]int{}
	for _, tile := range b {
		counts[tile.Kind]++
	}
	want := map[TileKind]int{
		TileProperty: 22, TileStation: 4, TileUtility: 2,
		TileChance: 3, TileChest: 3, TileTax: 2,
		TileGo: 1, TileJail: 1, TileParking: 1, TileGoToJail: 1,
	}
	for kind, n := range want {
		if counts[kind] != n {
			t.Errorf("%d %s squares, want %d", counts[kind], kind, n)
		}
	}

	// The corners are the four squares everything else is positioned relative to.
	for pos, kind := range map[int]TileKind{
		0: TileGo, JailTile: TileJail, 20: TileParking, GoToJailTile: TileGoToJail,
	} {
		if got := TileAt(pos).Kind; got != kind {
			t.Errorf("square %d is %s, want %s", pos, got, kind)
		}
	}
}

func TestEveryPropertyIsPricedAndNamed(t *testing.T) {
	for i, tile := range Board() {
		if tile.Name == "" || tile.NameMy == "" {
			t.Errorf("square %d (%s) is missing a name: %q / %q", i, tile.Kind, tile.Name, tile.NameMy)
		}
		if !tile.Kind.Buyable() {
			continue
		}
		if tile.Price <= 0 {
			t.Errorf("square %d (%s) has no price", i, tile.Name)
		}
		if tile.Kind != TileProperty {
			continue
		}
		if tile.Group == "" {
			t.Errorf("%s has no colour group", tile.Name)
		}
		if tile.House <= 0 {
			t.Errorf("%s has no house price", tile.Name)
		}
		// Rents have to climb, or a house is a purchase that loses you money.
		for r := 1; r < len(tile.Rent); r++ {
			if tile.Rent[r] <= tile.Rent[r-1] {
				t.Errorf("%s rent does not increase at %d houses: %v", tile.Name, r, tile.Rent)
				break
			}
		}
	}
}

func TestColourGroupsAreTwoOrThree(t *testing.T) {
	for _, group := range []string{
		GroupBrown, GroupLBlue, GroupPink, GroupOrange,
		GroupRed, GroupYellow, GroupGreen, GroupDBlue,
	} {
		n := groupSize(group)
		if n < 2 || n > 3 {
			t.Errorf("%s has %d properties, want 2 or 3", group, n)
		}
	}
	// The two-property sets are the cheapest and the most expensive, at the ends
	// of the board — the original's shape, and what makes Bagan and Shwedagon a
	// pair worth fighting over.
	if groupSize(GroupBrown) != 2 || groupSize(GroupDBlue) != 2 {
		t.Error("the first and last colour sets should be the pairs")
	}
}

// Prices should climb around the board. Not strictly — the original has equal
// pairs and the stations are all 200 — but a later property must never be
// cheaper than an earlier one, which is the mistake a typo makes.
func TestPropertyPricesClimb(t *testing.T) {
	last := 0
	for i, tile := range Board() {
		if tile.Kind != TileProperty {
			continue
		}
		if tile.Price < last {
			t.Errorf("square %d (%s) at %d is cheaper than an earlier property at %d",
				i, tile.Name, tile.Price, last)
		}
		last = tile.Price
	}
}

// ───────────────────────────────────────────────────────────────── the rules

func TestDealPutsEverybodyOnGoWithTheSameMoney(t *testing.T) {
	s := deal(t, 3)
	for _, p := range s.Players {
		if p.Pos != 0 {
			t.Errorf("%s starts on square %d, want GO", p.Name, p.Pos)
		}
		if p.Cash != StartingCash {
			t.Errorf("%s starts with %d, want %d", p.Name, p.Cash, StartingCash)
		}
		if !p.Alive {
			t.Errorf("%s is not in the game", p.Name)
		}
	}
	if s.Phase != PhaseRoll {
		t.Errorf("phase is %s, want a roll", s.Phase)
	}
	for i := range s.Owner {
		if s.Owner[i] != "" {
			t.Fatalf("square %d is owned before anyone has played", i)
		}
	}
}

func TestTooFewOrTooManyPlayersIsRefused(t *testing.T) {
	for _, n := range []int{0, 1, MaxPlayers + 1} {
		if _, err := NewGame(seats(n), prng.New(uint64(1))); err == nil {
			t.Errorf("%d players was accepted", n)
		}
	}
}

func TestOnlyTheCurrentPlayerCanRoll(t *testing.T) {
	s := deal(t, 3)
	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p1"}); err == nil {
		t.Fatal("a player rolled out of turn")
	}
	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: s.CurrentID()}); err != nil {
		t.Fatalf("the current player could not roll: %v", err)
	}
}

func TestRollingMovesYouAndOffersWhatYouLandOn(t *testing.T) {
	s := deal(t, 2)
	events, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"})
	if err != nil {
		t.Fatal(err)
	}

	total := s.Dice[0] + s.Dice[1]
	if total < 2 || total > 12 {
		t.Fatalf("the dice read %v", s.Dice)
	}
	if s.Players[0].Pos != total {
		t.Errorf("moved to %d on a roll of %d", s.Players[0].Pos, total)
	}
	if !hasKind(events, EvRolled) || !hasKind(events, EvMoved) {
		t.Errorf("the roll produced %v", kinds(events))
	}

	// Everything in the first twelve squares is either buyable or harmless, so
	// the phase after one roll is one of exactly two things.
	landed := TileAt(s.Players[0].Pos)
	if landed.Kind.Buyable() {
		if s.Phase != PhaseBuy {
			t.Errorf("landed on %s and was not offered it", landed.Name)
		}
		if s.PendingTile() != s.Players[0].Pos {
			t.Errorf("offered square %d, standing on %d", s.PendingTile(), s.Players[0].Pos)
		}
	}
}

func TestBuyingTakesTheMoneyAndTheDeed(t *testing.T) {
	s := deal(t, 2)
	// Put p0 on Myeik rather than fishing for a roll that lands there.
	s.Players[0].Pos = 1
	s.Phase = PhaseBuy
	s.Pending = 1

	before := s.Players[0].Cash
	if _, err := Apply(s, Action{Kind: ActBuy, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if s.Owner[1] != "p0" {
		t.Errorf("Myeik is owned by %q", s.Owner[1])
	}
	if want := before - TileAt(1).Price; s.Players[0].Cash != want {
		t.Errorf("cash is %d, want %d", s.Players[0].Cash, want)
	}
	if s.Phase == PhaseBuy {
		t.Error("still being offered a square it has already bought")
	}
}

func TestDecliningLeavesItWithTheBank(t *testing.T) {
	s := deal(t, 2)
	s.Players[0].Pos = 1
	s.Phase = PhaseBuy
	s.Pending = 1

	before := s.Players[0].Cash
	if _, err := Apply(s, Action{Kind: ActPass, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if s.Owner[1] != "" {
		t.Errorf("a declined square went to %q", s.Owner[1])
	}
	if s.Players[0].Cash != before {
		t.Error("declining cost money")
	}
}

func TestYouCannotBuyWhatSomebodyElseOwns(t *testing.T) {
	s := deal(t, 2)
	s.Owner[1] = "p1"
	s.Players[0].Pos = 1
	s.Phase = PhaseBuy
	s.Pending = 1

	if _, err := Apply(s, Action{Kind: ActBuy, PlayerID: "p0"}); err == nil {
		t.Fatal("bought a square out from under its owner")
	}
}

func TestLandingOnSomebodyElseMeansRent(t *testing.T) {
	s := deal(t, 2)
	s.Owner[1] = "p1" // Myeik, the cheapest square on the board
	s.Players[0].Pos = 0

	// Walked rather than rolled, so the test is about rent and not about dice.
	events := moveBy(s, &s.Players[0], 1)

	rent := TileAt(1).Rent[0]
	if got := s.Players[1].Cash; got != StartingCash+rent {
		t.Errorf("the owner has %d, want %d", got, StartingCash+rent)
	}
	if got := s.Players[0].Cash; got != StartingCash-rent {
		t.Errorf("the visitor has %d, want %d", got, StartingCash-rent)
	}
	if !hasKind(events, EvRent) {
		t.Errorf("no rent was reported: %v", kinds(events))
	}
}

func TestYourOwnSquareIsFree(t *testing.T) {
	s := deal(t, 2)
	s.Owner[1] = "p0"
	moveBy(s, &s.Players[0], 1)
	if s.Players[0].Cash != StartingCash {
		t.Errorf("paid %d to stand on their own square", StartingCash-s.Players[0].Cash)
	}
}

// A complete colour set doubles the unimproved rent. This is the reason the whole
// game has trading in it, so it is in the first slice.
func TestAFullColourSetDoublesTheRent(t *testing.T) {
	s := deal(t, 2)
	s.Owner[1] = "p1" // Myeik
	moveBy(s, &s.Players[0], 1)
	single := StartingCash - s.Players[0].Cash

	s = deal(t, 2)
	s.Owner[1] = "p1"
	s.Owner[3] = "p1" // and Dawei: the whole brown set
	moveBy(s, &s.Players[0], 1)
	both := StartingCash - s.Players[0].Cash

	if both != single*2 {
		t.Errorf("rent with the set is %d, want twice %d", both, single)
	}
}

func TestStationRentDoublesWithEachOneHeld(t *testing.T) {
	stations := []int{5, 15, 25, 35}
	want := []int{25 * kyat, 50 * kyat, 100 * kyat, 200 * kyat}

	for held := 1; held <= 4; held++ {
		s := deal(t, 2)
		for i := 0; i < held; i++ {
			s.Owner[stations[i]] = "p1"
		}
		if got := rentOn(s, stations[0]); got != want[held-1] {
			t.Errorf("holding %d stations charges %d, want %d", held, got, want[held-1])
		}
	}
}

func TestUtilityRentFollowsTheDice(t *testing.T) {
	s := deal(t, 2)
	s.Owner[12] = "p1" // one utility
	s.Dice = [2]int{3, 4}
	if got, want := rentOn(s, 12), 4*7*kyat; got != want {
		t.Errorf("one utility on a 7 charges %d, want %d", got, want)
	}

	s.Owner[28] = "p1" // both
	if got, want := rentOn(s, 12), 10*7*kyat; got != want {
		t.Errorf("two utilities on a 7 charges %d, want %d", got, want)
	}
}

func TestPassingGoPays(t *testing.T) {
	s := deal(t, 2)
	s.Players[0].Pos = 39 // Shwedagon, one square short of home
	events := moveBy(s, &s.Players[0], 3)

	if s.Players[0].Pos != 2 {
		t.Fatalf("ended on %d, want to have gone round to 2", s.Players[0].Pos)
	}
	if got := s.Players[0].Cash; got != StartingCash+PassGo {
		t.Errorf("cash is %d, want %d", got, StartingCash+PassGo)
	}
	if !hasKind(events, EvPassedGo) {
		t.Errorf("going round was not reported: %v", kinds(events))
	}
}

func TestLandingExactlyOnGoStillPays(t *testing.T) {
	s := deal(t, 2)
	s.Players[0].Pos = 38
	moveBy(s, &s.Players[0], 2)
	if s.Players[0].Pos != 0 {
		t.Fatalf("ended on %d", s.Players[0].Pos)
	}
	if s.Players[0].Cash != StartingCash+PassGo {
		t.Error("landing on GO did not pay")
	}
}

func TestTaxSquaresCharge(t *testing.T) {
	s := deal(t, 2)
	moveBy(s, &s.Players[0], 4) // Income Tax
	if want := StartingCash - TileAt(4).Tax; s.Players[0].Cash != want {
		t.Errorf("cash is %d after income tax, want %d", s.Players[0].Cash, want)
	}
}

func TestGoToJailSendsYouToTheCorner(t *testing.T) {
	s := deal(t, 2)
	events := moveBy(s, &s.Players[0], GoToJailTile)
	if s.Players[0].Pos != JailTile {
		t.Errorf("ended on %d, want the jail corner at %d", s.Players[0].Pos, JailTile)
	}
	if !hasKind(events, EvJailed) {
		t.Errorf("going to jail was not reported: %v", kinds(events))
	}
	// And it is the next player's turn: no free roll out of the corner.
	if s.CurrentID() == "p0" {
		t.Error("still p0's turn after being sent to jail")
	}
}

func TestThreeDoublesGoesToJail(t *testing.T) {
	s := deal(t, 2)
	s.Doubles = 2
	s.Phase = PhaseRoll
	// Seeded so the next roll is a double; found by asking rather than assuming.
	s.RNG = doublesRoller(t)

	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Pos != JailTile {
		t.Errorf("a third double left them on %d, want jail", s.Players[0].Pos)
	}
}

func TestDoublesEarnAnotherRoll(t *testing.T) {
	s := deal(t, 2)
	s.RNG = doublesRoller(t)

	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err != nil {
		t.Fatal(err)
	}
	// A buy prompt is still p0's to answer, so either way the turn has not passed.
	if s.CurrentID() != "p0" {
		t.Errorf("doubles passed the turn to %s", s.CurrentID())
	}
}

func TestRentYouCannotPayEndsYourGame(t *testing.T) {
	s := deal(t, 2)
	s.Owner[39] = "p1" // Shwedagon, the most expensive rent on the board
	s.Players[0].Cash = 1000
	s.Players[0].Pos = 38

	before := s.Players[1].Cash
	events := moveBy(s, &s.Players[0], 1)

	if s.Players[0].Alive {
		t.Fatal("survived a rent they could not pay")
	}
	if s.Players[0].Cash != 0 {
		t.Errorf("a bankrupt player kept %d", s.Players[0].Cash)
	}
	// Whatever they had crosses the table; the rest is written off.
	if got := s.Players[1].Cash; got != before+1000 {
		t.Errorf("the owner has %d, want %d", got, before+1000)
	}
	if !hasKind(events, EvBankrupt) {
		t.Errorf("bankruptcy was not reported: %v", kinds(events))
	}
	// Two players, so that is the game.
	if s.Phase != PhaseGameOver || s.WinnerID != "p1" {
		t.Errorf("phase %s, winner %q", s.Phase, s.WinnerID)
	}
}

// Every amount in the play-by-play has to be money that actually moved. A player
// who cannot cover a demand pays what they have, and reporting the demand instead
// tells the table that K200,000 left somebody who held K43,000 — and quietly
// breaks any accounting done from the events, which is how this was found.
func TestAmountsReportedAreAmountsPaid(t *testing.T) {
	t.Run("tax", func(t *testing.T) {
		s := deal(t, 3)
		s.Players[0].Cash = 43_000 // less than Income Tax
		events := moveBy(s, &s.Players[0], 4)

		tax := find(events, EvTax)
		if tax == nil {
			t.Fatal("landing on the tax square reported no tax")
		}
		if tax.Amount != 43_000 {
			t.Errorf("the tax line says %d, but only 43000 was there to take", tax.Amount)
		}
		if s.Players[0].Alive {
			t.Error("survived a tax they could not pay")
		}
	})

	t.Run("rent", func(t *testing.T) {
		s := deal(t, 3)
		s.Owner[39] = "p1" // Shwedagon, the biggest rent on the board
		s.Players[0].Cash = 12_000
		s.Players[0].Pos = 38
		before := s.Players[1].Cash

		events := moveBy(s, &s.Players[0], 1)

		rent := find(events, EvRent)
		if rent == nil {
			t.Fatal("landing on an owned square reported no rent")
		}
		if rent.Amount != 12_000 {
			t.Errorf("the rent line says %d, but only 12000 changed hands", rent.Amount)
		}
		// And the owner really did receive exactly what was reported.
		if got := s.Players[1].Cash - before; got != rent.Amount {
			t.Errorf("the owner gained %d while the log said %d", got, rent.Amount)
		}
	})
}

func TestBankruptcyReturnsTheDeedsToTheBank(t *testing.T) {
	s := deal(t, 3)
	s.Owner[1] = "p0"
	s.Owner[3] = "p0"
	s.Owner[39] = "p1"
	s.Players[0].Cash = 0
	s.Players[0].Pos = 38

	moveBy(s, &s.Players[0], 1)

	if owned := s.Owned("p0"); len(owned) != 0 {
		t.Errorf("a bankrupt player still holds %v", owned)
	}
	// Three players, so the game continues.
	if s.Phase == PhaseGameOver {
		t.Error("the game ended with two players still in it")
	}
}

func TestPlayPassesOverPlayersWhoAreOut(t *testing.T) {
	s := deal(t, 3)
	s.Players[1].Alive = false
	s.Current = 0
	s.Doubles = 0
	s.advance()
	if s.CurrentID() != "p2" {
		t.Errorf("play went to %s, want to skip the bankrupt seat", s.CurrentID())
	}
}

func TestNothingWorksOnceTheGameIsOver(t *testing.T) {
	s := deal(t, 2)
	s.Phase = PhaseGameOver
	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err == nil {
		t.Error("rolled after the game ended")
	}
}

func TestAMoveOutOfItsPhaseIsRefused(t *testing.T) {
	s := deal(t, 2)
	// PhaseRoll: there is nothing on offer to buy or decline.
	if _, err := Apply(s, Action{Kind: ActBuy, PlayerID: "p0"}); err == nil {
		t.Error("bought a square with no offer open")
	}
	if _, err := Apply(s, Action{Kind: ActPass, PlayerID: "p0"}); err == nil {
		t.Error("declined an offer that was not made")
	}

	s.Phase = PhaseBuy
	s.Pending = 1
	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err == nil {
		t.Error("rolled while an offer was open")
	}
}

// A game driven by legal moves only, from many seeds, checking the invariants on
// every single step rather than at the end.
//
// Deliberately not "and somebody wins". The first version of this test asserted
// a winner and then quietly logged "no winner in 4000 moves" for all forty seeds,
// which meant the money check underneath it never ran once — see
// TestTheSliceHasNoBankruptcyPressure for why, and web/testing/README.md for why
// a check that never runs is worse than no check.
func TestRandomGamesHoldTheirInvariants(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		s := deal(t, 3)
		s.RNG = prng.New(uint64(seed))
		driver := prng.New(uint64(seed + 5000))

		// The bank has no balance of its own, so what is conserved is the players'
		// total against everything the bank has paid out and taken in. Tracked from
		// the events rather than recomputed, so an event that fails to report an
		// amount shows up here as money going missing.
		fromBank := 0
		start := StartingCash * len(s.Players)

		for step := 0; step < 3000 && s.Phase != PhaseGameOver; step++ {
			a := nextAction(s, driver)
			events, err := Apply(s, a)
			if err != nil {
				t.Fatalf("seed %d step %d: %v (%s)", seed, step, err, s.Phase)
			}
			for _, e := range events {
				switch e.Kind {
				case EvPassedGo:
					fromBank += e.Amount
				case EvTax, EvBought, EvFine, EvBuilt:
					fromBank -= e.Amount
				case EvSold:
					fromBank += e.Amount
				case EvCardPay:
					// Signed already: positive when a card pays you, negative when
					// it takes. Adding it either way is what makes this line the
					// whole of a card's effect on the bank.
					fromBank += e.Amount
				}
			}

			total := 0
			for _, p := range s.Players {
				if p.Pos < 0 || p.Pos >= BoardSize {
					t.Fatalf("seed %d: %s is on square %d", seed, p.Name, p.Pos)
				}
				if p.Cash < 0 {
					t.Fatalf("seed %d: %s has %d", seed, p.Name, p.Cash)
				}
				total += p.Cash
			}
			// Rent moves between players and nets to nothing here; a bankrupt
			// player is emptied before they leave, so nothing is written off while
			// they still hold it.
			if total != start+fromBank {
				t.Fatalf("seed %d step %d: players hold %d, want %d (start %d, bank %+d)",
					seed, step, total, start+fromBank, start, fromBank)
			}
			// Somebody always has to be able to move, or the table is wedged — the
			// failure the browser suite would only find as a game that never ends.
			if s.Phase != PhaseGameOver && s.CurrentID() == "" {
				t.Fatalf("seed %d step %d: nobody's turn in phase %s", seed, step, s.Phase)
			}
		}
	}
}

// Games can be won now, which they could not be before the cards went in.
//
// This replaces a test that asserted the opposite. Without the two decks the
// board had no bankruptcy pressure at all: unimproved rents run K2,000–K50,000
// against a K200,000 lap, so everybody got steadily richer and nobody was ever
// broken. That test existed as a canary and said it would fail "the day building
// lands" — it fired early instead, because jail fines, card payments and turns
// missed while the board keeps charging turned out to be enough on their own.
//
// A rate rather than a demand for every seed: the dice are the dice, and a test
// that insisted on a winner every time would be testing luck. What it must not
// do is pass while *nothing* finishes, which is the failure the old one had.
//
// The rate is also the measurement that justifies building. With the driver's
// build branch switched off these same forty seeds finish 7 games; with it on
// they finish 23. That is what houses are for, and it is the only place the
// claim is checkable rather than asserted.
func TestGamesReachAWinner(t *testing.T) {
	const seeds = 40
	won, moves := 0, 0
	for seed := uint64(0); seed < seeds; seed++ {
		s := deal(t, 3)
		s.RNG = prng.New(seed)
		driver := prng.New(seed + 9000)

		step := 0
		for ; step < 4000 && s.Phase != PhaseGameOver; step++ {
			if _, err := Apply(s, nextAction(s, driver)); err != nil {
				t.Fatalf("seed %d step %d: %v (%s)", seed, step, err, s.Phase)
			}
		}
		if s.Phase == PhaseGameOver {
			if s.WinnerID == "" {
				t.Fatalf("seed %d: game over with no winner", seed)
			}
			won++
			moves += step
		}
	}

	if won == 0 {
		t.Fatal("no game in 40 reached a winner — the board has no pressure in it at all")
	}
	t.Logf("%d of %d games finished, averaging %d moves", won, seeds, moves/won)
}

// ─────────────────────────────────────────────────────────────────── helpers

func deal(t *testing.T, n int) *State {
	t.Helper()
	s, err := NewGame(seats(n), prng.New(uint64(1)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func seats(n int) []Seat {
	var out []Seat
	for i := 0; i < n; i++ {
		id := "p" + string(rune('0'+i))
		out = append(out, Seat{ID: id, Name: id})
	}
	return out
}

// doublesRoller finds a seed whose first throw is a double, rather than assuming
// one. Hard-coding a seed here would break the moment the roll changes shape.
func doublesRoller(t *testing.T) *prng.Source {
	t.Helper()
	for seed := int64(0); seed < 500; seed++ {
		r := prng.New(uint64(seed))
		if r.Intn(6) == r.Intn(6) {
			return prng.New(uint64(seed))
		}
	}
	t.Fatal("no seed in 500 threw a double")
	return nil
}

func kinds(events []Event) []EventKind {
	out := make([]EventKind, 0, len(events))
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

// find is the first event of a kind, or nil. A declaration rather than a `var`
// so it is usable from the tests above it as well as below.
func find(events []Event, kind EventKind) *Event {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

func hasKind(events []Event, kind EventKind) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

// nextAction is a legal move for whatever the table is waiting on. Shared by
// both drivers above so a phase added later is handled in one place rather than
// making one of them fail with "it isn't your turn" and the other pass.
//
// driver may be nil, in which case the choices are the greedy ones: buy, and pay
// your way out of jail if you can.
func nextAction(s *State, driver *prng.Source) Action {
	id := s.CurrentID()
	// Building, in the two phases it is legal in. Only sometimes, and never as
	// the only thing on offer, so the table still rolls and the game still ends:
	// a driver that built whenever it could would spin forever on a completed
	// set. Selling is rarer still — it is the escape hatch, not the strategy.
	if s.Phase == PhaseRoll || s.Phase == PhaseJail {
		if driver != nil && driver.Intn(3) == 0 {
			if up := s.BuildableFor(id); len(up) > 0 {
				return Action{Kind: ActBuild, PlayerID: id, Tile: up[driver.Intn(len(up))]}
			}
		}
		if driver != nil && driver.Intn(12) == 0 {
			if down := s.SellableFor(id); len(down) > 0 {
				return Action{Kind: ActSell, PlayerID: id, Tile: down[driver.Intn(len(down))]}
			}
		}
	}
	switch s.Phase {
	case PhaseBuy:
		if driver != nil && driver.Intn(4) == 0 {
			return Action{Kind: ActPass, PlayerID: id}
		}
		return Action{Kind: ActBuy, PlayerID: id}
	case PhaseCard:
		// Nothing to decide; somebody has to have read it.
		return Action{Kind: ActReadCard, PlayerID: id}
	case PhaseJail:
		p := s.Find(id)
		if p != nil && p.Pardons > 0 {
			return Action{Kind: ActUsePardon, PlayerID: id}
		}
		// Roll for a double, or buy the way out — both are legal, and taking
		// both paths keeps the fuzz over the whole of jail rather than one exit.
		if p != nil && p.Cash >= JailFine && (driver == nil || driver.Intn(3) == 0) {
			return Action{Kind: ActPayFine, PlayerID: id}
		}
		return Action{Kind: ActRoll, PlayerID: id}
	default:
		return Action{Kind: ActRoll, PlayerID: id}
	}
}
