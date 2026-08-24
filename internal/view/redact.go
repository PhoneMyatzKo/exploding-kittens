// Package view turns the authoritative game state into the per-player
// projection that is safe to put on the wire.
//
// This is the only place allowed to build client payloads out of a *game.State.
// The rule it exists to enforce: a client learns its own hand in full, everyone
// else's hand as a count, and the draw pile as a count — never its order.
package view

import (
	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/kittens/game"
)

// Membership and Entry are the room's, not this game's: every game reports what
// happened in the same shape, and the room stamps the same connection facts onto
// all of them. Aliased rather than converted so nothing in here has to translate
// between two identical structs.
type (
	Membership = core.Membership
	Entry      = core.Entry
)

// Seat is one player as seen by everybody.
type Seat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Avatar is the portrait's id, empty until the player picks one. Public on
	// purpose: the lobby greys out the ones already taken.
	Avatar    string `json:"avatar,omitempty"`
	HandCount int    `json:"handCount"`
	Alive     bool   `json:"alive"`
	Connected bool   `json:"connected"`
	Host      bool   `json:"host"`
	Current   bool   `json:"current"`
}

// NamedCard is a card *kind* rather than a specific card — what a Three of a Kind
// is asking for. Deliberately not a game.Card: that would carry an ID, and no
// card ID may reach a player who isn't holding it.
type NamedCard struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Pending describes the action sitting in an open Nope window. The cards are
// already public — they were played face up.
type Pending struct {
	Kind      string      `json:"kind"`
	ActorID   string      `json:"actorId"`
	TargetID  string      `json:"targetId,omitempty"`
	Cards     []game.Card `json:"cards"`
	Nopes     int         `json:"nopes"`
	Cancelled bool        `json:"cancelled"`
	// Named is the card a Three of a Kind is demanding, for the banner. Public.
	Named *NamedCard `json:"named,omitempty"`
	// RemainingMs is sent instead of an absolute deadline so the countdown does
	// not depend on the player's device clock agreeing with the server's. TotalMs
	// comes along so the client can size the bar without knowing the constant.
	RemainingMs int64 `json:"remainingMs"`
	TotalMs     int64 `json:"totalMs"`
}

// Me carries the viewer's private information and the affordances the current
// phase grants them. The client renders buttons straight off these booleans and
// validates nothing itself.
type Me struct {
	ID        string      `json:"id"`
	Hand      []game.Card `json:"hand"`
	Alive     bool        `json:"alive"`
	Host      bool        `json:"host"`
	MyTurn    bool        `json:"myTurn"`
	CanNope   bool        `json:"canNope"`
	CanPass   bool        `json:"canPass"`
	MustGive  bool        `json:"mustGive"`
	MustPlace bool        `json:"mustPlace"`
}

// View is the complete payload one client receives.
type View struct {
	Type    string `json:"type"` // always "state"
	Code    string `json:"code"`
	Started bool   `json:"started"`
	Phase   string `json:"phase"`

	Seats []Seat `json:"seats"`
	Me    Me     `json:"me"`

	DeckCount      int        `json:"deckCount"`
	KittensLeft    int        `json:"kittensLeft"`
	DiscardTop     *game.Card `json:"discardTop,omitempty"`
	CurrentID      string     `json:"currentId,omitempty"`
	TurnsRemaining int        `json:"turnsRemaining"`
	Pending        *Pending   `json:"pending,omitempty"`
	WinnerID       string     `json:"winnerId,omitempty"`

	// Public is the room's visibility, so the lobby can say whether the room is
	// advertised. Game is the catalogue slug, which tells the client which
	// renderer this table needs. Both are filled in by the room, which owns those
	// facts; the rules know nothing about either.
	Public bool   `json:"public"`
	Game   string `json:"game"`

	Log []Entry `json:"log"`
}

// Lobby renders a room that has not started yet.
func Lobby(code string, members []Membership, viewerID string) *View {
	v := &View{Type: "state", Code: code, Phase: string(game.PhaseLobby), Log: []Entry{}}
	for _, m := range members {
		v.Seats = append(v.Seats, Seat{ID: m.ID, Name: m.Name, Avatar: m.Avatar, Alive: true, Connected: m.Connected, Host: m.Host})
		if m.ID == viewerID {
			v.Me = Me{ID: m.ID, Alive: true, Host: m.Host, Hand: []game.Card{}}
		}
	}
	return v
}

// Countdown is the timer on an open Nope window. Zero means no window is open.
type Countdown struct {
	RemainingMs int64
	TotalMs     int64
}

// For renders an in-progress game from viewerID's seat.
func For(code string, members []Membership, s *game.State, viewerID string, cd Countdown, log []Entry) *View {
	v := &View{
		Type:           "state",
		Code:           code,
		Started:        true,
		Phase:          string(s.Phase),
		DeckCount:      s.DeckSize(),
		KittensLeft:    s.KittensInDeck(),
		DiscardTop:     s.DiscardTop(),
		TurnsRemaining: s.TurnsRemaining,
		WinnerID:       s.WinnerID,
		Log:            log,
	}
	if log == nil {
		v.Log = []Entry{}
	}
	if s.Phase != game.PhaseGameOver {
		v.CurrentID = s.CurrentID()
	}

	meta := map[string]Membership{}
	for _, m := range members {
		meta[m.ID] = m
	}

	// Other players contribute a hand *count* only. This is the load-bearing line.
	for _, p := range s.Players {
		m := meta[p.ID]
		v.Seats = append(v.Seats, Seat{
			ID:        p.ID,
			Name:      p.Name,
			Avatar:    m.Avatar,
			HandCount: len(p.Hand),
			Alive:     p.Alive,
			Connected: m.Connected,
			Host:      m.Host,
			Current:   s.Phase != game.PhaseGameOver && p.ID == s.CurrentID(),
		})
	}

	if p := s.Find(viewerID); p != nil {
		hand := make([]game.Card, len(p.Hand))
		copy(hand, p.Hand)
		v.Me = Me{
			ID:      p.ID,
			Hand:    hand,
			Alive:   p.Alive,
			Host:    meta[p.ID].Host,
			MyTurn:  s.Phase == game.PhaseTurn && p.Alive && s.CurrentID() == p.ID,
			CanNope: s.CanNope(p.ID),
			// Only the players who could actually Nope are asked to stand down;
			// the window closes once all of them have, so nobody else's answer
			// would change anything.
			CanPass:   s.CanNope(p.ID) && !s.HasPassed(p.ID),
			MustGive:  s.AwaitingGiftFrom() == p.ID,
			MustPlace: s.Phase == game.PhaseDefuse && s.CurrentID() == p.ID,
		}
	} else {
		v.Me = Me{ID: viewerID, Hand: []game.Card{}}
	}

	if s.Pending != nil && s.Phase == game.PhaseNope {
		v.Pending = &Pending{
			Kind:        string(s.Pending.Kind),
			ActorID:     s.Pending.ActorID,
			TargetID:    s.Pending.TargetID,
			Cards:       s.Pending.Cards,
			Nopes:       s.Pending.Nopes,
			Cancelled:   s.Pending.Cancelled(),
			RemainingMs: cd.RemainingMs,
			TotalMs:     cd.TotalMs,
		}
		if s.Pending.HasNamed {
			named := s.Pending.Named
			v.Pending.Named = &NamedCard{Name: named.String(), Slug: named.Slug()}
		}
	}
	return v
}
