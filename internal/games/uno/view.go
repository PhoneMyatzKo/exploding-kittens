// Package uno adapts the UNO rules to the room's Game interface, and renders
// them for one player at a time.
//
// The redaction rule is the same one Exploding Kittens is held to: you learn
// your own hand in full, everybody else's as a count, and the deck as a count —
// never its order. The one thing UNO adds is that the *colour in force* is
// public, because it is what everybody is playing against.
package uno

import (
	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/uno/game"
)

// Seat is one player as seen by everybody.
type Seat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Avatar is the portrait's id, empty until the player picks one. Public on
	// purpose: the lobby greys out the ones already taken.
	Avatar    string `json:"avatar,omitempty"`
	HandCount int    `json:"handCount"`
	Connected bool   `json:"connected"`
	Host      bool   `json:"host"`
	Current   bool   `json:"current"`
	Score     int    `json:"score"`
	// Uno is set for the player who is down to one card, whether or not they
	// remembered to say so. Called says whether they did — the pair is what the
	// catch button is drawn from.
	Uno    bool `json:"uno,omitempty"`
	Called bool `json:"called,omitempty"`
	// Alive exists because the shell's seat list is shared with a game where
	// players are eliminated. Nobody is ever out in UNO, so it is always true.
	Alive bool `json:"alive"`
}

// Me carries the viewer's private information and the affordances the current
// phase grants them. The client renders buttons straight off these booleans and
// validates nothing itself.
type Me struct {
	ID     string      `json:"id"`
	Hand   []game.Card `json:"hand"`
	Host   bool        `json:"host"`
	MyTurn bool        `json:"myTurn"`
	// Playable is the subset of Hand that can be laid down right now. Sent rather
	// than recomputed in the browser so the rule for what matches lives in exactly
	// one place.
	Playable []int `json:"playable"`
	CanDraw  bool  `json:"canDraw"`
	// CanPass is set once you have drawn and may decline what you drew.
	CanPass bool `json:"canPass"`
	// DrawnID is the card you just took, which is the only one you may play.
	DrawnID *int `json:"drawnId,omitempty"`
	// MustName is set while your wild is waiting for a colour.
	MustName bool `json:"mustName"`
	// MustAnswer is set while a Wild Draw Four is aimed at you.
	MustAnswer bool `json:"mustAnswer"`
	// CanCallUno is your own "UNO!" button; CatchID is somebody else's slip.
	CanCallUno bool    `json:"canCallUno"`
	CatchID    *string `json:"catchId,omitempty"`
	Alive      bool    `json:"alive"`
}

// Challenge is an open Wild Draw Four. Whether it was a bluff is deliberately
// absent: that is the fact the bet is on.
type Challenge struct {
	ActorID  string `json:"actorId"`
	TargetID string `json:"targetId"`
}

// View is the complete payload one client receives.
type View struct {
	Type    string `json:"type"` // always "state"
	Code    string `json:"code"`
	Started bool   `json:"started"`
	Phase   string `json:"phase"`

	Seats []Seat `json:"seats"`
	Me    Me     `json:"me"`

	DeckCount  int        `json:"deckCount"`
	DiscardTop *game.Card `json:"discardTop,omitempty"`
	// Colour is the colour in force, which is not always the top card's own — a
	// wild names one, and the name outlives the card.
	Colour    string `json:"colour,omitempty"`
	Direction int    `json:"direction"`
	CurrentID string `json:"currentId,omitempty"`

	Challenge *Challenge `json:"challenge,omitempty"`
	WinnerID  string     `json:"winnerId,omitempty"`
	// Points is what the winner scored, for the game-over card.
	Points int `json:"points,omitempty"`

	// Public is the room's visibility and Game the catalogue slug. Both are
	// filled in by the room, which owns those facts.
	Public bool   `json:"public"`
	Game   string `json:"game"`

	Log []core.Entry `json:"log"`
}

// lobby renders a room that has not been dealt yet. Deliberately the same shape
// as a live view with everything empty, so the shell's lobby code does not have
// to know which of the two it is looking at.
func lobby(sh core.Shell) *View {
	v := &View{Type: "state", Code: sh.Code, Phase: "lobby", Direction: 1,
		Log: []core.Entry{}, Me: Me{Hand: []game.Card{}, Alive: true}}
	for _, m := range sh.Members {
		v.Seats = append(v.Seats, Seat{
			ID: m.ID, Name: m.Name, Avatar: m.Avatar,
			Connected: m.Connected, Host: m.Host, Alive: true,
		})
		if m.ID == sh.ViewerID {
			v.Me.ID = m.ID
			v.Me.Host = m.Host
		}
	}
	return v
}

// render projects a live game from one seat.
func render(s *game.State, sh core.Shell) *View {
	v := &View{
		Type:       "state",
		Code:       sh.Code,
		Started:    true,
		Phase:      string(s.Phase),
		DeckCount:  s.DeckSize(),
		DiscardTop: s.DiscardTop(),
		Colour:     s.ActiveColour(),
		Direction:  s.Direction(),
		WinnerID:   s.WinnerID,
		Log:        sh.Log,
	}
	if v.Log == nil {
		v.Log = []core.Entry{}
	}
	if s.Phase != game.PhaseGameOver {
		v.CurrentID = s.CurrentID()
	}
	if s.WinnerID != "" {
		if p := s.Find(s.WinnerID); p != nil {
			v.Points = p.Score
		}
	}

	unoID, unoCalled, unoOpen := s.UnoWindow()
	meta := map[string]core.Membership{}
	for _, m := range sh.Members {
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
			Connected: m.Connected,
			Host:      m.Host,
			Current:   v.CurrentID == p.ID,
			Score:     p.Score,
			Uno:       unoOpen && unoID == p.ID,
			Called:    unoOpen && unoID == p.ID && unoCalled,
			Alive:     true,
		})
	}

	target, actor := s.Challenged()
	if target != "" {
		v.Challenge = &Challenge{ActorID: actor, TargetID: target}
	}

	p := s.Find(sh.ViewerID)
	if p == nil {
		// A spectator, or somebody whose seat was dealt out from under them.
		v.Me = Me{ID: sh.ViewerID, Hand: []game.Card{}, Alive: true}
		return v
	}

	hand := make([]game.Card, len(p.Hand))
	copy(hand, p.Hand)
	playable := s.PlayableIDs(p.ID)
	if playable == nil {
		playable = []int{}
	}
	v.Me = Me{
		ID:         p.ID,
		Hand:       hand,
		Host:       meta[p.ID].Host,
		MyTurn:     v.CurrentID == p.ID,
		Playable:   playable,
		CanDraw:    s.Phase == game.PhaseTurn && v.CurrentID == p.ID,
		CanPass:    s.Phase == game.PhaseDrawn && v.CurrentID == p.ID,
		MustName:   s.MustName() == p.ID,
		MustAnswer: target == p.ID,
		CanCallUno: unoOpen && unoID == p.ID && !unoCalled,
		Alive:      true,
	}
	if s.Phase == game.PhaseDrawn && s.Drawn != nil && v.CurrentID == p.ID {
		id := s.Drawn.ID
		v.Me.DrawnID = &id
	}
	if s.CanCatch(p.ID) {
		caught := unoID
		v.Me.CatchID = &caught
	}
	return v
}
