package game

import "fmt"

// Apply is the whole rulebook: one submitted action against the state, either
// refused with an error the player is shown, or accepted with the events it
// produced. Nothing else in this package mutates State.
//
// The order of the checks matters and is the same everywhere: is the game live,
// is this action legal in this phase, is it this player's to make, and only then
// what does it do.
func Apply(s *State, a Action) ([]Event, error) {
	if s.Phase == PhaseGameOver {
		return nil, ErrGameOver
	}
	p := s.playerByID(a.PlayerID)
	if p == nil {
		return nil, ErrNoSuchPlayer
	}

	// Main actions are turn-gated; free actions are not. Keeping that in one
	// place is what stops each handler having to remember which it is.
	if a.Kind.Main() {
		if s.current().ID != p.ID {
			return nil, ErrNotYourTurn
		}
		if p.Acted {
			return nil, ErrAlreadyActed
		}
	}

	switch a.Kind {
	case ActPass:
		return append([]Event{{Kind: EvPassed, ActorID: p.ID}}, s.spendTurn(p)...), nil
	case ActSkill:
		return s.useSkill(p, a)
	case ActPower:
		return s.playPower(p, a)
	case ActCleanRecord:
		return s.cleanRecord(p)
	case ActEvidence:
		return s.gatherEvidence(p)
	case ActSeize:
		return s.seize(p, a)
	case ActAccuse:
		return s.accuse(p, a)

	case ActOffer:
		return s.makeOffer(p, a)
	case ActAccept:
		return s.acceptOffer(p, a)
	case ActDecline:
		return s.declineOffer(p, a)
	case ActNews:
		return s.postNews(p, a)
	case ActLeak:
		return s.leak(p)
	case ActPardon:
		return s.pardon(p, a)
	case ActAdvance:
		return s.endRound(), nil
	}
	return nil, ErrWrongPhase
}

// ───────────────────────────────────────────────── main actions

// useSkill runs the actor's position. Every branch produces one public line that
// says a position was used and on whom, and one private line that says what was
// learned: the table always knows an investigation happened, and never what it
// found.
func (s *State) useSkill(p *Player, a Action) ([]Event, error) {
	if p.SkillsUsed >= p.Role.SkillUses() {
		return nil, ErrSkillSpent
	}
	events, err := s.skillEffect(p, a)
	if err != nil {
		return nil, err
	}
	p.SkillsUsed++
	// The Hacker's two peeks are two uses of one turn's action, so the turn is
	// only spent once the last of them is gone.
	if p.SkillsUsed >= p.Role.SkillUses() {
		events = append(events, s.spendTurn(p)...)
	}
	return events, nil
}

