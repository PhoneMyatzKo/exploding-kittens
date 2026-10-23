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
	// ActReadCard acknowledges the Chance or Community Chest card in front of
	// you. There is nothing to decide — the card says what happens — but somebody
	// has to have seen it before the table moves on.
	ActReadCard ActionKind = "read"
	// ActPayFine buys your way out of jail.
	ActPayFine ActionKind = "fine"
	// ActUsePardon spends a get-out-of-jail card you are holding.
	ActUsePardon ActionKind = "pardon"
	// ActBuild puts a house — or, on the fifth, a hotel — on Tile. Legal before
	// you roll, on a colour set you hold complete. Does not end your turn: you may
	// build across a whole set in one go.
	ActBuild ActionKind = "build"
	// ActSell takes one building back down for half its price, which is how
	// somebody who has over-built raises cash instead of going out.
	ActSell ActionKind = "sell"
)

// Action is one submitted move.
type Action struct {
	Kind     ActionKind
	PlayerID string
	// Tile is which square the move is about. Only the building moves name one —
	// everything else acts on wherever the player is standing, which the engine
	// already knows.
	Tile int
}

// EventKind is one thing that happened, for the play-by-play. Named rather than
// free text so the client can animate a beat and translate the line.
type EventKind string

const (
	EvRolled EventKind = "rolled"
	// EvCard is a card being turned face up; Card names which one.
	EvCard EventKind = "card"
	// EvCardPay is money a card moved. Separate from EvRent and EvTax so the log
	// can say "the card cost you" rather than inventing a landlord.
	EvCardPay    EventKind = "cardPay"
	EvMissTurn   EventKind = "missTurn"
	EvFine       EventKind = "fine"
	EvFreed      EventKind = "freed"
	EvPardonUsed EventKind = "pardonUsed"
	EvMoved      EventKind = "moved"
	EvPassedGo   EventKind = "passedGo"
	EvBought     EventKind = "bought"
	EvDeclined   EventKind = "declined"
	// EvBuilt and EvSold are a building going up or coming down. Amount is the
	// money; Count is the square's new level, so the log can say "a hotel" without
	// reading the state back.
	EvBuilt    EventKind = "built"
	EvSold     EventKind = "sold"
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
	// Count is a quantity that is not money: on EvBuilt and EvSold, how many
	// buildings now stand on the square.
	Count int
	// Dice is filled on a roll.
	Dice [2]int
	// Card is an index into the pack, on EvCard. Zero is a real card, so it is
	// only meaningful on that kind.
	Card int
}

// Errors a player can cause. Each is something their own client should not have
// let them do, so the room sends it back to them alone.
var (
	ErrNotYourTurn  = errors.New("it isn't your turn")
	ErrWrongPhase   = errors.New("you can't do that right now")
	ErrNotForSale   = errors.New("that square isn't for sale")
	ErrCantAfford   = errors.New("you can't afford that")
	ErrUnknownMove  = errors.New("unrecognised move")
	ErrNoPardon     = errors.New("you have no get-out-of-jail card")
	ErrGameFinished = errors.New("the game is over")

	// Building. Each one is a distinct refusal because each has a distinct thing
	// the player should do about it — complete the set, build the other square
	// first, wait for somebody to sell.
	ErrNotYours      = errors.New("you don't own that square")
	ErrIncompleteSet = errors.New("you need the whole colour set to build")
	ErrBuildUnevenly = errors.New("build evenly across the set first")
	ErrFullyBuilt    = errors.New("that square already has a hotel")
	ErrNothingBuilt  = errors.New("there is nothing to sell there")
	ErrNoHousesLeft  = errors.New("the bank has no houses left")
	ErrNoHotelsLeft  = errors.New("the bank has no hotels left")
)
