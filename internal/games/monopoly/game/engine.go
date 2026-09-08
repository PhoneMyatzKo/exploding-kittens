package game

import (
	"boardgame/kittens/internal/prng"
	"fmt"
)

// NewGame deals a table: everyone on GO with the same money, nobody owning
// anything, and the first seat to roll.
//
// No shuffle and no starting-player draw — seat order is the order people
// joined, which is the order the lobby already showed them.
func NewGame(seats []Seat, rng *prng.Source) (*State, error) {
	if len(seats) < MinPlayers || len(seats) > MaxPlayers {
		return nil, fmt.Errorf("monopoly takes %d-%d players, got %d", MinPlayers, MaxPlayers, len(seats))
	}
	if rng == nil {
		return nil, fmt.Errorf("monopoly needs dice")
	}
	s := &State{Phase: PhaseRoll, RNG: rng, Drawn: -1}
	for _, seat := range seats {
		s.Players = append(s.Players, Player{
			ID: seat.ID, Name: seat.Name, Cash: StartingCash, Alive: true,
		})
	}
	s.shuffleDecks()
	return s, nil
}

// Apply runs one move. The only entry point: everything below is reachable only
// through here, so there is one place where "is this legal" is decided.
func Apply(s *State, a Action) ([]Event, error) {
	if s.Phase == PhaseGameOver {
		return nil, ErrGameFinished
	}
	p := s.Find(a.PlayerID)
	if p == nil || !p.Alive {
		return nil, ErrNotYourTurn
	}
	if a.PlayerID != s.CurrentID() {
		return nil, ErrNotYourTurn
	}

	switch a.Kind {
	case ActRoll:
		// Rolling means something different while you are being held: it is an
		// attempt at doubles rather than a move.
		if s.Phase == PhaseJail {
			return rollInJail(s, p)
		}
		return applyRoll(s, p)
	case ActBuy:
		return applyBuy(s, p)
	case ActPass:
		return applyPass(s, p)
	case ActReadCard:
		return resolveDrawn(s, p)
	case ActPayFine:
		return applyPayFine(s, p)
	case ActUsePardon:
		return applyUsePardon(s, p)
	case ActBuild:
		return applyBuild(s, p, a.Tile)
	case ActSell:
		return applySell(s, p, a.Tile)
	}
	return nil, ErrUnknownMove
}

// applyRoll throws two dice from the state's own generator, so a game is
// reproducible from its seed — see TODO.md, "Serializable state".
func applyRoll(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseRoll {
		return nil, ErrWrongPhase
	}
	die1, die2 := s.RNG.Intn(6)+1, s.RNG.Intn(6)+1
	s.Dice = [2]int{die1, die2}

	events := []Event{{Kind: EvRolled, ActorID: p.ID, Dice: s.Dice, Tile: -1}}

	// Three doubles in a row is a trip to jail, and the roll does not count.
	if die1 == die2 {
		s.Doubles++
		if s.Doubles >= 3 {
			return append(events, sendToJail(s, p)...), nil
		}
	} else {
		s.Doubles = 0
	}

	return append(events, moveBy(s, p, die1+die2)...), nil
}

// moveBy walks a player forward and resolves wherever they land. Split out
// because a Chance card that says "advance to Bagan" will want it too.
func moveBy(s *State, p *Player, steps int) []Event {
	from := p.Pos
	// Modulo that works for negative steps: Go's % keeps the sign of the left
	// operand, so a card that walks you back past GO would otherwise put you on a
	// negative square.
	p.Pos = ((from+steps)%BoardSize + BoardSize) % BoardSize
	events := []Event{{Kind: EvMoved, ActorID: p.ID, Tile: p.Pos, Amount: steps}}

	// Round the corner and the bank pays you — going forwards only. Several cards
	// walk you backwards, and wrapping the wrong way round is not a lap.
	if steps > 0 && p.Pos < from {
		p.Cash += PassGo
		events = append(events, Event{Kind: EvPassedGo, ActorID: p.ID, Tile: 0, Amount: PassGo})
	}
	return append(events, land(s, p)...)
}

