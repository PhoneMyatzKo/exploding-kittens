package game

// Houses and hotels — the thing that makes the game finishable.
//
// Unimproved rent runs K2,000 to K50,000 against a K200,000 lap, so a board
// without building is a board where everybody slowly gets richer: the fuzz test
// that used to assert exactly that is in engine_test.go with the paragraph
// explaining why it was deleted. Rent[3] on the cheapest orange is K550,000.
// That is the difference between a game with an ending and a game people abandon.
//
// Two rules do all the work here and both are the original's:
//
//   - You may only build on a colour set you hold complete. This is why anybody
//     trades, and why a set nobody can complete is a set nobody improves.
//   - You must build evenly. No second house anywhere in a set until every
//     property in it has one, and the same in reverse when selling. Without it
//     the whole strategy collapses into stacking a hotel on the one square
//     people land on most.
//
// The bank's stock is finite, and that is a rule rather than an implementation
// detail: with only thirty-two houses on the table, buying up the cheap sets to
// starve everybody else of building material is a real move. The stock is
// *counted* from State.Houses rather than stored alongside it — a second copy of
// a derivable number is a second copy to get wrong, and this one would go wrong
// silently as a table that slowly grows more houses than the box holds.

// HotelLevel is what Houses[pos] reads when a hotel stands there. Five rather
// than a separate flag because it is the fifth level of building and rent looks
// it up in the same slice — Rent[5] is the hotel figure.
const HotelLevel = 5

// The box's contents. Trading four houses for a hotel is what keeps a table from
// running out: the four go back on the shelf.
const (
	HouseSupply = 32
	HotelSupply = 12
)

// sellDivisor is what a building is sold back for — half what it cost, the
// original's rate. A house is never priced oddly on this board, so this divides
// exactly and there is no rounding to argue about.
const sellDivisor = 2

// housesStanding is how many of the bank's houses are out on the board. A hotel
// is not four houses: those went back when it was built.
func (s *State) housesStanding() int {
	n := 0
	for _, h := range s.Houses {
		if h > 0 && h < HotelLevel {
			n += h
		}
	}
	return n
}

// hotelsStanding is how many of the bank's hotels are out on the board.
func (s *State) hotelsStanding() int {
	n := 0
	for _, h := range s.Houses {
		if h == HotelLevel {
			n++
		}
	}
	return n
}

// HousesLeft and HotelsLeft are the bank's remaining stock, for the client to
// explain a refusal with rather than leaving a button dead.
func (s *State) HousesLeft() int { return HouseSupply - s.housesStanding() }
func (s *State) HotelsLeft() int { return HotelSupply - s.hotelsStanding() }

// BuildingsOn is how many buildings a player has standing, counting a hotel as
// one. For the seat strip, which has room for a number and not for a board.
func (s *State) BuildingsOn(playerID string) int {
	n := 0
	for i, h := range s.Houses {
		if h > 0 && s.Owner[i] == playerID {
			n++
		}
	}
	return n
}

// levelRange is the least and most built property in a colour set. The two
// numbers the even-build rule is expressed in.
func (s *State) levelRange(group string) (low, high int) {
	low, high = HotelLevel, 0
	for i, t := range board {
		if t.Group != group {
			continue
		}
		if s.Houses[i] < low {
			low = s.Houses[i]
		}
		if s.Houses[i] > high {
			high = s.Houses[i]
		}
	}
	return low, high
}

// CanBuildOn reports why a player may not put a building on a square, or nil if
// they may. Exported because the view calls it: the client renders its buttons
// from the server's answer rather than working the rules out a second time, which
// is the only way a button is never offered that would be refused.
func (s *State) CanBuildOn(playerID string, pos int) error {
	if pos < 0 || pos >= BoardSize {
		return ErrNotForSale
	}
	tile := board[pos]
	if tile.Kind != TileProperty {
		return ErrNotForSale
	}
	if s.Owner[pos] != playerID || playerID == "" {
		return ErrNotYours
	}
	if !s.ownsGroup(playerID, tile.Group) {
		return ErrIncompleteSet
	}
	if s.Houses[pos] >= HotelLevel {
		return ErrFullyBuilt
	}
	// Evenly: this square must be one of the least built in its set.
	if low, _ := s.levelRange(tile.Group); s.Houses[pos] > low {
		return ErrBuildUnevenly
	}
	// The fifth building is a hotel, and the four houses under it come back.
	if s.Houses[pos] == HotelLevel-1 {
		if s.HotelsLeft() < 1 {
			return ErrNoHotelsLeft
		}
	} else if s.HousesLeft() < 1 {
		return ErrNoHousesLeft
	}
	p := s.Find(playerID)
	if p == nil || p.Cash < tile.House {
		return ErrCantAfford
	}
	return nil
}

