package main

import "fmt"

func stateGoTmpl(g game) string {
	return gofmtOrPanic(fmt.Sprintf(`// Package game implements %s's rules.
//
// TODO: this is a scaffold, not a ruleset. Replace State, Phase and the
// helpers below with whatever %s actually needs, the way
// internal/games/kittens/game and internal/games/uno/game do for theirs.
package game

// MinPlayers and MaxPlayers bound the table. TODO: the real numbers.
const (
	MinPlayers = %d
	MaxPlayers = %d
)

// Seat is one player being dealt in.
type Seat struct {
	ID   string
	Name string
}

// Phase is where the round currently stands.
type Phase string

const (
	PhaseTurn     Phase = "turn"
	PhaseGameOver Phase = "gameOver"
)

// State is one round of %s.
type State struct {
	Seats []Seat
	Phase Phase
}

// CurrentID is whoever's turn it is. TODO: track this for real instead of
// always returning the first seat.
func (s *State) CurrentID() string {
	if len(s.Seats) == 0 {
		return ""
	}
	return s.Seats[0].ID
}

func (s *State) Find(playerID string) *Seat {
	for i := range s.Seats {
		if s.Seats[i].ID == playerID {
			return &s.Seats[i]
		}
	}
	return nil
}
`, g.Name, g.Name, g.Min, g.Max, g.Name))
}

func engineGoTmpl(g game) string {
	return gofmtOrPanic(fmt.Sprintf(`package game

import (
	"errors"
	"fmt"
)

// ActionKind names one legal move. TODO: replace with %s's real moves.
type ActionKind string

const (
	ActPlay ActionKind = "play"
)

// Action is one player's move.
type Action struct {
	Kind     ActionKind
	PlayerID string
}

// EventKind names one line of the play-by-play the room forwards to clients.
type EventKind string

// Event is one thing that happened.
type Event struct {
	Kind    EventKind
	ActorID string
}

// NewGame deals a fresh round. TODO: shuffle a deck, deal hands, decide who
// goes first — whatever %s's setup actually is.
func NewGame(seats []Seat, rng *prng.Source) (*State, error) {
	if len(seats) < MinPlayers || len(seats) > MaxPlayers {
		return nil, fmt.Errorf("%s takes %%d-%%d players, got %%d", MinPlayers, MaxPlayers, len(seats))
	}
	return &State{Seats: seats, Phase: PhaseTurn}, nil
}

// Apply runs one action against the state. TODO: implement %s's rules.
func Apply(s *State, a Action) ([]Event, error) {
	return nil, errors.New("%s: rules not implemented yet")
}
`, g.Name, g.Name, g.Slug, g.Name, g.Slug))
}

func adapterGoTmpl(g game) string {
	return gofmtOrPanic(fmt.Sprintf(`// Package %s adapts %s to the room's Game interface.
//
// TODO: this is a scaffold. Fill in the rules under ./game, then wire
// Submit, AutoMove and View to them the way internal/games/kittens/kittens.go
// and internal/games/uno/uno.go do for theirs — those two are the reference.
package %s

import (
	"time"

	"boardgame/kittens/internal/prng"
	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/%s/game"
)

// Game is one %s table.
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

// Submit runs one player's message. TODO: map msg.Type onto a game.Action the
// way kittens.go's and uno.go's toAction() do, then call game.Apply.
func (g *Game) Submit(playerID string, msg core.ClientMsg) ([]core.Entry, error) {
	if g.state == nil {
		return nil, core.ErrNotStarted
	}
	return nil, core.ErrUnknownAction
}

// BlockedOn is the player everyone else is waiting for. TODO: return "" for
// phases nobody is blocking on, the way kittens.go and uno.go do.
func (g *Game) BlockedOn() string {
	if g.state == nil {
		return ""
	}
	return g.state.CurrentID()
}

// AutoMove plays for a player who has gone idle. TODO: the least destructive
// legal move for each phase, once there are phases.
func (g *Game) AutoMove(playerID string) ([]core.Entry, error) {
	return nil, core.ErrNoMove
}

// Window: no timed window until %s has one. TODO: see kittens.go's Nope
// window for the shape a real one takes.
func (g *Game) Window() (time.Duration, int, bool) { return 0, 0, false }

func (g *Game) WindowExpired() []core.Entry { return nil }

// View builds the payload one client receives. TODO: report real state —
// this stub sends only what the shell needs to draw the shared lobby chrome.
func (g *Game) View(sh core.Shell) any {
	return map[string]any{
		"public": sh.Public,
		"game":   sh.Game,
	}
}
`, g.Pkg, g.Name, g.Pkg, g.Slug, g.Name, g.Name))
}

func tableHTMLTmpl(g game) string {
	return fmt.Sprintf(`<!-- %s — everything on screen once a round is dealt.
     Injected by mountGame() in app.js; see its doc comment (web/app.js,
     "mounting a game") for the contract this file and index.js must meet.

     table, seats, leave-table, rules-table, sound-table are the ids every
     game's table carries — the shell wires leave-table itself and expects
     the others to exist. Everything past that is %s's own; this is a
     scaffold, so it is close to the minimum that satisfies the shell. -->

<section id="table" class="screen table" hidden>
  <header class="topbar">
    <span id="table-code" class="pill"></span>
    <span id="turn-banner" class="turn-banner"></span>
    <span id="sound-table"></span>
    <button id="rules-table" class="btn ghost small" aria-label="How to play" title="How to play">?</button>
    <button id="leave-table" class="btn ghost small" aria-label="Leave the game" title="Leave the game">
      <i class="fa-solid fa-right-from-bracket"></i>
    </button>
  </header>

  <div id="seats" class="seats"></div>

  <div class="table-body">
    <div class="center">
      <!-- TODO: %s's board/hand/discard markup goes here. -->
    </div>
    <aside id="log-panel" class="log-panel">
      <div id="log" class="log"></div>
    </aside>
  </div>
</section>
`, g.Name, g.Name, g.Name)
}

func indexJSTmpl(g game) string {
	return fmt.Sprintf(`// %s — the client half of one game.
//
// TODO: this is a scaffold. See web/games/uno/index.js for a fully wired
// example of the contract mountGame() expects (web/app.js, "mounting a
// game"): mount() wires the markup table.html injects, render() draws every
// state once the round has started, and the shell handles the lobby itself.

import { $ } from "../../core/dom.js";

let ctx = null;

export default {
  // TODO: false if %s has no use for the cat portraits, the way uno sets this.
  avatars: true,

  // Mute-button slots this game's markup carries, beyond the shell's own.
  slots: ["sound-table"],

  mount(context) {
    ctx = context;
    $("rules-table").onclick = () => this.showRules(true);
  },

  unmount() {
    ctx = null;
  },

  onState(view) {
    // TODO: sounds, cinematics — anything that should fire once per state
    // rather than on every re-render.
  },

  render(view) {
    // TODO: draw %s's table from view.
  },

  leaveTable() {
    // TODO: tear down whatever render() put up, without unmounting.
  },

  onPrivate(entry) {
    // TODO: a message only this player is entitled to see.
  },

  onEscape() {
    return false;
  },

  showRules(on) {
    // TODO: the rules overlay is the shell's markup; wire its open/close here.
  },
};
`, g.Name, g.Name, g.Name)
}
