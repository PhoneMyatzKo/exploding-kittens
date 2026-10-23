package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgame/kittens/internal/games"
	"boardgame/kittens/internal/room"
)

// The preview gate: a password that reveals games which work but are not ready
// to be shown to everybody who opens the page.
//
// Both halves are tested here and both matter. Hiding the tile is what somebody
// sees; refusing to open the room is what makes the hiding mean anything, since
// a menu filtered in the browser is one hand-written POST — or one shared invite
// link — away from no gate at all.

// menuSlugs is the menu as one visitor would receive it.
func menuSlugs(t *testing.T, base, key string) []string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/api/games", nil)
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set(partyKeyHeader, key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games returned %d", res.StatusCode)
	}
	var body struct {
		Games []games.Info `json:"games"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(body.Games))
	for _, g := range body.Games {
		out = append(out, g.Slug)
	}
	return out
}

func makeRoom(t *testing.T, base, slug, key string) int {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"game": slug, "public": true})
	req, err := http.NewRequest(http.MethodPost, base+"/api/rooms", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(partyKeyHeader, key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

func unlock(t *testing.T, base, key string) int {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"key": key})
	res, err := http.Post(base+"/api/unlock", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

// previewSlug is whichever game is currently behind the gate. Found rather than
// named, so this file does not have to be edited every time one graduates.
func previewSlug(t *testing.T) string {
	t.Helper()
	for _, g := range games.All() {
		if g.Preview {
			return g.Slug
		}
	}
	t.Skip("no game is behind the preview gate")
	return ""
}

func TestAPreviewGameIsNotOnTheMenuWithoutTheKey(t *testing.T) {
	base, _ := newTestServer(t)
	hidden := previewSlug(t)

	open := menuSlugs(t, base, "")
	for _, slug := range open {
		if slug == hidden {
			t.Fatalf("%s is on the menu for a visitor with no key", hidden)
		}
	}
	// And the rest of the menu is still there — a gate that empties the page is
	// not what this is for.
	if len(open) == 0 {
		t.Fatal("the menu is empty without the key")
	}
}

func TestTheKeyRevealsThePreviewGame(t *testing.T) {
	base, _ := newTestServer(t)
	hidden := previewSlug(t)

	var found bool
	for _, slug := range menuSlugs(t, base, testPartyKey) {
		if slug == hidden {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s is still hidden from a visitor holding the key", hidden)
	}
	// The whole catalogue, not a different one.
	if got, want := len(menuSlugs(t, base, testPartyKey)), len(games.All()); got != want {
		t.Errorf("the unlocked menu has %d games, the catalogue has %d", got, want)
	}
}

// The half that makes the hiding worth anything.
func TestAPreviewRoomCannotBeOpenedWithoutTheKey(t *testing.T) {
	base, _ := newTestServer(t)
	hidden := previewSlug(t)

	if got := makeRoom(t, base, hidden, ""); got != http.StatusForbidden {
		t.Errorf("POST /api/rooms for %s returned %d, want %d",
			hidden, got, http.StatusForbidden)
	}
	if got := makeRoom(t, base, hidden, testPartyKey); got != http.StatusOK {
		t.Errorf("POST /api/rooms for %s with the key returned %d, want 200", hidden, got)
	}
	// An ordinary game is unaffected either way, or the gate has become a wall.
	if got := makeRoom(t, base, games.Kittens, ""); got != http.StatusOK {
		t.Errorf("POST /api/rooms for kittens returned %d, want 200", got)
	}
}

func TestUnlockAnswersThePassword(t *testing.T) {
	base, _ := newTestServer(t)

	for _, wrong := range []string{"", "nope", testPartyKey + "x", "OPEN-SESAME"} {
		if got := unlock(t, base, wrong); got != http.StatusUnauthorized {
			t.Errorf("unlock(%q) returned %d, want 401", wrong, got)
		}
	}
	if got := unlock(t, base, testPartyKey); got != http.StatusOK {
		t.Errorf("unlock with the right password returned %d, want 200", got)
	}
}

// A gate that opens when it is misconfigured is not a gate. An empty key is how
// the flag is switched off, and switching it off must hide the preview games
// from everybody rather than reveal them to everybody.
func TestAnEmptyKeyLocksThePreviewGamesForEveryone(t *testing.T) {
	mgr := room.NewManager()
	srv := httptest.NewServer(New(mgr, Analytics{}, ""))
	t.Cleanup(func() { srv.Close(); mgr.Shutdown() })
	hidden := previewSlug(t)

	for _, key := range []string{"", "anything"} {
		for _, slug := range menuSlugs(t, srv.URL, key) {
			if slug == hidden {
				t.Fatalf("%s is on the menu with key %q and no key configured", hidden, key)
			}
		}
		if got := makeRoom(t, srv.URL, hidden, key); got != http.StatusForbidden {
			t.Errorf("room for %s with key %q returned %d, want 403", hidden, key, got)
		}
	}
	if got := unlock(t, srv.URL, "anything"); got != http.StatusUnauthorized {
		t.Errorf("unlock with no key configured returned %d, want 401", got)
	}
}

// Preview and Playable are different questions and must not be conflated: a
// preview game has rules behind it and is dealt normally once unlocked, so it
// must stay Playable or TestEveryPlayableGameHasRules and the room would
// disagree about whether it exists.
func TestAPreviewGameIsStillPlayable(t *testing.T) {
	hidden := previewSlug(t)
	if !games.Playable(hidden) {
		t.Errorf("%s is behind the gate and also marked unplayable — "+
			"hiding a game must not claim it has no rules", hidden)
	}
}
