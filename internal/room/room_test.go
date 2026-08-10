package room

import (
	"encoding/json"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"boardgame/kittens/internal/view"
)

// recorder is a fake client connection. It decodes whatever the room sends so
// tests can drive play from exactly the information a real browser would have.
type recorder struct {
	mu     sync.Mutex
	state  *view.View
	states int
	errs   []string
	closed bool
}

func (r *recorder) Send(b []byte) {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(b, &head) != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch head.Type {
	case "state":
		var v view.View
		if json.Unmarshal(b, &v) == nil {
			r.state = &v
			r.states++
		}
	case "error":
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(b, &e) == nil {
			r.errs = append(r.errs, e.Message)
		}
	}
}

func (r *recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

func (r *recorder) snapshot() (*view.View, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, r.states
}

func (r *recorder) errors() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.errs...)
}

// await blocks until the recorder has received a state newer than `since`.
func (r *recorder) await(t *testing.T, since int) *view.View {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if v, n := r.snapshot(); n > since && v != nil {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for a state update past #%d", since)
	return nil
}

type harness struct {
	room *Room
	recs []*recorder
	ids  []string
}

func newHarness(t *testing.T, n int) *harness {
	t.Helper()
	h := &harness{room: newRoom("TEST")}
	t.Cleanup(h.room.close)
	for i := 0; i < n; i++ {
		rec := &recorder{}
		id, token, err := h.room.Join("", "P"+string(rune('A'+i)), rec)
		if err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		if token == "" {
			t.Fatal("join returned an empty token")
		}
		h.recs = append(h.recs, rec)
		h.ids = append(h.ids, id)
	}
	return h
}

// act submits a message on behalf of a player and waits until *every* recorder
// has the resulting state. Waiting on only the actor would race: broadcast walks
// the members in order, so a later member can still be holding the previous view
// when the actor's has already landed.
func (h *harness) act(t *testing.T, i int, msg ClientMsg) *view.View {
	t.Helper()
	before := make([]int, len(h.recs))
	for j, rec := range h.recs {
		_, before[j] = rec.snapshot()
	}
	h.room.Submit(h.ids[i], msg)
	var out *view.View
	for j, rec := range h.recs {
		v := rec.await(t, before[j])
		if j == i {
			out = v
		}
	}
	return out
}

func (h *harness) start(t *testing.T) *view.View {
	t.Helper()
	return h.act(t, 0, ClientMsg{Type: "start"})
}

// -------------------------------------------------------------------- tests

func TestLobbyJoinAndStart(t *testing.T) {
	h := newHarness(t, 3)

	// Join returns as soon as the seat exists; the first broadcast follows.
	v := h.recs[2].await(t, 0)
	if v.Started {
		t.Fatal("expected an un-started lobby view")
	}
	if len(v.Seats) != 3 {
		t.Fatalf("seats = %d, want 3", len(v.Seats))
	}
	if !v.Seats[0].Host || v.Seats[2].Host {
		t.Error("the first player to join should be the only host")
	}

	v = h.start(t)
	if !v.Started {
		t.Fatal("game did not start")
	}
	for i, rec := range h.recs {
		got, _ := rec.snapshot()
		if len(got.Me.Hand) != 8 {
			t.Errorf("player %d hand = %d cards, want 8", i, len(got.Me.Hand))
		}
	}
}

func TestOnlyHostCanStart(t *testing.T) {
	h := newHarness(t, 2)
	h.room.Submit(h.ids[1], ClientMsg{Type: "start"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if errs := h.recs[1].errors(); len(errs) > 0 {
			if !strings.Contains(errs[0], "host") {
				t.Errorf("error = %q, want it to mention the host", errs[0])
			}
			if v, _ := h.recs[1].snapshot(); v.Started {
				t.Error("a non-host managed to start the game")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("non-host start was silently accepted")
}

func TestTooFewPlayersCannotStart(t *testing.T) {
	h := newHarness(t, 1)
	h.room.Submit(h.ids[0], ClientMsg{Type: "start"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if errs := h.recs[0].errors(); len(errs) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("a solo game was allowed to start")
}

func TestReconnectRestoresTheSameSeatAndHand(t *testing.T) {
	h := &harness{room: newRoom("TEST")}
	t.Cleanup(h.room.close)

	recA, recB := &recorder{}, &recorder{}
	idA, tokenA, _ := h.room.Join("", "Ann", recA)
	idB, _, _ := h.room.Join("", "Bob", recB)
	h.recs = []*recorder{recA, recB}
	h.ids = []string{idA, idB}

	before := h.start(t)
	handBefore := before.Me.Hand

	// Ann reopens the tab while the old socket is still attached — the room must
	// hand the seat to the new connection and hang up on the stale one.
	recA2 := &recorder{}
	idA2, tokenA2, err := h.room.Join(tokenA, "Ann", recA2)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if idA2 != idA {
		t.Errorf("player id = %s, want the original %s", idA2, idA)
	}
	if tokenA2 != tokenA {
		t.Error("reconnecting should not mint a new token")
	}

	after := recA2.await(t, 0)
	if len(after.Me.Hand) != len(handBefore) {
		t.Fatalf("hand after reconnect = %d cards, want %d", len(after.Me.Hand), len(handBefore))
	}
	for i := range handBefore {
		if after.Me.Hand[i].ID != handBefore[i].ID {
			t.Fatalf("hand changed across the reconnect")
		}
	}
	if !recA.closed {
		t.Error("the superseded connection should have been closed")
	}
}

func TestJoiningAGameInProgressIsRefused(t *testing.T) {
	h := newHarness(t, 2)
	h.start(t)

	if _, _, err := h.room.Join("", "Latecomer", &recorder{}); err != ErrInProgress {
		t.Errorf("err = %v, want ErrInProgress", err)
	}
}

func TestRoomCapacity(t *testing.T) {
	h := newHarness(t, 5)
	if _, _, err := h.room.Join("", "Sixth", &recorder{}); err != ErrRoomFull {
		t.Errorf("err = %v, want ErrRoomFull", err)
	}
}

// TestFullGameThroughTheRoom drives a complete game over the room's real command
// channel — goroutine, timers, broadcasts and all — using only the information
// each redacted view exposes. It is the closest thing to three browser tabs that
// can run in CI.
func TestFullGameThroughTheRoom(t *testing.T) {
	h := newHarness(t, 3)
	h.start(t)
	rng := rand.New(rand.NewSource(42))

	for step := 0; step < 600; step++ {
		if v, _ := h.recs[0].snapshot(); v.Phase == "game_over" {
			if v.WinnerID == "" {
				t.Fatal("game over with no winner")
			}
			// Everyone must agree on the outcome.
			for i, rec := range h.recs {
				got, _ := rec.snapshot()
				if got.WinnerID != v.WinnerID {
					t.Fatalf("player %d sees winner %q, player 0 sees %q", i, got.WinnerID, v.WinnerID)
				}
			}
			return
		}

		i, msg, ok := nextMove(h, rng)
		if !ok {
			t.Fatalf("step %d: nobody could act (phase=%s)", step, mustPhase(h))
		}
		h.act(t, i, msg)

		for j, rec := range h.recs {
			if errs := rec.errors(); len(errs) > 0 {
				t.Fatalf("step %d: player %d was rejected: %v", step, j, errs)
			}
		}
	}
	t.Fatalf("game did not finish in 600 moves (phase=%s)", mustPhase(h))
}

func mustPhase(h *harness) string {
	v, _ := h.recs[0].snapshot()
	if v == nil {
		return "?"
	}
	return v.Phase
}

// nextMove picks a legal action for whoever the redacted views say must act.
func nextMove(h *harness, rng *rand.Rand) (int, ClientMsg, bool) {
	for i, rec := range h.recs {
		v, _ := rec.snapshot()
		if v == nil {
			continue
		}
		switch {
		case v.Me.MustPlace:
			return i, ClientMsg{Type: "place", Index: rng.Intn(v.DeckCount + 1)}, true
		case v.Me.MustGive:
			return i, ClientMsg{Type: "give", CardIDs: []int{v.Me.Hand[0].ID}}, true
		case v.Me.CanPass:
			// Half the time, actually Nope it — this is what exercises the
			// window reopening and the Yup-back path.
			if v.Me.CanNope && rng.Intn(2) == 0 {
				return i, ClientMsg{Type: "nope"}, true
			}
			return i, ClientMsg{Type: "pass"}, true
		}
	}

	// Nobody is blocking; let the active player move.
	for i, rec := range h.recs {
		v, _ := rec.snapshot()
		if v == nil || !v.Me.MyTurn {
			continue
		}
		if rng.Intn(3) > 0 {
			if msg, ok := somePlay(v, rng); ok {
				return i, msg, true
			}
		}
		return i, ClientMsg{Type: "draw"}, true
	}
	return 0, ClientMsg{}, false
}

func somePlay(v *view.View, rng *rand.Rand) (ClientMsg, bool) {
	var others []string
	for _, s := range v.Seats {
		if s.Alive && s.ID != v.Me.ID {
			others = append(others, s.ID)
		}
	}
	if len(others) == 0 {
		return ClientMsg{}, false
	}
	target := others[rng.Intn(len(others))]

	cats := map[string][]int{}
	var singles []string
	var singleIDs []int
	for _, c := range v.Me.Hand {
		switch {
		case strings.HasPrefix(c.Slug, "cat-"):
			cats[c.Slug] = append(cats[c.Slug], c.ID)
		case c.Slug == "skip", c.Slug == "attack", c.Slug == "shuffle", c.Slug == "future", c.Slug == "favor":
			singles = append(singles, c.Slug)
			singleIDs = append(singleIDs, c.ID)
		}
	}
	for _, ids := range cats {
		if len(ids) >= 2 {
			return ClientMsg{Type: "play", CardIDs: ids[:2], TargetID: target}, true
		}
	}
	if len(singles) == 0 {
		return ClientMsg{}, false
	}
	k := rng.Intn(len(singles))
	m := ClientMsg{Type: "play", CardIDs: []int{singleIDs[k]}}
	if singles[k] == "favor" {
		m.TargetID = target
	}
	return m, true
}