func (s *State) skillEffect(p *Player, a Action) ([]Event, error) {
	switch p.Role {
	case Police:
		t, err := s.other(p, a.TargetID)
		if err != nil {
			return nil, err
		}
		text := fmt.Sprintf("%s is hiding %s.", t.Name, t.Weakness.Name)
		p.learn(s.Round, t.ID, t.Weakness.Slug, text)
		return []Event{
			{Kind: EvSkill, ActorID: p.ID, TargetID: t.ID, Text: "opened a file on"},
			priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID, TargetID: t.ID,
				Cards: []Card{t.Weakness}, Text: text}),
		}, nil

	case Hacker:
		t, err := s.other(p, a.TargetID)
		if err != nil {
			return nil, err
		}
		text := fmt.Sprintf("%s's secret is %s in nature.", t.Name, t.Weakness.Cat)
		p.learnCat(s.Round, t.ID, t.Weakness.Cat, text)
		return []Event{
			{Kind: EvSkill, ActorID: p.ID, TargetID: t.ID, Text: "was inside the machines of"},
			priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID, TargetID: t.ID, Text: text}),
		}, nil

	case Lawyer:
		if !KnownWeakness(a.Slug) {
			return nil, ErrNoSuchWeakness
		}
		n := 0
		for _, q := range s.Players {
			if q.Weakness.Slug == a.Slug {
				n++
			}
		}
		name := WeaknessName(a.Slug)
		text := fmt.Sprintf("%d of you are carrying %s.", n, name)
		p.learn(s.Round, "", a.Slug, text)
		return []Event{
			{Kind: EvSkill, ActorID: p.ID, Text: "went through the record books"},
			priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID, Count: n, Text: text}),
		}, nil

	case President:
		t, err := s.other(p, a.TargetID)
		if err != nil {
			return nil, err
		}
		// A decree is the one skill whose result is public. It pays the President
		// nothing in secrecy and everything in leverage: now everybody knows which
		// deck to go looking in, and the President is the one they know it from.
		p.Influence++
		text := fmt.Sprintf("%s's secret is %s in nature.", t.Name, t.Weakness.Cat)
		for _, q := range s.Players {
			q.learnCat(s.Round, t.ID, t.Weakness.Cat, text)
		}
		return []Event{
			{Kind: EvSkill, ActorID: p.ID, TargetID: t.ID, Text: "issued a decree against"},
			{Kind: EvDisclosed, ActorID: p.ID, TargetID: t.ID, Text: string(t.Weakness.Cat), Points: 1},
		}, nil

	case Citizen:
		// A rumour is uncontrolled on purpose: the Citizen's edge is that nobody
		// can predict what they were told, including the Citizen.
		others := make([]*Player, 0, len(s.Players)-1)
		for _, q := range s.Players {
			if q.ID != p.ID {
				others = append(others, q)
			}
		}
		t := others[s.rng.Intn(len(others))]
		p.Influence++
		text := fmt.Sprintf("Word is that %s is hiding %s.", t.Name, t.Weakness.Name)
		p.learn(s.Round, t.ID, t.Weakness.Slug, text)
		return []Event{
			{Kind: EvSkill, ActorID: p.ID, Text: "listened to the corridors"},
			priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID, TargetID: t.ID,
				Cards: []Card{t.Weakness}, Text: text, Points: 1}),
		}, nil
	}
	return nil, ErrWrongPhase
}

// playPower plays one Power card at one player.
//
// The result is private to the two of them. The table sees only that a move was
// made, and against whom — never whether it landed. That asymmetry is the whole
// bluffing surface of the game: a burned card and a successful blackmail look
// exactly alike from the outside.
func (s *State) playPower(p *Player, a Action) ([]Event, error) {
	if s.Phase == PhaseColdWar {
		return nil, ErrWrongPhase
	}
	t, err := s.other(p, a.TargetID)
	if err != nil {
		return nil, err
	}
	card, ok := p.findCard(a.CardID)
	if !ok {
		return nil, ErrNoSuchCard
	}
	if card.Kind != KindPower {
		return nil, ErrNotPower
	}
	if t.pawnOf(p.ID) {
		return nil, ErrAlreadyOwned
	}
	if t.Pardoned {
		return nil, ErrPardoned
	}

	p.Hand = removeCard(p.Hand, card.ID)
	s.Burned = append(s.Burned, card)
	s.dropOffersTouching(card.ID)

	events := []Event{{Kind: EvMove, ActorID: p.ID, TargetID: t.ID}}
	if card.Lands(t.Weakness.Slug) {
		// Taking a Pawn off somebody else is legal and is not announced. The
		// previous owner is told, because a Pawn walking away is the loudest
		// signal in the game and hiding it would make control unverifiable.
		if prev := t.MasterID; prev != "" && prev != p.ID {
			events = append(events, priv(prev, Event{Kind: EvBurned, TargetID: t.ID,
				Text: fmt.Sprintf("%s has slipped out of your hands — somebody else got to them.", t.Name)}))
		}
		t.MasterID = p.ID
		t.Evidence = 0
		t.KnowsMaster = false
		p.Influence++
		p.learn(s.Round, t.ID, t.Weakness.Slug,
			fmt.Sprintf("%s is yours: %s landed.", t.Name, card.Name))
		t.learn(s.Round, "", "", "You were compromised. Somebody is holding your secret.")
		events = append(events,
			priv(p.ID, Event{Kind: EvOwned, ActorID: p.ID, TargetID: t.ID, Cards: []Card{card},
				Text: fmt.Sprintf("%s landed. %s is your Pawn.", card.Name, t.Name), Points: 1}),
			priv(t.ID, Event{Kind: EvOwned, TargetID: t.ID,
				Text: "Somebody just proved they know what you did. You are a Pawn."}))
	} else {
		// A miss is worth knowing about, and the target learns something real:
		// which card was spent on them and therefore which secret they do not have.
		p.learn(s.Round, t.ID, "", fmt.Sprintf("%s does not have %s.", t.Name, WeaknessName(card.Exploits)))
		events = append(events,
			priv(p.ID, Event{Kind: EvBurned, ActorID: p.ID, TargetID: t.ID, Cards: []Card{card},
				Text: fmt.Sprintf("%s was wasted on %s.", card.Name, t.Name)}),
			priv(t.ID, Event{Kind: EvBurned, ActorID: p.ID, TargetID: t.ID, Cards: []Card{card},
				Text: fmt.Sprintf("%s came at you with %s. It missed.", p.Name, card.Name)}))
	}
	return append(events, s.spendTurn(p)...), nil
}

