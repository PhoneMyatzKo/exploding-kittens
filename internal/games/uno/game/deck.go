package game

import "boardgame/kittens/internal/prng"

// Throughout this package a []Card used as a draw pile is ordered top-first:
// index 0 is the next card anyone would draw. The discard is the other way
// round — its last element is the face-up top — because that is the end cards
// are appended to.

// shuffle randomises a pile in place.
func shuffle(pile []Card, rng *prng.Source) {
	rng.Shuffle(len(pile), func(i, j int) { pile[i], pile[j] = pile[j], pile[i] })
}

// removeCard pulls the card with the given ID out of pile. The bool reports
// whether it was there at all.
func removeCard(pile []Card, id int) ([]Card, Card, bool) {
	for i, c := range pile {
		if c.ID == id {
			out := make([]Card, 0, len(pile)-1)
			out = append(out, pile[:i]...)
			out = append(out, pile[i+1:]...)
			return out, c, true
		}
	}
	return pile, Card{}, false
}

// handPoints totals what a hand is worth to whoever emptied theirs first.
func handPoints(hand []Card) int {
	n := 0
	for _, c := range hand {
		n += c.Points()
	}
	return n
}

// refill turns the discard pile back into a draw pile when the draw pile runs
// out: everything below the face-up top card is shuffled and becomes the new
// deck. Reports whether anything was recovered, which is false only in the
// pathological case where the cards are all in people's hands.
//
// A Wild recovered this way keeps NoColour, which is right: the colour someone
// named for it was a property of that play, not of the card, and the card is
// about to be played again by somebody else.
func (s *State) refill() bool {
	if len(s.Draw) > 0 || len(s.Discard) <= 1 {
		return false
	}
	top := s.Discard[len(s.Discard)-1]
	recycled := make([]Card, len(s.Discard)-1)
	copy(recycled, s.Discard[:len(s.Discard)-1])
	shuffle(recycled, s.RNG)
	s.Draw = recycled
	s.Discard = []Card{top}
	return true
}

// drawCards moves n cards from the deck into a player's hand, reshuffling the
// discard back in if the deck runs dry mid-draw. It returns what was actually
// taken, which can be fewer than n — with 108 cards and ten players that needs
// a truly perverse game, but "fewer" is a legal answer and a panic is not.
func (s *State) drawCards(p *Player, n int) (drawn []Card, reshuffled bool) {
	for i := 0; i < n; i++ {
		if len(s.Draw) == 0 {
			if !s.refill() {
				break
			}
			reshuffled = true
		}
		c := s.Draw[0]
		s.Draw = s.Draw[1:]
		p.Hand = append(p.Hand, c)
		drawn = append(drawn, c)
	}
	return drawn, reshuffled
}

// drawEvents describes a draw the way the table sees it: publicly, only the
// count, and privately, the cards themselves. Every draw in the game goes
// through here so no path can leak a hand by accident.
func drawEvents(p *Player, drawn []Card, reason string) []Event {
	if len(drawn) == 0 {
		return nil
	}
	return []Event{
		{Kind: EvDrew, ActorID: p.ID, Count: len(drawn), Text: reason},
		{Kind: EvDrew, ActorID: p.ID, Count: len(drawn), Cards: drawn, Text: reason, OnlyFor: p.ID},
	}
}
