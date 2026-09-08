package game

// Drawing and resolving a Chance or Community Chest card, and the jail the cards
// can put you in.
//
// Kept out of engine.go because it is a self-contained half of the rules: a card
// is drawn, read, and then does one or two things to you. The interesting part is
// the order — see resolveDrawn.

// shuffleDecks fills both piles and shuffles them. Called at the deal.
func (s *State) shuffleDecks() {
	s.ChanceDraw = deckOf(DeckChance)
	s.ChestDraw = deckOf(DeckChest)
	s.ChanceDiscard = nil
	s.ChestDiscard = nil
	s.Drawn = -1
	s.RNG.Shuffle(len(s.ChanceDraw), func(i, j int) {
		s.ChanceDraw[i], s.ChanceDraw[j] = s.ChanceDraw[j], s.ChanceDraw[i]
	})
	s.RNG.Shuffle(len(s.ChestDraw), func(i, j int) {
		s.ChestDraw[i], s.ChestDraw[j] = s.ChestDraw[j], s.ChestDraw[i]
	})
}

// drawFrom takes the top card of a pile, refilling from its discards when the
// pile is empty — the way a table does when the deck runs out. Returns -1 only if
// every card of that deck is being held, which the pardon makes possible.
func (s *State) drawFrom(d Deck) int {
	draw, discard := &s.ChanceDraw, &s.ChanceDiscard
	if d == DeckChest {
		draw, discard = &s.ChestDraw, &s.ChestDiscard
	}

	if len(*draw) == 0 {
		*draw, *discard = *discard, nil
		s.RNG.Shuffle(len(*draw), func(i, j int) {
			(*draw)[i], (*draw)[j] = (*draw)[j], (*draw)[i]
		})
	}
	if len(*draw) == 0 {
		return -1
	}
	card := (*draw)[0]
	*draw = (*draw)[1:]
	return card
}

// discardTo puts a spent card at the back of its pile's discards.
func (s *State) discardTo(d Deck, card int) {
	if d == DeckChest {
		s.ChestDiscard = append(s.ChestDiscard, card)
		return
	}
	s.ChanceDiscard = append(s.ChanceDiscard, card)
}

// drawCard turns a card face up in front of the current player and stops there.
// Nothing happens until they acknowledge it, which is what PhaseCard is for: a
// card that resolved silently would leave the log as the only place anybody
// learned what it said.
func (s *State) drawCard(p *Player, d Deck) []Event {
	idx := s.drawFrom(d)
	if idx < 0 {
		// Every card of this deck is in somebody's hand. Nothing to draw, so the
		// square does nothing — which is better than inventing a card.
		return endTurn(s, p)
	}

	// A pardon is kept rather than read out and resolved.
	if cards[idx].Held() {
		p.Pardons++
		return append(
			[]Event{{Kind: EvCard, ActorID: p.ID, Tile: p.Pos, Card: idx}},
			endTurn(s, p)...,
		)
	}

	s.Drawn = idx
	s.Phase = PhaseCard
	return []Event{{Kind: EvCard, ActorID: p.ID, Tile: p.Pos, Card: idx}}
}

// resolveDrawn applies the card in front of the current player and moves the
// table on. Submitted by them, or by the room's watchdog if they have gone away —
// there is no choice to make, only an acknowledgement.
func resolveDrawn(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseCard {
		return nil, ErrWrongPhase
	}
	idx := s.Drawn
	if idx < 0 || idx >= len(cards) {
		s.Drawn = -1
		return endTurn(s, p), nil
	}
	card := cards[idx]
	s.Drawn = -1
	s.Phase = PhaseRoll // cleared before the effects decide where play goes
	s.discardTo(card.Deck, idx)

	var events []Event
	for _, e := range card.Effects {
		done, evs := applyEffect(s, p, e)
		events = append(events, evs...)
		// A card that put them in jail, bankrupted them or moved them onto
		// something that needs answering has already decided where the table
		// goes. Anything after it on the card is spent.
		if done {
			return events, nil
		}
	}
	return append(events, endTurn(s, p)...), nil
}

