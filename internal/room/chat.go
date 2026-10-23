package room

// Chat: people talking to each other, as opposed to the table reporting itself.
//
// This lives in internal/room and not behind core.Game, and that one decision
// shapes everything else here. handleMsg already answers three of its own
// message types — "start", "lobby" and "avatar" — before the game sees anything,
// and before the "has the game started" guard. Chat is the fourth. Which means
// it works in the lobby, mid-game, after a game is over, and in a game nobody
// has written yet, at no cost to any of them. Routed through Game.Submit instead
// it would have to be implemented identically in every game and would still
// leave the lobby silent.
//
// It is also not part of the state payload. Each game builds its own view struct,
// so a Chat field on core.Shell would mean editing every game's view to copy a
// value none of them care about. A chat line is its own socket message instead,
// handled by the shell in web/app.js — the same shape sendPrivate already uses.
// No game's Go, template or client module is touched by this feature at all.

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

const (
	// chatLimit caps the replayed conversation. Its own number and its own buffer
	// rather than sharing logbuf, because logbuf is emptied on dealing and on
	// returning to the lobby — a conversation kept in there would be wiped the
	// moment somebody started the next round.
	chatLimit = 100
	// chatMaxLen is counted in runes, not bytes, and that is not a detail here:
	// Burmese is three bytes a character, so a byte cap would cut one in half and
	// put invalid UTF-8 on the wire. Anything longer is truncated rather than
	// refused — the client caps the input at the same number, so reaching this
	// means a hand-written client rather than a person with a lot to say.
	chatMaxLen = 300
	// A token bucket per member: chatBurst messages at once, then one more every
	// chatRefill. Without it a single client can fill the room's goroutine and the
	// ring buffer faster than anyone can read either.
	chatBurst  = 5.0
	chatRefill = 2 * time.Second
)

// ChatLine is one thing somebody said.
//
// Name is denormalised onto the line on purpose. The seat list is the usual way
// to turn an id into a name, but somebody who has since left the room is not in
// it any more, and their messages must not turn into "somebody said" when they
// close the tab.
type ChatLine struct {
	Seq     int    `json:"seq"`
	ActorID string `json:"actorId"`
	Name    string `json:"name"`
	Text    string `json:"text"`
}

// handleChat takes one message from a player. Called from handleMsg before the
// game is consulted, so it works whatever the table is or is not doing.
func (r *Room) handleChat(m *member, raw string) {
	text, ok := cleanChat(raw)
	if !ok {
		return // empty, or nothing but spaces and control characters
	}
	if !m.allowChat(time.Now()) {
		r.sendErr(m, ErrChatTooFast.Error())
		return
	}

	r.chatSeq++
	line := ChatLine{Seq: r.chatSeq, ActorID: m.ID, Name: m.Name, Text: text}
	r.chatbuf = append(r.chatbuf, line)
	if len(r.chatbuf) > chatLimit {
		r.chatbuf = r.chatbuf[len(r.chatbuf)-chatLimit:]
	}

	// Broadcast the one line rather than the buffer: everybody already holds the
	// history, and a table talking quickly should not re-send a hundred lines per
	// message.
	b, err := json.Marshal(chatPayload{Type: "chat", Lines: []ChatLine{line}})
	if err != nil {
		return
	}
	for _, mm := range r.members {
		if mm.Conn != nil {
			mm.Conn.Send(b)
		}
	}
}

// sendChatHistory hands one client the whole conversation, replacing whatever it
// had. Sent when somebody joins and again whenever they reconnect: unlike the
// play-by-play, which the room rebuilds into every state payload, chat arrives
// only as it happens — so without this a phone that went to sleep comes back to
// an empty panel and the conversation looks lost.
func (r *Room) sendChatHistory(m *member) {
	if m.Conn == nil {
		return
	}
	b, err := json.Marshal(chatPayload{Type: "chat", Reset: true, Lines: r.chatbuf})
	if err != nil {
		return
	}
	m.Conn.Send(b)
}

// chatPayload is the wire shape. Reset means "this is the whole conversation,
// replace what you have"; without it the lines are new and get appended.
type chatPayload struct {
	Type  string     `json:"type"`
	Reset bool       `json:"reset,omitempty"`
	Lines []ChatLine `json:"lines"`
}

// cleanChat trims a message and flattens it to one line, reporting false for
// anything with nothing left in it.
//
// No escaping happens here and none is needed: every client writes chat with
// textContent, never innerHTML, so markup arrives as the characters somebody
// typed. That is a property worth keeping rather than re-deriving — it is what
// makes free text from strangers safe by construction.
func cleanChat(raw string) (string, bool) {
	// Newlines and tabs become spaces rather than being stripped: the panel shows
	// one line per message, and a pasted paragraph should read as a sentence
	// rather than have its words run together.
	flat := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)

	text := strings.TrimSpace(flat)
	if text == "" {
		return "", false
	}
	// Truncated by runes. See chatMaxLen: cutting Burmese by byte count produces
	// invalid UTF-8.
	if runes := []rune(text); len(runes) > chatMaxLen {
		text = strings.TrimSpace(string(runes[:chatMaxLen]))
	}
	return text, text != ""
}

// allowChat is the token bucket, refilled by elapsed time rather than by a
// ticker so an idle room costs nothing.
func (m *member) allowChat(now time.Time) bool {
	if m.chatSeen.IsZero() {
		m.chatTokens = chatBurst
	} else {
		m.chatTokens += now.Sub(m.chatSeen).Seconds() / chatRefill.Seconds()
		if m.chatTokens > chatBurst {
			m.chatTokens = chatBurst
		}
	}
	m.chatSeen = now

	if m.chatTokens < 1 {
		return false
	}
	m.chatTokens--
	return true
}
