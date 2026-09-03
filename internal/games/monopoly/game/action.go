package game

import "errors"

// ActionKind is one move a player can make. Small on purpose: this slice has
// three, and each one is only legal in a single phase.
type ActionKind string

const (
	// ActRoll throws the dice and moves. Legal in PhaseRoll, for the current
	// player only.
	ActRoll ActionKind = "roll"
	// ActBuy takes the square you are standing on at its printed price.
	ActBuy ActionKind = "buy"
	// ActPass declines it. In the original the square then goes to auction; here
	// it simply stays with the bank — see the gap list in engine.go.
	ActPass ActionKind = "pass"
)

// Action is one submitted move.
type Action struct {
	Kind     ActionKind
	PlayerID string
}

// EventKind is one thing that happened, for the play-by-play. Named rather than
// free text so the client can animate a beat and translate the line.
type EventKind string

const (
	EvRolled   EventKind = "rolled"
	EvMoved    EventKind = "moved"
	EvPassedGo EventKind = "passedGo"
	EvBought   EventKind = "bought"
	EvDeclined EventKind = "declined"
	EvRent     EventKind = "rent"
	EvTax      EventKind = "tax"
	EvJailed   EventKind = "jailed"
	EvBankrupt EventKind = "bankrupt"
	EvTurn     EventKind = "turn"
	EvWon      EventKind = "won"
)

// Event is one line of what happened. Amounts and squares are carried as
// numbers, never as a formatted string: the client formats kyat, and it has to
// be able to do it in either language.
type Event struct {
	Kind     EventKind
	ActorID  string
	TargetID string
	// Tile is a square index, or -1 when the event is not about one.
	Tile int
	// Amount is money moving, in kyat.
	Amount int
	// Dice is filled on a roll.
	Dice [2]int
}

// Errors a player can cause. Each is something their own client should not have
// let them do, so the room sends it back to them alone.
var (
	ErrNotYourTurn  = errors.New("it isn't your turn")
	ErrWrongPhase   = errors.New("you can't do that right now")
	ErrNotForSale   = errors.New("that square isn't for sale")
	ErrCantAfford   = errors.New("you can't afford that")
	ErrUnknownMove  = errors.New("unrecognised move")
	ErrGameFinished = errors.New("the game is over")
)
