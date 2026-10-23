package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"boardgame/kittens/internal/analytics"
	"boardgame/kittens/internal/room"
)

func postSuggestion(t *testing.T, base string, in analytics.SuggestionInput) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(in)
	resp, err := http.Post(base+"/api/suggestions", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func getAdmin(t *testing.T, base, path, token string, out any) int {
	t.Helper()
	req, _ := http.NewRequest("GET", base+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

func TestSuggestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := newSuggestLimiter()
	l.nowFunc = func() time.Time { return now }
	for i := 0; i < suggestBurst; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("suggestion %d refused, want allowed", i+1)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("one over the burst was allowed")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("another address was refused")
	}
	now = now.Add(suggestWindow)
	if !l.allow("1.2.3.4") {
		t.Fatal("refused after the window ended")
	}
}

func TestAdminEndpointsRefuseWithoutToken(t *testing.T) {
	mgr := room.NewManager()
	srv := httptest.NewServer(New(mgr, Analytics{Token: "s3cret"}, testPartyKey))
	t.Cleanup(func() { srv.Close(); mgr.Shutdown() })

	for _, path := range []string{"/api/export", "/api/suggestions"} {
		if got := getAdmin(t, srv.URL, path, "guess", nil); got != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", path, got)
		}
	}
	// Without a database there is nowhere to keep a suggestion.
	if got, _ := postSuggestion(t, srv.URL, analytics.SuggestionInput{GameName: "Ludo", Category: "family"}); got != http.StatusServiceUnavailable {
		t.Fatalf("POST without a store: status = %d, want 503", got)
	}
}

func TestSuggestionsAndExport(t *testing.T) {
	store := testStore(t, "ek_suggest_test")
	rec := analytics.NewRecorder(store, nil)

	const token = "test-token"
	mgr := room.NewManager()
	srv := httptest.NewServer(New(mgr, Analytics{Store: store, Recorder: rec, Suggestions: store, Token: token}, testPartyKey))
	t.Cleanup(func() { srv.Close(); mgr.Shutdown() })

	if got, body := postSuggestion(t, srv.URL, analytics.SuggestionInput{GameName: "Poker", Category: "card"}); got != http.StatusBadRequest {
		t.Fatalf("poker: status = %d (%v), want 400", got, body)
	}
	for _, name := range []string{"Sushi Go", "Ludo"} {
		cat := analytics.CategoryCard
		if name == "Ludo" {
			cat = analytics.CategoryFamily
		}
		if got, body := postSuggestion(t, srv.URL, analytics.SuggestionInput{
			GameName: name, Category: cat, SuggestedBy: "Ann",
		}); got != http.StatusOK {
			t.Fatalf("%s: status = %d (%v), want 200", name, got, body)
		}
	}

	var list struct{ Suggestions []analytics.Suggestion }
	if got := getAdmin(t, srv.URL, "/api/suggestions", token, &list); got != http.StatusOK {
		t.Fatalf("list: status = %d", got)
	}
	if len(list.Suggestions) != 2 || list.Suggestions[0].GameName != "Sushi Go" ||
		list.Suggestions[1].Category != "family" || list.Suggestions[0].CountryCode != "LAN" {
		t.Fatalf("suggestions = %+v", list.Suggestions)
	}
	list.Suggestions = nil
	getAdmin(t, srv.URL, "/api/suggestions?after=1", token, &list)
	if len(list.Suggestions) != 1 || list.Suggestions[0].GameName != "Ludo" {
		t.Fatalf("after=1: %+v, want only Ludo", list.Suggestions)
	}

	// Two rooms, two players in the first.
	code := createRoom(t, srv.URL)
	a := dial(t, srv.URL, code, "Ann", "")
	a.await(t, 0)
	b := dial(t, srv.URL, code, "Bob", "")
	b.await(t, 0)
	settle(t, []*player{a, b})
	createRoom(t, srv.URL)
	rec.Close()

	var ex analytics.Export
	if got := getAdmin(t, srv.URL, "/api/export", token, &ex); got != http.StatusOK {
		t.Fatalf("export: status = %d", got)
	}
	if len(ex.Rooms) != 2 || len(ex.Players) != 2 || ex.Players[0].RoomID != ex.Rooms[0].ID {
		t.Fatalf("export = %+v", ex)
	}

	// A page of one room holds back the players of rooms beyond it, so the
	// reader is never handed a player whose room it does not have.
	ex = analytics.Export{}
	getAdmin(t, srv.URL, "/api/export?limit=1", token, &ex)
	if len(ex.Rooms) != 1 || len(ex.Players) != 1 {
		t.Fatalf("limit=1: %+v, want one room and one player", ex)
	}
	ex = analytics.Export{}
	getAdmin(t, srv.URL, "/api/export?rooms_after=0&players_after=0&limit=0", token, &ex)
	// limit=0 is refused, so nothing was decoded.
	if len(ex.Rooms) != 0 {
		t.Fatalf("limit=0 was accepted: %+v", ex)
	}
}
