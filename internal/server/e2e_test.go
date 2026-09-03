package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"boardgame/kittens/internal/prng"
	"boardgame/kittens/internal/room"
	"boardgame/kittens/internal/view"

	"github.com/gorilla/websocket"
)

// player is a headless browser: it speaks the same JSON over the same WebSocket
// a real client does, and knows nothing the redacted view doesn't tell it.
type player struct {
	conn  *websocket.Conn
	mu    sync.Mutex
	state *view.View
	seen  int
	errs  []string
	id    string
	token string
}

func dial(t *testing.T, base, code, name, token string) *player {
	t.Helper()
	u, _ := url.Parse(base)
	u.Scheme = "ws"
	u.Path = "/ws"
	u.RawQuery = url.Values{"code": {code}, "name": {name}, "token": {token}}.Encode()

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (http %d)", name, err, status)
	}
	p := &player{conn: conn}
	t.Cleanup(func() { conn.Close() })
	go p.readLoop()
	return p
}

func (p *player) readLoop() {
	for {
		_, data, err := p.conn.ReadMessage()
		if err != nil {
			return
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &head) != nil {
			continue
		}
		p.mu.Lock()
		switch head.Type {
		case "joined":
			var j struct{ PlayerID, Token string }
			var raw map[string]string
			_ = json.Unmarshal(data, &raw)
			j.PlayerID, j.Token = raw["playerId"], raw["token"]
			p.id, p.token = j.PlayerID, j.Token
		case "state":
			var v view.View
			if json.Unmarshal(data, &v) == nil {
				p.state = &v
				p.seen++
			}
		case "error", "fatal":
			var e struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(data, &e)
			p.errs = append(p.errs, e.Message)
		}
		p.mu.Unlock()
	}
}

func (p *player) snapshot() (*view.View, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, p.seen
}

func (p *player) errors() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.errs...)
}