// cleanRecord is the Arms Race's escape hatch: pay, and the secret you have been
// sweating over all game is replaced with one nobody has looked for yet.
//
// It is barred to a Pawn on purpose. Once somebody is holding your file, buying
// a new file does not get it back — the spec's expiry is for secrets that were
// never exploited.
func (s *State) cleanRecord(p *Player) ([]Event, error) {
	if s.Phase == PhaseColdWar {
		return nil, ErrWrongPhase
	}
	if p.MasterID != "" {
		return nil, ErrCleanNoOne
	}
	if p.Influence < costClean {
		return nil, ErrInfluence
	}
	fresh, ok := s.drawWeakness(p.Weakness)
	if !ok {
		return nil, ErrCleanClear
	}
	old := p.Weakness
	p.Weakness = fresh
	p.CleanedOn = s.Round
	p.Influence -= costClean
	p.learn(s.Round, p.ID, fresh.Slug,
		fmt.Sprintf("Clean record: %s is gone. You now carry %s.", old.Name, fresh.Name))
	// Everybody's notes about this player are now wrong, and they are not told
	// that. A public line saying who bought the record is enough: working out
	// that your file is stale is the other players' problem.
	events := []Event{
		{Kind: EvClean, ActorID: p.ID, Points: costClean},
		priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID, Cards: []Card{fresh},
			Text: "Your record is clean. New secret: " + fresh.Name + "."}),
	}
	return append(events, s.spendTurn(p)...), nil
}

// gatherEvidence is a Pawn working out who owns them, and then working their way
// out. Two turns buys a name; a third buys freedom.
func (s *State) gatherEvidence(p *Player) ([]Event, error) {
	if p.MasterID == "" {
		return nil, ErrNotCompromised
	}
	p.Evidence++
	events := []Event{priv(p.ID, Event{Kind: EvEvidence, ActorID: p.ID, Count: p.Evidence,
		Text: fmt.Sprintf("You have %d piece(s) of evidence.", p.Evidence)})}

	if p.Evidence >= evidenceToName && !p.KnowsMaster {
		p.KnowsMaster = true
		if m := s.playerByID(p.MasterID); m != nil {
			text := fmt.Sprintf("%s is the one holding your secret.", m.Name)
			p.learn(s.Round, m.ID, "", text)
			events = append(events, priv(p.ID, Event{Kind: EvEvidence, ActorID: p.ID,
				TargetID: m.ID, Count: p.Evidence, Text: text}))
		}
	}
	if p.Evidence >= evidenceToFree {
		master := p.MasterID
		p.MasterID = ""
		p.Evidence = 0
		p.Pardoned = true
		// A mutiny is public: somebody walked out, and everybody can see it. Who
		// they walked out on is not said, which keeps the Mastermind's name safe
		// while making the fact of a hidden owner undeniable.
		events = append(events,
			Event{Kind: EvMutiny, ActorID: p.ID, Text: "broke free"},
			priv(master, Event{Kind: EvMutiny, TargetID: p.ID,
				Text: fmt.Sprintf("%s has mutinied. You have lost them.", p.Name)}))
	}
	return append(events, s.spendTurn(p)...), nil
}

