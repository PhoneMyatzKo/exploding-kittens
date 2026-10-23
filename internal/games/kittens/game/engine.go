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
	case ActAlterFuture:
		return applyAlterFuture(s, a)
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
	if len(a.CardIDs) == 0 || len(a.CardIDs) > 3 {
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

	if len(cards) > 1 {
		// Multi-card plays are matching cat sets: two steal at random, three let
		// you name what you want.
		if !matchingCats(cards) {
			return nil, ErrBadCatSet
		}
		t := s.playerByID(a.TargetID)
		if t == nil || !t.Alive || t.ID == p.ID {
			return nil, ErrBadTarget
		}
		pend.TargetID = t.ID

		if len(cards) == 2 {
			pend.Kind = PendCatPair
		} else {
			named, ok := TypeFromSlug(a.Named)
			if !ok {
				return nil, ErrNoNamedCard
			}
			pend.Kind = PendCatTriple
			pend.Named = named
			pend.HasNamed = true
		}
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
		case Reverse:
			pend.Kind = PendReverse
		case DrawFromBottom:
			pend.Kind = PendBottom
		case AlterTheFuture:
			pend.Kind = PendAlter
		case Favor, TargetedAttack:
			// Both name a victim. Yourself is not a legal choice for either: a
			// Favor from yourself is a no-op, and the printed Targeted Attack is
			// there to point the turns at somebody else.
			t := s.playerByID(a.TargetID)
			if t == nil || !t.Alive || t.ID == p.ID {
				return nil, ErrBadTarget
			}
			pend.TargetID = t.ID
			if cards[0].Type == Favor {
				pend.Kind = PendFavor
			} else {
				pend.Kind = PendTargeted
			}
		default:
			// Defuse, Nope, a lone cat (Feral included), or a kitten.
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

// matchingCats reports whether the cards form a playable set: every one of them
// takes part in cat combos, and they all agree on which cat they are.
//
// The Feral Cat is a wildcard, so it agrees with anything — which makes a set of
// nothing but Feral Cats legal too, since they can all stand in for the same
// kind.
func matchingCats(cards []Card) bool {
	if len(cards) < 2 {
		return false
	}
	kind := CardType(-1)
	for _, c := range cards {
		if !c.Type.IsCatLike() {
			return false
		}
		if c.Type == FeralCat {
			continue
		}
		if kind != -1 && c.Type != kind {
			return false
		}
		kind = c.Type
	}
	return true
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

	case PendTargeted:
		// Attack, but the turns are pointed at somebody chosen rather than at
		// whoever happens to be next.
		target := s.playerByID(pend.TargetID)
		if target == nil || !target.Alive {
			break // the victim was eliminated while the window was open
		}
		carried := s.TurnsRemaining - 1
		if carried < 0 {
			carried = 0
		}
		for i, q := range s.Players {
			if q.ID == target.ID {
				s.Current = i
			}
		}
		s.TurnsRemaining = carried + 2

	case PendReverse:
		// The direction flips first and the turn ends second, so play passes to
		// whoever is now next rather than to whoever was next before.
		s.Direction = -s.step()
		events = append(events, Event{Kind: EvReversed, ActorID: pend.ActorID})
		s.endTurn()

	case PendBottom:
		// This *is* the turn-ending draw, just taken from the other end — and it
		// can explode exactly like a normal one.
		if len(s.Draw) == 0 {
			break
		}
		events = append(events, s.drawInto(actor, true)...)

	case PendAlter:
		if len(s.Draw) == 0 {
			break // nothing to rearrange
		}
		s.Phase = PhaseAlter
		s.Altering = pend.ActorID

	case PendShuffle:
		shuffle(s.Draw, s.RNG)
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
		i := s.RNG.Intn(len(target.Hand))
		stolen := target.Hand[i]
		target.Hand = append(target.Hand[:i], target.Hand[i+1:]...)
		actor.Hand = append(actor.Hand, stolen)
		// The stolen card's identity is private to the two players involved.
		events = append(events,
			Event{Kind: EvStole, ActorID: actor.ID, TargetID: target.ID},
			Event{Kind: EvStole, ActorID: actor.ID, TargetID: target.ID, Cards: []Card{stolen}, OnlyFor: actor.ID},
			Event{Kind: EvStole, ActorID: actor.ID, TargetID: target.ID, Cards: []Card{stolen}, OnlyFor: target.ID},
		)

	case PendCatTriple:
		// Unlike a pair, this is all public: the demand is made out loud and the
		// table sees whether it was met. The card is named in Text rather than
		// carried in Cards, so a specific card's identity still never travels to
		// players who are not holding it.
		events = append(events, Event{
			Kind: EvDemanded, ActorID: pend.ActorID, TargetID: pend.TargetID,
			Text: pend.Named.String(),
		})

		target := s.playerByID(pend.TargetID)
		if target == nil || !target.Alive {
			break
		}
		got, ok := target.findType(pend.Named)
		if !ok {
			events = append(events, Event{
				Kind: EvMissed, ActorID: pend.ActorID, TargetID: pend.TargetID,
				Text: pend.Named.String(),
			})
			break
		}
		hand, card, _ := removeCard(target.Hand, got.ID)
		target.Hand = hand
		actor.Hand = append(actor.Hand, card)
		events = append(events, Event{
			Kind: EvStole, ActorID: actor.ID, TargetID: target.ID, Text: card.Name,
		})
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
	return append(s.drawInto(p, false), s.turnEvents()...), nil
}

// drawInto takes a card off the deck and deals with whatever it turns out to be.
// Shared by the ordinary draw and by Draw From the Bottom, which is the same act
// from the other end of the pile and carries exactly the same risk.
//
// Returns without a turn marker: the caller appends one, so that a draw resolved
// inside a Nope window does not emit two.
func (s *State) drawInto(p *Player, fromBottom bool) []Event {
	var card Card
	if fromBottom {
		card = s.Draw[len(s.Draw)-1]
		s.Draw = s.Draw[:len(s.Draw)-1]
	} else {
		card = s.Draw[0]
		s.Draw = s.Draw[1:]
	}
	events := []Event{{Kind: EvDrew, ActorID: p.ID}}

	if !card.Type.Kills() {
		p.Hand = append(p.Hand, card)
		events = append(events, Event{Kind: EvDrew, ActorID: p.ID, Cards: []Card{card}, OnlyFor: p.ID})
		s.endTurn()
		return events
	}

	// The Imploding Kitten's first appearance is a warning, not a death: it goes
	// back into the deck face up, costs no Defuse, and ends the turn. Nobody can
	// stop it and nobody chooses to play it, so there is no Nope window.
	if card.Type == ImplodingKitten && !card.FaceUp {
		kitten := card
		kitten.FaceUp = true
		s.PendingKitten = &kitten
		s.Phase = PhaseDefuse
		return append(events, Event{Kind: EvArmed, ActorID: p.ID, Cards: []Card{card}})
	}

	events = append(events, Event{Kind: EvExploded, ActorID: p.ID, Cards: []Card{card}})

	// A Defuse is no help against the Imploding Kitten — that is the whole point
	// of it. Against an Exploding Kitten it is mandatory and automatic, since
	// there is no decision in it; the only choice is where the kitten goes back.
	if defuse, ok := p.findType(Defuse); ok && card.Type == ExplodingKitten {
		s.discardFrom(p, defuse.ID)
		kitten := card
		s.PendingKitten = &kitten
		s.Phase = PhaseDefuse
		return append(events, Event{Kind: EvDefused, ActorID: p.ID, Cards: []Card{defuse}})
	}

	return append(events, s.eliminate(p, card)...)
}

// eliminate takes a player out of the game, discarding the card that got them
// along with their whole hand. Returns without a turn marker; the caller appends
// one, and it is nil once the game is over.
func (s *State) eliminate(p *Player, by Card) []Event {
	p.Alive = false
	s.Discard = append(s.Discard, by)
	s.Discard = append(s.Discard, p.Hand...)
	p.Hand = nil
	events := []Event{{Kind: EvEliminated, ActorID: p.ID}}

	if s.checkWin() {
		return append(events, Event{Kind: EvGameOver, ActorID: s.WinnerID})
	}
	// The eliminated player's remaining turn debt dies with them.
	s.Current = s.nextAliveAfter(s.Current)
	s.TurnsRemaining = 1
	return events
}

func applyPlaceKitten(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseDefuse || s.PendingKitten == nil {
		return nil, ErrWrongPhase
	}
	p := s.current()
	if p.ID != a.PlayerID {
		return nil, ErrNotYourTurn
	}

	kitten := *s.PendingKitten
	s.Draw = insertAt(s.Draw, a.Index, kitten)
	s.PendingKitten = nil
	s.Phase = PhaseTurn
	// Drawing is what ends a turn, and the kitten was drawn.
	s.endTurn()

	// Where it went is secret either way. What differs is what the table now
	// knows: a defused Exploding Kitten is one of several, while the Imploding
	// Kitten is now armed and will take whoever reaches it.
	line := Event{Kind: EvDefused, ActorID: p.ID, Text: "hid the kitten back in the deck"}
	if kitten.Type == ImplodingKitten {
		line = Event{Kind: EvArmed, ActorID: p.ID,
			Text: "buried the Imploding Kitten face up — no Defuse will stop it"}
	}
	return append([]Event{line}, s.turnEvents()...), nil
}

// ------------------------------------------------------- Alter the Future

func applyAlterFuture(s *State, a Action) ([]Event, error) {
	if s.Phase != PhaseAlter {
		return nil, ErrWrongPhase
	}
	if a.PlayerID != s.Altering {
		return nil, ErrWrongPhase
	}

	// Order is a permutation of positions, not card IDs — see Action.Order.
	k := s.alterCount()
	if len(a.Order) != k {
		return nil, ErrBadOrder
	}
	seen := make([]bool, k)
	for _, pos := range a.Order {
		if pos < 0 || pos >= k || seen[pos] {
			return nil, ErrBadOrder
		}
		seen[pos] = true
	}

	// Committed from here on.
	top := make([]Card, k)
	for i, pos := range a.Order {
		top[i] = s.Draw[pos]
	}
	copy(s.Draw[:k], top)

	s.Phase = PhaseTurn
	s.Altering = ""
	// Public that it happened, private what it did: the whole point is that only
	// the player who looked knows the new order.
	events := []Event{{Kind: EvAltered, ActorID: a.PlayerID}}
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