func (p *player) await(t *testing.T, since int) *view.View {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v, n := p.snapshot(); n > since && v != nil {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for a state message")
	return nil
}

func (p *player) send(t *testing.T, msg room.ClientMsg) {
	t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func newTestServer(t *testing.T) (string, *room.Manager) {
	t.Helper()
	mgr := room.NewManager()
	srv := httptest.NewServer(New(mgr))
	t.Cleanup(func() {
		srv.Close()
		mgr.Shutdown()
	})
	return srv.URL, mgr
}

// createRoomVisible posts an explicit visibility, the way the two buttons on the
// home screen do.
func createRoomVisible(t *testing.T, base string, public bool) string {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"public":%t}`, public))
	resp, err := http.Post(base+"/api/rooms", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Code   string `json:"code"`
		Public bool   `json:"public"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Public != public {
		t.Fatalf("created room public = %v, want %v", out.Public, public)
	}
	return out.Code
}

func listRooms(t *testing.T, base string) []room.Summary {
	t.Helper()
	resp, err := http.Get(base + "/api/rooms")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/rooms: status %d", resp.StatusCode)
	}
	var out struct {
		Rooms []room.Summary `json:"rooms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Rooms
}

func listedCodes(rooms []room.Summary) []string {
	out := make([]string, len(rooms))
	for i, s := range rooms {
		out[i] = s.Code
	}
	return out
}

// TestPublicLobbyListing walks the browser's whole story over HTTP: a public room
// with somebody in it shows up, a private one never does, and the row carries what
// the list needs to render.
func TestPublicLobbyListing(t *testing.T) {
	base, _ := newTestServer(t)

	pubCode := createRoomVisible(t, base, true)
	privCode := createRoomVisible(t, base, false)

	// Neither is listed yet: nobody has connected, so there is no host to join.
	if got := listedCodes(listRooms(t, base)); len(got) != 0 {
		t.Fatalf("listing = %v, want empty before anyone connects", got)
	}

	pub := dial(t, base, pubCode, "Ann", "")
	pub.await(t, 0)
	priv := dial(t, base, privCode, "Zoe", "")
	priv.await(t, 0)

	var row *room.Summary
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && row == nil {
		for _, s := range listRooms(t, base) {
			if s.Code == pubCode {
				row = &s
			}
		}
		if row == nil {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if row == nil {
		t.Fatalf("public room %s never appeared in %v", pubCode, listedCodes(listRooms(t, base)))
	}
	if got := listedCodes(listRooms(t, base)); slices.Contains(got, privCode) {
		t.Errorf("private room %s leaked into the listing %v", privCode, got)
	}
	if row.Players != 1 || row.Host != "Ann" || !row.Joinable {
		t.Errorf("row = %+v, want 1 player hosted by Ann and joinable", *row)
	}

	// Somebody found it in the list and walked in.
	second := dial(t, base, pubCode, "Bob", "")
	second.await(t, 0)
	settle(t, []*player{pub, second})
	act(t, []*player{pub, second}, 0, room.ClientMsg{Type: "start"})

	// Dealt: it should stop being advertised.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !slices.Contains(listedCodes(listRooms(t, base)), pubCode) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("room %s still listed after dealing", pubCode)
}

func TestRoomsDefaultToPublic(t *testing.T) {
	base, _ := newTestServer(t)

	// An empty POST is what a client that doesn't care sends.
	resp, err := http.Post(base+"/api/rooms", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Code   string `json:"code"`
		Public bool   `json:"public"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Public {
		t.Error("a room created with no stated visibility should be public")
	}

	p := dial(t, base, out.Code, "Ann", "")
	p.await(t, 0)
	waitUntil(t, func() bool {
		return slices.Contains(listedCodes(listRooms(t, base)), out.Code)
	}, "default room to be listed")
}

func waitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func createRoom(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Post(base+"/api/rooms", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Code) != 4 {
		t.Fatalf("room code = %q, want 4 characters", out.Code)
	}
	return out.Code
}

// ------------------------------------------------------------------ tests

func TestStaticClientIsServed(t *testing.T) {
	base, _ := newTestServer(t)
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestJoiningAnUnknownRoomIsRejected(t *testing.T) {
	base, _ := newTestServer(t)
	u, _ := url.Parse(base)
	u.Scheme = "ws"
	u.Path = "/ws"
	u.RawQuery = "code=ZZZZ&name=Nobody"

	_, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err == nil {
		t.Fatal("dial to a nonexistent room succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %v, want 404", resp)
	}
}

func TestRoomLookupEndpoint(t *testing.T) {
	base, _ := newTestServer(t)
	code := createRoom(t, base)

	// Lower case and stray punctuation should still find the room.
	resp, err := http.Get(base + "/api/rooms/" + strings.ToLower(code))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.Get(base + "/api/rooms/ZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown code", resp.StatusCode)
	}
}

// TestFullGameOverWebSockets plays a real game to a winner across three live
// WebSocket connections, driven purely from what each client is told. This is
// the automated stand-in for opening three browser tabs.
func TestFullGameOverWebSockets(t *testing.T) {
	base, _ := newTestServer(t)
	code := createRoom(t, base)

	names := []string{"Ann", "Bob", "Cid"}
	players := make([]*player, len(names))
	for i, n := range names {
		players[i] = dial(t, base, code, n, "")
		players[i].await(t, 0)
	}
	settle(t, players)

	act(t, players, 0, room.ClientMsg{Type: "start"})
	for i, p := range players {
		v, _ := p.snapshot()
		if !v.Started || len(v.Me.Hand) != 8 {
			t.Fatalf("player %d: started=%v hand=%d, want started with 8 cards", i, v.Started, len(v.Me.Hand))
		}
	}

	rng := prng.New(uint64(7))
	for step := 0; step < 800; step++ {
		v, _ := players[0].snapshot()
		if v.Phase == "game_over" {
			if v.WinnerID == "" {
				t.Fatal("game over with no winner")
			}
			for i, p := range players {
				got, _ := p.snapshot()
				if got.WinnerID != v.WinnerID {
					t.Fatalf("player %d disagrees about the winner: %q vs %q", i, got.WinnerID, v.WinnerID)
				}
			}
			t.Logf("winner: %s after %d moves", v.WinnerID, step)
			return
		}

		i, msg, ok := chooseMove(players, rng)
		if !ok {
			t.Fatalf("step %d: nobody could act (phase=%s)", step, v.Phase)
		}
		act(t, players, i, msg)

		for j, p := range players {
			if errs := p.errors(); len(errs) > 0 {
				t.Fatalf("step %d: player %d rejected: %v", step, j, errs)
			}
		}
	}
	t.Fatal("game did not finish in 800 moves")
}

func TestReconnectOverWebSocket(t *testing.T) {
	base, _ := newTestServer(t)
	code := createRoom(t, base)

	a := dial(t, base, code, "Ann", "")
	a.await(t, 0)
	b := dial(t, base, code, "Bob", "")
	b.await(t, 0)
	settle(t, []*player{a, b})

	act(t, []*player{a, b}, 0, room.ClientMsg{Type: "start"})
	before, _ := a.snapshot()

	a.mu.Lock()
	token, id := a.token, a.id
	a.mu.Unlock()
	if token == "" {
		t.Fatal("no token was issued")
	}

	a.conn.Close()
	a2 := dial(t, base, code, "Ann", token)
	after := a2.await(t, 0)

	a2.mu.Lock()
	newID := a2.id
	a2.mu.Unlock()
	if newID != id {
		t.Errorf("player id after reconnect = %s, want %s", newID, id)
	}
	if len(after.Me.Hand) != len(before.Me.Hand) {
		t.Errorf("hand after reconnect = %d cards, want %d", len(after.Me.Hand), len(before.Me.Hand))
	}
}

// Card faces are chosen by the server and travel in the redacted view, so this
// checks the whole chain at once: the catalogue built from the embedded scans,
// the per-copy pick at deal time, the JSON field, and the route the browser will
// actually put in an <img src>.
func TestDealtCardsCarryServableArt(t *testing.T) {
	base, _ := newTestServer(t)
	code := createRoom(t, base)

	a := dial(t, base, code, "Ann", "")
	a.await(t, 0)
	b := dial(t, base, code, "Bob", "")
	b.await(t, 0)
	settle(t, []*player{a, b})
	act(t, []*player{a, b}, 0, room.ClientMsg{Type: "start"})

	v, _ := a.snapshot()
	if len(v.Me.Hand) != 8 {
		t.Fatalf("hand = %d cards, want 8", len(v.Me.Hand))
	}

	// Copies of one kind must differ, which is the whole point — a hand holding
	// two Skips should show two different ways of skipping.
	byKind := map[string]map[string]bool{}
	for _, c := range v.Me.Hand {
		if c.Art == "" {
			t.Errorf("%s (card %d) was dealt with no art", c.Name, c.ID)
			continue
		}
		resp, err := http.Get(base + "/cards/" + c.Art)
		if err != nil {
			t.Fatalf("GET art for %s: %v", c.Name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: /cards/%s -> %d, want 200", c.Name, c.Art, resp.StatusCode)
		}
		if byKind[c.Slug] == nil {
			byKind[c.Slug] = map[string]bool{}
		}
		// Cats are printed one design each, so only the deep pools are checked.
		if byKind[c.Slug][c.Art] && !strings.HasPrefix(c.Slug, "cat-") {
			t.Errorf("two %s in one hand share the face %q", c.Name, c.Art)
		}
		byKind[c.Slug][c.Art] = true
	}

	// And both players have to be looking at the same picture for the same card:
	// a Defuse played face up is one physical card, whoever is holding it.
	bv, _ := b.snapshot()
	mine := map[int]string{}
	for _, c := range v.Me.Hand {
		mine[c.ID] = c.Art
	}
	for _, c := range bv.Me.Hand {
		if art, ok := mine[c.ID]; ok && art != c.Art {
			t.Errorf("card %d looks like %q to Ann and %q to Bob", c.ID, art, c.Art)
		}
	}
}

// ------------------------------------------------------------------ driving

// settle waits until every client can see the whole table. Without it, a message
// count captured for act() can be satisfied by a join broadcast still in flight
// rather than by the state the test actually triggered. Broadcasts are ordered
// per connection, so once the last join is visible everywhere, nothing earlier
// can still arrive.
func settle(t *testing.T, players []*player) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		done := true
		for _, p := range players {
			if v, _ := p.snapshot(); v == nil || len(v.Seats) != len(players) {
				done = false
				break
			}
		}
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("not every client saw all %d seats", len(players))
}

// act sends a message and waits until every client has observed the result, so
// the next decision is never made from a stale view.
func act(t *testing.T, players []*player, i int, msg room.ClientMsg) {
	t.Helper()
	before := make([]int, len(players))
	for j, p := range players {
		_, before[j] = p.snapshot()
	}
	players[i].send(t, msg)
	for j, p := range players {
		p.await(t, before[j])
	}
}

func chooseMove(players []*player, rng *prng.Source) (int, room.ClientMsg, bool) {
	for i, p := range players {
		v, _ := p.snapshot()
		if v == nil {
			continue
		}
		switch {
		case v.Me.MustPlace:
			return i, room.ClientMsg{Type: "place", Index: rng.Intn(v.DeckCount + 1)}, true
		case v.Me.MustGive:
			return i, room.ClientMsg{Type: "give", CardIDs: []int{v.Me.Hand[0].ID}}, true
		case v.Me.CanPass:
			if v.Me.CanNope && rng.Intn(2) == 0 {
				return i, room.ClientMsg{Type: "nope"}, true
			}
			return i, room.ClientMsg{Type: "pass"}, true
		}
	}
	for i, p := range players {
		v, _ := p.snapshot()
		if v == nil || !v.Me.MyTurn {
			continue
		}
		if rng.Intn(3) > 0 {
			if msg, ok := pickPlay(v, rng); ok {
				return i, msg, true
			}
		}
		return i, room.ClientMsg{Type: "draw"}, true
	}
	return 0, room.ClientMsg{}, false
}

func pickPlay(v *view.View, rng *prng.Source) (room.ClientMsg, bool) {
	var others []string
	for _, s := range v.Seats {
		if s.Alive && s.ID != v.Me.ID {
			others = append(others, s.ID)
		}
	}
	if len(others) == 0 {
		return room.ClientMsg{}, false
	}
	target := others[rng.Intn(len(others))]

	cats := map[string][]int{}
	var slugs []string
	var ids []int
	for _, c := range v.Me.Hand {
		switch {
		case strings.HasPrefix(c.Slug, "cat-"):
			cats[c.Slug] = append(cats[c.Slug], c.ID)
		case c.Slug == "skip", c.Slug == "attack", c.Slug == "shuffle", c.Slug == "future", c.Slug == "favor":
			slugs = append(slugs, c.Slug)
			ids = append(ids, c.ID)
		}
	}
	// Three matching cats demand a named card; two steal at random. Playing the
	// trio when it is available is what gets the demand path exercised here.
	for _, set := range cats {
		if len(set) >= 3 {
			return room.ClientMsg{
				Type: "play", CardIDs: set[:3], TargetID: target,
				Named: []string{"defuse", "nope", "skip"}[rng.Intn(3)],
			}, true
		}
		if len(set) >= 2 {
			return room.ClientMsg{Type: "play", CardIDs: set[:2], TargetID: target}, true
		}
	}
	if len(slugs) == 0 {
		return room.ClientMsg{}, false
	}
	k := rng.Intn(len(slugs))
	m := room.ClientMsg{Type: "play", CardIDs: []int{ids[k]}}
	if slugs[k] == "favor" {
		m.TargetID = target
	}
	return m, true
}