// seize takes a Power card off somebody you own. This is what control actually
// costs the Pawn, and it is the reason a Mastermind's majority is worth having
// before round four rather than only at the end of it.
func (s *State) seize(p *Player, a Action) ([]Event, error) {
	t, err := s.other(p, a.TargetID)
	if err != nil {
		return nil, err
	}
	if t.MasterID == "" {
		return nil, ErrNotPawn
	}
	if !t.pawnOf(p.ID) {
		return nil, ErrNotYourPawn
	}
	if len(t.Hand) == 0 {
		return nil, ErrPawnEmpty
	}
	// The owner may name a card — they can see the Pawn's hand — and gets a
	// random one if they don't.
	card, ok := t.findCard(a.CardID)
	if !ok {
		card = t.Hand[s.rng.Intn(len(t.Hand))]
	}
	t.Hand = removeCard(t.Hand, card.ID)
	p.Hand = append(p.Hand, card)
	s.dropOffersTouching(card.ID)

	events := []Event{
		priv(p.ID, Event{Kind: EvSeized, ActorID: p.ID, TargetID: t.ID, Cards: []Card{card},
			Text: fmt.Sprintf("You took %s from %s.", card.Name, t.Name)}),
		priv(t.ID, Event{Kind: EvSeized, TargetID: t.ID, Cards: []Card{card},
			Text: fmt.Sprintf("%s was taken from you. You were not asked.", card.Name)}),
	}
	return append(events, s.spendTurn(p)...), nil
}

// accuse is the Resistance's one shot. Right, and it is over; wrong, and the
// Mastermind is three influence richer and the room has learned that whoever
// just spoke is the Anti-Coup.
func (s *State) accuse(p *Player, a Action) ([]Event, error) {
	if s.Phase != PhaseCatalyst {
		return nil, ErrWrongPhase
	}
	if !p.AntiCoup {
		return nil, ErrNotResistance
	}
	if p.Accused {
		return nil, ErrAccusationUsed
	}
	t, err := s.other(p, a.TargetID)
	if err != nil {
		return nil, err
	}
	p.Accused = true
	events := []Event{{Kind: EvAccused, ActorID: p.ID, TargetID: t.ID}}
	if t.Coup {
		return append(events, s.finish(SideResistance, p.ID, fmt.Sprintf(
			"%s named %s, and was right. The coup is over.", p.Name, t.Name))...), nil
	}
	if m := s.mastermind(); m != nil {
		m.Influence += 3
		events = append(events, priv(m.ID, Event{Kind: EvIntel, ActorID: m.ID, Points: 3,
			Text: fmt.Sprintf("%s accused %s and was wrong. They are the Resistance, and they have nothing left.", p.Name, t.Name)}))
		m.learn(s.Round, p.ID, "", p.Name+" holds the Anti-Coup.")
	}
	events = append(events, Event{Kind: EvAccused, ActorID: p.ID, TargetID: t.ID,
		Text: "wrong", Points: 3})
	return append(events, s.spendTurn(p)...), nil
}

// ───────────────────────────────────────────────── free actions

// makeOffer puts a proposal on one other player's screen. Nothing is checked
// beyond "you have what you are promising": a demand for influence somebody
// cannot pay is a perfectly good threat, and a promise you intend to break is
// this game's core competency.
func (s *State) makeOffer(p *Player, a Action) ([]Event, error) {
	t, err := s.other(p, a.TargetID)
	if err != nil {
		return nil, err
	}
	give, err := ownedCards(p, a.CardIDs)
	if err != nil {
		return nil, err
	}
	want, err := ownedCards(t, a.WantIDs)
	if err != nil {
		return nil, err
	}
	pay, demand := max0(a.Amount), max0(a.Demand)
	if len(give) == 0 && len(want) == 0 && pay == 0 && demand == 0 {
		return nil, ErrEmptyOffer
	}
	if pay > p.Influence {
		return nil, ErrInfluence
	}

	s.nextOfferID++
	o := Offer{ID: s.nextOfferID, FromID: p.ID, ToID: t.ID, Pay: pay, Demand: demand,
		Note: truncate(a.Text, noteLimit), Round: s.Round}
	for _, c := range give {
		o.Give = append(o.Give, c.ID)
	}
	for _, c := range want {
		o.Want = append(o.Want, c.ID)
	}
	s.Offers = append(s.Offers, o)

	return []Event{
		priv(t.ID, Event{Kind: EvOffer, ActorID: p.ID, TargetID: t.ID, Cards: give,
			Text: o.Note, Points: o.ID}),
		priv(p.ID, Event{Kind: EvOffer, ActorID: p.ID, TargetID: t.ID,
			Text: "Proposal sent to " + t.Name + ".", Points: o.ID}),
	}, nil
}

