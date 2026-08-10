// Package room owns the lifetime of a single game table.
//
// Concurrency model: every Room has exactly one goroutine (run) that touches its
// members and its *game.State. WebSocket readers never mutate anything; they
// hand commands to that goroutine over a channel. That is what makes the whole
// server race-free without a single mutex around game logic.
package room

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"boardgame/kittens/internal/game"
	"boardgame/kittens/internal/view"
)

const (
	// nopeWindow is how long an action sits on the table before it resolves.
	nopeWindow = 7 * time.Second
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

// ClientMsg is an inbound message from a browser.
type ClientMsg struct {
	Type     string `json:"type"`
	CardIDs  []int  `json:"cardIds"`
	TargetID string `json:"targetId"`
	Index    int    `json:"index"`
}

var (
	ErrRoomFull    = errors.New("this room is full")
	ErrInProgress  = errors.New("that game has already started")
	ErrNotHost     = errors.New("only the host can start the game")
	ErrUnknownRoom = errors.New("no room with that code")
)

type member struct {
	ID        string
	Name      string
	Token     string
	Conn      Sender
	Connected bool
	Host      bool
}

// Room is one table. All fields below cmds are owned exclusively by run().
type Room struct {
	Code string

	cmds      chan command
	done      chan struct{}
	closeOnce sync.Once

	members []*member
	seq     int
	state   *game.State
	logbuf  []view.Entry
	rng     *rand.Rand

	// Nope window bookkeeping.
	nopeTimer    *time.Timer
	nopeDeadline time.Time
	inWindow     bool
	lastNopes    int

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

func (cmdJoin) isCommand()      {}
func (cmdLeave) isCommand()     {}
func (cmdMsg) isCommand()       {}
func (cmdIdleCheck) isCommand() {}

type joinResult struct {
	PlayerID string
	Token    string
	Err      error
}

func newRoom(code string) *Room {
	r := &Room{
		Code:      code,
		cmds:      make(chan command, 32),
		done:      make(chan struct{}),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		lastEmpty: time.Now(),
	}
	// Timers start stopped; Go 1.23+ guarantees no stale value survives Stop.
	r.nopeTimer = time.NewTimer(time.Hour)
	r.nopeTimer.Stop()
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
			}
		case <-r.nopeTimer.C:
			r.inWindow = false
			if r.state != nil && r.state.Phase == game.PhaseNope {
				r.applyAction(game.Action{Kind: game.ActNopeExpired})
			}
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
				if r.state != nil {
					if p := r.state.Find(m.ID); p != nil {
						p.Name = m.Name
					}
				}
				c.reply <- joinResult{PlayerID: m.ID, Token: m.Token}
				r.rearmTimers()
				r.broadcast()
				return
			}
		}
	}

	if r.state != nil && r.state.Phase != game.PhaseGameOver {
		c.reply <- joinResult{Err: ErrInProgress}
		return
	}
	if len(r.members) >= game.MaxPlayers {
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
	r.appendLog(view.Entry{Kind: "joined", ActorID: m.ID})
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
		if r.state == nil {
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
	if r.state == nil {
		r.sendErr(m, "the game hasn't started yet")
		return
	}

	a, ok := toAction(m.ID, c.msg)
	if !ok {
		r.sendErr(m, "unrecognised action")
		return
	}
	if err := r.applyAction(a); err != nil {
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
	if r.state != nil && r.state.Phase != game.PhaseGameOver {
		r.sendErr(m, ErrInProgress.Error())
		return
	}
	// Only players who are actually present get dealt in.
	var seats []game.Seat
	var present []*member
	for _, mm := range r.members {
		if mm.Connected {
			seats = append(seats, game.Seat{ID: mm.ID, Name: mm.Name})
			present = append(present, mm)
		}
	}
	s, err := game.NewGame(seats, r.rng)
	if err != nil {
		r.sendErr(m, err.Error())
		return
	}
	r.members = present
	r.state = s
	r.logbuf = nil
	r.inWindow = false
	r.appendLog(view.Entry{Kind: "started"})
	r.appendLog(view.Entry{Kind: "turn", ActorID: s.CurrentID()})
	r.rearmTimers()
	r.broadcast()
}

// applyAction runs one action through the engine and fans its events out. It is
// the only caller of game.Apply.
func (r *Room) applyAction(a game.Action) error {
	events, err := game.Apply(r.state, a)
	if err != nil {
		return err
	}
	for _, e := range events {
		entry := view.Entry{
			Kind: string(e.Kind), ActorID: e.ActorID, TargetID: e.TargetID,
			Cards: e.Cards, Text: e.Text,
		}
		if e.OnlyFor != "" {
			r.sendPrivate(e.OnlyFor, entry)
			continue
		}
		r.appendLog(entry)
	}
	return nil
}

// toAction maps a wire message onto an engine action. ActNopeExpired is
// deliberately absent: only the room's own timer may produce it.
func toAction(playerID string, m ClientMsg) (game.Action, bool) {
	a := game.Action{PlayerID: playerID, CardIDs: m.CardIDs, TargetID: m.TargetID, Index: m.Index}
	switch m.Type {
	case "play":
		a.Kind = game.ActPlay
	case "draw":
		a.Kind = game.ActDraw
	case "nope":
		a.Kind = game.ActNope
	case "pass":
		a.Kind = game.ActPass
	case "give":
		a.Kind = game.ActGiveCard
	case "place":
		a.Kind = game.ActPlaceKitten
	default:
		return a, false
	}
	return a, true
}

// ------------------------------------------------------------------- timers

// rearmTimers reconciles both timers with the current phase. It runs after every
// state change so the timers are always a pure function of the state.
func (r *Room) rearmTimers() {
	r.rearmNope()
	r.rearmIdle()
}

func (r *Room) rearmNope() {
	if r.state == nil || r.state.Phase != game.PhaseNope || r.state.Pending == nil {
		r.inWindow = false
		r.nopeTimer.Stop()
		return
	}
	// A newly played Nope refreshes the window so everyone can Yup it back.
	if !r.inWindow || r.state.Pending.Nopes != r.lastNopes {
		r.inWindow = true
		r.lastNopes = r.state.Pending.Nopes
		r.nopeDeadline = time.Now().Add(nopeWindow)
		r.nopeTimer.Stop()
		r.nopeTimer.Reset(nopeWindow)
	}
}

// waitingOn returns the member the table is currently blocked on, if any.
func (r *Room) waitingOn() *member {
	if r.state == nil {
		return nil
	}
	switch r.state.Phase {
	case game.PhaseTurn, game.PhaseDefuse:
		return r.find(r.state.CurrentID())
	case game.PhaseFavor:
		return r.find(r.state.AwaitingGiftFrom())
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
	var a game.Action
	switch r.state.Phase {
	case game.PhaseTurn:
		a = game.Action{Kind: game.ActDraw, PlayerID: m.ID}
	case game.PhaseDefuse:
		a = game.Action{Kind: game.ActPlaceKitten, PlayerID: m.ID, Index: r.rng.Intn(r.state.DeckSize() + 1)}
	case game.PhaseFavor:
		p := r.state.Find(m.ID)
		if p == nil || len(p.Hand) == 0 {
			return
		}
		a = game.Action{Kind: game.ActGiveCard, PlayerID: m.ID, CardIDs: []int{p.Hand[0].ID}}
	default:
		return
	}
	if err := r.applyAction(a); err != nil {
		log.Printf("room %s: auto-move for absent %s failed: %v", r.Code, m.ID, err)
		return
	}
	r.appendLog(view.Entry{Kind: "auto", ActorID: m.ID})
}

// ---------------------------------------------------------------- broadcasting

func (r *Room) memberships() []view.Membership {
	out := make([]view.Membership, 0, len(r.members))
	for _, m := range r.members {
		out = append(out, view.Membership{ID: m.ID, Name: m.Name, Connected: m.Connected, Host: m.Host})
	}
	return out
}

func (r *Room) viewFor(id string) *view.View {
	ms := r.memberships()
	if r.state == nil {
		return view.Lobby(r.Code, ms, id)
	}
	var remaining int64
	if r.inWindow {
		remaining = max(0, time.Until(r.nopeDeadline).Milliseconds())
	}
	return view.For(r.Code, ms, r.state, id, remaining, r.logbuf)
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

func (r *Room) sendPrivate(playerID string, e view.Entry) {
	m := r.find(playerID)
	if m == nil || m.Conn == nil {
		return
	}
	b, err := json.Marshal(struct {
		Type  string     `json:"type"`
		Event view.Entry `json:"event"`
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

func (r *Room) appendLog(e view.Entry) {
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
