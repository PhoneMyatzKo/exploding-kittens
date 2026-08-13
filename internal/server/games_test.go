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

	var kittens *games.Info
	playable := 0
	for i, g := range out.Games {
		if g.Slug == games.Kittens {
			kittens = &out.Games[i]
		}
		if g.Playable {
			playable++
		}
	}
	if kittens == nil {
		t.Fatalf("catalogue does not list %q: %+v", games.Kittens, out.Games)
	}
	if !kittens.Playable {
		t.Error("the one implemented game is not marked playable")
	}
	if kittens.Name == "" || kittens.Max < kittens.Min || kittens.Min < 2 {
		t.Errorf("kittens tile is not renderable: %+v", *kittens)
	}
	if playable != 1 {
		t.Errorf("%d games marked playable, want exactly 1", playable)
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

	for _, slug := range []string{"uno", "chess", "kittens "} {
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
