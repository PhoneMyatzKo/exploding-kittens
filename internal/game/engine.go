package game

// Apply validates and executes a single action, mutating s in place and returning
// the events it produced.
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
	case ActNope:
		return applyNope(s, a)
	case ActPass:
		return applyPass(s, a)
	case ActNopeExpired:
		return applyNopeExpired(s)
	case ActGiveCard:
		return applyGiveCard(s, a)
	case ActPlaceKitten:
		return applyPlaceKitten(s, a)
	}
	return nil, ErrWrongPhase
}

// ---------------------------------------------------------------- playing cards

func applyPlay(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseTurn {
		return nil, ErrWrongPhase
	}
	p := s.current()
	if p.ID != a.PlayerID {
		return nil, ErrNotYourTurn
	}
	if len(a.CardIDs) == 0 || len(a.CardIDs) > 2 {
		return nil, ErrNotPlayable
	}

	// Resolve the requested cards without committing, so a bad request leaves the
	// hand alone.
	cards := make([]Card, 0, len(a.CardIDs))
	seen := map[int]bool{}
	for _, id := range a.CardIDs {
		if seen[id] {
			return nil, ErrNoSuchCard
		}
		seen[id] = true
		var found bool
		for _, c := range p.Hand {
			if c.ID == id {
				cards = append(cards, c)
				found = true
				break
			}
		}
		if !found {
			return nil, ErrNoSuchCard
		}
	}

	pend := &PendingAction{ActorID: p.ID, Cards: cards, Passed: map[string]bool{}}

	if len(cards) == 2 {
		// The only legal two-card play in the base game is a matching cat pair.
		if !cards[0].Type.IsCat() || cards[0].Type != cards[1].Type {
			return nil, ErrBadCatPair
		}
		t := s.playerByID(a.TargetID)
		if t == nil || !t.Alive || t.ID == p.ID {
			return nil, ErrBadTarget
		}
		pend.Kind = PendCatPair
		pend.TargetID = t.ID
	} else {
		switch cards[0].Type {
		case Skip:
			pend.Kind = PendSkip
		case Attack:
			pend.Kind = PendAttack
		case Shuffle:
			pend.Kind = PendShuffle
		case SeeTheFuture:
			pend.Kind = PendFuture
		case Favor:
			t := s.playerByID(a.TargetID)
			if t == nil || !t.Alive || t.ID == p.ID {
				return nil, ErrBadTarget
			}
			pend.Kind = PendFavor
			pend.TargetID = t.ID
		default:
			// Defuse, Nope, a lone cat, or a Kitten.
			return nil, ErrNotPlayable
		}
	}

	// Committed from here on.
	for _, c := range cards {
		s.discardFrom(p, c.ID)
	}
	pend.LastPlayerID = p.ID
	s.Pending = pend
	s.Phase = PhaseNope

	events := []Event{{Kind: EvPlayed, ActorID: p.ID, TargetID: pend.TargetID, Cards: cards}}
	return append(events, s.settleWindow()...), nil
}

// discardFrom moves one card from a player's hand to the top of the discard pile.
func (s *State) discardFrom(p *Player, cardID int) {
	hand, c, ok := removeCard(p.Hand, cardID)
	if !ok {
		return
	}
	p.Hand = hand
	s.Discard = append(s.Discard, c)
}

// ------------------------------------------------------------- the Nope window

// eligibleNopers lists the living players who could still stack a Nope: they must
// hold one, and they can't Nope their own most recent card.
func (s *State) eligibleNopers() []*Player {
	var out []*Player
	for _, p := range s.Players {
		if p.Alive && p.ID != s.Pending.LastPlayerID && p.hasType(Nope) {
			out = append(out, p)
		}
	}
	return out
}

// settleWindow either resolves the pending action immediately (nobody left who
// could Nope, or everyone eligible has passed) or announces that the window is
// open so clients can start their countdown.
func (s *State) settleWindow() []Event {
	eligible := s.eligibleNopers()
	open := false
	for _, p := range eligible {
		if !s.Pending.Passed[p.ID] {
			open = true
			break
		}
	}
	if open {
		return []Event{{Kind: EvNopeWindow, ActorID: s.Pending.ActorID}}
	}
	return s.resolvePending()
}