// acceptOffer settles a proposal. Anything that has moved since it was made
// voids it rather than half-executing: a trade for a card somebody has already
// played is not a trade.
func (s *State) acceptOffer(p *Player, a Action) ([]Event, error) {
	o, ok := s.findOffer(a.OfferID)
	if !ok {
		return nil, ErrNoOffer
	}
	if o.ToID != p.ID {
		return nil, ErrNotYourOffer
	}
	from := s.playerByID(o.FromID)
	if from == nil {
		return nil, ErrNoSuchPlayer
	}
	give, err := ownedCards(from, o.Give)
	if err != nil {
		s.dropOffer(o.ID)
		return nil, ErrNoOffer
	}
	want, err := ownedCards(p, o.Want)
	if err != nil {
		s.dropOffer(o.ID)
		return nil, ErrNoOffer
	}
	if o.Pay > from.Influence || o.Demand > p.Influence {
		return nil, ErrInfluence
	}

	for _, c := range give {
		from.Hand = removeCard(from.Hand, c.ID)
		p.Hand = append(p.Hand, c)
	}
	for _, c := range want {
		p.Hand = removeCard(p.Hand, c.ID)
		from.Hand = append(from.Hand, c)
	}
	from.Influence += o.Demand - o.Pay
	p.Influence += o.Pay - o.Demand

	s.dropOffer(o.ID)
	// Every other proposal that named one of these cards is now unhonourable.
	moved := make([]int, 0, len(give)+len(want))
	for _, c := range append(append([]Card(nil), give...), want...) {
		moved = append(moved, c.ID)
	}
	s.dropOffersTouching(moved...)

	// The table sees that two people closed a deal, and how much money moved.
	// What changed hands is between them: that is the fact worth paying for.
	events := []Event{{Kind: EvTraded, ActorID: from.ID, TargetID: p.ID,
		Count: len(give) + len(want), Points: o.Pay + o.Demand}}
	if len(give) > 0 || o.Pay > 0 {
		events = append(events, priv(p.ID, Event{Kind: EvTraded, ActorID: from.ID, TargetID: p.ID,
			Cards: give, Points: o.Pay, Text: "You received:"}))
	}
	if len(want) > 0 || o.Demand > 0 {
		events = append(events, priv(from.ID, Event{Kind: EvTraded, ActorID: from.ID, TargetID: p.ID,
			Cards: want, Points: o.Demand, Text: "You received:"}))
	}
	return events, nil
}

// declineOffer refuses a proposal, or withdraws one you made. Both are the same
// action because both mean the same thing to the object: it is gone.
func (s *State) declineOffer(p *Player, a Action) ([]Event, error) {
	o, ok := s.findOffer(a.OfferID)
	if !ok {
		return nil, ErrNoOffer
	}
	if o.ToID != p.ID && o.FromID != p.ID {
		return nil, ErrNotYourOffer
	}
	s.dropOffer(o.ID)
	other := o.FromID
	if other == p.ID {
		other = o.ToID
	}
	return []Event{priv(other, Event{Kind: EvDeclined, ActorID: p.ID, TargetID: other,
		Text: p.Name + " walked away from the deal."})}, nil
}

// postNews puts an anonymous line on the public board. Nothing about it is
// checked for truth, which is the point of it existing.
func (s *State) postNews(p *Player, a Action) ([]Event, error) {
	text := truncate(a.Text, newsLimit)
	if text == "" {
		return nil, ErrNoText
	}
	if p.Influence < costNews {
		return nil, ErrInfluence
	}
	p.Influence -= costNews
	s.nextPostID++
	post := Post{ID: s.nextPostID, Text: text, Round: s.Round, AuthorID: p.ID}
	s.Feed = append(s.Feed, post)
	return []Event{
		{Kind: EvNews, Text: text, Count: post.ID},
		priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID, Points: costNews,
			Text: "Your leak is on the board. Nobody knows it was you."}),
	}, nil
}