// land resolves the square a player is standing on.
func land(s *State, p *Player) []Event {
	tile := TileAt(p.Pos)

	switch tile.Kind {
	case TileGoToJail:
		return sendToJail(s, p)

	case TileTax:
		// Reported after the fact, with what actually moved: somebody who cannot
		// cover the tax pays what they have and goes out, and a log line claiming
		// the full K200,000 left a player who held K43,000 is simply false.
		paid, settled, evs := transfer(s, p, nil, tile.Tax)
		events := append([]Event{{Kind: EvTax, ActorID: p.ID, Tile: p.Pos, Amount: paid}}, evs...)
		if settled {
			return events
		}
		return append(events, endTurn(s, p)...)

	case TileProperty, TileStation, TileUtility:
		owner := s.Owner[p.Pos]
		switch {
		case owner == "":
			// Yours to take, if you can pay for it. Somebody who cannot afford it
			// is not asked — the prompt would have one button and it would be
			// disabled.
			if p.Cash >= tile.Price {
				s.Phase = PhaseBuy
				s.Pending = p.Pos
				return nil
			}
		case owner != p.ID:
			paid, settled, evs := transfer(s, p, s.Find(owner), rentOn(s, p.Pos))
			events := append([]Event{{
				Kind: EvRent, ActorID: p.ID, TargetID: owner, Tile: p.Pos, Amount: paid,
			}}, evs...)
			if settled {
				return events
			}
			return append(events, endTurn(s, p)...)
		}

	case TileChance:
		return s.drawCard(p, DeckChance)
	case TileChest:
		return s.drawCard(p, DeckChest)
	}

	// Nothing to settle: GO, Just Visiting, Free Parking, and a square you own.
	return endTurn(s, p)
}

// rentOn is what the square at pos charges its visitor.
func rentOn(s *State, pos int) int {
	tile := TileAt(pos)
	owner := s.Owner[pos]

	switch tile.Kind {
	case TileStation:
		// 25, 50, 100, 200 by how many of the four you hold. The count cannot be
		// zero — somebody owns this square or rent would not be being charged —
		// but a negative shift panics, so it is floored rather than assumed.
		held := s.countKind(owner, TileStation)
		if held < 1 {
			held = 1
		}
		return (25 * kyat) << (held - 1)
	case TileUtility:
		// Four times the throw, or ten times if you hold both — the one rent in
		// the game that depends on the dice rather than on the square.
		multiplier := 4
		if s.countKind(owner, TileUtility) == 2 {
			multiplier = 10
		}
		return multiplier * (s.Dice[0] + s.Dice[1]) * kyat
	default:
		// Built on, and the printed figure for that many buildings applies. It
		// already dwarfs the doubled unimproved rent, so the set bonus is not
		// applied on top — the two are alternatives in the original and the rent
		// slice is the whole answer once anything is standing.
		if h := s.Houses[pos]; h > 0 {
			return tile.Rent[h]
		}
		rent := tile.Rent[0]
		// A complete colour set doubles the unimproved rent. This is the whole
		// reason anybody trades, and it is in the slice for that reason.
		if s.ownsGroup(owner, tile.Group) {
			rent *= 2
		}
		return rent
	}
}

// transfer moves money and takes a player out if they cannot cover it. A nil
// creditor is the bank.
//
// Deliberately does *not* end the turn, and that separation is the fix for two
// bugs. It used to: which meant a card reading "pay K30,000 and go back one
// square" never went back, because the payment had already passed play on — and
// paying your way out of jail ended the turn you had just bought, instead of
// letting you roll.
//
// Three returns, because the caller needs all three. paid is what actually
// changed hands, which is not always what was owed: the caller reports the real
// figure rather than the demand, so every amount in the play-by-play is money
// that moved. settled says the transfer has already decided where the table goes,
// which happens when it broke somebody — a bankrupt player's turn is over by
// definition, and the caller must not end it a second time.
//
// Bankruptcy here is simpler than the original in two ways. Everything the loser
// held goes back to the bank rather than to whoever broke them. And nothing is
// sold on their behalf: buildings *can* be sold back (build.go) and a player who
// sees a rent coming can raise the cash that way, but a rent they cannot cover at
// the moment it falls due takes them out rather than forcing a sale first. That
// needs somebody to be asked which buildings go, which needs a phase.
func transfer(s *State, from *Player, to *Player, amount int) (paid int, settled bool, events []Event) {
	if amount <= 0 {
		return 0, false, nil
	}
	if from.Cash >= amount {
		from.Cash -= amount
		if to != nil {
			to.Cash += amount
		}
		return amount, false, nil
	}

	// Everything they have goes across, then they are out.
	paid = from.Cash
	from.Cash = 0
	if to != nil {
		to.Cash += paid
	}
	return paid, true, bankrupt(s, from, to)
}

