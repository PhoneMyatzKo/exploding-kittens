package analytics

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeOdoo records what the webhook routes receive. The first failures
// requests to any route get a 503, as from an Odoo that is restarting.
type fakeOdoo struct {
	mu       sync.Mutex
	failures int
	calls    []string
	bodies   []map[string]any
	auth     []string
}

func (f *fakeOdoo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.URL.Path)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	if f.failures > 0 {
		f.failures--
		http.Error(w, "restarting", http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		http.Error(w, `{"error": "forbidden"}`, http.StatusForbidden)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.bodies = append(f.bodies, body)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok": true, "id": 42}`))
}

func newTestOdoo(t *testing.T, f *fakeOdoo, token string) *Odoo {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	o := NewOdoo(srv.URL+"/", token)
	o.retryWait = []time.Duration{time.Millisecond, time.Millisecond}
	return o
}

func TestOdooRecorderSendsEventsInOrder(t *testing.T) {
	f := &fakeOdoo{}
	rec := NewRecorder(newTestOdoo(t, f, "tok"), nil)
	rec.RoomCreated("ABCD", "uno", true)
	rec.PlayerJoined("ABCD", "p1", net.ParseIP("192.168.1.5"))
	rec.Close()

	want := []string{"/kittens/webhook/room", "/kittens/webhook/player"}
	if len(f.calls) != 2 || f.calls[0] != want[0] || f.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
	if f.auth[0] != "Bearer tok" {
		t.Errorf("Authorization = %q", f.auth[0])
	}
	room, player := f.bodies[0], f.bodies[1]
	if room["code"] != "ABCD" || room["game_slug"] != "uno" || room["public"] != true || room["at"] == "" {
		t.Errorf("room body = %v", room)
	}
	if player["code"] != "ABCD" || player["player_id"] != "p1" || player["country_code"] != "LAN" {
		t.Errorf("player body = %v", player)
	}
}

func TestOdooRetriesWhileRestarting(t *testing.T) {
	f := &fakeOdoo{failures: 2}
	o := newTestOdoo(t, f, "tok")
	if err := o.RoomCreated(context.Background(), RoomEvent{Code: "ABCD"}); err != nil {
		t.Fatalf("RoomCreated: %v", err)
	}
	if len(f.calls) != 3 {
		t.Errorf("tries = %d, want 3", len(f.calls))
	}
}

func TestOdooDoesNotRetryAWrongToken(t *testing.T) {
	f := &fakeOdoo{}
	o := newTestOdoo(t, f, "wrong")
	if err := o.RoomCreated(context.Background(), RoomEvent{Code: "ABCD"}); err == nil {
		t.Fatal("RoomCreated with a wrong token: want an error")
	}
	if len(f.calls) != 1 {
		t.Errorf("tries = %d, want 1", len(f.calls))
	}
}

func TestOdooAddSuggestion(t *testing.T) {
	f := &fakeOdoo{}
	o := newTestOdoo(t, f, "tok")
	in := SuggestionInput{GameName: "Ludo", Category: "family", SuggestedBy: "Ann"}
	id, err := o.AddSuggestion(context.Background(), in, Country{Code: "TH", Name: "Thailand"})
	if err != nil {
		t.Fatalf("AddSuggestion: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
	body := f.bodies[0]
	if f.calls[0] != "/kittens/webhook/suggestion" || body["game_name"] != "Ludo" || body["country_code"] != "TH" {
		t.Errorf("call %s, body %v", f.calls[0], body)
	}
}
