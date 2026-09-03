// Package room owns the lifetime of a single game table.
//
// Concurrency model: every Room has exactly one goroutine (run) that touches its
// members and its core.Game. WebSocket readers never mutate anything; they hand
// commands to that goroutine over a channel. That is what makes the whole server
// race-free without a single mutex around game logic — and it is why a Game's
// methods may hand out pointers into their own state without locking.
//
// The room knows which game it is hosting only as a catalogue slug. Everything
// that slug means — which cards, how many seats, what a legal move is — lives
// behind core.Game, so nothing here imports a game's rules.
package room

import (
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games"
	"boardgame/kittens/internal/prng"
	"boardgame/kittens/static"
)

const (
	// idleGrace is how long the table waits for a disconnected player who is
	// holding everyone up before acting on their behalf.
	idleGrace = 25 * time.Second
	// logLimit caps the replayed play-by-play.
	logLimit = 80
)

// Sender is the room's view of a client connection. internal/ws implements it.
type Sender interface {
	Send([]byte)
	Close()
}

// ClientMsg is an inbound message from a browser. Defined in internal/core
// because both the transport and every game's rules have to agree on it, and
// neither of those may import the other.
type ClientMsg = core.ClientMsg

var (
	ErrRoomFull    = errors.New("this room is full")
	ErrInProgress  = errors.New("that game has already started")
	ErrNotHost     = errors.New("only the host can start the game")
	ErrUnknownRoom = errors.New("no room with that code")
	ErrAvatarTaken = errors.New("somebody already picked that cat")
	ErrNoAvatar    = errors.New("no such cat")
)

type member struct {
	ID   string
	Name string
	// Avatar belongs to the seat, so it survives a reconnect.
	Avatar    string
	Token     string
	Conn      Sender
	Connected bool
	Host      bool
}

// Options are the choices made when a room is created, before anybody has
// joined. Grouped rather than passed as loose arguments because they are all
// booleans and slugs that would be easy to swap by accident.
type Options struct {
	// Public lists the room in the lobby browser.
	Public bool
	// Game is the catalogue slug this table is playing. The room carries it
	// without interpreting it: which games exist, which are playable and which
	// deck each one deals are the catalogue's business, not the table's — the
	// slug goes to games.New and the rules that come back are all the room sees.
	Game string
}

// Room is one table. All fields below cmds are owned exclusively by run().
type Room struct {
	Code string
	// Public and Game are set once before run() starts and never written again,
	// so reading them needs no synchronisation.
	Public bool
	Game   string

	cmds      chan command
	done      chan struct{}
	closeOnce sync.Once

	members []*member
	seq     int
	// game is the rules this table is playing, created with the room and reused
	// across rounds. The room never inspects it: what a card is, whose turn it is
	// and what a legal move looks like are all behind the interface.
	game   core.Game
	logbuf []core.Entry
	logSeq int
	rng    *prng.Source

	// Action-window bookkeeping. Some games let the rest of the table interrupt a
	// play for a few seconds (Exploding Kittens' Nope); the room runs the clock
	// for them and knows nothing else about it.
	windowTimer    *time.Timer
	windowDeadline time.Time
	windowLength   time.Duration
	inWindow       bool
	lastToken      int

	// Disconnected-player watchdog.
	idleTimer *time.Timer
	idleArmed bool

	lastEmpty time.Time
}

type command interface{ isCommand() }

type cmdJoin struct {
	token, name string
	conn        Sender
	reply       chan joinResult
}
type cmdLeave struct {
	memberID string
	conn     Sender
}
type cmdMsg struct {
	memberID string
	msg      ClientMsg
}
type cmdIdleCheck struct{ reply chan bool }
type cmdSummary struct{ reply chan Summary }

func (cmdJoin) isCommand()      {}
func (cmdLeave) isCommand()     {}
func (cmdMsg) isCommand()       {}
func (cmdIdleCheck) isCommand() {}
func (cmdSummary) isCommand()   {}

