package game

import (
	"math/rand"
	"slices"
)

// Phase is which round of the story the table is in. It drives what is legal,
// so every rule that changes between rounds asks the phase rather than the round
// number.
type Phase string

const (
	// PhaseColdWar is round one: positions and secrets, and nothing to attack
	// anybody with yet. Information only.
	PhaseColdWar Phase = "cold_war"
	// PhaseArmsRace is round two: Power cards are dealt and the blackmail starts.
	PhaseArmsRace Phase = "arms_race"
	// PhaseCatalyst is rounds three and four: one Coup card, one Anti-Coup card,
	// and a clock.
	PhaseCatalyst Phase = "catalyst"
	PhaseGameOver Phase = "game_over"
)

// Round numbers. Written down because three separate rules key off "the last
// round" and a literal 4 in each of them is how they drift apart.
const (
	RoundColdWar   = 1
	RoundArmsRace  = 2
	RoundCatalyst  = 3
	RoundFinal     = 4
	influenceStart = 3
	influenceRound = 2

	costNews    = 1
	costClean   = 3
	costPardon  = 4
	costOffer   = 0
	handSize    = 2 // Power cards dealt per player in the Arms Race
	catalystTop = 1 // extra Power card each player gets when the Catalyst lands

	// evidenceToName is how much digging tells a Pawn who owns them, and
	// evidenceToFree is what it takes to break out unaided.
	evidenceToName = 2
	evidenceToFree = 3

	// coupNumerator/coupDenominator is the Mastermind's target share of the
	// table: three fifths, rounded up, which is the spec's 60%.
	coupNumerator   = 3
	coupDenominator = 5

	// newsLimit and noteLimit cap free text. Long enough to lie in, short enough
	// that nobody can paste a novel onto the public board.
	newsLimit = 180
	noteLimit = 120
)

// Side is who won.
type Side string

const (
	SideMastermind Side = "mastermind"
	SideResistance Side = "resistance"
	SideState      Side = "state"
)

// Intel is one thing a player privately knows. It is kept on the player rather
// than being left to live in the log, because a reconnecting client has to be
// able to rebuild the dossier it was shown an hour ago.
type Intel struct {
	Round int    `json:"round"`
	Text  string `json:"text"`
	// AboutID is who the fact is about, where it is about somebody. Used by the
	// client to pin a note to a seat.
	AboutID string `json:"aboutId,omitempty"`
	// Slug is the Weakness named, when the intel is precise enough to name one.
	// This is what lets a client grey out the Power cards that would be wasted.
	Slug string `json:"slug,omitempty"`
	// Cat is the category, when that is all the intel establishes. Structured
	// rather than left in the prose so the client can shade a seat by what is
	// known about it without parsing English.
	Cat Category `json:"category,omitempty"`
}

// Offer is an open proposal between two players: a swap, a payment, a gift, or a
// demand with menaces. All four are the same object because the difference
// between a trade and an extortion is which fields are filled in, and the engine
// has no business having an opinion about which one this is.
type Offer struct {
	ID     int    `json:"id"`
	FromID string `json:"fromId"`
	ToID   string `json:"toId"`
	// Give is Power cards the proposer hands over; Want is cards they ask for.
	Give []int `json:"give"`
	Want []int `json:"want"`
	// Pay is influence the proposer hands over; Demand is influence they want.
	Pay    int    `json:"pay"`
	Demand int    `json:"demand"`
	Note   string `json:"note,omitempty"`
	Round  int    `json:"round"`
}

// Post is one line on the anonymous public newsfeed. The author is stored so the
// endgame reveal can print it, and is never sent to a client before then — that
// redaction lives in view.go and is the only reason this field is unexported in
// spirit but exported in fact.
type Post struct {
	ID    int    `json:"id"`
	Text  string `json:"text"`
	Round int    `json:"round"`
	// AuthorID is redacted until the game is over.
	AuthorID string `json:"-"`
}

// Player is one seat.
type Player struct {
	ID   string
	Name string
	Role Role
	// Weakness is the secret dealt in round one. Replaceable by buying a clean
	// record, as often as you can afford it.
	Weakness Card
	// CleanedOn is the round the secret was last replaced, zero if never. It is
	// how a viewer's own notes can be marked stale without telling them what the
	// new secret is: they learn that their file is old, not what replaced it.
	CleanedOn int
	// Hand is Power cards. Empty until the Arms Race.
	Hand      []Card
	Influence int

	// SkillsUsed counts this round's uses of the position's power.
	SkillsUsed int
	// Acted is whether this round's main action has been spent.
	Acted bool

	// MasterID is who owns them, empty if nobody does. This is the single most
	// private field in the game: the Pawn is told they are compromised, and is
	// told by whom only once they have dug for it.
	MasterID string
	// Evidence is how far a Pawn has got towards naming their owner.
	Evidence int
	// KnowsMaster is set once digging has paid off. Kept separately from
	// Evidence because a pardon resets control without unlearning what was found.
	KnowsMaster bool
	// Pardoned protects a freed Pawn for the rest of the round, so a pardon
	// cannot simply be undone by replaying the same card on the same turn.
	Pardoned bool
	// LeakedThisRound throttles the whistleblower to one report a round.
	LeakedThisRound bool

	// Coup and AntiCoup are the Catalyst cards, dealt in round three.
	Coup     bool
	AntiCoup bool
	// Accused is set once the Resistance has spent its one accusation.
	Accused bool

	// Dossier is everything this player has privately learned, oldest first.
	Dossier []Intel
}

