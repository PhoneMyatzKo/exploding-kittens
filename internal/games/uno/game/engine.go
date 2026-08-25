package game

// Apply validates and executes a single action, mutating s in place and
// returning the events it produced.
//
// s is mutated rather than copied: exactly one goroutine owns a given State (see
// internal/room), and deep-copying slices of slices on every keystroke is a bug
// farm with no upside here.
//
// On error, s is left untouched — every mutation happens after validation.
func Apply(s *State, a Action) ([]Event, error) {
	if s.Phase == PhaseGameOver {
		return nil, ErrGameOver
	}
	switch a.Kind {
	case ActPlay:
		return applyPlay(s, a)
	case ActDraw:
		return applyDraw(s, a)
	case ActPass:
		return applyPass(s, a)
	case ActColour:
		return applyColour(s, a)
	case ActCallUno:
		return applyCallUno(s, a)
	case ActCatchUno:
		return applyCatchUno(s, a)
	case ActChallenge:
		return applyWildFour(s, a, true)
	case ActAcceptDraw:
		return applyWildFour(s, a, false)
	case ActNextRound:
		return applyNextRound(s, a)
	}
	return nil, ErrWrongPhase
}

// ---------------------------------------------------------------- playing cards

func applyPlay(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseTurn && s.Phase != PhaseDrawn {
		return nil, ErrWrongPhase
	}
	p := s.current()
	if p.ID != a.PlayerID {
		return nil, ErrNotYourTurn
	}
	card, ok := p.findCard(a.CardID)
	if !ok {
		return nil, ErrNoSuchCard
	}
	// Having drawn, the only card you may play is the one you drew. The rest of
	// the hand was already declined the moment you reached for the deck.
	if s.Phase == PhaseDrawn && (s.Drawn == nil || s.Drawn.ID != card.ID) {
		return nil, ErrCantPlayDrawn
	}
	if !s.playable(card) {
		return nil, ErrNotPlayable
	}

	named := NoColour
	if a.Colour != "" {
		if !card.Rank.IsWild() {
			return nil, ErrNoColourYet
		}
		c, ok := ColourFromSlug(a.Colour)
		if !ok {
			return nil, ErrNeedColour
		}
		named = c
	}

	// Whether a Draw Four was a bluff is a question about the hand as it stands
	// at this instant, so it has to be answered before the card leaves it.
	illegal := card.Rank == RankWildDrawFour && p.hasColour(s.Colour)

	// Committed from here on.
	hand, played, _ := removeCard(p.Hand, card.ID)
	p.Hand = hand
	s.Discard = append(s.Discard, played)
	s.Drawn = nil

	events := []Event{{Kind: EvPlayed, ActorID: p.ID, Cards: []Card{played}, Count: len(p.Hand)}}
	events = append(events, s.declareUno(p, a.SayUno)...)

	if !played.Rank.IsWild() {
		s.Colour = played.Colour
		return append(events, s.resolveSymbol(p, played)...), nil
	}

	// Going out on a wild: no colour anybody names could ever be played on, so
	// the pick is skipped. A parting Draw Four still lands, because those four
	// cards count towards the score the round is about to be totted up on.
	if len(p.Hand) == 0 {
		if played.Rank == RankWildDrawFour {
			victim := s.Players[s.step(s.Current)]
			drawn, re := s.drawCards(victim, 4)
			events = append(events, s.reshuffleEvents(re)...)
			events = append(events, drawEvents(victim, drawn, "wild draw four")...)
		}
		return append(events, s.endRound(p)...), nil
	}

	s.Wild = &wildPlay{Card: played, ActorID: p.ID, Illegal: illegal}
	// No colour is in force between the card landing and a colour being named,
	// which is what stops anything at all being playable in the meantime.
	s.Colour = NoColour
	if named == NoColour {
		s.Phase = PhaseColour
		return events, nil
	}
	return append(events, s.chooseColour(named)...), nil
}

// resolveSymbol applies a coloured card's printed effect and hands the turn on.
// The number cards land here too: their effect is that there isn't one.
func (s *State) resolveSymbol(actor *Player, card Card) []Event {
	var events []Event
	// Computed before the turn moves, because "next" is about to mean somebody
	// else.
	victim := s.Players[s.step(s.Current)]

	switch card.Rank {
	case RankSkip:
		events = append(events, Event{Kind: EvSkipped, ActorID: victim.ID})
		s.advance(2)

	case RankReverse:
		// With two players a Reverse comes straight back to whoever played it, so
		// the rules simply call it a Skip. Flipping the direction as well is
		// harmless with two seats and keeps the arrow honest for a third joining
		// the next game.
		s.Dir = -s.Dir
		events = append(events, Event{Kind: EvReversed, ActorID: actor.ID})
		if len(s.Players) == 2 {
			events = append(events, Event{Kind: EvSkipped, ActorID: victim.ID})
			s.advance(2)
		} else {
			s.advance(1)
		}

	case RankDrawTwo:
		drawn, re := s.drawCards(victim, 2)
		events = append(events, s.reshuffleEvents(re)...)
		events = append(events, drawEvents(victim, drawn, "draw two")...)
		events = append(events, Event{Kind: EvSkipped, ActorID: victim.ID})
		s.advance(2)

	default:
		s.advance(1)
	}

	if len(actor.Hand) == 0 {
		return append(events, s.endRound(actor)...)
	}
	s.Phase = PhaseTurn
	return append(events, s.turnEvents()...)
}

