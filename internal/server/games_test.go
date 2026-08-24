package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"boardgame/kittens/internal/games"
)

// The menu is built from the catalogue endpoint, so a game the server cannot
// deal must be visibly not-playable rather than absent — the tile is meant to
// say "coming soon", and hiding it would leave the hub looking empty.
func TestCatalogueIsServed(t *testing.T) {
	base, _ := newTestServer(t)

	resp, err := http.Get(base + "/api/games")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games: status %d", resp.StatusCode)
	}

	var out struct {
		Games []games.Info `json:"games"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Games) < 2 {
		t.Fatalf("catalogue has %d entries, want the playable game and at least one announced", len(out.Games))
	}

	byslug := map[string]games.Info{}
	playable := 0
	for _, g := range out.Games {
		byslug[g.Slug] = g
		if g.Playable {
			playable++
		}
	}
	for _, slug := range []string{games.Kittens, games.UNO} {
		g, ok := byslug[slug]
		if !ok {
			t.Fatalf("catalogue does not list %q: %+v", slug, out.Games)
		}
		if !g.Playable {
			t.Errorf("%q is implemented but not marked playable", slug)
		}
		if g.Name == "" || g.Max < g.Min || g.Min < 2 {
			t.Errorf("%q tile is not renderable: %+v", slug, g)
		}
	}
	// Something has to still be announced-but-unbuilt, or the "soon" badge and
	// the refusal at POST /api/rooms are both untested paths.
	if playable == len(out.Games) {
		t.Error("every game is playable, so nothing exercises the locked tile")
	}
}

// A tile the menu offers must be a tile the server can actually deal. This is
// the check that keeps the catalogue and the rules registry from drifting: the
// catalogue is data, and it is very easy to flip a flag with nothing behind it.
func TestEveryPlayableGameHasRules(t *testing.T) {
	for _, g := range games.All() {
		_, err := games.New(g.Slug, nil)
		if g.Playable && err != nil {
			t.Errorf("%q is playable but has no rules: %v", g.Slug, err)
		}
		if !g.Playable && err == nil {
			t.Errorf("%q has rules but is not marked playable", g.Slug)
		}
	}
}

// A client that predates the menu sends no game at all. It has to keep working,
// which is the whole reason the field defaults rather than being required.
func TestRoomsDefaultToKittens(t *testing.T) {
	base, _ := newTestServer(t)

	code, slug := createRoomForGame(t, base, "")
	if slug != games.Kittens {
		t.Errorf("created game = %q, want %q", slug, games.Kittens)
	}

	// The client picks its renderer off the view, so the slug has to survive the
	// trip through the room to the socket, not just the POST response.
	p := dial(t, base, code, "Ann", "")
	defer p.conn.Close()
	waitUntil(t, func() bool {
		v, _ := p.snapshot()
		return v != nil
	}, "the first state")

	v, _ := p.snapshot()
	if v.Game != games.Kittens {
		t.Errorf("view game = %q, want %q", v.Game, games.Kittens)
	}
}

// An announced-but-unbuilt game must be refused at the door. Letting one through
// would open a room nothing downstream knows how to deal.
func TestUnbuiltAndUnknownGamesAreRefused(t *testing.T) {
	base, _ := newTestServer(t)

	for _, slug := range []string{"uno-no-mercy", "chess", "kittens "} {
		t.Run(slug, func(t *testing.T) {
			body := strings.NewReader(`{"game":` + quote(slug) + `}`)
			resp, err := http.Post(base+"/api/rooms", "application/json", body)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("POST game=%q: status %d, want %d", slug, resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

// The lobby browser spans every game once there is more than one, so a row has
// to say what it is before somebody taps it.
func TestListingSaysWhichGame(t *testing.T) {
	base, _ := newTestServer(t)

	code, _ := createRoomForGame(t, base, games.Kittens)
	p := dial(t, base, code, "Ann", "")
	defer p.conn.Close()

	waitUntil(t, func() bool {
		for _, s := range listRooms(t, base) {
			if s.Code == code {
				return true
			}
		}
		return false
	}, "the room to be listed")

	for _, s := range listRooms(t, base) {
		if s.Code != code {
			continue
		}
		if s.Game != games.Kittens {
			t.Errorf("listed game = %q, want %q", s.Game, games.Kittens)
		}
		return
	}
}

// createRoomForGame posts a room for a slug, or for whatever the server defaults
// to when slug is empty, and returns the code and the game it actually got.
func createRoomForGame(t *testing.T, base, slug string) (code, game string) {
	t.Helper()

	var body *strings.Reader
	if slug == "" {
		body = strings.NewReader(`{}`)
	} else {
		body = strings.NewReader(`{"game":` + quote(slug) + `}`)
	}

	resp, err := http.Post(base+"/api/rooms", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/rooms game=%q: status %d", slug, resp.StatusCode)
	}

	var out struct {
		Code string `json:"code"`
		Game string `json:"game"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Code, out.Game
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