// Summary is one row of the public lobby browser. Deliberately thin: a code, who
// is waiting and how many — nothing about anyone's cards, because this is served
// to people who are not in the room.
type Summary struct {
	Code string `json:"code"`
	// Game is the catalogue slug, so a listing that spans several games can say
	// what each row is before you commit to tapping it.
	Game     string   `json:"game"`
	Host     string   `json:"host"`
	Players  int      `json:"players"`
	Capacity int      `json:"capacity"`
	Names    []string `json:"names"`
	// Joinable is false once a game is under way or the seats are full. The
	// browser only ever shows joinable rooms, but the flag is what decides that.
	Joinable bool `json:"joinable"`
	Public   bool `json:"-"`
}

type joinResult struct {
	PlayerID string
	Token    string
	Err      error
}

func newRoom(code string, opts Options) *Room {
	rng := prng.NewSeeded()
	slug := opts.Game
	if slug == "" {
		slug = games.Kittens
	}
	g, err := games.New(slug, rng)
	if err != nil {
		// Unbuilt games are refused at POST /api/rooms, so this is a slug that got
		// past that — a stale client or a hand-written request. Dealing the default
		// beats opening a table nothing can play.
		log.Printf("room %s: %v, dealing %s instead", code, err, games.Kittens)
		slug = games.Kittens
		g, _ = games.New(slug, rng)
	}
	r := &Room{
		Code:   code,
		Public: opts.Public,
		// slug, not opts.Game: an unbuilt one has just been swapped for the
		// default, and the room must report what it is actually dealing.
		Game:      slug,
		game:      g,
		cmds:      make(chan command, 32),
		done:      make(chan struct{}),
		rng:       rng,
		lastEmpty: time.Now(),
	}
	// Timers start stopped; Go 1.23+ guarantees no stale value survives Stop.
	r.windowTimer = time.NewTimer(time.Hour)
	r.windowTimer.Stop()
	r.idleTimer = time.NewTimer(time.Hour)
	r.idleTimer.Stop()
	go r.run()
	return r
}

// Join attaches a connection to the room, reclaiming an existing seat when the
// browser presents a token it was given before.
func (r *Room) Join(token, name string, conn Sender) (playerID, newTok string, err error) {
	reply := make(chan joinResult, 1)
	select {
	case r.cmds <- cmdJoin{token: token, name: name, conn: conn, reply: reply}:
	case <-r.done:
		return "", "", ErrUnknownRoom
	}
	res := <-reply
	return res.PlayerID, res.Token, res.Err
}

// Leave detaches a connection. conn is checked so that a stale socket closing
// after its owner has already reconnected doesn't mark the seat offline.
func (r *Room) Leave(memberID string, conn Sender) {
	select {
	case r.cmds <- cmdLeave{memberID: memberID, conn: conn}:
	case <-r.done:
	}
}

// Submit queues a player's message.
func (r *Room) Submit(memberID string, msg ClientMsg) {
	select {
	case r.cmds <- cmdMsg{memberID: memberID, msg: msg}:
	case <-r.done:
	}
}

// ---------------------------------------------------------------- the run loop

func (r *Room) run() {
	for {
		select {
		case c := <-r.cmds:
			switch c := c.(type) {
			case cmdJoin:
				r.handleJoin(c)
			case cmdLeave:
				r.handleLeave(c)
			case cmdMsg:
				r.handleMsg(c)
			case cmdIdleCheck:
				c.reply <- r.isReapable()
			case cmdSummary:
				c.reply <- r.summary()
			case cmdSave:
				r.handleSave(c)
			}
		case <-r.windowTimer.C:
			r.inWindow = false
			r.fan(r.game.WindowExpired())
			r.rearmTimers()
			r.broadcast()
		case <-r.idleTimer.C:
			r.idleArmed = false
			r.actForAbsentPlayer()
			r.rearmTimers()
			r.broadcast()
		case <-r.done:
			// Hanging up is done here rather than in close() because r.members
			// belongs to this goroutine and nobody else may read it.
			for _, m := range r.members {
				if m.Conn != nil {
					m.Conn.Close()
				}
			}
			return
		}
	}
}

