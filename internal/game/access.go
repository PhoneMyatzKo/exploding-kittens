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

// KittensInDeck counts the Exploding Kittens left to draw. Not a leak: it is
// players-1 less the eliminations, which any real table can work out. One being
// reinserted counts too, since it is on its way back in.
func (s *State) KittensInDeck() int {
	n := 0
	for _, c := range s.Draw {
		if c.Type == ExplodingKitten {
			n++
		}
	}
	if s.PendingKitten != nil {
		n++
	}
	return n
}

// DiscardTop is the visible top of the discard pile, or nil when it is empty.
func (s *State) DiscardTop() *Card {
	if len(s.Discard) == 0 {
		return nil
	}
	c := s.Discard[len(s.Discard)-1]
	return &c
}
