package game

import "errors"

// ActionKind names the moves a player (or the room, on behalf of one who has
// dropped off) can submit.
type ActionKind string

const (
	ActPlay   ActionKind = "play"   // play one card from your hand
	ActDraw   ActionKind = "draw"   // take the one card the rules allow you
	ActPass   ActionKind = "pass"   // decline to play the card you just drew
	ActColour ActionKind = "colour" // name a colour for a Wild that needs one

	// ActCallUno is shouting "UNO!". It may also ride along on the play that
	// takes you down to one card, via Action.SayUno.
	ActCallUno ActionKind = "uno"
	// ActCatchUno is another player pointing out that you didn't.
	ActCatchUno ActionKind = "catch"

	// ActChallenge and ActAcceptDraw are the two answers to a Wild Draw Four
	// played against you: call the bluff, or take the four cards.
	ActChallenge  ActionKind = "challenge"
	ActAcceptDraw ActionKind = "accept"

	// ActNextRound deals the next hand after a round has been scored.
	ActNextRound ActionKind = "next_round"
)

// Action is a single submitted move. Only the fields relevant to Kind are read.
type Action struct {
	Kind     ActionKind
	PlayerID string
	// CardID is the card being played. Unused by every other kind: UNO never
	// plays more than one card at a time.
	CardID int
	// Colour is the slug of the colour named for a Wild, on ActPlay or ActColour.
	Colour string
	// TargetID is who is being caught out on ActCatchUno.
	TargetID string
	// SayUno declares UNO as part of a play, which is what the physical game
	// does — you slap the card down and shout at the same time.
	SayUno bool
}

var (
	ErrNotYourTurn   = errors.New("it is not your turn")
	ErrWrongPhase    = errors.New("you can't do that right now")
	ErrNoSuchCard    = errors.New("you don't have that card")
	ErrNotPlayable   = errors.New("that card doesn't match the colour or the symbol")
	ErrNeedColour    = errors.New("name a colour for that wild")
	ErrNoColourYet   = errors.New("only a wild needs a colour")
	ErrGameOver      = errors.New("the game is over")
	ErrRoundLive     = errors.New("this round isn't finished")
	ErrNothingToCall = errors.New("you don't have exactly one card")
	ErrAlreadyCalled = errors.New("uno has already been called")
	ErrNoCatch       = errors.New("there's nobody to catch")
	ErrCantPlayDrawn = errors.New("you can only play the card you just drew")
)

// EventKind labels an entry in the shared play-by-play log.
type EventKind string

const (
	EvStarted     EventKind = "started"      // a round was dealt
	EvPlayed      EventKind = "played"       // someone put a card down
	EvColour      EventKind = "colour"       // a wild named a colour
	EvDrew        EventKind = "drew"         // someone took cards (contents private)
	EvPassed      EventKind = "passed"       // ...and couldn't use the one they drew
	EvSkipped     EventKind = "skipped"      // a player lost their turn
	EvReversed    EventKind = "reversed"     // play changed direction
	EvReshuffled  EventKind = "reshuffled"   // the discard became the new deck
	EvUnoCalled   EventKind = "uno_called"   //
	EvUnoCaught   EventKind = "uno_caught"   // caught silent on one card: draw two
	EvChallenge   EventKind = "challenge"    // a Wild Draw Four was challenged
	EvBluffed     EventKind = "bluffed"      // ...and the challenge was good
	EvBluffFailed EventKind = "bluff_failed" // ...and it wasn't
	EvRevealed    EventKind = "revealed"     // private: a challenged hand shown
	EvTurn        EventKind = "turn"         // the active player changed
	EvRoundOver   EventKind = "round_over"   // somebody went out; points scored
	EvGameOver    EventKind = "game_over"    // somebody reached the target
)

// Event is one line of the game log.
//
// Cards carried by an event are only ever the ones every recipient is entitled
// to see, except when OnlyFor is set — that restricts delivery to a single
// player and is how private information (the cards you drew, the hand a
// challenge made you show) reaches exactly one client.
type Event struct {
	Kind     EventKind `json:"kind"`
	ActorID  string    `json:"actorId,omitempty"`
	TargetID string    `json:"targetId,omitempty"`
	Cards    []Card    `json:"cards,omitempty"`
	// Count is how many cards a draw moved. Public where Cards is not, which is
	// the whole point: everyone may count your hand, nobody may read it.
	Count int `json:"count,omitempty"`
	// Points is the round score on EvRoundOver, and the running total on
	// EvGameOver.
	Points int    `json:"points,omitempty"`
	Text   string `json:"text,omitempty"`
	// Colour is the colour in force after this event, where the event changed it.
	Colour  string `json:"colour,omitempty"`
	OnlyFor string `json:"-"`
}