func (r *Room) handleJoin(c cmdJoin) {
	name := sanitizeName(c.name)

	// Reconnect path: the token identifies a seat that already exists.
	if c.token != "" {
		for _, m := range r.members {
			if m.Token == c.token {
				if m.Conn != nil && m.Conn != c.conn {
					m.Conn.Close()
				}
				m.Conn = c.conn
				m.Connected = true
				if name != "" {
					m.Name = name
				}
				r.game.Rename(m.ID, m.Name)
				c.reply <- joinResult{PlayerID: m.ID, Token: m.Token}
				r.rearmTimers()
				r.broadcast()
				return
			}
		}
	}

	if r.game.Started() && !r.game.Over() {
		c.reply <- joinResult{Err: ErrInProgress}
		return
	}
	if len(r.members) >= r.game.MaxPlayers() {
		c.reply <- joinResult{Err: ErrRoomFull}
		return
	}

	r.seq++
	m := &member{
		ID:        "p" + strconv.Itoa(r.seq),
		Name:      name,
		Token:     newToken(),
		Conn:      c.conn,
		Connected: true,
		Host:      len(r.members) == 0,
	}
	r.members = append(r.members, m)
	c.reply <- joinResult{PlayerID: m.ID, Token: m.Token}
	r.appendLog(core.Entry{Kind: "joined", ActorID: m.ID})
	r.broadcast()
}

func (r *Room) handleLeave(c cmdLeave) {
	for i, m := range r.members {
		if m.ID != c.memberID {
			continue
		}
		// A superseded socket closing late must not knock the live one offline.
		if m.Conn != c.conn {
			return
		}
		m.Conn = nil
		m.Connected = false

		// Before the game starts there is no seat worth preserving, so drop them
		// entirely and let someone else take the slot.
		if !r.game.Started() {
			r.members = append(r.members[:i], r.members[i+1:]...)
			if m.Host && len(r.members) > 0 {
				r.members[0].Host = true
			}
		}
		break
	}
	if r.connectedCount() == 0 {
		r.lastEmpty = time.Now()
	}
	r.rearmTimers()
	r.broadcast()
}

func (r *Room) handleMsg(c cmdMsg) {
	m := r.find(c.memberID)
	if m == nil {
		return
	}

	if c.msg.Type == "start" {
		r.handleStart(m)
		return
	}
	if c.msg.Type == "lobby" {
		r.handleReturnToLobby(m)
		return
	}
	if c.msg.Type == "avatar" {
		r.handleAvatar(m, c.msg.Avatar)
		return
	}
	if !r.game.Started() {
		r.sendErr(m, core.ErrNotStarted.Error())
		return
	}

	if err := r.submit(m.ID, c.msg); err != nil {
		r.sendErr(m, err.Error())
		// Still resend state so a client that got out of sync snaps back.
		r.sendTo(m)
		return
	}
	r.rearmTimers()
	r.broadcast()
}

func (r *Room) handleStart(m *member) {
	if !m.Host {
		r.sendErr(m, ErrNotHost.Error())
		return
	}
	if r.game.Started() && !r.game.Over() {
		r.sendErr(m, ErrInProgress.Error())
		return
	}
	// Only players who are actually present get dealt in.
	var seats []core.Seat
	var present []*member
	for _, mm := range r.members {
		if mm.Connected {
			seats = append(seats, core.Seat{ID: mm.ID, Name: mm.Name})
			present = append(present, mm)
		}
	}
	entries, err := r.game.Deal(seats)
	if err != nil {
		r.sendErr(m, err.Error())
		return
	}
	// Cleared after the deal, not before: a refused deal must leave the finished
	// game's play-by-play on screen, and the entries the deal itself produced —
	// who starts, what was turned over — have to survive the clearing.
	r.logbuf = nil
	r.inWindow = false
	r.members = present
	r.fan(entries)
	r.rearmTimers()
	r.broadcast()
}

// handleAvatar records a player's portrait: one per table, lobby only.
func (r *Room) handleAvatar(m *member, id string) {
	if r.game.Started() {
		r.sendErr(m, "you can only pick a cat in the lobby")
		return
	}
	if !static.HasAvatar(id) {
		r.sendErr(m, ErrNoAvatar.Error())
		return
	}
	if m.Avatar == id {
		return
	}
	for _, mm := range r.members {
		if mm != m && mm.Avatar == id {
			r.sendErr(m, ErrAvatarTaken.Error())
			r.sendTo(m) // their picker is out of date
			return
		}
	}
	m.Avatar = id
	r.broadcast()
}

