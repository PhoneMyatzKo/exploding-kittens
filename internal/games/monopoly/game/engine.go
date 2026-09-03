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
	s := &State{Phase: PhaseRoll, RNG: rng}
	for _, seat := range seats {
		s.Players = append(s.Players, Player{
			ID: seat.ID, Name: seat.Name, Cash: StartingCash, Alive: true,
		})
	}
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
		return applyRoll(s, p)
	case ActBuy:
		return applyBuy(s, p)
	case ActPass:
		return applyPass(s, p)
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
	p.Pos = (p.Pos + steps) % BoardSize
	events := []Event{{Kind: EvMoved, ActorID: p.ID, Tile: p.Pos, Amount: steps}}

	// Round the corner and the bank pays you. Checked by comparing positions
	// rather than by a flag, so a card that moves you backwards past GO — which
	// the original has — does not pay out by accident.
	if p.Pos < from {
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
		paid, events := pay(s, p, nil, tile.Tax)
		return append([]Event{{Kind: EvTax, ActorID: p.ID, Tile: p.Pos, Amount: paid}}, events...)

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
			paid, events := pay(s, p, s.Find(owner), rentOn(s, p.Pos))
			return append([]Event{{
				Kind: EvRent, ActorID: p.ID, TargetID: owner, Tile: p.Pos, Amount: paid,
			}}, events...)
		}
	}

	// Nothing to settle: GO, Just Visiting, Free Parking, a square you already
	// own, and — until the decks land — Chance and Community Chest.
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
		rent := tile.Rent[0]
		// A complete colour set doubles the unimproved rent. This is the whole
		// reason anybody trades, and it is in the slice for that reason.
		if s.ownsGroup(owner, tile.Group) {
			rent *= 2
		}
		return rent
	}
}

// pay moves money and takes a player out if they cannot cover it. A nil creditor
// is the bank.
//
// Returns what actually changed hands, which is not always what was owed — the
// caller reports the real figure rather than the demand. Every amount in the
// play-by-play is therefore money that moved.
//
// Bankruptcy here is simpler than the original: everything the loser held goes
// back to the bank rather than to whoever broke them, and there is no selling
// houses or mortgaging to try to survive. Both need rules this slice does not
// have yet.
func pay(s *State, from *Player, to *Player, amount int) (int, []Event) {
	if amount <= 0 {
		return 0, endTurn(s, from)
	}
	if from.Cash >= amount {
		from.Cash -= amount
		if to != nil {
			to.Cash += amount
		}
		return amount, endTurn(s, from)
	}

	// Everything they have goes across, then they are out.
	paid := from.Cash
	from.Cash = 0
	if to != nil {
		to.Cash += paid
	}
	return paid, bankrupt(s, from, to)
}

func bankrupt(s *State, p *Player, to *Player) []Event {
	p.Alive = false
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

func sendToJail(s *State, p *Player) []Event {
	p.Pos = JailTile
	// Doubles cleared first: three of them is what got you here, and the extra
	// roll they would otherwise earn is the thing being taken away.
	s.Doubles = 0
	events := []Event{{Kind: EvJailed, ActorID: p.ID, Tile: JailTile}}
	// Deliberately not "and you lose your next turn": until jail has rules, this
	// is a move to the corner and nothing more. Flagged in the gap list below so
	// it does not read as a bug.
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
	s.advance()
	return []Event{{Kind: EvTurn, ActorID: s.CurrentID(), Tile: -1}}
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
//   - Chance and Community Chest. Both decks' squares are on the board and do
//     nothing when you land on them.
//   - Jail. Go To Jail moves you to the corner; it does not hold you there, so
//     there is no fee, no doubles-to-escape and no Get Out of Jail Free.
//   - Auctions. A square you decline stays with the bank instead of going up for
//     bidding. core.Game's Window() is the mechanism for this when it lands —
//     it is a timed window any player can act into, which is what an auction is.
//   - Houses, hotels and the building shortage.
//   - Mortgaging, and selling buildings to raise cash rather than going bankrupt.
//   - Trading. Nothing blocks the table while an offer is open, so this needs no
//     new phase — but it does need richer messages than core.ClientMsg carries.
//   - Bankruptcy pays the bank, not the creditor who broke you.
