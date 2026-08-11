package game

import "errors"

// ActionKind names the moves a player (or the room's timer) can submit.
type ActionKind string

const (
	ActPlay        ActionKind = "play"         // play one card, or a matching cat pair
	ActDraw        ActionKind = "draw"         // draw to end your turn
	ActNope        ActionKind = "nope"         // stack a Nope onto the open window
	ActPass        ActionKind = "pass"         // decline to Nope
	ActNopeExpired ActionKind = "nope_expired" // internal: the window timer fired
	ActGiveCard    ActionKind = "give"         // Favor target hands a card over
	ActPlaceKitten ActionKind = "place"        // reinsert a defused Kitten
)

// Action is a single submitted move. Only the fields relevant to Kind are read.
type Action struct {
	Kind     ActionKind
	PlayerID string
	CardIDs  []int  // ActPlay: one card, or two/three matching cats. ActNope/ActGiveCard: one.
	TargetID string // ActPlay for Favor and cat sets
	Index    int    // ActPlaceKitten: 0 = top of deck, len(Draw) = bottom
	Named    string // ActPlay for a three-cat set: the slug of the card demanded
}

var (
	ErrNotYourTurn   = errors.New("it is not your turn")
	ErrWrongPhase    = errors.New("you can't do that right now")
	ErrNoSuchCard    = errors.New("you don't have that card")
	ErrNotPlayable   = errors.New("that card can't be played on its own")
	ErrBadTarget     = errors.New("pick a different player")
	ErrBadCatSet     = errors.New("cat cards must be played as a matching pair or trio")
	ErrNoNamedCard   = errors.New("name the card you want")
	ErrDead          = errors.New("you have been eliminated")
	ErrGameOver      = errors.New("the game is over")
	ErrNoNopeCard    = errors.New("you don't have a Nope card")
	ErrAlreadyPassed = errors.New("you already passed")
)

// EventKind labels an entry in the shared play-by-play log.
type EventKind string

const (
	EvPlayed     EventKind = "played"      // someone played card(s)
	EvNoped      EventKind = "noped"       // a Nope landed
	EvResolved   EventKind = "resolved"    // a pending action took effect
	EvCancelled  EventKind = "cancelled"   // a pending action was noped away
	EvDrew       EventKind = "drew"        // someone drew (contents private)
	EvExploded   EventKind = "exploded"    // an Exploding Kitten came up
	EvDefused    EventKind = "defused"     //
	EvEliminated EventKind = "eliminated"  //
	EvStole      EventKind = "stole"       // cat set took a card
	EvDemanded   EventKind = "demanded"    // a Three of a Kind named a card
	EvMissed     EventKind = "missed"      // ...and the target didn't have one
	EvGave       EventKind = "gave"        // Favor handed a card over
	EvFuture     EventKind = "future"      // private: the top three cards
	EvTurn       EventKind = "turn"        // the active player changed
	EvGameOver   EventKind = "game_over"   //
	EvShuffled   EventKind = "shuffled"    //
	EvNopeWindow EventKind = "nope_window" // a window opened; clients start a countdown
)

// Event is one line of the game log. Cards carried by an event are only ever the
// ones every recipient is entitled to see, except when OnlyFor is set — that
// restricts delivery to a single player and is how private information (See the
// Future, the card you drew, the card you were given) reaches exactly one client.
type Event struct {
	Kind     EventKind `json:"kind"`
	ActorID  string    `json:"actorId,omitempty"`
	TargetID string    `json:"targetId,omitempty"`
	Cards    []Card    `json:"cards,omitempty"`
	Text     string    `json:"text,omitempty"`
	OnlyFor  string    `json:"-"`
}