// handleReturnToLobby drops the finished game so the whole table lands back in
// the lobby together, which is the only way to pick up players who joined or
// reconnected during the last round — handleStart deals in whoever is present
// at that moment and forgets the rest.
//
// Restricted to a finished game on purpose: mid-round this would be a way for
// the host to wipe a losing position.
func (r *Room) handleReturnToLobby(m *member) {
	if !m.Host {
		r.sendErr(m, ErrNotHost.Error())
		return
	}
	if !r.game.Started() {
		return // already in the lobby; nothing to undo
	}
	if !r.game.Over() {
		r.sendErr(m, ErrInProgress.Error())
		return
	}
	r.game.Reset()
	r.logbuf = nil
	r.inWindow = false
	r.rearmTimers() // both timers stop themselves once nothing is dealt
	r.broadcast()
}

// submit hands one player's message to the rules and fans out whatever came of
// it. It is the only path from a socket into a game.
func (r *Room) submit(playerID string, msg ClientMsg) error {
	entries, err := r.game.Submit(playerID, msg)
	if err != nil {
		return err
	}
	r.fan(entries)
	return nil
}

// fan splits a game's entries into the shared log and the private deliveries.
// An entry marked for one player never reaches the log every other client
// replays, which is what keeps a drawn card or a revealed hand from leaking.
func (r *Room) fan(entries []core.Entry) {
	for _, e := range entries {
		if e.OnlyFor != "" {
			r.sendPrivate(e.OnlyFor, e)
			continue
		}
		r.appendLog(e)
	}
}

// ------------------------------------------------------------------- timers

// rearmTimers reconciles both timers with the current phase. It runs after every
// state change so the timers are always a pure function of the state.
func (r *Room) rearmTimers() {
	r.rearmWindow()
	r.rearmIdle()
}

// rearmWindow reconciles the action-window timer with whatever the rules say is
// open. The token is the game's way of saying "restart it": Exploding Kittens
// bumps it on every Nope, so stacking one gives the table its time back.
func (r *Room) rearmWindow() {
	length, token, open := r.game.Window()
	if !open {
		r.inWindow = false
		r.windowTimer.Stop()
		return
	}
	if !r.inWindow || token != r.lastToken {
		r.inWindow = true
		r.lastToken = token
		r.windowLength = length
		r.windowDeadline = time.Now().Add(length)
		r.windowTimer.Stop()
		r.windowTimer.Reset(length)
	}
}

// waitingOn returns the member the table is currently blocked on, if any.
func (r *Room) waitingOn() *member {
	if id := r.game.BlockedOn(); id != "" {
		return r.find(id)
	}
	return nil
}

func (r *Room) rearmIdle() {
	m := r.waitingOn()
	if m == nil || m.Connected {
		r.idleArmed = false
		r.idleTimer.Stop()
		return
	}
	if !r.idleArmed {
		r.idleArmed = true
		r.idleTimer.Stop()
		r.idleTimer.Reset(idleGrace)
	}
}

// actForAbsentPlayer keeps the table moving when whoever must act has dropped
// off. It always picks the least destructive legal move.
func (r *Room) actForAbsentPlayer() {
	m := r.waitingOn()
	if m == nil || m.Connected {
		return
	}
	entries, err := r.game.AutoMove(m.ID)
	if err != nil {
		if err != core.ErrNoMove {
			log.Printf("room %s: auto-move for absent %s failed: %v", r.Code, m.ID, err)
		}
		return
	}
	r.fan(entries)
	r.appendLog(core.Entry{Kind: "auto", ActorID: m.ID})
}

// ---------------------------------------------------------------- broadcasting

func (r *Room) memberships() []core.Membership {
	out := make([]core.Membership, 0, len(r.members))
	for _, m := range r.members {
		out = append(out, core.Membership{ID: m.ID, Name: m.Name, Avatar: m.Avatar, Connected: m.Connected, Host: m.Host})
	}
	return out
}

