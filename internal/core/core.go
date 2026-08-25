// Package core is the seam between a room and the rules it happens to be
// hosting.
//
// internal/room owns sockets, seats, reconnects and timers, and after this
// package exists it owns nothing else: which cards are in play, whose turn it is
// and what a legal move looks like all live behind Game. That is what lets one
// room loop deal Exploding Kittens at one table and UNO at the next without
// either game learning what a WebSocket is.
//
// Nothing here imports a game, and no game imports internal/room. The types in
// this file are the whole vocabulary they share.
package core

import (
	"errors"
	"time"
)

// Seat identifies a player being dealt into a game.
type Seat struct {
	ID   string
	Name string
}

// Membership is the connection-level information the room layer owns. The rules
// know nothing about sockets, hosting or portraits.
type Membership struct {
	ID        string
	Name      string
	Avatar    string
	Connected bool
	Host      bool
}

// ClientMsg is an inbound message from a browser.
//
// One struct for every game, rather than opaque bytes decoded per game: the
// fields are few, they are all scalars, and a single shape means the room can
// still log and rate-limit a message it does not understand. A game reads the
// fields its own moves use and ignores the rest.
type ClientMsg struct {
	Type string `json:"type"`

	// Shell-level, handled by the room itself.
	Avatar string `json:"avatar"`

	// Shared by every game with a target and a card.
	TargetID string `json:"targetId"`
	CardID   int    `json:"cardId"`

	// Exploding Kittens: a play can be several cards, a Kitten goes back at an
	// index, a Three of a Kind names what it wants, and Alter the Future sends
	// back the order the top of the deck should end up in.
	CardIDs []int  `json:"cardIds"`
	Index   int    `json:"index"`
	Named   string `json:"named"`
	// Order is positions, not card IDs: the client is shown those cards' faces
	// without ever being told which cards they are, so it has nothing else to
	// name them by. See view.FaceCard.
	Order []int `json:"order"`

	// UNO: a wild names a colour, and going down to one card is announced.
	Colour string `json:"colour"`
	SayUno bool   `json:"sayUno"`

	// State of Shadows: a proposal puts cards and money on both sides of the
	// table at once, so CardIDs above is what is offered and WantIDs is what is
	// asked for; Amount is influence promised and Demand is influence extorted.
	// Slug names a secret (the Lawyer's inspection), Text is a leak or a note
	// attached to a proposal, and OfferID answers one that is already open.
	WantIDs []int  `json:"wantIds"`
	Amount  int    `json:"amount"`
	Demand  int    `json:"demand"`
	Slug    string `json:"slug"`
	Text    string `json:"text"`
	OfferID int    `json:"offerId"`
}

// Entry is one line of the shared play-by-play, and the only shape in which a
// game reports what happened.
//
// Cards is any so that each game can put its own card type on the wire; the room
// only ever forwards it. OnlyFor restricts an entry to a single player, which is
// how private information (a drawn card, a revealed hand) reaches exactly one
// client — the room never adds it to the log every other client replays.
type Entry struct {
	// Seq counts up for the life of the room, so a client can tell a new event
	// from one it has already animated. Zero on private entries.
	Seq      int    `json:"seq,omitempty"`
	Kind     string `json:"kind"`
	ActorID  string `json:"actorId,omitempty"`
	TargetID string `json:"targetId,omitempty"`
	Cards    any    `json:"cards,omitempty"`
	Text     string `json:"text,omitempty"`
	// Count, Points and Colour are UNO's: how many cards a draw moved, what a
	// round scored, and which colour is in force. Omitted everywhere else, so a
	// game that has no use for them costs nothing.
	Count   int    `json:"count,omitempty"`
	Points  int    `json:"points,omitempty"`
	Colour  string `json:"colour,omitempty"`
	OnlyFor string `json:"-"`
}

// Shell is everything a view needs that the rules do not own: which room this is,
// who is connected, who is looking, and the log so far.
type Shell struct {
	Code     string
	Public   bool
	Game     string
	Members  []Membership
	ViewerID string
	Log      []Entry
	// RemainingMs and TotalMs describe an open action window. Sent as a remainder
	// rather than a deadline so a countdown does not depend on the player's clock
	// agreeing with the server's. Both zero when no window is open.
	RemainingMs int64
	TotalMs     int64
}

// Game is one table's rules, as the room sees them. One instance per room,
// created when the room is, and reused across rounds.
//
// Every method is called from the room's single goroutine, so implementations
// need no locking and may hand out pointers into their own state.
type Game interface {
	// MinPlayers and MaxPlayers bound the seats. The room enforces them; the
	// numbers belong to the rules.
	MinPlayers() int
	MaxPlayers() int

	// Started reports whether a game is under way — the room shows its lobby
	// until it is. Over reports that the current game has finished but is still
	// on screen, which is when the host may deal again or go back to the lobby.
	Started() bool
	Over() bool

	// Deal starts a game for exactly these seats. Called with everybody who is
	// present, and only when the room is in its lobby or showing a finished game.
	Deal(seats []Seat) ([]Entry, error)
	// Reset throws the finished game away so the room lands back in its lobby.
	Reset()

	// Rename keeps a reconnecting player's display name current. A no-op before
	// the deal, when the room owns the only copy of the name.
	Rename(playerID, name string)

	// Submit runs one player's message. An error is the player's to see; the room
	// sends it back to them alone and changes nothing.
	Submit(playerID string, msg ClientMsg) ([]Entry, error)

	// BlockedOn is the player everyone else is waiting for, or "" if the table is
	// not blocked on anybody in particular. The room uses it to notice that the
	// game is being held up by somebody who has closed their laptop.
	BlockedOn() string
	// AutoMove plays the least destructive legal move on that player's behalf.
	AutoMove(playerID string) ([]Entry, error)

	// Window describes a timed window that other players may still act into —
	// Exploding Kittens' Nope window, and nothing in UNO. total is how long the
	// window lasts; token changes whenever the window must be restarted, which is
	// how a fresh Nope gives everybody their time back. open is false when the
	// rules have no window open, and the room stops its timer.
	Window() (total time.Duration, token int, open bool)
	// WindowExpired is submitted by the room's timer, never by a client.
	WindowExpired() []Entry

	// View builds the payload one client receives, lobby included: only the game
	// knows which of its fields its own renderer needs.
	View(sh Shell) any
}

// Errors a Game may return that the room recognises rather than merely forwards.
var (
	ErrNotStarted    = errors.New("the game hasn't started yet")
	ErrUnknownAction = errors.New("unrecognised action")
	// ErrNoMove is AutoMove saying there is nothing safe to play. The room logs it
	// and waits rather than treating it as a failure.
	ErrNoMove = errors.New("no automatic move available")
)
