package game

import (
	"testing"

	"boardgame/kittens/internal/prng"
)

// The colour sets these tests build on, by square. Written out rather than
// searched for, because a test that goes looking for "some complete set" would
// keep passing while proving something else.
var (
	brown = []int{1, 3}    // Myeik, Dawei — the cheapest pair
	lblue = []int{6, 8, 9} // Magway, Monywa, Pathein
	pink  = []int{11, 13, 14}
)

// giveSet hands a whole colour set to a player and leaves them enough money to
// build on it. Both are needed for every building test, and forgetting the
// second reads as an even-build failure.
func giveSet(s *State, playerID string, squares []int) {
	for _, i := range squares {
		s.Owner[i] = playerID
	}
	if p := s.Find(playerID); p != nil {
		p.Cash = 10_000 * kyat
	}
}

func build(t *testing.T, s *State, playerID string, pos int) []Event {
	t.Helper()
	events, err := Apply(s, Action{Kind: ActBuild, PlayerID: playerID, Tile: pos})
	if err != nil {
		t.Fatalf("building on %s: %v", TileAt(pos).Name, err)
	}
	return events
}

// ───────────────────────────────────────────────────── who may build, and where

func TestBuildingNeedsTheWholeColourSet(t *testing.T) {
	s := deal(t, 2)
	// One of the pair, not both.
	s.Owner[1] = "p0"
	s.Players[0].Cash = 10_000 * kyat

	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrIncompleteSet {
		t.Fatalf("built on half a set: %v", err)
	}

	s.Owner[3] = "p0"
	build(t, s, "p0", 1)
	if s.Houses[1] != 1 {
		t.Errorf("Myeik has %d houses, want 1", s.Houses[1])
	}
}

func TestBuildingNeedsToBeYourSquare(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p1", brown)
	// p0 is on turn, and the set is p1's.
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrNotYours {
		t.Fatalf("built on somebody else's square: %v", err)
	}
}

func TestOnlyPropertiesCanBeBuiltOn(t *testing.T) {
	s := deal(t, 2)
	s.Players[0].Cash = 10_000 * kyat
	// A station, a utility, and a corner. Owning them changes nothing: they
	// charge by how many of their kind you hold, and there is no set to complete.
	for _, pos := range []int{5, 12, 0, 20} {
		s.Owner[pos] = "p0"
		if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: pos}); err == nil {
			t.Errorf("built on %s", TileAt(pos).Name)
		}
	}
}

func TestBuildingYouCannotAffordIsRefused(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Players[0].Cash = TileAt(1).House - 1

	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrCantAfford {
		t.Fatalf("built without the money: %v", err)
	}
	if s.Houses[1] != 0 {
		t.Error("a refused build left a house standing")
	}
}

func TestYouMustBuildEvenlyAcrossASet(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", lblue)

	build(t, s, "p0", 6)
	// Magway now has one and the other two have none, so a second there is out.
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 6}); err != ErrBuildUnevenly {
		t.Fatalf("stacked a second house on an uneven set: %v", err)
	}
	build(t, s, "p0", 8)
	build(t, s, "p0", 9)
	// Level across the set: a second is now legal anywhere in it.
	build(t, s, "p0", 6)
	if got := []int{s.Houses[6], s.Houses[8], s.Houses[9]}; got[0] != 2 || got[1] != 1 || got[2] != 1 {
		t.Errorf("the set stands at %v, want [2 1 1]", got)
	}
}

func TestSellingComesDownEvenlyToo(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1] = 2
	s.Houses[3] = 1

	// Dawei is the lower of the two, so it is not the one that may come down.
	if _, err := Apply(s, Action{Kind: ActSell, PlayerID: "p0", Tile: 3}); err != ErrBuildUnevenly {
		t.Fatalf("sold from under the taller square: %v", err)
	}
	if _, err := Apply(s, Action{Kind: ActSell, PlayerID: "p0", Tile: 1}); err != nil {
		t.Fatalf("selling from Myeik: %v", err)
	}
	if s.Houses[1] != 1 {
		t.Errorf("Myeik stands at %d, want 1", s.Houses[1])
	}
}

