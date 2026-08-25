package game

// Accessors used by the view layer. They are deliberately read-only: nothing
// outside this package may mutate a State except through Apply.

// Find returns the player with the given ID, or nil.
func (s *State) Find(id string) *Player { return s.playerByID(id) }

// CurrentID is the player whose turn it is.
func (s *State) CurrentID() string { return s.Players[s.Current].ID }

// CanNope reports whether the given player is currently allowed to stack a Nope:
// there must be an open window, they must be alive and holding a Nope, and they
// can't Nope the card they themselves just played.
func (s *State) CanNope(id string) bool {
	if s.Phase != PhaseNope || s.Pending == nil {
		return false
	}
	p := s.playerByID(id)
	return p != nil && p.Alive && p.ID != s.Pending.LastPlayerID && p.hasType(Nope)
}

// HasPassed reports whether the player already declined the open Nope window.
func (s *State) HasPassed(id string) bool {
	return s.Pending != nil && s.Pending.Passed[id]
}

// AwaitingGiftFrom is the player who must hand a card over during PhaseFavor.
func (s *State) AwaitingGiftFrom() string {
	if s.Phase != PhaseFavor || s.Pending == nil {
		return ""
	}
	return s.Pending.TargetID
}

// DeckSize is the number of cards left to draw.
func (s *State) DeckSize() int { return len(s.Draw) }

// KittensInDeck counts the kittens left to draw, of either sort. Not a leak: it
// is players-1 less the eliminations, which any real table can work out. One
// being put back counts too, since it is on its way in.
func (s *State) KittensInDeck() int {
	n := 0
	for _, c := range s.Draw {
		if c.Type.Kills() {
			n++
		}
	}
	if s.PendingKitten != nil {
		n++
	}
	return n
}

// ImplodingArmed reports whether the Imploding Kitten has been drawn once and is
// back in the deck face up, so the next player to reach it is out.
//
// Public on purpose, and not a leak: at a real table everybody watched it go
// back in. Where it went stays secret, which is the part that matters.
func (s *State) ImplodingArmed() bool {
	for _, c := range s.Draw {
		if c.Type == ImplodingKitten && c.FaceUp {
			return true
		}
	}
	return s.PendingKitten != nil && s.PendingKitten.Type == ImplodingKitten
}

// TopFaceUp is the top card of the deck when it is lying face up, and nil when
// it is face down — which is every card but one.
//
// Only the Imploding Kitten is ever face up in the deck, and it got there by
// being put back in front of everybody. A face-up card on top of a face-down
// pile is visible at a real table, so naming it here discloses nothing the room
// does not already see. The card below it stays secret, and so does the kitten's
// position for as long as something is stacked on top of it.
func (s *State) TopFaceUp() *Card {
	if len(s.Draw) == 0 || !s.Draw[0].FaceUp {
		return nil
	}
	top := s.Draw[0]
	return &top
}

// Placing names the kind of kitten awaiting reinsertion during PhaseDefuse, so
// the client can say whether it is being hidden or armed. Empty otherwise.
func (s *State) Placing() string {
	if s.Phase != PhaseDefuse || s.PendingKitten == nil {
		return ""
	}
	return s.PendingKitten.Slug
}

// PlayDirection is +1 for normal play or -1 once a Reverse has been played.
// Named around the field rather than shadowing it.
func (s *State) PlayDirection() int { return s.step() }

// AlteringID is the player who must reorder the top of the deck, or "".
func (s *State) AlteringID() string {
	if s.Phase != PhaseAlter {
		return ""
	}
	return s.Altering
}

// AlterFaces is the run of cards that player is rearranging, top first.
//
// Returned as whole Cards for the view to strip: their identities must not reach
// the wire even for the player looking at them, because a card ID would let a
// client track that specific card once it is back in the deck. The view sends
// faces without IDs and takes positions back.
func (s *State) AlterFaces() []Card {
	if s.Phase != PhaseAlter {
		return nil
	}
	return append([]Card(nil), s.Draw[:s.alterCount()]...)
}

// DiscardTop is the visible top of the discard pile, or nil when it is empty.
func (s *State) DiscardTop() *Card {
	if len(s.Discard) == 0 {
		return nil
	}
	c := s.Discard[len(s.Discard)-1]
	return &c
}
