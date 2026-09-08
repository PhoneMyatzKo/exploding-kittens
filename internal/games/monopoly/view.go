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

	// Houses is one entry per square: how many buildings stand on it, 5 being a
	// hotel. Beside Owner and for the same reason — both are read once per square
	// while the board is drawn.
	Houses []int `json:"houses"`
	// What the bank has left to sell. On the wire because running out is a rule
	// rather than an accident, and a player denied a house deserves to be told it
	// is the box that is empty and not their wallet.
	HousesLeft int `json:"housesLeft"`
	HotelsLeft int `json:"hotelsLeft"`

	// Drawn is the Chance or Community Chest card face up on the table, as an
	// index into the pack the client fetched with the board, or -1. Public: at a
	// real table the card is read out, and everybody watching it happen is half
	// the point of the deck.
	Drawn int `json:"drawn"`

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
	Deeds int `json:"deeds"`
	// Buildings is how many of their squares carry anything, a hotel counting as
	// one. The seat strip has room for a number, not for a board.
	Buildings int `json:"buildings,omitempty"`
	// Jailed is being held, as opposed to standing on the corner as a visitor.
	// Pardons is how many get-out-of-jail cards they are holding — public,
	// because at a table you can see who is sitting on one.
	Jailed    bool `json:"jailed,omitempty"`
	Pardons   int  `json:"pardons,omitempty"`
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
	// Jailed, and what they can do about it. The client renders affordances from
	// these rather than working them out, so a button is never offered that the
	// server would refuse.
	Jailed     bool `json:"jailed,omitempty"`
	CanPayFine bool `json:"canPayFine,omitempty"`
	CanPardon  bool `json:"canPardon,omitempty"`
	Pardons    int  `json:"pardons,omitempty"`
	// MustRead is a card waiting to be acknowledged by this player.
	MustRead bool `json:"mustRead,omitempty"`
	// Offer is the square being offered for sale, or -1. Its price is on the
	// board the client already has.
	Offer int `json:"offer"`

	// CanBuild and CanSell are the squares this player may build on and sell
	// from, right now, in board order. Lists rather than a flag per square,
	// because the client's job is to render exactly them: the even-build rule and
	// the bank's stock are the server's to work out, and a client that decided for
	// itself would be a second implementation of the rule that could disagree.
	//
	// Empty on everybody but the current player, and empty for them outside the
	// two phases building is legal in.
	CanBuild []int `json:"canBuild,omitempty"`
	CanSell  []int `json:"canSell,omitempty"`
}

// Lobby renders a room whose game has not started.
func Lobby(code string, members []core.Membership, viewerID string) *View {
	v := &View{
		Type: "state", Phase: "lobby", Code: code,
		Log: []core.Entry{}, Owner: []string{}, Houses: []int{}, Drawn: -1,
		HousesLeft: game.HouseSupply, HotelsLeft: game.HotelSupply,
	}
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
		Drawn:     -1,
		Log:       log,
		Me:        Me{ID: viewerID, Offer: -1},
	}
	if c := s.DrawnCard(); c != nil {
		v.Drawn = s.Drawn
	}
	if log == nil {
		v.Log = []core.Entry{}
	}

	v.Owner = make([]string, game.BoardSize)
	v.Houses = make([]int, game.BoardSize)
	for i := 0; i < game.BoardSize; i++ {
		v.Owner[i] = s.OwnerOf(i)
		v.Houses[i] = s.Houses[i]
	}
	v.HousesLeft, v.HotelsLeft = s.HousesLeft(), s.HotelsLeft()

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
			Deeds: len(s.Owned(p.ID)), Buildings: s.BuildingsOn(p.ID),
			Current: p.ID == s.CurrentID(),
			Jailed:  p.Jailed, Pardons: p.Pardons,
			Connected: m.Connected, Host: m.Host,
		}
		v.Seats = append(v.Seats, seat)

		if p.ID != viewerID {
			continue
		}
		mine := p.ID == s.CurrentID()
		v.Me = Me{
			ID: p.ID, Cash: p.Cash, Pos: p.Pos, Alive: p.Alive, Host: m.Host,
			MyTurn: mine && s.Phase != game.PhaseGameOver,
			// Rolling in jail is an attempt at a double rather than a move, but it
			// is the same button and the same message, so it is offered the same
			// way.
			CanRoll:    mine && (s.Phase == game.PhaseRoll || s.Phase == game.PhaseJail),
			Jailed:     p.Jailed,
			Pardons:    p.Pardons,
			CanPayFine: mine && s.Phase == game.PhaseJail && p.Cash >= game.JailFine,
			CanPardon:  mine && s.Phase == game.PhaseJail && p.Pardons > 0,
			MustRead:   mine && s.Phase == game.PhaseCard,
			Offer:      -1,
		}
		if s.Phase == game.PhaseBuy && p.ID == s.CurrentID() {
			v.Me.Offer = s.PendingTile()
		}
		// Building is legal before you roll, and while you are being held — the
		// two phases where the table is waiting on you and nothing is
		// half-resolved. Asked of the engine rather than worked out here, so the
		// buttons and the rule cannot disagree.
		if mine && (s.Phase == game.PhaseRoll || s.Phase == game.PhaseJail) {
			v.Me.CanBuild = s.BuildableFor(p.ID)
			v.Me.CanSell = s.SellableFor(p.ID)
		}
	}
	return v
}
