// Package shadows adapts the State of Shadows rules to the room's Game
// interface, and renders them for one player at a time.
//
// The redaction rule is stricter here than in either card game, because almost
// everything in this one is secret. What a client is allowed to know is exactly:
// the seat list with positions and influence, the anonymous newsfeed, its own
// secret, its own hand, its own dossier, the hands of its own Pawns, and its own
// proposals. Everything else — who owns whom, who holds the Coup, who wrote
// which leak — is withheld until the game is over, and the projection in view.go
// is the only code allowed to decide that.
package shadows

import (
	"math/rand"
	"time"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/shadows/game"
)

// Game is one State of Shadows table.
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
	s, events, err := game.NewGame(own, g.rng)
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
			Points: e.Points, OnlyFor: e.OnlyFor,
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

// toAction maps a wire message onto an engine action. ActAdvance is deliberately
// absent: forcing a round on is the room's business, never a client's.
func toAction(playerID string, m core.ClientMsg) (game.Action, bool) {
	a := game.Action{
		PlayerID: playerID,
		TargetID: m.TargetID,
		CardID:   m.CardID,
		CardIDs:  m.CardIDs,
		WantIDs:  m.WantIDs,
		Amount:   m.Amount,
		Demand:   m.Demand,
		Slug:     m.Slug,
		Text:     m.Text,
		OfferID:  m.OfferID,
	}
	switch m.Type {
	case "skill":
		a.Kind = game.ActSkill
	case "power":
		a.Kind = game.ActPower
	case "clean":
		a.Kind = game.ActCleanRecord
	case "evidence":
		a.Kind = game.ActEvidence
	case "seize":
		a.Kind = game.ActSeize
	case "accuse":
		a.Kind = game.ActAccuse
	case "pass":
		a.Kind = game.ActPass
	case "offer":
		a.Kind = game.ActOffer
	case "accept":
		a.Kind = game.ActAccept
	case "decline":
		a.Kind = game.ActDecline
	case "news":
		a.Kind = game.ActNews
	case "leak":
		a.Kind = game.ActLeak
	case "pardon":
		a.Kind = game.ActPardon
	default:
		return a, false
	}
	return a, true
}

// BlockedOn is whoever owes the table a main action. Every seat owes exactly one
// per round, so a player who has closed their laptop stops the round rather than
// only their own turn — which is why AutoMove below matters more here than in a
// card game.
func (g *Game) BlockedOn() string {
	if g.state == nil {
		return ""
	}
	return g.state.Blocked()
}

// AutoMove passes. There is no such thing as a harmless investigation in this
// game: every skill hands somebody information, every Power card is spent
// forever, and a pardon costs four influence. Passing is the only move that
// cannot be regretted on somebody's behalf.
func (g *Game) AutoMove(playerID string) ([]core.Entry, error) {
	if g.state == nil {
		return nil, core.ErrNotStarted
	}
	if g.state.CurrentID() != playerID {
		return nil, core.ErrNoMove
	}
	return g.apply(game.Action{Kind: game.ActPass, PlayerID: playerID})
}

// Window: none. Nothing in this game is interrupted — the pressure comes from
// the round running out, not from a countdown on a single card.
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
