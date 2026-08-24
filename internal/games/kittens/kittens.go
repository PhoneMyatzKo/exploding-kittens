// Package kittens adapts the Exploding Kittens rules to the room's Game
// interface.
//
// It is a translator and nothing more: every rule lives in ./game and every
// projection in internal/view, both untouched by this file. What it does own is
// the handful of things that used to sit in internal/room and are in fact
// kitten-specific — the twenty-second Nope window, which phases block the table,
// and what to play for somebody whose phone has gone to sleep.
package kittens

import (
	"math/rand"
	"time"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/kittens/game"
	"boardgame/kittens/internal/view"
)

// NopeWindow is how long an action sits on the table before it resolves. It is
// sent to clients with every open window, so the countdown bar cannot drift out
// of step with it — and exported so the room's tests can assert that it arrives
// there rather than hard-coding twenty seconds of their own.
const NopeWindow = 20 * time.Second

// Game is one Exploding Kittens table.
type Game struct {
	state *game.State
	rng   *rand.Rand
}

// New returns a table with nothing dealt yet.
func New(rng *rand.Rand) core.Game {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Game{rng: rng}
}

func (g *Game) MinPlayers() int { return game.MinPlayers }
func (g *Game) MaxPlayers() int { return game.MaxPlayers }

func (g *Game) Started() bool { return g.state != nil }
func (g *Game) Over() bool    { return g.state != nil && g.state.Phase == game.PhaseGameOver }

func (g *Game) Deal(seats []core.Seat) ([]core.Entry, error) {
	own := make([]game.Seat, 0, len(seats))
	for _, s := range seats {
		own = append(own, game.Seat{ID: s.ID, Name: s.Name})
	}
	s, err := game.NewGame(own, g.rng)
	if err != nil {
		return nil, err
	}
	g.state = s
	return []core.Entry{
		{Kind: "started"},
		{Kind: "turn", ActorID: s.CurrentID()},
	}, nil
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
	out := make([]core.Entry, 0, len(events))
	for _, e := range events {
		out = append(out, core.Entry{
			Kind: string(e.Kind), ActorID: e.ActorID, TargetID: e.TargetID,
			Cards: cardsOrNil(e.Cards), Text: e.Text, OnlyFor: e.OnlyFor,
		})
	}
	return out, nil
}

// cardsOrNil keeps an empty card list off the wire. Entry.Cards is an interface,
// so a typed nil slice inside it is not nil and would serialise as `[]` where the
// old shape omitted the field.
func cardsOrNil(cards []game.Card) any {
	if len(cards) == 0 {
		return nil
	}
	return cards
}

// toAction maps a wire message onto an engine action. ActNopeExpired is
// deliberately absent: only the room's own timer may produce it.
func toAction(playerID string, m core.ClientMsg) (game.Action, bool) {
	a := game.Action{
		PlayerID: playerID, CardIDs: m.CardIDs, TargetID: m.TargetID,
		Index: m.Index, Named: m.Named,
	}
	switch m.Type {
	case "play":
		a.Kind = game.ActPlay
	case "draw":
		a.Kind = game.ActDraw
	case "nope":
		a.Kind = game.ActNope
	case "pass":
		a.Kind = game.ActPass
	case "give":
		a.Kind = game.ActGiveCard
	case "place":
		a.Kind = game.ActPlaceKitten
	default:
		return a, false
	}
	return a, true
}

// BlockedOn is the player the table cannot proceed without. The Nope window is
// not one of these: it is nobody's obligation, which is why it has a timer of its
// own rather than an idle watchdog.
func (g *Game) BlockedOn() string {
	if g.state == nil {
		return ""
	}
	switch g.state.Phase {
	case game.PhaseTurn, game.PhaseDefuse:
		return g.state.CurrentID()
	case game.PhaseFavor:
		return g.state.AwaitingGiftFrom()
	}
	return ""
}

// AutoMove keeps the table moving when whoever must act has dropped off. It
// always picks the least destructive legal move.
func (g *Game) AutoMove(playerID string) ([]core.Entry, error) {
	if g.state == nil {
		return nil, core.ErrNotStarted
	}
	var a game.Action
	switch g.state.Phase {
	case game.PhaseTurn:
		a = game.Action{Kind: game.ActDraw, PlayerID: playerID}
	case game.PhaseDefuse:
		a = game.Action{Kind: game.ActPlaceKitten, PlayerID: playerID,
			Index: g.rng.Intn(g.state.DeckSize() + 1)}
	case game.PhaseFavor:
		p := g.state.Find(playerID)
		if p == nil || len(p.Hand) == 0 {
			return nil, core.ErrNoMove
		}
		a = game.Action{Kind: game.ActGiveCard, PlayerID: playerID, CardIDs: []int{p.Hand[0].ID}}
	default:
		return nil, core.ErrNoMove
	}
	return g.apply(a)
}

// Window is the open Nope window. The token is the Nope count, so stacking one
// restarts the countdown and everybody gets their time back to Yup it.
func (g *Game) Window() (time.Duration, int, bool) {
	if g.state == nil || g.state.Phase != game.PhaseNope || g.state.Pending == nil {
		return 0, 0, false
	}
	return NopeWindow, g.state.Pending.Nopes, true
}

func (g *Game) WindowExpired() []core.Entry {
	if g.state == nil || g.state.Phase != game.PhaseNope {
		return nil
	}
	out, err := g.apply(game.Action{Kind: game.ActNopeExpired})
	if err != nil {
		return nil
	}
	return out
}

func (g *Game) View(sh core.Shell) any {
	var v *view.View
	if g.state == nil {
		v = view.Lobby(sh.Code, sh.Members, sh.ViewerID)
	} else {
		v = view.For(sh.Code, sh.Members, g.state, sh.ViewerID,
			view.Countdown{RemainingMs: sh.RemainingMs, TotalMs: sh.TotalMs}, sh.Log)
	}
	// Visibility and which game this is belong to the room, not to the rules.
	v.Public = sh.Public
	v.Game = sh.Game
	return v
}