// leak is the whistleblower: a Pawn reporting upward without knowing who they
// are reporting to. The engine routes it to whoever holds the Anti-Coup, which
// is the only reason the Resistance is worth being.
func (s *State) leak(p *Player) ([]Event, error) {
	if p.MasterID == "" {
		return nil, ErrNotCompromised
	}
	if p.LeakedThisRound {
		return nil, ErrLeakSpent
	}
	p.LeakedThisRound = true
	r := s.resistance()
	if r == nil {
		// Before the Catalyst there is nobody listening. The Pawn is told that,
		// rather than being allowed to think their report went somewhere.
		return []Event{priv(p.ID, Event{Kind: EvLeak, ActorID: p.ID,
			Text: "You put out a report. Nobody is listening yet."})}, nil
	}
	text := fmt.Sprintf("%s reports being under somebody's control.", p.Name)
	if p.KnowsMaster {
		if m := s.playerByID(p.MasterID); m != nil {
			text = fmt.Sprintf("%s reports being controlled by %s.", p.Name, m.Name)
			r.learn(s.Round, m.ID, "", text)
		}
	} else {
		r.learn(s.Round, p.ID, "", text)
	}
	return []Event{
		priv(p.ID, Event{Kind: EvLeak, ActorID: p.ID,
			Text: "Your report reached somebody who wanted it."}),
		priv(r.ID, Event{Kind: EvLeak, ActorID: p.ID, TargetID: r.ID, Text: text}),
	}, nil
}

// pardon is the Resistance's tug-of-war move: pay, and one Pawn is free and
// cannot be retaken this round. It is private on both ends, so the Mastermind
// finds out by discovering their count has slipped.
func (s *State) pardon(p *Player, a Action) ([]Event, error) {
	if !p.AntiCoup {
		return nil, ErrNotResistance
	}
	t, err := s.other(p, a.TargetID)
	if err != nil {
		return nil, err
	}
	if t.MasterID == "" {
		return nil, ErrNotPawn
	}
	if p.Influence < costPardon {
		return nil, ErrInfluence
	}
	p.Influence -= costPardon
	master := t.MasterID
	t.MasterID = ""
	t.Pardoned = true
	t.Evidence = 0
	t.learn(s.Round, "", "", "Your debt was settled by somebody you cannot name. You are free.")
	return []Event{
		priv(p.ID, Event{Kind: EvPardon, ActorID: p.ID, TargetID: t.ID, Points: costPardon,
			Text: fmt.Sprintf("You pardoned %s. They are out of somebody's hands.", t.Name)}),
		priv(t.ID, Event{Kind: EvPardon, TargetID: t.ID,
			Text: "A pardon came through. Your secret no longer holds you."}),
		priv(master, Event{Kind: EvPardon, TargetID: t.ID,
			Text: fmt.Sprintf("%s has been pardoned. You have lost them.", t.Name)}),
	}, nil
}

// ───────────────────────────────────────────────── helpers

// other resolves a target id to somebody who is not the actor.
func (s *State) other(p *Player, id string) (*Player, error) {
	if id == p.ID {
		return nil, ErrSelfTarget
	}
	t := s.playerByID(id)
	if t == nil {
		return nil, ErrNoSuchPlayer
	}
	return t, nil
}

func (s *State) findOffer(id int) (Offer, bool) {
	for _, o := range s.Offers {
		if o.ID == id {
			return o, true
		}
	}
	return Offer{}, false
}

// ownedCards resolves ids against a hand, refusing the whole list if any one of
// them is missing. An offer that is half-valid is not valid.
func ownedCards(p *Player, ids []int) ([]Card, error) {
	out := make([]Card, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		c, ok := p.findCard(id)
		if !ok {
			return nil, ErrNoSuchCard
		}
		out = append(out, c)
	}
	return out, nil
}

func removeCard(hand []Card, id int) []Card {
	for i, c := range hand {
		if c.ID == id {
			out := make([]Card, 0, len(hand)-1)
			out = append(out, hand[:i]...)
			return append(out, hand[i+1:]...)
		}
	}
	return hand
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