// applyColour answers a PhaseColour prompt: the wild on the table gets a colour.
func applyColour(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseColour || s.Wild == nil {
		return nil, ErrWrongPhase
	}
	if a.PlayerID != s.Wild.ActorID {
		return nil, ErrNotYourTurn
	}
	c, ok := ColourFromSlug(a.Colour)
	if !ok {
		return nil, ErrNeedColour
	}
	return s.chooseColour(c), nil
}

// chooseColour puts a colour in force and carries on from wherever the wild left
// off: a plain Wild simply ends the turn, a Draw Four now has a victim to answer
// it, and the card the game was dealt on hands the turn to nobody because the
// first player hasn't had it yet.
func (s *State) chooseColour(c Colour) []Event {
	w := s.Wild
	s.Colour = c
	events := []Event{{Kind: EvColour, ActorID: w.ActorID, Colour: c.Slug()}}

	if w.Card.Rank == RankWildDrawFour {
		w.TargetID = s.Players[s.step(s.Current)].ID
		s.Phase = PhaseChallenge
		return append(events, Event{
			Kind: EvChallenge, ActorID: w.ActorID, TargetID: w.TargetID,
			Colour: c.Slug(), Text: "open",
		})
	}

	s.Wild = nil
	s.Phase = PhaseTurn
	if !w.opening {
		s.advance(1)
	}
	return append(events, s.turnEvents()...)
}

// applyWildFour settles a Draw Four: challenge it, or take the cards.
//
//	accepted            four cards, and you lose your turn
//	challenge, bluff    the player who tried it draws the four instead, and your
//	                    turn goes ahead as normal
//	challenge, honest   six cards, and you still lose your turn
func applyWildFour(s *State, a Action, challenge bool) ([]Event, error) {
	if s.Phase != PhaseChallenge || s.Wild == nil {
		return nil, ErrWrongPhase
	}
	if a.PlayerID != s.Wild.TargetID {
		return nil, ErrNotYourTurn
	}
	w := s.Wild
	s.Wild = nil
	actor := s.playerByID(w.ActorID)
	victim := s.playerByID(w.TargetID)
	if actor == nil || victim == nil {
		return nil, ErrWrongPhase
	}

	var events []Event
	switch {
	case !challenge:
		drawn, re := s.drawCards(victim, 4)
		events = append(events, s.reshuffleEvents(re)...)
		events = append(events, drawEvents(victim, drawn, "wild draw four")...)
		events = append(events, Event{Kind: EvSkipped, ActorID: victim.ID})
		s.advance(2)

	case w.Illegal:
		// Challenging earns you a look at the hand, and only you.
		events = append(events,
			Event{Kind: EvRevealed, ActorID: actor.ID, TargetID: victim.ID,
				Cards: append([]Card(nil), actor.Hand...), OnlyFor: victim.ID},
			Event{Kind: EvBluffed, ActorID: victim.ID, TargetID: actor.ID})
		drawn, re := s.drawCards(actor, 4)
		events = append(events, s.reshuffleEvents(re)...)
		events = append(events, drawEvents(actor, drawn, "caught bluffing")...)
		// The challenger's turn goes ahead: they neither draw nor lose it.
		s.advance(1)

	default:
		events = append(events,
			Event{Kind: EvRevealed, ActorID: actor.ID, TargetID: victim.ID,
				Cards: append([]Card(nil), actor.Hand...), OnlyFor: victim.ID},
			Event{Kind: EvBluffFailed, ActorID: victim.ID, TargetID: actor.ID})
		drawn, re := s.drawCards(victim, 6)
		events = append(events, s.reshuffleEvents(re)...)
		events = append(events, drawEvents(victim, drawn, "failed challenge")...)
		events = append(events, Event{Kind: EvSkipped, ActorID: victim.ID})
		s.advance(2)
	}

	s.Phase = PhaseTurn
	return append(events, s.turnEvents()...), nil
}

// ---------------------------------------------------------------------- drawing

