package game

import (
	"errors"
	"fmt"
	"math/rand"
)

const (
	// MinPlayers and MaxPlayers bound the table. Five is the fewest that makes
	// three fifths of the room a meaningful thing to own; past eight nobody can
	// keep track of who owes what, which is a different game.
	MinPlayers = 5
	MaxPlayers = 8
)

// Seat identifies a player being dealt in.
type Seat struct {
	ID   string
	Name string
}

var ErrPlayerCount = errors.New("state of shadows needs 5 to 8 players")

// NewGame deals round one: positions face up, one secret each, and nothing yet
// to do anything with them.
func NewGame(seats []Seat, rng *rand.Rand) (*State, []Event, error) {
	if len(seats) < MinPlayers || len(seats) > MaxPlayers {
		return nil, nil, ErrPlayerCount
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}

	s := &State{
		Phase: PhaseColdWar,
		Round: RoundColdWar,
		seats: append([]Seat(nil), seats...),
		rng:   rng,
	}
	s.WeaknessDeck = WeaknessDeck(1000)
	s.PowerDeck = PowerDeck(2000)
	shuffle(s.WeaknessDeck, rng)
	shuffle(s.PowerDeck, rng)

	roles := dealRoles(len(seats), rng)
	for i, seat := range seats {
		p := &Player{ID: seat.ID, Name: seat.Name, Role: roles[i], Influence: influenceStart}
		p.Weakness = s.WeaknessDeck[0]
		s.WeaknessDeck = s.WeaknessDeck[1:]
		s.Players = append(s.Players, p)
	}
	// The first mover is random, so hosting is not an advantage, and it walks one
	// seat per round so it is not a standing one either.
	s.Current = rng.Intn(len(s.Players))
	s.first = s.Current

	events := []Event{{Kind: EvStarted, Count: len(s.Players),
		Text: "Positions are public. Secrets are not."}}
	events = append(events, Event{Kind: EvRound, Count: RoundColdWar,
		Text: "The Cold War: find out what people are hiding."})
	for _, p := range s.Players {
		events = append(events, priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID,
			Cards: []Card{p.Weakness},
			Text:  fmt.Sprintf("You are %s. Your secret: %s.", p.Role, p.Weakness.Name)}))
		p.learn(s.Round, p.ID, p.Weakness.Slug, "Your own secret: "+p.Weakness.Name+".")
	}
	return s, append(events, s.turnEvent()), nil
}

