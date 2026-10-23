package room

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Chat is the room's, not a game's, and almost every test here is really about
// that one claim: it has to work when no game is running, keep working across a
// deal, and never end up in the play-by-play.

func say(t *testing.T, h *harness, i int, text string) {
	t.Helper()
	h.room.Submit(h.ids[i], ClientMsg{Type: "chat", Text: text})
	h.sync(t)
}

func TestAMessageReachesEverybodyInTheRoom(t *testing.T) {
	h := newHarness(t, 3)
	say(t, h, 0, "hello")

	for i, rec := range h.recs {
		got := rec.chatTexts()
		if len(got) != 1 || got[0] != "hello" {
			// Continue rather than index into nothing: a panic here would bury
			// every other failure in the package behind a stack trace.
			t.Errorf("player %d sees %v", i, got)
			continue
		}
		line := rec.chatLines()[0]
		if line.ActorID != h.ids[0] {
			t.Errorf("player %d sees it from %q, want %q", i, line.ActorID, h.ids[0])
		}
		// The name travels with the line so it can still be rendered by somebody
		// who never saw that player in a seat list.
		if line.Name != "PA" {
			t.Errorf("player %d sees the name %q", i, line.Name)
		}
	}
}

// The whole reason chat is handled above the "has the game started" guard.
func TestPeopleCanTalkInTheLobby(t *testing.T) {
	h := newHarness(t, 2)
	if h.room.game.Started() {
		t.Fatal("the game started on its own")
	}
	say(t, h, 1, "are we waiting for anyone?")

	if got := h.recs[0].chatTexts(); len(got) != 1 {
		t.Fatalf("nothing was said in the lobby: %v", got)
	}
	// And a *game* move in the lobby is still refused, so the exemption is chat's
	// alone rather than a hole in the guard.
	h.room.Submit(h.ids[1], ClientMsg{Type: "play"})
	h.sync(t)
	if errs := h.recs[1].errors(); len(errs) == 0 {
		t.Error("a game move was accepted before the deal")
	}
}

// logbuf is emptied on every deal. Chat keeps its own buffer precisely so a
// conversation is not wiped when somebody starts the next round.
func TestTheConversationSurvivesADeal(t *testing.T) {
	h := newHarness(t, 3)
	say(t, h, 0, "before the deal")
	h.start(t)
	say(t, h, 1, "after the deal")

	want := []string{"before the deal", "after the deal"}
	for i, rec := range h.recs {
		got := rec.chatTexts()
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("player %d sees %v, want %v", i, got, want)
		}
	}
}

// Chat must never reach the game or the play-by-play: it is conversation, not a
// record of what the table did, and mixing them buries the moves.
func TestChatNeverEntersThePlayByPlay(t *testing.T) {
	h := newHarness(t, 3)
	h.start(t)
	before := len(h.recs[0].await(t, 0).Log)

	say(t, h, 0, "nice card")
	h.sync(t)

	v, _ := h.recs[0].snapshot()
	if got := len(v.Log); got != before {
		t.Errorf("the play-by-play grew from %d to %d lines on a chat message", before, got)
	}
	for _, e := range v.Log {
		if strings.Contains(e.Text, "nice card") {
			t.Fatalf("a chat message was written into the log: %+v", e)
		}
	}
}

// Chat arrives only as it happens, so unlike the play-by-play it has to be
// handed over on the way back in — otherwise a phone that slept comes back to an
// empty panel and the conversation reads as lost.
func TestReconnectingBringsTheConversationBack(t *testing.T) {
	h := newHarness(t, 2)
	say(t, h, 0, "first")
	say(t, h, 1, "second")

	// Same token, new connection — the ordinary reconnect path.
	fresh := &recorder{}
	tok := h.room.members[1].Token
	if _, _, err := h.room.Join(tok, "PB", fresh); err != nil {
		t.Fatal(err)
	}
	h.sync(t)

	got := fresh.chatTexts()
	if strings.Join(got, "|") != "first|second" {
		t.Errorf("came back to %v", got)
	}
}

func TestSomebodyJoiningSeesWhatWasSaid(t *testing.T) {
	h := newHarness(t, 2)
	say(t, h, 0, "we need one more")

	late := &recorder{}
	if _, _, err := h.room.Join("", "PC", late); err != nil {
		t.Fatal(err)
	}
	h.sync(t)

	if got := late.chatTexts(); len(got) != 1 || got[0] != "we need one more" {
		t.Errorf("the new arrival sees %v", got)
	}
}