func applyDraw(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseTurn {
		return nil, ErrWrongPhase
	}
	p := s.current()
	if p.ID != a.PlayerID {
		return nil, ErrNotYourTurn
	}

	drawn, re := s.drawCards(p, 1)
	events := s.reshuffleEvents(re)
	events = append(events, drawEvents(p, drawn, "")...)

	if len(drawn) == 0 {
		// Every card is in somebody's hand. Nothing to draw and nothing to
		// reshuffle, so pass the turn rather than wedge the table.
		s.advance(1)
		s.Phase = PhaseTurn
		return append(events, s.turnEvents()...), nil
	}

	// A playable draw leaves the turn open, which does tell the table that the
	// card was usable — the same tell a physical game gives when somebody stops
	// to think. What it never tells them is which card it was.
	if c := drawn[0]; s.playable(c) {
		s.Phase = PhaseDrawn
		s.Drawn = &c
		return events, nil
	}

	events = append(events, Event{Kind: EvPassed, ActorID: p.ID})
	s.advance(1)
	s.Phase = PhaseTurn
	return append(events, s.turnEvents()...), nil
}

// applyPass declines the card just drawn. Playing it is a right, not a duty.
func applyPass(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseDrawn {
		return nil, ErrWrongPhase
	}
	p := s.current()
	if p.ID != a.PlayerID {
		return nil, ErrNotYourTurn
	}
	s.Drawn = nil
	s.advance(1)
	s.Phase = PhaseTurn
	return append([]Event{{Kind: EvPassed, ActorID: p.ID}}, s.turnEvents()...), nil
}

// reshuffleEvents announces that the discard pile became the deck. Public, and
// it has to be: everybody watched it happen.
func (s *State) reshuffleEvents(happened bool) []Event {
	if !happened {
		return nil
	}
	return []Event{{Kind: EvReshuffled, Count: len(s.Draw)}}
}

// -------------------------------------------------------------------------- UNO

// declareUno opens the catch window when a play leaves somebody on one card, and
// closes any window that is now meaningless.
func (s *State) declareUno(p *Player, said bool) []Event {
	if len(p.Hand) != 1 {
		s.Uno = nil
		return nil
	}
	s.Uno = &unoWindow{PlayerID: p.ID, Called: said, openedOn: s.Turns}
	if !said {
		return nil
	}
	return []Event{{Kind: EvUnoCalled, ActorID: p.ID}}
}

func applyCallUno(s *State, a Action) ([]Event, error) {
	if s.Uno == nil || s.Uno.PlayerID != a.PlayerID {
		return nil, ErrNothingToCall
	}
	if s.Uno.Called {
		return nil, ErrAlreadyCalled
	}
	s.Uno.Called = true
	return []Event{{Kind: EvUnoCalled, ActorID: a.PlayerID}}, nil
}

func applyCatchUno(s *State, a Action) ([]Event, error) {
	if s.Uno == nil {
		return nil, ErrNoCatch
	}
	if s.Uno.Called {
		return nil, ErrAlreadyCalled
	}
	if s.Uno.PlayerID == a.PlayerID {
		return nil, ErrNoCatch
	}
	catcher := s.playerByID(a.PlayerID)
	victim := s.playerByID(s.Uno.PlayerID)
	if catcher == nil || victim == nil {
		return nil, ErrNoCatch
	}

	s.Uno = nil
	drawn, re := s.drawCards(victim, 2)
	events := []Event{{Kind: EvUnoCaught, ActorID: catcher.ID, TargetID: victim.ID}}
	events = append(events, s.reshuffleEvents(re)...)
	// Nobody's turn changes: a penalty is not a move.
	return append(events, drawEvents(victim, drawn, "uno penalty")...), nil
}

// ------------------------------------------------------------------- scoring

// endRound totals the hands nobody managed to empty and credits them to the
// player who did. Digits score their face value, coloured action cards twenty
// and either wild fifty.
func (s *State) endRound(winner *Player) []Event {
	points := 0
	for _, p := range s.Players {
		if p != winner {
			points += handPoints(p.Hand)
		}
	}
	winner.Score += points
	s.RoundWinnerID = winner.ID
	s.Uno = nil
	s.Drawn = nil
	s.Wild = nil

	events := []Event{{Kind: EvRoundOver, ActorID: winner.ID, Points: points}}
	if s.Target == 0 || winner.Score >= s.Target {
		s.Phase = PhaseGameOver
		s.WinnerID = winner.ID
		return append(events, Event{Kind: EvGameOver, ActorID: winner.ID, Points: winner.Score})
	}
	s.Phase = PhaseRoundOver
	return events
}

// applyNextRound deals the following hand. Whoever went out starts it, which is
// the closest thing this game has to the winner dealing.
func applyNextRound(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseRoundOver {
		return nil, ErrRoundLive
	}
	if s.playerByID(a.PlayerID) == nil {
		return nil, ErrWrongPhase
	}
	first := 0
	for i, p := range s.Players {
		if p.ID == s.RoundWinnerID {
			first = i
			break
		}
	}
	return s.deal(first), nil
}
