package game

// Everything a view is allowed to ask the state. The engine mutates and this
// file reads, and keeping the two apart is what makes the redaction rule
// checkable: if a fact is not reachable through a method here, no view can leak
// it by accident.

// Find returns a player by id, or nil.
func (s *State) Find(id string) *Player { return s.playerByID(id) }

// CurrentID is the seat whose main action the table is waiting on, or "" once the
// game is over.
func (s *State) CurrentID() string {
	if s.Phase == PhaseGameOver || len(s.Players) == 0 {
		return ""
	}
	return s.current().ID
}

// CoupTarget is how many Pawns the Coup needs. Public: everybody is told the
// number when the Catalyst lands, because a threat nobody can measure is not a
// threat.
func (s *State) CoupTarget() int { return s.coupTarget() }

// PawnsOf counts the seats a player owns. Only ever asked about the viewer
// themselves, or about everybody once the game is over.
func (s *State) PawnsOf(id string) int { return s.pawnsOf(id) }

// PawnIDs lists the seats a player owns, in seat order.
func (s *State) PawnIDs(id string) []string {
	var out []string
	for _, p := range s.Players {
		if p.pawnOf(id) {
			out = append(out, p.ID)
		}
	}
	return out
}

// MastermindID and ResistanceID are for the endgame reveal and for the room's
// own bookkeeping. Never for a live view of somebody else's seat.
func (s *State) MastermindID() string {
	if m := s.mastermind(); m != nil {
		return m.ID
	}
	return ""
}

func (s *State) ResistanceID() string {
	if r := s.resistance(); r != nil {
		return r.ID
	}
	return ""
}

// PowerDeckCount and BurnedCount are public counts. Which cards are in either
// pile is not.
func (s *State) PowerDeckCount() int { return len(s.PowerDeck) }
func (s *State) BurnedCount() int    { return len(s.Burned) }

// OffersFor is every open proposal a player is party to, either end.
func (s *State) OffersFor(id string) []Offer {
	var out []Offer
	for _, o := range s.Offers {
		if o.FromID == id || o.ToID == id {
			out = append(out, o)
		}
	}
	return out
}

// CardByID finds a card anywhere it could legitimately be shown to the viewer:
// in the hand of the viewer, or in the hand of one of their Pawns. Used to
// render the faces named by an offer without letting a client ask about
// arbitrary ids.
func (s *State) CardByID(viewerID string, id int) (Card, bool) {
	viewer := s.playerByID(viewerID)
	if viewer == nil {
		return Card{}, false
	}
	if c, ok := viewer.findCard(id); ok {
		return c, true
	}
	for _, p := range s.Players {
		if !p.pawnOf(viewerID) {
			continue
		}
		if c, ok := p.findCard(id); ok {
			return c, true
		}
	}
	// A card that has been offered to the viewer is one they are entitled to see
	// the face of — otherwise no proposal could be evaluated.
	for _, o := range s.Offers {
		if o.ToID != viewerID && o.FromID != viewerID {
			continue
		}
		if !offerMentions(o, id) {
			continue
		}
		for _, p := range s.Players {
			if p.ID != o.FromID && p.ID != o.ToID {
				continue
			}
			if c, ok := p.findCard(id); ok {
				return c, true
			}
		}
	}
	return Card{}, false
}

// CanCleanRecord reports whether the clean-record button should be live: right
// round, not owned by anybody, and able to pay.
func (s *State) CanCleanRecord(id string) bool {
	p := s.playerByID(id)
	return p != nil && s.Phase != PhaseColdWar && s.Phase != PhaseGameOver &&
		p.MasterID == "" && p.Influence >= costClean
}

// CanPlayPower reports whether Power cards may be played at all this round.
func (s *State) CanPlayPower() bool {
	return s.Phase == PhaseArmsRace || s.Phase == PhaseCatalyst
}

// Costs are exported so the client can print prices without hard-coding them in
// two languages.
func CostNews() int   { return costNews }
func CostClean() int  { return costClean }
func CostPardon() int { return costPardon }

// EvidenceToFree and EvidenceToName let the client draw the progress a Pawn is
// making towards a name and towards the door.
func EvidenceToName() int { return evidenceToName }
func EvidenceToFree() int { return evidenceToFree }

// FinalRound is the last round, so a client can say "last round" without
// counting.
func FinalRound() int { return RoundFinal }

// Blocked reports whether the table cannot proceed without a particular player.
// Always the current seat while a game is live: every round needs every seat to
// spend its main action, even if that action is a pass.
func (s *State) Blocked() string { return s.CurrentID() }