func TestSellingRefundsHalfThePrice(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1] = 1
	before := s.Players[0].Cash

	events, err := Apply(s, Action{Kind: ActSell, PlayerID: "p0", Tile: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := TileAt(1).House / 2
	if got := s.Players[0].Cash - before; got != want {
		t.Errorf("selling a house paid %d, want %d", got, want)
	}
	sold := find(events, EvSold)
	if sold == nil {
		t.Fatal("selling reported nothing")
	}
	if sold.Amount != want {
		t.Errorf("the event says %d, the wallet says %d", sold.Amount, want)
	}
	if sold.Count != 0 {
		t.Errorf("the event says %d buildings left, want 0", sold.Count)
	}
}

func TestNothingToSellIsRefused(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	if _, err := Apply(s, Action{Kind: ActSell, PlayerID: "p0", Tile: 1}); err != ErrNothingBuilt {
		t.Fatalf("sold a house that was not there: %v", err)
	}
}

// ───────────────────────────────────────────────────────────── the fifth one

func TestTheFifthBuildingIsAHotel(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1], s.Houses[3] = 4, 4

	events := build(t, s, "p0", 1)
	if s.Houses[1] != HotelLevel {
		t.Fatalf("Myeik stands at %d, want a hotel at %d", s.Houses[1], HotelLevel)
	}
	if built := find(events, EvBuilt); built == nil || built.Count != HotelLevel {
		t.Errorf("the event reported %v", events)
	}
	// And nothing more goes on it.
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrFullyBuilt {
		t.Fatalf("built on top of a hotel: %v", err)
	}
}

// The four houses under a hotel go back on the shelf. Without this the box runs
// dry after eight properties and nobody can build again — which would look like
// the even-build rule misfiring rather than a supply bug.
func TestAHotelReturnsItsFourHouses(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1], s.Houses[3] = 4, 4
	if s.HousesLeft() != HouseSupply-8 {
		t.Fatalf("eight houses out leaves %d", s.HousesLeft())
	}

	build(t, s, "p0", 1)
	if got, want := s.HousesLeft(), HouseSupply-4; got != want {
		t.Errorf("after a hotel the bank holds %d houses, want %d", got, want)
	}
	if got, want := s.HotelsLeft(), HotelSupply-1; got != want {
		t.Errorf("the bank holds %d hotels, want %d", got, want)
	}
}

func TestSellingAHotelLeavesFourHouses(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1] = HotelLevel

	if _, err := Apply(s, Action{Kind: ActSell, PlayerID: "p0", Tile: 1}); err != nil {
		t.Fatal(err)
	}
	if s.Houses[1] != HotelLevel-1 {
		t.Errorf("the hotel came down to %d, want %d", s.Houses[1], HotelLevel-1)
	}
	if got, want := s.HousesLeft(), HouseSupply-4; got != want {
		t.Errorf("the bank holds %d houses, want %d", got, want)
	}
}

// The bank has to be able to supply the four houses a hotel comes down as, and
// that is a real rule rather than a technicality: a table with every house out
// is a table whose hotels are stuck where they are.
func TestSellingAHotelNeedsFourHousesInTheBank(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1] = HotelLevel
	// Soak up the shelf: 29 houses out leaves three, one short of the four a
	// hotel comes down as.
	giveSet(s, "p1", pink)
	s.Houses[11], s.Houses[13], s.Houses[14] = 4, 4, 4
	giveSet(s, "p1", lblue)
	s.Houses[6], s.Houses[8], s.Houses[9] = 4, 4, 4
	s.Houses[3] = 4
	s.Owner[21] = "p1"
	s.Houses[21] = 1
	if got := s.HousesLeft(); got != 3 {
		t.Fatalf("the setup left %d houses, want 3", got)
	}

	if _, err := Apply(s, Action{Kind: ActSell, PlayerID: "p0", Tile: 1}); err != ErrNoHousesLeft {
		t.Fatalf("a hotel came down with three houses on the shelf: %v", err)
	}
}

// Buying up the cheap sets to starve everybody else of building material is a
// real move, and it only exists because the box is finite.
func TestTheBankRunsOutOfHouses(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	// Eight sets of four is more than the box holds, so fill it by hand and check
	// the thirty-third is refused.
	filled := 0
	for i := range board {
		if board[i].Kind != TileProperty || i == 1 || i == 3 {
			continue
		}
		if filled+4 > HouseSupply {
			break
		}
		s.Houses[i] = 4
		filled += 4
	}
	if s.HousesLeft() != 0 {
		t.Fatalf("the setup left %d houses on the shelf", s.HousesLeft())
	}

	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrNoHousesLeft {
		t.Fatalf("built the thirty-third house: %v", err)
	}
	// A hotel is a different shelf, and this player has no square ready for one.
	s.Houses[1], s.Houses[3] = 4, 4
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != nil {
		t.Fatalf("a hotel was refused for want of houses: %v", err)
	}
}

