package game

// Accessors used by the view layer. They are deliberately read-only: nothing
// outside this package may mutate a State except through Apply.

// Find returns the player with the given ID, or nil.
func (s *State) Find(id string) *Player { return s.playerByID(id) }

// CurrentID is the player whose turn it is.
func (s *State) CurrentID() string { return s.Players[s.Current].ID }

// DeckSize is the number of cards left to draw. Not a leak: everyone can see the
// stack, and its height is the whole basis of counting cards in this game.
func (s *State) DeckSize() int { return len(s.Draw) }

// DiscardTop is the visible top of the discard pile, or nil when it is empty.
func (s *State) DiscardTop() *Card {
	if len(s.Discard) == 0 {
		return nil
	}
	c := s.Discard[len(s.Discard)-1]
	return &c
}

// ActiveColour is the colour in force, as the slug the client draws with. Empty
// while a wild is waiting to be named, which is exactly when the table should be
// showing a colour picker rather than a colour.
func (s *State) ActiveColour() string {
	if s.Colour == NoColour {
		return ""
	}
	return s.Colour.Slug()
}

// Direction is +1 or -1, for the arrow around the table.
func (s *State) Direction() int { return s.Dir }

// Playable reports whether that one card in that player's hand could be laid
// down right now. The client dims everything this says no to.
func (s *State) Playable(playerID string, cardID int) bool {
	p := s.playerByID(playerID)
	if p == nil || p.ID != s.CurrentID() {
		return false
	}
	c, ok := p.findCard(cardID)
	if !ok {
		return false
	}
	switch s.Phase {
	case PhaseTurn:
		return s.playable(c)
	case PhaseDrawn:
		// Only the freshly drawn card, and only if it plays.
		return s.Drawn != nil && s.Drawn.ID == cardID && s.playable(c)
	}
	return false
}

// PlayableIDs lists the cards the given player may play right now.
func (s *State) PlayableIDs(playerID string) []int {
	p := s.playerByID(playerID)
	if p == nil {
		return nil
	}
	var out []int
	for _, c := range p.Hand {
		if s.Playable(playerID, c.ID) {
			out = append(out, c.ID)
		}
	}
	return out
}

// MustName is the player who owes the table a colour, if any.
func (s *State) MustName() string {
	if s.Phase != PhaseColour || s.Wild == nil {
		return ""
	}
	return s.Wild.ActorID
}

// Challenged describes an open Wild Draw Four: who must answer it, and who
// played it. Both empty when there is nothing to answer.
//
// Whether it was a bluff is deliberately not exposed. That is the one fact the
// challenge is a bet on, and a view that carried it would let a client show the
// answer before the bet.
func (s *State) Challenged() (targetID, actorID string) {
	if s.Phase != PhaseChallenge || s.Wild == nil {
		return "", ""
	}
	return s.Wild.TargetID, s.Wild.ActorID
}

// UnoWindow describes the open catch-them-out window: who is on one card, and
// whether they remembered to say so. Public on purpose — the whole mechanic is
// other people noticing.
func (s *State) UnoWindow() (playerID string, called bool, open bool) {
	if s.Uno == nil {
		return "", false, false
	}
	return s.Uno.PlayerID, s.Uno.Called, true
}

// CanCatch reports whether this player may pounce on a missing "UNO!".
func (s *State) CanCatch(playerID string) bool {
	return s.Uno != nil && !s.Uno.Called && s.Uno.PlayerID != playerID &&
		s.playerByID(playerID) != nil
}

// HandSizes is what every player is allowed to know about every hand: how big
// it is.
func (s *State) HandSizes() map[string]int {
	out := make(map[string]int, len(s.Players))
	for _, p := range s.Players {
		out[p.ID] = len(p.Hand)
	}
	return out
}

// Scores is the running total per player.
func (s *State) Scores() map[string]int {
	out := make(map[string]int, len(s.Players))
	for _, p := range s.Players {
		out[p.ID] = p.Score
	}
	return out
}