func applyNope(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseNope || s.Pending == nil {
		return nil, ErrWrongPhase
	}
	p := s.playerByID(a.PlayerID)
	if p == nil || !p.Alive {
		return nil, ErrDead
	}
	if p.ID == s.Pending.LastPlayerID {
		return nil, ErrWrongPhase
	}
	nope, ok := p.findType(Nope)
	if !ok {
		return nil, ErrNoNopeCard
	}

	s.discardFrom(p, nope.ID)
	s.Pending.Nopes++
	s.Pending.LastPlayerID = p.ID
	// A fresh card resets the window: everyone gets another chance to Yup it.
	s.Pending.Passed = map[string]bool{}

	events := []Event{{Kind: EvNoped, ActorID: p.ID, Cards: []Card{nope}}}
	return append(events, s.settleWindow()...), nil
}

func applyPass(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseNope || s.Pending == nil {
		return nil, ErrWrongPhase
	}
	p := s.playerByID(a.PlayerID)
	if p == nil || !p.Alive {
		return nil, ErrDead
	}
	if s.Pending.Passed[p.ID] {
		return nil, ErrAlreadyPassed
	}
	s.Pending.Passed[p.ID] = true
	return s.settleWindow(), nil
}

// applyNopeExpired is submitted by the room's timer, not by a client.
func applyNopeExpired(s *State) ([]Event, error) {
	if s.Phase != PhaseNope || s.Pending == nil {
		return nil, ErrWrongPhase
	}
	return s.resolvePending(), nil
}

// resolvePending closes the window and either performs the effect or discards it,
// depending on the parity of the Nope stack.
func (s *State) resolvePending() []Event {
	pend := s.Pending
	s.Pending = nil
	s.Phase = PhaseTurn

	if pend.Cancelled() {
		return []Event{{Kind: EvCancelled, ActorID: pend.ActorID, Cards: pend.Cards}}
	}

	actor := s.playerByID(pend.ActorID)
	events := []Event{{Kind: EvResolved, ActorID: pend.ActorID, TargetID: pend.TargetID, Cards: pend.Cards}}

	switch pend.Kind {
	case PendSkip:
		// Ends one turn without drawing. Under Attack that burns only one of the
		// two turns owed, which falls out of endTurn for free.
		s.endTurn()

	case PendAttack:
		// The attacker forfeits every turn they still owe; the next player takes
		// those plus two more.
		carried := s.TurnsRemaining - 1
		if carried < 0 {
			carried = 0
		}
		s.advance()
		s.TurnsRemaining = carried + 2

	case PendShuffle:
		shuffle(s.Draw, s.rng)
		events = append(events, Event{Kind: EvShuffled, ActorID: pend.ActorID})

	case PendFuture:
		top := peekTop(s.Draw, 3)
		events = append(events, Event{
			Kind: EvFuture, ActorID: pend.ActorID, Cards: top, OnlyFor: pend.ActorID,
		})

	case PendFavor:
		target := s.playerByID(pend.TargetID)
		if target == nil || !target.Alive || len(target.Hand) == 0 {
			break // nothing to give; the Favor simply fizzles
		}
		s.Phase = PhaseFavor
		s.FavorRequester = pend.ActorID
		s.Pending = &PendingAction{Kind: PendFavor, ActorID: pend.ActorID, TargetID: target.ID}

	case PendCatPair:
		target := s.playerByID(pend.TargetID)
		if target == nil || !target.Alive || len(target.Hand) == 0 {
			break
		}
		i := s.rng.Intn(len(target.Hand))
		stolen := target.Hand[i]
		target.Hand = append(target.Hand[:i], target.Hand[i+1:]...)
		actor.Hand = append(actor.Hand, stolen)
		// The stolen card's identity is private to the two players involved.
		events = append(events,
			Event{Kind: EvStole, ActorID: actor.ID, TargetID: target.ID},
			Event{Kind: EvStole, ActorID: actor.ID, TargetID: target.ID, Cards: []Card{stolen}, OnlyFor: actor.ID},
			Event{Kind: EvStole, ActorID: actor.ID, TargetID: target.ID, Cards: []Card{stolen}, OnlyFor: target.ID},
		)
	}

	return append(events, s.turnEvents()...)
}