// pawnOf reports whether p is under q's control.
func (p *Player) pawnOf(id string) bool { return p.MasterID != "" && p.MasterID == id }

func (p *Player) findCard(id int) (Card, bool) {
	for _, c := range p.Hand {
		if c.ID == id {
			return c, true
		}
	}
	return Card{}, false
}

func (p *Player) learn(round int, about, slug, text string) {
	p.Dossier = append(p.Dossier, Intel{Round: round, Text: text, AboutID: about, Slug: slug})
}

// learnCat records intel that pins down a category but not a card.
func (p *Player) learnCat(round int, about string, cat Category, text string) {
	p.Dossier = append(p.Dossier, Intel{Round: round, Text: text, AboutID: about, Cat: cat})
}

// State is the complete, unredacted game: every secret, every hand, who owns
// whom, and who is holding the Coup. It must never be serialised to a client —
// see view.go, which is the only thing allowed to read it for that purpose.
type State struct {
	Players []*Player

	Phase Phase
	Round int
	// Current is the seat whose main action the table is waiting on.
	Current int
	// turnsThisRound counts main actions spent, so the round ends when it reaches
	// the number of seats rather than when the index happens to wrap.
	turnsThisRound int

	// WeaknessDeck is what a clean record draws from. Ordered top-first.
	WeaknessDeck []Card
	// PowerDeck is what the Arms Race and the Catalyst deal from, top-first.
	PowerDeck []Card
	// Burned is Power cards that have been played, landed or not. Public as a
	// count only: which cards have been spent is a real clue.
	Burned []Card

	Offers []Offer
	Feed   []Post

	// Winner is the side that took it, set with the phase.
	Winner Side
	// WinnerIDs is who personally won: the Mastermind, the Resistance Leader, or
	// everybody who was never anybody's Pawn.
	WinnerIDs []string
	// Reason is the one-line explanation the game-over card prints.
	Reason string

	nextOfferID int
	nextPostID  int
	// first is the seat that opens the round. It walks one place per round so
	// that going first is not a standing advantage.
	first int
	seats []Seat
	rng   *rand.Rand
}

func (s *State) playerByID(id string) *Player {
	for _, p := range s.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *State) current() *Player { return s.Players[s.Current] }

// mastermind is the player holding the Coup card, or nil before the Catalyst.
func (s *State) mastermind() *Player {
	for _, p := range s.Players {
		if p.Coup {
			return p
		}
	}
	return nil
}

// resistance is the player holding the Anti-Coup card, or nil before the
// Catalyst.
func (s *State) resistance() *Player {
	for _, p := range s.Players {
		if p.AntiCoup {
			return p
		}
	}
	return nil
}

// pawnsOf counts how many seats a player owns.
func (s *State) pawnsOf(id string) int {
	n := 0
	for _, p := range s.Players {
		if p.pawnOf(id) {
			n++
		}
	}
	return n
}

// coupTarget is how many Pawns the Mastermind needs: three fifths of the table,
// rounded up. Eight players needs five, five players needs three.
func (s *State) coupTarget() int {
	n := len(s.Players)
	return (n*coupNumerator + coupDenominator - 1) / coupDenominator
}

// drawPower takes the top Power card, reshuffling the burn pile back in if the
// deck has run dry. Returns false only if there is genuinely no card left
// anywhere, which needs a table that has played every card in the game.
func (s *State) drawPower() (Card, bool) {
	if len(s.PowerDeck) == 0 {
		if len(s.Burned) == 0 {
			return Card{}, false
		}
		s.PowerDeck = append([]Card(nil), s.Burned...)
		s.Burned = nil
		shuffle(s.PowerDeck, s.rng)
	}
	c := s.PowerDeck[0]
	s.PowerDeck = s.PowerDeck[1:]
	return c, true
}

// drawWeakness takes the top Weakness card. The old one goes to the bottom, so
// a clean record cannot deal you back the secret you just paid to be rid of
// until the deck has gone round.
func (s *State) drawWeakness(old Card) (Card, bool) {
	if len(s.WeaknessDeck) == 0 {
		return Card{}, false
	}
	c := s.WeaknessDeck[0]
	s.WeaknessDeck = append(s.WeaknessDeck[1:], old)
	return c, true
}

// shuffle randomises a pile in place.
func shuffle(pile []Card, rng *rand.Rand) {
	rng.Shuffle(len(pile), func(i, j int) { pile[i], pile[j] = pile[j], pile[i] })
}

// dropOffer removes a proposal by id. Reports whether it was there.
func (s *State) dropOffer(id int) (Offer, bool) {
	for i, o := range s.Offers {
		if o.ID == id {
			out := s.Offers[i]
			s.Offers = append(s.Offers[:i], s.Offers[i+1:]...)
			return out, true
		}
	}
	return Offer{}, false
}

// dropOffersTouching clears every open proposal that mentions a card which has
// just left somebody's hand. Without this a player could promise the same card
// to three people and then play it, leaving proposals that can never be honoured
// sitting on other people's screens.
func (s *State) dropOffersTouching(cardIDs ...int) {
	keep := s.Offers[:0]
	for _, o := range s.Offers {
		if offerMentions(o, cardIDs...) {
			continue
		}
		keep = append(keep, o)
	}
	s.Offers = keep
}

func offerMentions(o Offer, ids ...int) bool {
	for _, id := range ids {
		if slices.Contains(o.Give, id) || slices.Contains(o.Want, id) {
			return true
		}
	}
	return false
}

// truncate caps free text at n runes without splitting one in half.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
