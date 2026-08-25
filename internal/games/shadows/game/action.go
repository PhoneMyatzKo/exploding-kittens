package game

import "errors"

// ActionKind names every move a player can submit.
//
// They divide into two groups, and the division is the whole shape of a turn:
//
//   - Main actions cost you your turn. Exactly one per round, in seat order.
//   - Free actions can be taken at any moment by anybody, on or off turn, and
//     are paid for in influence instead. Trading, extortion, leaking and lying
//     are all free actions, which is what makes the table talk.
type ActionKind string

const (
	// Main actions.
	ActSkill       ActionKind = "skill"        // use your position's power
	ActPower       ActionKind = "power"        // play a Power card at somebody
	ActCleanRecord ActionKind = "clean_record" // burn influence to replace your secret
	ActEvidence    ActionKind = "evidence"     // a Pawn digging for its owner's name
	ActSeize       ActionKind = "seize"        // take a card off a Pawn you own
	ActAccuse      ActionKind = "accuse"       // the Resistance naming the Mastermind
	ActPass        ActionKind = "pass"         // do nothing and hand the turn on

	// Free actions.
	ActOffer   ActionKind = "offer"   // propose a trade, a payment or an extortion
	ActAccept  ActionKind = "accept"  // take a proposal
	ActDecline ActionKind = "decline" // refuse one, or withdraw your own
	ActNews    ActionKind = "news"    // post an anonymous leak, true or otherwise
	ActLeak    ActionKind = "leak"    // a Pawn whistleblowing to the Resistance
	ActPardon  ActionKind = "pardon"  // the Resistance freeing a Pawn

	// Submitted by the room, never by a client.
	ActAdvance ActionKind = "advance" // force the round on
)

// Main reports whether this kind spends the actor's turn.
func (k ActionKind) Main() bool {
	switch k {
	case ActSkill, ActPower, ActCleanRecord, ActEvidence, ActSeize, ActAccuse, ActPass:
		return true
	}
	return false
}

// Action is a single submitted move. Only the fields relevant to Kind are read.
type Action struct {
	Kind     ActionKind
	PlayerID string
	TargetID string
	// CardID is the Power card being played or seized.
	CardID int
	// CardIDs is what an offer puts on the table; WantIDs is what it asks for.
	CardIDs []int
	WantIDs []int
	// Amount is influence: offered to the other side when positive on the giving
	// half of a trade, and demanded from them via Demand.
	Amount int
	Demand int
	// Slug is a named Weakness, for the Lawyer's inspection.
	Slug string
	// Text is a newsfeed post or a note attached to an offer. Truncated, never
	// interpreted.
	Text string
	// OfferID picks out an open proposal for ActAccept and ActDecline.
	OfferID int
}

// Errors a player can see. Every one of them is phrased as something to do
// differently rather than as a fault, because they are shown verbatim.
var (
	ErrNotYourTurn    = errors.New("wait for your turn")
	ErrAlreadyActed   = errors.New("you have already acted this round")
	ErrWrongPhase     = errors.New("you can't do that in this round")
	ErrGameOver       = errors.New("the game is over")
	ErrNoSuchPlayer   = errors.New("there's nobody by that name at this table")
	ErrSelfTarget     = errors.New("you can't aim that at yourself")
	ErrNoSuchCard     = errors.New("you don't hold that card")
	ErrNotPower       = errors.New("that isn't a Power card")
	ErrSkillSpent     = errors.New("you've used your position this round")
	ErrNoSuchWeakness = errors.New("no such secret exists")
	ErrInfluence      = errors.New("you can't afford that")
	ErrAlreadyOwned   = errors.New("they are already under your control")
	ErrPardoned       = errors.New("they are protected this round")
	ErrNotPawn        = errors.New("they aren't under anybody's control")
	ErrNotYourPawn    = errors.New("they aren't yours")
	ErrPawnEmpty      = errors.New("they have nothing left to take")
	ErrNotCompromised = errors.New("you have nothing to dig for")
	ErrNotResistance  = errors.New("only the Resistance can do that")
	ErrAccusationUsed = errors.New("you have already made your accusation")
	ErrNoOffer        = errors.New("that proposal is gone")
	ErrNotYourOffer   = errors.New("that proposal isn't yours to answer")
	ErrEmptyOffer     = errors.New("a proposal has to put something on the table")
	ErrNoText         = errors.New("write something first")
	ErrLeakSpent      = errors.New("you've already leaked this round")
	ErrCleanClear     = errors.New("only the compromised need a clean record")
	ErrCleanNoOne     = errors.New("you're under control — a clean record won't save you now")
)

// EventKind labels one line of the play-by-play.
type EventKind string

const (
	EvStarted   EventKind = "started"   // the table was dealt
	EvRound     EventKind = "round"     // a new round opened
	EvTurn      EventKind = "turn"      // the active player changed
	EvSkill     EventKind = "skill"     // somebody used their position (public, vague)
	EvIntel     EventKind = "intel"     // private: something you learned
	EvDisclosed EventKind = "disclosed" // public: a decree exposed a category
	EvMove      EventKind = "move"      // public: X moved against Y. Result withheld.
	EvOwned     EventKind = "owned"     // private: the blackmail landed
	EvBurned    EventKind = "burned"    // private: it didn't
	EvOffer     EventKind = "offer"     // private: a proposal arrived
	EvTraded    EventKind = "traded"    // public: two players closed a deal
	EvDeclined  EventKind = "declined"  // private: a proposal was refused
	EvNews      EventKind = "news"      // public: an anonymous leak
	EvClean     EventKind = "clean"     // public: somebody bought a clean record
	EvEvidence  EventKind = "evidence"  // private: a Pawn dug
	EvMutiny    EventKind = "mutiny"    // public: a Pawn broke free
	EvSeized    EventKind = "seized"    // private: a Pawn was robbed
	EvLeak      EventKind = "leak"      // private: a whistleblower reported
	EvPardon    EventKind = "pardon"    // private: a Pawn was freed
	EvCatalyst  EventKind = "catalyst"  // public: the Coup cards are in play
	EvAccused   EventKind = "accused"   // public: the Resistance named somebody
	EvGameOver  EventKind = "game_over" // it's finished
	EvRevealed  EventKind = "revealed"  // the full table, once it no longer matters
	EvPassed    EventKind = "passed"    // public: a turn went by
)

// Event is one line of the log.
//
// OnlyFor restricts an event to a single player and is how every private fact in
// this game reaches exactly one client. Anything that is not marked reaches
// everybody, so the default is public and the exceptions are visible.
type Event struct {
	Kind     EventKind `json:"kind"`
	ActorID  string    `json:"actorId,omitempty"`
	TargetID string    `json:"targetId,omitempty"`
	Cards    []Card    `json:"cards,omitempty"`
	Text     string    `json:"text,omitempty"`
	// Count is a round number on EvRound, a tally on EvIntel and an evidence
	// level on EvEvidence.
	Count int `json:"count,omitempty"`
	// Points is influence spent or gained, where the line is about money.
	Points  int    `json:"points,omitempty"`
	OnlyFor string `json:"-"`
}

// priv is a private event, addressed to one player.
func priv(only string, e Event) Event {
	e.OnlyFor = only
	return e
}
