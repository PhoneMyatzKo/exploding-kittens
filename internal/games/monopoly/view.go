package monopoly

import (
	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/monopoly/game"
)

// The per-player projection.
//
// Monopoly is a game of open information: money, deeds and positions are all
// face up at a real table, so unlike Exploding Kittens there is almost nothing
// to hide and this file is a rename rather than a redaction. What *will* need
// care is the two card decks, whose order must stay secret — the comment on
// Chance in engine.go's gap list is the reminder.
//
// The board itself is not here. It is static for the whole game and is fetched
// once from GET /api/board; sending forty squares of names and prices with every
// roll would be most of the payload.

// View is what one client receives.
type View struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Started bool   `json:"started"`
	Phase   string `json:"phase"`

	// Public and Game are the room's, filled in by the adapter's View().
	Public bool   `json:"public"`
	Game   string `json:"game"`

	Seats []Seat `json:"seats"`
	Me    Me     `json:"me"`

	// CurrentID is whose turn it is; Dice is the last throw, both faces, so the
	// client can show what was rolled rather than only the total.
	CurrentID string `json:"currentId,omitempty"`
	Dice      [2]int `json:"dice"`

	// Owner is one entry per square: the player holding it, or "" for the bank.
	// A flat array indexed by square, because that is how the board is drawn.
	Owner []string `json:"owner"`

	WinnerID string `json:"winnerId,omitempty"`

	Log []core.Entry `json:"log"`
}

// Seat is one player as everybody else sees them — which, here, is everything
// about them.
type Seat struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	Cash   int    `json:"cash"`
	Pos    int    `json:"pos"`
	Alive  bool   `json:"alive"`
	// Deeds is how many squares they hold. The squares themselves are in Owner,
	// which the board is drawn from; this is for the seat strip.
	Deeds     int  `json:"deeds"`
	Current   bool `json:"current"`
	Connected bool `json:"connected"`
	Host      bool `json:"host"`
}

// Me is the viewer's own row plus what they may do next. The client renders
// affordances from these rather than deciding for itself, so a button is never
// offered that the server would refuse.
type Me struct {
	ID    string `json:"id"`
	Cash  int    `json:"cash"`
	Pos   int    `json:"pos"`
	Alive bool   `json:"alive"`
	Host  bool   `json:"host"`

	MyTurn  bool `json:"myTurn"`
	CanRoll bool `json:"canRoll"`
	// Offer is the square being offered for sale, or -1. Its price is on the
	// board the client already has.
	Offer int `json:"offer"`
}

// Lobby renders a room whose game has not started.
func Lobby(code string, members []core.Membership, viewerID string) *View {
	v := &View{Type: "state", Phase: "lobby", Code: code, Log: []core.Entry{}, Owner: []string{}}
	for _, m := range members {
		v.Seats = append(v.Seats, Seat{
			ID: m.ID, Name: m.Name, Avatar: m.Avatar,
			Alive: true, Connected: m.Connected, Host: m.Host,
		})
		if m.ID == viewerID {
			v.Me = Me{ID: m.ID, Alive: true, Host: m.Host, Offer: -1}
		}
	}
	return v
}

// For renders a game in progress from viewerID's seat.
func For(code string, members []core.Membership, s *game.State, viewerID string, log []core.Entry) *View {
	v := &View{
		Type:      "state",
		Code:      code,
		Started:   true,
		Phase:     string(s.Phase),
		CurrentID: s.CurrentID(),
		Dice:      s.Dice,
		WinnerID:  s.WinnerID,
		Log:       log,
		Me:        Me{ID: viewerID, Offer: -1},
	}
	if log == nil {
		v.Log = []core.Entry{}
	}

	v.Owner = make([]string, game.BoardSize)
	for i := 0; i < game.BoardSize; i++ {
		v.Owner[i] = s.OwnerOf(i)
	}

	// Connection and host are the room's facts; cash and position are the game's.
	// Walked in the room's order so the seat strip matches the lobby's.
	for _, m := range members {
		p := s.Find(m.ID)
		if p == nil {
			continue
		}
		seat := Seat{
			ID: p.ID, Name: p.Name, Avatar: m.Avatar,
			Cash: p.Cash, Pos: p.Pos, Alive: p.Alive,
			Deeds: len(s.Owned(p.ID)), Current: p.ID == s.CurrentID(),
			Connected: m.Connected, Host: m.Host,
		}
		v.Seats = append(v.Seats, seat)

		if p.ID != viewerID {
			continue
		}
		v.Me = Me{
			ID: p.ID, Cash: p.Cash, Pos: p.Pos, Alive: p.Alive, Host: m.Host,
			MyTurn:  p.ID == s.CurrentID() && s.Phase != game.PhaseGameOver,
			CanRoll: p.ID == s.CurrentID() && s.Phase == game.PhaseRoll,
			Offer:   -1,
		}
		if s.Phase == game.PhaseBuy && p.ID == s.CurrentID() {
			v.Me.Offer = s.PendingTile()
		}
	}
	return v
}