// applyEffect does one line of a card. The bool is "this has settled the turn
// already" — a move resolves whatever it lands on, and that can end the turn, ask
// a question, or end the game.
func applyEffect(s *State, p *Player, e Effect) (bool, []Event) {
	switch e.Kind {
	case EffMoney:
		if e.Money >= 0 {
			p.Cash += e.Money
			return false, []Event{{Kind: EvCardPay, ActorID: p.ID, Amount: e.Money, Tile: -1}}
		}
		owed := -e.Money
		// The expired-licence card: if you cannot cover it you are arrested
		// rather than ruined.
		if e.OrJail && p.Cash < owed {
			return true, append(
				[]Event{{Kind: EvCardPay, ActorID: p.ID, Amount: 0, Tile: -1}},
				sendToJail(s, p)...,
			)
		}
		paid, settled, evs := transfer(s, p, nil, owed)
		// settled only when it broke them. Otherwise the card carries on — the
		// flat tyre costs money *and* a square, and paying is not the whole card.
		return settled, append(
			[]Event{{Kind: EvCardPay, ActorID: p.ID, Amount: -paid, Tile: -1}}, evs...)

	case EffMove:
		return true, moveBy(s, p, e.Steps)

	case EffSkip:
		p.Missing += e.Turns
		return false, []Event{{Kind: EvMissTurn, ActorID: p.ID, Amount: e.Turns, Tile: -1}}

	case EffJail:
		return true, sendToJail(s, p)

	case EffRelease:
		if !p.Jailed {
			return false, nil
		}
		p.Jailed, p.Tries = false, 0
		return false, []Event{{Kind: EvFreed, ActorID: p.ID, Tile: JailTile}}

	case EffPardon:
		p.Pardons++
		return false, nil
	}
	return false, nil
}

// ─────────────────────────────────────────────────────────────────────── jail

// applyPayFine buys the way out. Legal only while being held.
func applyPayFine(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseJail || !p.Jailed {
		return nil, ErrWrongPhase
	}
	if p.Cash < JailFine {
		return nil, ErrCantAfford
	}
	paid, settled, evs := transfer(s, p, nil, JailFine)
	out := append([]Event{{Kind: EvFine, ActorID: p.ID, Amount: paid, Tile: JailTile}}, evs...)
	if settled {
		// The fine broke them. The table has already moved on.
		return out, nil
	}
	p.Jailed, p.Tries = false, 0
	// Out, and free to roll — this turn. Buying your way out is not meant to cost
	// you the turn you just bought.
	s.Phase = PhaseRoll
	return append(out, Event{Kind: EvFreed, ActorID: p.ID, Tile: JailTile}), nil
}

// applyUsePardon spends a held card. The card goes back to the bottom of its own
// pile, the way it does on a table.
func applyUsePardon(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseJail || !p.Jailed {
		return nil, ErrWrongPhase
	}
	if p.Pardons < 1 {
		return nil, ErrNoPardon
	}
	p.Pardons--
	p.Jailed, p.Tries = false, 0
	s.Phase = PhaseRoll
	// Back into the deck it came from, so it can be drawn again.
	for i, c := range cards {
		if c.Held() {
			s.discardTo(c.Deck, i)
			break
		}
	}
	return []Event{{Kind: EvPardonUsed, ActorID: p.ID, Tile: JailTile}}, nil
}

// rollInJail is a turn spent trying to throw doubles. Three failures and the fine
// is taken and you are let out anyway, which is the original's rule and the thing
// that stops a bad run lasting the whole game.
func rollInJail(s *State, p *Player) ([]Event, error) {
	if s.Phase != PhaseJail || !p.Jailed {
		return nil, ErrWrongPhase
	}
	die1, die2 := s.RNG.Intn(6)+1, s.RNG.Intn(6)+1
	s.Dice = [2]int{die1, die2}
	events := []Event{{Kind: EvRolled, ActorID: p.ID, Dice: s.Dice, Tile: -1}}

	if die1 == die2 {
		p.Jailed, p.Tries = false, 0
		s.Phase = PhaseRoll
		events = append(events, Event{Kind: EvFreed, ActorID: p.ID, Tile: JailTile})
		// Out on a double, and the throw is the move — but it does not earn
		// another go, which is what the original says and what stops a lucky
		// escape becoming a free lap.
		s.Doubles = 0
		return append(events, moveBy(s, p, die1+die2)...), nil
	}

	p.Tries++
	if p.Tries < JailAttempts {
		// Still held. The turn is over.
		events = append(events, s.advance()...)
		return append(events, Event{Kind: EvTurn, ActorID: s.CurrentID(), Tile: -1}), nil
	}

	// Out of attempts: the fine is taken and you are let out anyway.
	paid, settled, evs := transfer(s, p, nil, JailFine)
	events = append(events, Event{Kind: EvFine, ActorID: p.ID, Amount: paid, Tile: JailTile})
	events = append(events, evs...)
	if settled {
		return events, nil
	}
	p.Jailed, p.Tries = false, 0
	s.Phase = PhaseRoll
	s.Doubles = 0
	return append(events, moveBy(s, p, die1+die2)...), nil
}