// viewFor builds one client's payload. Its shape is the game's business — the
// room only supplies the things the rules do not own: the room code, who is
// connected, and how much of an open window is left.
func (r *Room) viewFor(id string) any {
	sh := core.Shell{
		Code: r.Code, Public: r.Public, Game: r.Game,
		Members: r.memberships(), ViewerID: id, Log: r.logbuf,
	}
	if r.inWindow {
		sh.RemainingMs = max(0, time.Until(r.windowDeadline).Milliseconds())
		sh.TotalMs = r.windowLength.Milliseconds()
	}
	return r.game.View(sh)
}

func (r *Room) broadcast() {
	for _, m := range r.members {
		r.sendTo(m)
	}
}

func (r *Room) sendTo(m *member) {
	if m.Conn == nil {
		return
	}
	b, err := json.Marshal(r.viewFor(m.ID))
	if err != nil {
		log.Printf("room %s: marshal view: %v", r.Code, err)
		return
	}
	m.Conn.Send(b)
}

func (r *Room) sendPrivate(playerID string, e core.Entry) {
	m := r.find(playerID)
	if m == nil || m.Conn == nil {
		return
	}
	b, err := json.Marshal(struct {
		Type  string     `json:"type"`
		Event core.Entry `json:"event"`
	}{"private", e})
	if err != nil {
		return
	}
	m.Conn.Send(b)
}

func (r *Room) sendErr(m *member, msg string) {
	if m.Conn == nil {
		return
	}
	b, _ := json.Marshal(struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{"error", msg})
	m.Conn.Send(b)
}

// appendLog stamps the entry with the next sequence number. It keeps counting
// across rounds, so a client's high-water mark never points at a replayed event.
func (r *Room) appendLog(e core.Entry) {
	r.logSeq++
	e.Seq = r.logSeq
	r.logbuf = append(r.logbuf, e)
	if len(r.logbuf) > logLimit {
		r.logbuf = r.logbuf[len(r.logbuf)-logLimit:]
	}
}

// -------------------------------------------------------------------- helpers

func (r *Room) find(id string) *member {
	for _, m := range r.members {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (r *Room) connectedCount() int {
	n := 0
	for _, m := range r.members {
		if m.Connected {
			n++
		}
	}
	return n
}

// isReapable reports whether the room has sat empty long enough to discard.
func (r *Room) isReapable() bool {
	return r.connectedCount() == 0 && time.Since(r.lastEmpty) > 15*time.Minute
}

// summary describes the room for the lobby browser. Computed here, in the
// goroutine that owns members and state, so the manager never reads either.
func (r *Room) summary() Summary {
	s := Summary{
		Code:     r.Code,
		Game:     r.Game,
		Capacity: r.game.MaxPlayers(),
		Public:   r.Public,
		Players:  r.connectedCount(),
	}
	for _, m := range r.members {
		if m.Connected {
			s.Names = append(s.Names, m.Name)
			if m.Host {
				s.Host = m.Name
			}
		}
	}
	// A room with nobody in it is a room the reaper has not got to yet; offering
	// it would hand somebody an empty table with no host.
	s.Joinable = !r.game.Started() && s.Players > 0 && s.Players < r.game.MaxPlayers()
	return s
}

// Summarize asks the room to describe itself. Returns false if the room is
// shutting down or wedged, so a slow room cannot stall the whole listing.
func (r *Room) Summarize(timeout time.Duration) (Summary, bool) {
	reply := make(chan Summary, 1)
	select {
	case r.cmds <- cmdSummary{reply: reply}:
	case <-r.done:
		return Summary{}, false
	case <-time.After(timeout):
		return Summary{}, false
	}
	select {
	case s := <-reply:
		return s, true
	case <-r.done:
		return Summary{}, false
	case <-time.After(timeout):
		return Summary{}, false
	}
}

// close asks the room to shut down. It is safe to call from any goroutine and
// more than once; run() performs the actual teardown.
func (r *Room) close() {
	r.closeOnce.Do(func() { close(r.done) })
}

func sanitizeName(s string) string {
	out := []rune{}
	for _, c := range s {
		if c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		out = append(out, c)
		if len(out) >= 16 {
			break
		}
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "Player"
	}
	return name
}