func TestTheBankRunsOutOfHotels(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	s.Houses[1], s.Houses[3] = 4, 4

	// Twelve hotels standing elsewhere.
	placed := 0
	for i := range board {
		if board[i].Kind != TileProperty || i == 1 || i == 3 || placed == HotelSupply {
			continue
		}
		s.Houses[i] = HotelLevel
		placed++
	}
	if s.HotelsLeft() != 0 {
		t.Fatalf("the setup left %d hotels", s.HotelsLeft())
	}

	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrNoHotelsLeft {
		t.Fatalf("built the thirteenth hotel: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────── rent

// The whole point of the feature. Rent[0] on Myeik is K2,000 against a K200,000
// lap; Rent[5] is K250,000. That is the difference between a board that ends and
// one that does not.
func TestRentFollowsTheBuildings(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)
	tile := TileAt(1)

	// Unimproved but complete: the set bonus, not the rent slice.
	if got, want := rentOn(s, 1), tile.Rent[0]*2; got != want {
		t.Errorf("an unimproved whole set charges %d, want %d", got, want)
	}
	for level := 1; level <= HotelLevel; level++ {
		s.Houses[1] = level
		if got, want := rentOn(s, 1), tile.Rent[level]; got != want {
			t.Errorf("%d buildings charge %d, want %d", level, got, want)
		}
	}
	// And the printed figures have to climb, or the feature does nothing.
	for level := 1; level <= HotelLevel; level++ {
		if tile.Rent[level] <= tile.Rent[level-1] {
			t.Fatalf("Myeik's rent at %d buildings (%d) does not beat %d (%d)",
				level, tile.Rent[level], level-1, tile.Rent[level-1])
		}
	}
}

func TestABuiltSquareIsWhatTheVisitorPays(t *testing.T) {
	s := deal(t, 3)
	giveSet(s, "p1", brown)
	s.Houses[1], s.Houses[3] = 3, 3
	s.Players[0].Pos = 0
	s.Players[0].Cash = 1_000 * kyat
	landlord := s.Players[1].Cash

	events := moveBy(s, &s.Players[0], 1)
	rent := find(events, EvRent)
	if rent == nil {
		t.Fatal("landing on a built square charged nothing")
	}
	if want := TileAt(1).Rent[3]; rent.Amount != want {
		t.Errorf("three houses charged %d, want %d", rent.Amount, want)
	}
	if got := s.Players[1].Cash - landlord; got != rent.Amount {
		t.Errorf("the landlord received %d, the log says %d", got, rent.Amount)
	}
}

// ────────────────────────────────────────────────────────── when you may build

func TestBuildingIsOnlyLegalBeforeYouRollOrInJail(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", brown)

	// Mid-decision: an offer is open, so the table is half-resolved.
	s.Phase = PhaseBuy
	s.Pending = 5
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrWrongPhase {
		t.Fatalf("built with a purchase open: %v", err)
	}

	// A card face up is the same: it has not been applied yet.
	s.Phase = PhaseCard
	s.Drawn = 0
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p0", Tile: 1}); err != ErrWrongPhase {
		t.Fatalf("built with a card unread: %v", err)
	}

	// Held in jail: your board is still yours.
	s.Phase = PhaseJail
	s.Players[0].Jailed = true
	build(t, s, "p0", 1)
}

// Building must not pass play on, or a player could only ever put up one house a
// lap. Its own test because the same mistake has already been made twice here —
// see the comment on transfer() in engine.go.
func TestBuildingDoesNotEndYourTurn(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p0", lblue)

	for _, pos := range lblue {
		build(t, s, "p0", pos)
		if s.CurrentID() != "p0" {
			t.Fatalf("building on %d passed the turn to %s", pos, s.CurrentID())
		}
		if s.Phase != PhaseRoll {
			t.Fatalf("building left the table in %s", s.Phase)
		}
	}
	// And the roll that was always going to happen still can.
	if _, err := Apply(s, Action{Kind: ActRoll, PlayerID: "p0"}); err != nil {
		t.Fatalf("could not roll after building: %v", err)
	}
}

func TestOnlyTheCurrentPlayerCanBuild(t *testing.T) {
	s := deal(t, 2)
	giveSet(s, "p1", brown)
	// p1 owns a complete set but it is p0's turn.
	if _, err := Apply(s, Action{Kind: ActBuild, PlayerID: "p1", Tile: 1}); err != ErrNotYourTurn {
		t.Fatalf("built out of turn: %v", err)
	}
}

func TestBankruptcyReturnsBuildingsToTheBank(t *testing.T) {
	s := deal(t, 3)
	giveSet(s, "p0", brown)
	s.Houses[1], s.Houses[3] = 3, 2
	s.Owner[39] = "p1"
	s.Houses[39] = 0
	s.Players[0].Cash = 0
	s.Players[0].Pos = 38

	moveBy(s, &s.Players[0], 1)

	if !s.Players[0].Alive {
		if got := s.Houses[1] + s.Houses[3]; got != 0 {
			t.Errorf("%d buildings still stand on a bankrupt player's squares", got)
		}
		if s.HousesLeft() != HouseSupply {
			t.Errorf("the bank is %d houses short", HouseSupply-s.HousesLeft())
		}
	}
}

// ───────────────────────────────────────────────── the lists the client renders

// The client renders exactly CanBuild and CanSell, so a square in one of those
// lists that Apply then refuses is a dead button — and a square left out of them
// that Apply would have accepted is a move the player can never make. Both
// directions, over every square, on a board arranged to sit on every edge of the
// rule at once: a complete set part-built, an incomplete set, somebody else's
// set, and a hotel.
func TestTheBuildableListsAreExactlyWhatApplyAccepts(t *testing.T) {
	setup := func() *State {
		s, err := NewGame(seats(2), prng.New(7))
		if err != nil {
			t.Fatal(err)
		}
		giveSet(s, "p0", lblue)
		s.Houses[6], s.Houses[8], s.Houses[9] = 2, 1, 1
		giveSet(s, "p0", brown)
		s.Houses[1], s.Houses[3] = HotelLevel, 4
		// An incomplete set, and one that is somebody else's.
		s.Owner[11] = "p0"
		giveSet(s, "p1", pink)
		s.Owner[11] = "p0"
		s.Players[0].Cash = 10_000 * kyat
		return s
	}

	base := setup()
	canBuild := map[int]bool{}
	for _, i := range base.BuildableFor("p0") {
		canBuild[i] = true
	}
	canSell := map[int]bool{}
	for _, i := range base.SellableFor("p0") {
		canSell[i] = true
	}

	for i := 0; i < BoardSize; i++ {
		accepted := func(kind ActionKind) bool {
			fresh := setup()
			_, err := Apply(fresh, Action{Kind: kind, PlayerID: "p0", Tile: i})
			return err == nil
		}
		if got := accepted(ActBuild); got != canBuild[i] {
			t.Errorf("square %d (%s): the list says buildable=%v, Apply says %v",
				i, TileAt(i).Name, canBuild[i], got)
		}
		if got := accepted(ActSell); got != canSell[i] {
			t.Errorf("square %d (%s): the list says sellable=%v, Apply says %v",
				i, TileAt(i).Name, canSell[i], got)
		}
	}

	// And the lists have to be non-empty, or the assertion above holds vacuously
	// — the failure this whole file exists to avoid.
	if len(canBuild) == 0 || len(canSell) == 0 {
		t.Fatalf("nothing was buildable (%d) or sellable (%d) on a board built for both",
			len(canBuild), len(canSell))
	}
}

// The supply is counted from the board rather than tracked, so the two can never
// drift — but only if nothing ever writes a level outside 0..5. Walked over a
// fuzz rather than asserted once, because the levels are what every other rule
// here reads.
func TestBuildingLevelsStayInRange(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		s := deal(t, 3)
		s.RNG = prng.New(seed)
		driver := prng.New(seed + 400)

		for step := 0; step < 1500 && s.Phase != PhaseGameOver; step++ {
			if _, err := Apply(s, nextAction(s, driver)); err != nil {
				t.Fatalf("seed %d step %d: %v (%s)", seed, step, err, s.Phase)
			}
			for i, h := range s.Houses {
				if h < 0 || h > HotelLevel {
					t.Fatalf("seed %d: square %d stands at %d", seed, i, h)
				}
				if h > 0 && board[i].Kind != TileProperty {
					t.Fatalf("seed %d: %s carries %d buildings", seed, board[i].Name, h)
				}
				if h > 0 && s.Owner[i] == "" {
					t.Fatalf("seed %d: %s is the bank's and carries %d buildings",
						seed, board[i].Name, h)
				}
			}
			if s.HousesLeft() < 0 || s.HotelsLeft() < 0 {
				t.Fatalf("seed %d: the bank is overdrawn (%d houses, %d hotels)",
					seed, s.HousesLeft(), s.HotelsLeft())
			}
		}
	}
}