// CanSellOn reports why a player may not sell a building back, or nil if they
// may. Selling is how somebody who has over-built raises the cash to pay a rent,
// and it is the only alternative this slice has to going out.
func (s *State) CanSellOn(playerID string, pos int) error {
	if pos < 0 || pos >= BoardSize {
		return ErrNotForSale
	}
	tile := board[pos]
	if tile.Kind != TileProperty {
		return ErrNotForSale
	}
	if s.Owner[pos] != playerID || playerID == "" {
		return ErrNotYours
	}
	if s.Houses[pos] < 1 {
		return ErrNothingBuilt
	}
	// Evenly, in reverse: this square must be one of the most built in its set.
	if _, high := s.levelRange(tile.Group); s.Houses[pos] < high {
		return ErrBuildUnevenly
	}
	// A hotel comes down as four houses, and the bank has to be able to supply
	// them. A real rule and not a technicality — a table where every house is out
	// is a table where the hotels are stuck.
	if s.Houses[pos] == HotelLevel && s.HousesLeft() < HotelLevel-1 {
		return ErrNoHousesLeft
	}
	return nil
}

// BuildableFor and SellableFor are the squares a player may act on right now, in
// board order. The view sends both lists and the client renders exactly them —
// so a square whose set is incomplete, or whose neighbour is one house behind,
// simply has no button.
func (s *State) BuildableFor(playerID string) []int {
	var out []int
	for i := range board {
		if s.CanBuildOn(playerID, i) == nil {
			out = append(out, i)
		}
	}
	return out
}

func (s *State) SellableFor(playerID string) []int {
	var out []int
	for i := range board {
		if s.CanSellOn(playerID, i) == nil {
			out = append(out, i)
		}
	}
	return out
}

// applyBuild puts one building up.
//
// Deliberately does not end the turn: building happens before you roll, and you
// may build on several squares in one go. Which is also why it is legal in
// PhaseRoll and PhaseJail and nowhere else — those are the two phases where the
// table is waiting on you and nothing is half-resolved. In the original you may
// build at any moment, including in the middle of somebody else's turn; that
// needs a rule for who gets the last house when two people want it, and this
// does not have one.
func applyBuild(s *State, p *Player, pos int) ([]Event, error) {
	if s.Phase != PhaseRoll && s.Phase != PhaseJail {
		return nil, ErrWrongPhase
	}
	if err := s.CanBuildOn(p.ID, pos); err != nil {
		return nil, err
	}
	tile := board[pos]
	p.Cash -= tile.House
	s.Houses[pos]++
	return []Event{{
		Kind: EvBuilt, ActorID: p.ID, Tile: pos,
		Amount: tile.House, Count: s.Houses[pos],
	}}, nil
}

// applySell takes one building back down for half its price.
func applySell(s *State, p *Player, pos int) ([]Event, error) {
	if s.Phase != PhaseRoll && s.Phase != PhaseJail {
		return nil, ErrWrongPhase
	}
	if err := s.CanSellOn(p.ID, pos); err != nil {
		return nil, err
	}
	tile := board[pos]
	if s.Houses[pos] == HotelLevel {
		// A hotel comes down as the four houses it replaced, so the refund is one
		// building's worth and the level drops by one like any other sale.
		s.Houses[pos] = HotelLevel - 1
	} else {
		s.Houses[pos]--
	}
	refund := tile.House / sellDivisor
	p.Cash += refund
	return []Event{{
		Kind: EvSold, ActorID: p.ID, Tile: pos,
		Amount: refund, Count: s.Houses[pos],
	}}, nil
}

// razeBuildings clears a player's board when they go out. The buildings go back
// to the bank with the deeds rather than to whoever broke them — the same
// simplification bankruptcy already makes, noted in transfer().
func (s *State) razeBuildings(playerID string) {
	for i := range s.Houses {
		if s.Owner[i] == playerID {
			s.Houses[i] = 0
		}
	}
}