// turnEvents emits a turn marker whenever the game is back in a state where the
// active player is expected to act.
func (s *State) turnEvents() []Event {
	if s.Phase != PhaseTurn {
		return nil
	}
	return []Event{{Kind: EvTurn, ActorID: s.current().ID}}
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
	if len(s.Draw) == 0 {
		// Can't happen with a correct setup — kittens run out before the deck
		// does — but refuse rather than panic if it ever does.
		return nil, ErrWrongPhase
	}

	card := s.Draw[0]
	s.Draw = s.Draw[1:]
	events := []Event{{Kind: EvDrew, ActorID: p.ID}}

	if card.Type != ExplodingKitten {
		p.Hand = append(p.Hand, card)
		events = append(events, Event{Kind: EvDrew, ActorID: p.ID, Cards: []Card{card}, OnlyFor: p.ID})
		s.endTurn()
		return append(events, s.turnEvents()...), nil
	}

	events = append(events, Event{Kind: EvExploded, ActorID: p.ID, Cards: []Card{card}})

	defuse, ok := p.findType(Defuse)
	if !ok {
		// No Defuse: the player is out, and their whole hand hits the discard.
		p.Alive = false
		s.Discard = append(s.Discard, card)
		s.Discard = append(s.Discard, p.Hand...)
		p.Hand = nil
		events = append(events, Event{Kind: EvEliminated, ActorID: p.ID})

		if s.checkWin() {
			return append(events, Event{Kind: EvGameOver, ActorID: s.WinnerID}), nil
		}
		// The eliminated player's remaining turn debt dies with them.
		s.Current = s.nextAliveAfter(s.Current)
		s.TurnsRemaining = 1
		return append(events, s.turnEvents()...), nil
	}

	// Playing the Defuse is mandatory and free of choice, so it is automatic; the
	// only decision is where the Kitten goes back.
	s.discardFrom(p, defuse.ID)
	kitten := card
	s.PendingKitten = &kitten
	s.Phase = PhaseDefuse
	events = append(events, Event{Kind: EvDefused, ActorID: p.ID, Cards: []Card{defuse}})
	return events, nil
}

func applyPlaceKitten(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseDefuse || s.PendingKitten == nil {
		return nil, ErrWrongPhase
	}
	p := s.current()
	if p.ID != a.PlayerID {
		return nil, ErrNotYourTurn
	}

	s.Draw = insertAt(s.Draw, a.Index, *s.PendingKitten)
	s.PendingKitten = nil
	s.Phase = PhaseTurn
	// Drawing is what ends a turn, and the Kitten was drawn.
	s.endTurn()

	events := []Event{{Kind: EvDefused, ActorID: p.ID, Text: "hid the kitten back in the deck"}}
	return append(events, s.turnEvents()...), nil
}

// ------------------------------------------------------------------------ Favor

func applyGiveCard(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseFavor || s.Pending == nil {
		return nil, ErrWrongPhase
	}
	if a.PlayerID != s.Pending.TargetID {
		return nil, ErrWrongPhase
	}
	if len(a.CardIDs) != 1 {
		return nil, ErrNoSuchCard
	}

	giver := s.playerByID(a.PlayerID)
	receiver := s.playerByID(s.FavorRequester)
	if giver == nil || receiver == nil {
		return nil, ErrWrongPhase
	}
	hand, card, ok := removeCard(giver.Hand, a.CardIDs[0])
	if !ok {
		return nil, ErrNoSuchCard
	}
	giver.Hand = hand
	receiver.Hand = append(receiver.Hand, card)

	s.Pending = nil
	s.FavorRequester = ""
	s.Phase = PhaseTurn // Favor does not end the requester's turn

	events := []Event{
		{Kind: EvGave, ActorID: giver.ID, TargetID: receiver.ID},
		{Kind: EvGave, ActorID: giver.ID, TargetID: receiver.ID, Cards: []Card{card}, OnlyFor: receiver.ID},
		{Kind: EvGave, ActorID: giver.ID, TargetID: receiver.ID, Cards: []Card{card}, OnlyFor: giver.ID},
	}
	return append(events, s.turnEvents()...), nil
}