func bankrupt(s *State, p *Player, to *Player) []Event {
	p.Alive = false
	// Buildings first: razeBuildings finds them by owner, so clearing the deeds
	// before it runs would leave houses standing on squares the bank now holds.
	s.razeBuildings(p.ID)
	for i := range s.Owner {
		if s.Owner[i] == p.ID {
			s.Owner[i] = ""
		}
	}
	target := ""
	if to != nil {
		target = to.ID
	}
	events := []Event{{Kind: EvBankrupt, ActorID: p.ID, TargetID: target, Tile: -1}}

	if s.aliveCount() <= 1 {
		for _, q := range s.Players {
			if q.Alive {
				s.WinnerID = q.ID
			}
		}
		s.Phase = PhaseGameOver
		return append(events, Event{Kind: EvWon, ActorID: s.WinnerID, Tile: -1})
	}
	return append(events, endTurn(s, p)...)
}

// sendToJail holds a player at the corner. They are *held* now rather than
// visiting: getting out costs a fine, a held card, or a thrown double.
func sendToJail(s *State, p *Player) []Event {
	p.Pos = JailTile
	p.Jailed = true
	p.Tries = 0
	// Doubles cleared first: three of them is one of the ways to get here, and
	// the extra roll they would otherwise earn is the thing being taken away.
	s.Doubles = 0
	events := []Event{{Kind: EvJailed, ActorID: p.ID, Tile: JailTile}}
	return append(events, endTurn(s, p)...)
}

// endTurn hands play on — unless the roll was doubles, which earns another go.
func endTurn(s *State, p *Player) []Event {
	if s.Phase == PhaseGameOver {
		return nil
	}
	if p.Alive && s.Doubles > 0 && p.ID == s.CurrentID() {
		s.Phase = PhaseRoll
		return []Event{{Kind: EvTurn, ActorID: p.ID, Tile: -1}}
	}
	// advance reports the turns it stepped over, and those lines matter: a turn
	// silently skipped reads as the table forgetting whose go it is.
	events := s.advance()
	return append(events, Event{Kind: EvTurn, ActorID: s.CurrentID(), Tile: -1})
}

func applyBuy(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseBuy {
		return nil, ErrWrongPhase
	}
	pos := s.Pending
	tile := TileAt(pos)
	if !tile.Kind.Buyable() || s.Owner[pos] != "" {
		return nil, ErrNotForSale
	}
	if p.Cash < tile.Price {
		return nil, ErrCantAfford
	}

	p.Cash -= tile.Price
	s.Owner[pos] = p.ID
	s.Phase = PhaseRoll // cleared before endTurn decides where play goes
	events := []Event{{Kind: EvBought, ActorID: p.ID, Tile: pos, Amount: tile.Price}}
	return append(events, endTurn(s, p)...), nil
}

func applyPass(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseBuy {
		return nil, ErrWrongPhase
	}
	pos := s.Pending
	s.Phase = PhaseRoll
	events := []Event{{Kind: EvDeclined, ActorID: p.ID, Tile: pos}}
	return append(events, endTurn(s, p)...), nil
}

// Not here yet, and each one is a rules change rather than a bug:
//
//   - Auctions. A square you decline stays with the bank instead of going up for
//     bidding. core.Game's Window() is the mechanism for this when it lands —
//     it is a timed window any player can act into, which is what an auction is.
//   - Mortgaging. Selling buildings back is in (build.go), so there *is* a way
//     to raise cash short of going out; mortgaging a bare deed is not.
//   - A forced sale when a rent falls due. See transfer() — you can sell ahead of
//     a bill you can see coming, but not to cover one that has already landed.
//   - Building only between your own turns. The original lets you build at any
//     moment, including during somebody else's turn, which needs a rule for who
//     gets the last house when two people want it at once.
//   - Trading. Nothing blocks the table while an offer is open, so this needs no
//     new phase — but it does need richer messages than core.ClientMsg carries.
//   - Bankruptcy pays the bank, not the creditor who broke you, and the loser's
//     deeds and buildings go back to the box rather than across the table.
