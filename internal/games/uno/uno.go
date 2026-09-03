package uno

import (
	"bytes"
	"encoding/gob"
	"errors"
	"time"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/uno/game"
	"boardgame/kittens/internal/prng"
)

// Game is one UNO table.
type Game struct {
	state *game.State
	rng   *prng.Source
}

// New returns a table with nothing dealt yet.
func New(rng *prng.Source) core.Game {
	if rng == nil {
		rng = prng.NewSeeded()
	}
	return &Game{rng: rng}
}

func (g *Game) MinPlayers() int { return game.MinPlayers }
func (g *Game) MaxPlayers() int { return game.MaxPlayers }

func (g *Game) Started() bool { return g.state != nil }
func (g *Game) Over() bool    { return g.state != nil && g.state.Phase == game.PhaseGameOver }

// Deal starts a single round.
//
// The rules run to 500 points over as many rounds as that takes, and the engine
// implements that in full — but a room already has a way to play again, and it
// resets the table properly, picks up whoever joined while the last one was
// running and hands the host the button. A round is therefore a game here, and
// the winner's score is what they took off everybody else's hands.
func (g *Game) Deal(seats []core.Seat) ([]core.Entry, error) {
	own := make([]game.Seat, 0, len(seats))
	for _, s := range seats {
		own = append(own, game.Seat{ID: s.ID, Name: s.Name})
	}
	s, events, err := game.NewGameTo(own, g.rng, 0)
	if err != nil {
		return nil, err
	}
	g.state = s
	return entries(events), nil
}

func (g *Game) Reset() { g.state = nil }

func (g *Game) Rename(playerID, name string) {
	if g.state == nil {
		return
	}
	if p := g.state.Find(playerID); p != nil {
		p.Name = name
	}
}

func (g *Game) Submit(playerID string, msg core.ClientMsg) ([]core.Entry, error) {
	if g.state == nil {
		return nil, core.ErrNotStarted
	}
	a, ok := toAction(playerID, msg)
	if !ok {
		return nil, core.ErrUnknownAction
	}
	return g.apply(a)
}

func (g *Game) apply(a game.Action) ([]core.Entry, error) {
	events, err := game.Apply(g.state, a)
	if err != nil {
		return nil, err
	}
	return entries(events), nil
}

func entries(events []game.Event) []core.Entry {
	out := make([]core.Entry, 0, len(events))
	for _, e := range events {
		out = append(out, core.Entry{
			Kind: string(e.Kind), ActorID: e.ActorID, TargetID: e.TargetID,
			Cards: cardsOrNil(e.Cards), Text: e.Text, Count: e.Count,
			Points: e.Points, Colour: e.Colour, OnlyFor: e.OnlyFor,
		})
	}
	return out
}

// cardsOrNil keeps an empty card list off the wire: Entry.Cards is an interface,
// and a typed nil slice inside one is not nil, so it would serialise as `[]`.
func cardsOrNil(cards []game.Card) any {
	if len(cards) == 0 {
		return nil
	}
	return cards
}

// toAction maps a wire message onto an engine action.
func toAction(playerID string, m core.ClientMsg) (game.Action, bool) {
	a := game.Action{
		PlayerID: playerID, CardID: m.CardID, Colour: m.Colour,
		TargetID: m.TargetID, SayUno: m.SayUno,
	}
	switch m.Type {
	case "play":
		a.Kind = game.ActPlay
	case "draw":
		a.Kind = game.ActDraw
	case "pass":
		a.Kind = game.ActPass
	case "colour":
		a.Kind = game.ActColour
	case "uno":
		a.Kind = game.ActCallUno
	case "catch":
		a.Kind = game.ActCatchUno
	case "challenge":
		a.Kind = game.ActChallenge
	case "accept":
		a.Kind = game.ActAcceptDraw
	default:
		return a, false
	}
	return a, true
}

// BlockedOn is the player the table cannot proceed without: whoever holds the
// turn, or whoever a Draw Four is aimed at.
func (g *Game) BlockedOn() string {
	if g.state == nil {
		return ""
	}
	switch g.state.Phase {
	case game.PhaseTurn, game.PhaseDrawn, game.PhaseColour:
		return g.state.CurrentID()
	case game.PhaseChallenge:
		target, _ := g.state.Challenged()
		return target
	}
	return ""
}

// AutoMove plays for somebody whose phone has gone to sleep. Every branch picks
// the move that gives away least: draw rather than play, keep rather than use,
// and take the four cards rather than gamble on a challenge.
func (g *Game) AutoMove(playerID string) ([]core.Entry, error) {
	if g.state == nil {
		return nil, core.ErrNotStarted
	}
	switch g.state.Phase {
	case game.PhaseTurn:
		return g.apply(game.Action{Kind: game.ActDraw, PlayerID: playerID})
	case game.PhaseDrawn:
		return g.apply(game.Action{Kind: game.ActPass, PlayerID: playerID})
	case game.PhaseColour:
		return g.apply(game.Action{Kind: game.ActColour, PlayerID: playerID,
			Colour: g.bestColour(playerID)})
	case game.PhaseChallenge:
		return g.apply(game.Action{Kind: game.ActAcceptDraw, PlayerID: playerID})
	}
	return nil, core.ErrNoMove
}

// bestColour is the colour a player holds most of, so a wild played on their
// behalf is at least not actively self-defeating. Falls back to red, which is as
// arbitrary as any other and never invalid.
func (g *Game) bestColour(playerID string) string {
	p := g.state.Find(playerID)
	if p == nil {
		return game.Red.Slug()
	}
	best, count := game.Red, -1
	for _, colour := range game.Colours() {
		n := 0
		for _, card := range p.Hand {
			if card.Colour == colour {
				n++
			}
		}
		if n > count {
			best, count = colour, n
		}
	}
	return best.Slug()
}

// Window: UNO has none. Every move belongs to exactly one player, so there is
// nothing for the rest of the table to interrupt and nothing to count down.
func (g *Game) Window() (time.Duration, int, bool) { return 0, 0, false }

func (g *Game) WindowExpired() []core.Entry { return nil }

func (g *Game) View(sh core.Shell) any {
	var v *View
	if g.state == nil {
		v = lobby(sh)
	} else {
		v = render(g.state, sh)
	}
	v.Public = sh.Public
	v.Game = sh.Game
	return v
}

// ─────────────────────────────────────────────────────── saving and resuming

func (g *Game) Snapshot() ([]byte, error) {
	if g.state == nil {
		return nil, nil // nothing dealt: the room's own lobby is the whole state
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(g.state); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *Game) Restore(data []byte) error {
	if len(data) == 0 {
		g.state = nil
		return nil
	}
	var st game.State
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&st); err != nil {
		return err
	}
	if st.RNG == nil {
		return errors.New("uno: snapshot has no dice")
	}
	g.state, g.rng = &st, st.RNG
	return nil
}