// One client must not be able to fill the room's goroutine or push the whole
// conversation out of the ring buffer.
func TestFloodingIsRefusedAndSaysSo(t *testing.T) {
	h := newHarness(t, 2)
	for i := 0; i < int(chatBurst)+4; i++ {
		say(t, h, 0, "spam")
	}

	got := len(h.recs[0].chatTexts())
	if got > int(chatBurst) {
		t.Errorf("%d messages got through a burst of %v", got, chatBurst)
	}
	if got == 0 {
		t.Fatal("nothing got through at all, so the bucket never filled")
	}
	// Refused rather than silently dropped: somebody typing into a dead box
	// deserves to be told.
	if errs := h.recs[0].errors(); len(errs) == 0 {
		t.Error("the flood was dropped without telling anybody")
	}
	// And it is per member, so the flood does not silence the table.
	say(t, h, 1, "still here")
	if texts := h.recs[1].chatTexts(); texts[len(texts)-1] != "still here" {
		t.Error("one player's flood stopped another from talking")
	}
}

func TestTheBucketRefills(t *testing.T) {
	m := &member{}
	now := time.Now()
	for i := 0; i < int(chatBurst); i++ {
		if !m.allowChat(now) {
			t.Fatalf("message %d of the opening burst was refused", i)
		}
	}
	if m.allowChat(now) {
		t.Fatal("the burst had no floor")
	}
	if !m.allowChat(now.Add(chatRefill)) {
		t.Errorf("still refused after %v", chatRefill)
	}
}

func TestEmptyAndWhitespaceMessagesAreDropped(t *testing.T) {
	h := newHarness(t, 2)
	for _, raw := range []string{"", "   ", "\n\n", "\t"} {
		say(t, h, 0, raw)
	}
	if got := h.recs[0].chatTexts(); len(got) != 0 {
		t.Errorf("empty messages became %v", got)
	}
}

func TestAMessageIsFlattenedToOneLine(t *testing.T) {
	h := newHarness(t, 2)
	say(t, h, 0, "one\ntwo\tthree")

	got := h.recs[0].chatTexts()
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if strings.ContainsAny(got[0], "\n\t\r") {
		t.Errorf("a newline survived: %q", got[0])
	}
	// Flattened to spaces rather than stripped, or the words run together.
	if got[0] != "one two three" {
		t.Errorf("got %q, want %q", got[0], "one two three")
	}
}

// Truncation is by rune, not byte. Burmese is three bytes a character, so a byte
// cap would cut one in half and put invalid UTF-8 on the wire — which is the
// whole reason this is worth a test rather than a comment.
func TestALongMessageIsCutOnACharacterBoundary(t *testing.T) {
	h := newHarness(t, 2)
	long := strings.Repeat("မင်္ဂလာပါ", 200)
	say(t, h, 0, long)

	got := h.recs[0].chatTexts()
	if len(got) != 1 {
		t.Fatalf("got %v lines", len(got))
	}
	if !utf8.ValidString(got[0]) {
		t.Fatal("the truncated message is not valid UTF-8")
	}
	if n := utf8.RuneCountInString(got[0]); n > chatMaxLen {
		t.Errorf("%d runes survived a cap of %d", n, chatMaxLen)
	}
	if !strings.HasPrefix(long, got[0]) {
		t.Error("the truncation changed the text rather than shortening it")
	}
}

func TestTheConversationIsCapped(t *testing.T) {
	h := newHarness(t, 2)
	// Past the ring buffer, stepping the clock so the rate limiter is not what
	// is being measured here.
	for i := 0; i < chatLimit+20; i++ {
		h.room.members[0].chatSeen = time.Time{} // refill
		say(t, h, 0, "line")
	}
	if got := len(h.room.chatbuf); got > chatLimit {
		t.Errorf("the room holds %d lines, cap is %d", got, chatLimit)
	}

	// And a late arrival gets the capped buffer rather than everything.
	late := &recorder{}
	if _, _, err := h.room.Join("", "PZ", late); err != nil {
		t.Fatal(err)
	}
	h.sync(t)
	if got := len(late.chatTexts()); got > chatLimit {
		t.Errorf("a new arrival was sent %d lines", got)
	}
}

func TestChatSeqsClimb(t *testing.T) {
	h := newHarness(t, 2)
	for i := 0; i < 3; i++ {
		h.room.members[0].chatSeen = time.Time{}
		say(t, h, 0, "x")
	}
	last := 0
	for _, l := range h.recs[0].chatLines() {
		if l.Seq <= last {
			t.Fatalf("seq %d followed %d", l.Seq, last)
		}
		last = l.Seq
	}
}

func TestCleanChat(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
		ok             bool
	}{
		{"plain", "hello", "hello", true},
		{"trimmed", "  hi  ", "hi", true},
		{"empty", "", "", false},
		{"spaces only", "   ", "", false},
		{"newlines become spaces", "a\nb", "a b", true},
		{"control characters go", "a\x00\x07b", "ab", true},
		{"burmese is left alone", "မင်္ဂလာပါ", "မင်္ဂလာပါ", true},
		// Markup is not escaped and does not need to be: every client writes chat
		// with textContent, so this arrives as the characters somebody typed.
		{"markup is text", "<b>hi</b>", "<b>hi</b>", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cleanChat(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v (got %q)", ok, tc.ok, got)
			}
			if ok && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