// dealRoles hands out the four positions and fills the rest of the table with
// Citizens, then shuffles so the President is not always in the host's seat.
func dealRoles(n int, rng *rand.Rand) []Role {
	out := make([]Role, 0, n)
	for i := 0; i < n; i++ {
		if i < len(roleOrder) {
			out = append(out, roleOrder[i])
		} else {
			out = append(out, Citizen)
		}
	}
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// turnEvent announces whose main action the table is waiting on.
func (s *State) turnEvent() Event {
	if s.Phase == PhaseGameOver {
		return Event{Kind: EvGameOver}
	}
	return Event{Kind: EvTurn, ActorID: s.current().ID, Count: s.Round}
}

// spendTurn marks the actor's main action as used and hands the turn on, ending
// the round once every seat has had one.
func (s *State) spendTurn(p *Player) []Event {
	p.Acted = true
	s.turnsThisRound++
	if s.turnsThisRound >= len(s.Players) {
		return s.endRound()
	}
	// Walk to the next seat that still owes an action. Every seat owes exactly
	// one per round, so this always terminates.
	for i := 0; i < len(s.Players); i++ {
		s.Current = (s.Current + 1) % len(s.Players)
		if !s.current().Acted {
			break
		}
	}
	return []Event{s.turnEvent()}
}

// endRound closes the round and opens the next, or ends the game if the last one
// has just run out.
func (s *State) endRound() []Event {
	// A Mastermind who reaches the target holds it at the moment the round ends,
	// which is the only moment the count is checked for a win.
	if ev, done := s.checkCoup(); done {
		return ev
	}
	if s.Round >= RoundFinal {
		return s.finish(SideState, "",
			"Round four closed with the state intact. Nobody owned this room.")
	}
	return s.openRound(s.Round + 1)
}

// openRound resets everything that is scoped to a round, pays everybody their
// influence, and deals whatever the new round brings.
func (s *State) openRound(round int) []Event {
	s.Round = round
	s.turnsThisRound = 0
	// Proposals do not survive the round. Half of them were made against a board
	// that no longer exists, and leaving them up makes the screen a graveyard.
	s.Offers = nil

	for _, p := range s.Players {
		p.Acted = false
		p.SkillsUsed = 0
		p.Pardoned = false
		p.LeakedThisRound = false
		p.Influence += influenceRound
	}
	// The opening seat walks one place a round.
	s.first = (s.first + 1) % len(s.Players)
	s.Current = s.first

	var events []Event
	switch round {
	case RoundArmsRace:
		s.Phase = PhaseArmsRace
		events = append(events, Event{Kind: EvRound, Count: round,
			Text: "The Arms Race: Power cards are dealt. Trade, pay or extort — a card that matches a secret owns the person keeping it."})
		events = append(events, s.dealPower(handSize)...)
	case RoundCatalyst:
		s.Phase = PhaseCatalyst
		events = append(events, Event{Kind: EvRound, Count: round,
			Text: "The Catalyst: one Coup card and one Anti-Coup card are in this room."})
		events = append(events, s.dealPower(catalystTop)...)
		events = append(events, s.dealCatalyst()...)
	default:
		events = append(events, Event{Kind: EvRound, Count: round,
			Text: "The last round. Whoever owns this room at the end of it, owns it."})
		events = append(events, s.dealPower(catalystTop)...)
	}
	return append(events, s.turnEvent())
}

// dealPower gives everybody n Power cards. Publicly it is a count; privately it
// is the cards, exactly like a draw in the other two games.
func (s *State) dealPower(n int) []Event {
	var events []Event
	for _, p := range s.Players {
		var got []Card
		for i := 0; i < n; i++ {
			c, ok := s.drawPower()
			if !ok {
				break
			}
			p.Hand = append(p.Hand, c)
			got = append(got, c)
		}
		if len(got) == 0 {
			continue
		}
		events = append(events, priv(p.ID, Event{Kind: EvIntel, ActorID: p.ID,
			Cards: got, Text: dealtLine(got)}))
	}
	return events
}

func dealtLine(got []Card) string {
	if len(got) == 1 {
		return "Dealt to you: " + got[0].Name + "."
	}
	return fmt.Sprintf("Dealt to you: %d Power cards.", len(got))
}

// dealCatalyst hands the Coup card to one player and the Anti-Coup to another.
// Both are secret; the table is told only that they exist, which is the whole
// engine of round three.
func (s *State) dealCatalyst() []Event {
	n := len(s.Players)
	a := s.rng.Intn(n)
	b := s.rng.Intn(n - 1)
	if b >= a {
		b++
	}
	master, rebel := s.Players[a], s.Players[b]
	master.Coup = true
	rebel.AntiCoup = true

	events := []Event{{Kind: EvCatalyst, Count: s.coupTarget(),
		Text: fmt.Sprintf("Somebody in this room needs %d of you. Somebody else is looking for them.", s.coupTarget())}}
	events = append(events, priv(master.ID, Event{Kind: EvIntel, ActorID: master.ID,
		Cards: []Card{coupCard(3000)},
		Text:  fmt.Sprintf("You hold the Coup. Own %d players when round four closes and the state is yours.", s.coupTarget())}))
	master.learn(s.Round, master.ID, "", "You hold the Coup card.")
	events = append(events, priv(rebel.ID, Event{Kind: EvIntel, ActorID: rebel.ID,
		Cards: []Card{antiCoupCard(3001)},
		Text:  "You hold the Anti-Coup. Pardon their Pawns, take their leaks, and name them before round four closes."}))
	rebel.learn(s.Round, rebel.ID, "", "You hold the Anti-Coup card.")
	return events
}

// checkCoup asks whether the Mastermind has taken the room. Called at the end of
// every round rather than after every action: a majority you cannot hold until
// the round closes is not a coup, it is a moment.
func (s *State) checkCoup() ([]Event, bool) {
	m := s.mastermind()
	if m == nil {
		return nil, false
	}
	if s.pawnsOf(m.ID) < s.coupTarget() {
		return nil, false
	}
	return s.finish(SideMastermind, m.ID, fmt.Sprintf(
		"%s held %d of %d players and took the state.",
		m.Name, s.pawnsOf(m.ID), len(s.Players))), true
}

// finish ends the game and reveals everything. The reveal is a single public
// event carrying every secret, because after this point there is nothing left to
// protect and a table wants to know what it was actually playing.
func (s *State) finish(side Side, winnerID, reason string) []Event {
	s.Phase = PhaseGameOver
	s.Winner = side
	s.Reason = reason
	s.WinnerIDs = nil
	switch side {
	case SideMastermind, SideResistance:
		if winnerID != "" {
			s.WinnerIDs = []string{winnerID}
		}
	case SideState:
		// The state winning means everybody who was never owned won. A Pawn at the
		// final bell survived, but they did not win.
		for _, p := range s.Players {
			if p.MasterID == "" {
				s.WinnerIDs = append(s.WinnerIDs, p.ID)
			}
		}
	}
	return []Event{
		{Kind: EvGameOver, Text: reason, ActorID: winnerID},
		{Kind: EvRevealed, Text: "The files are open."},
	}
}
