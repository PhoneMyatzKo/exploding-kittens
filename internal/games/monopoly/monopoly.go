// Package monopoly adapts Monopoly Myanmar to the room's Game interface.
//
// A translator and nothing more: every rule is in ./game and every projection in
// ./view.go, both untouched by this file. What it owns is the handful of things
// that are Monopoly's rather than the room's — which phases block the table, and
// what to play for somebody whose phone has gone to sleep.
package monopoly

import (
	"bytes"
	"encoding/gob"
	"errors"
	"time"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/monopoly/game"
	"boardgame/kittens/internal/prng"
)

// Game is one Monopoly Myanmar table.
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
		entry := core.Entry{
			Kind: string(e.Kind), ActorID: e.ActorID, TargetID: e.TargetID,
			// Count is the money, in kyat. Sent as a number and never as a
			// formatted string, because the client has to be able to write it in
			// either language.
			Count: e.Amount,
		}
		if e.Tile >= 0 {
			tile := e.Tile
			entry.Tile = &tile
		}
		if e.Kind == game.EvRolled {
			entry.Dice = []int{e.Dice[0], e.Dice[1]}
		}
		out = append(out, entry)
	}
	return out, nil
}

// toAction maps a wire message onto an engine action. Three moves, and each is
// legal in exactly one phase — the engine re-checks that, so this only has to
// recognise the name.
func toAction(playerID string, m core.ClientMsg) (game.Action, bool) {
	a := game.Action{PlayerID: playerID}
	switch m.Type {
	case "roll":
		a.Kind = game.ActRoll
	case "buy":
		a.Kind = game.ActBuy
	case "pass":
		a.Kind = game.ActPass
	default:
		return a, false
	}
	return a, true
}

// BlockedOn is the player the table cannot proceed without. Every phase here has
// exactly one, which is what makes the watchdog straightforward: unlike the Nope
// window, nothing in this slice is anybody-may-act.
func (g *Game) BlockedOn() string {
	if g.state == nil || g.state.Phase == game.PhaseGameOver {
		return ""
	}
	return g.state.CurrentID()
}

// AutoMove keeps the table moving when whoever must act has dropped off.
//
// Rolling for an absent player is safe — the dice are the server's anyway — but
// buying is not: it spends their money on a square they never chose. So an absent
// player passes, which costs them an opportunity and nothing else.
func (g *Game) AutoMove(playerID string) ([]core.Entry, error) {
	if g.state == nil {
		return nil, core.ErrNotStarted
	}
	var a game.Action
	switch g.state.Phase {
	case game.PhaseRoll:
		a = game.Action{Kind: game.ActRoll, PlayerID: playerID}
	case game.PhaseBuy:
		a = game.Action{Kind: game.ActPass, PlayerID: playerID}
	default:
		return nil, core.ErrNoMove
	}
	return g.apply(a)
}

// Window: nothing in this slice is a timed free-for-all. Auctions will be one,
// and this is the hook they use — see the gap list at the foot of engine.go.
func (g *Game) Window() (time.Duration, int, bool) { return 0, 0, false }

func (g *Game) WindowExpired() []core.Entry { return nil }

func (g *Game) View(sh core.Shell) any {
	var v *View
	if g.state == nil {
		v = Lobby(sh.Code, sh.Members, sh.ViewerID)
	} else {
		v = For(sh.Code, sh.Members, g.state, sh.ViewerID, sh.Log)
	}
	// Visibility and which game this is belong to the room, not to the rules.
	v.Public = sh.Public
	v.Game = sh.Game
	return v
}

// ─────────────────────────────────────────────────────── saving and resuming
//
// The one game here that really needs this: a Monopoly session runs for hours,
// and losing it to a deploy is the difference between a game people finish and
// one they abandon. The board itself is not saved — it is a constant, served
// from /api/board — so a snapshot is the players, the deeds and the dice.

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
		return errors.New("monopoly: snapshot has no dice")
	}
	g.state, g.rng = &st, st.RNG
	return nil
}
