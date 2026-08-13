package room

import (
	"testing"
	"time"

	"boardgame/kittens/internal/games/kittens/game"
)

// waitFor polls until cond holds, since room work happens on its own goroutine.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// joinN puts n connected players into a room and returns their recorders so the
// caller can keep them alive for the length of the test.
func joinN(t *testing.T, r *Room, n int) []*recorder {
	t.Helper()
	out := make([]*recorder, 0, n)
	for i := 0; i < n; i++ {
		rec := &recorder{}
		if _, _, err := r.Join("", "P"+string(rune('A'+i)), rec); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		out = append(out, rec)
	}
	return out
}

func codesOf(rooms []Summary) []string {
	out := make([]string, len(rooms))
	for i, s := range rooms {
		out[i] = s.Code
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

func TestListOnlyShowsPublicRooms(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	pub := m.Create(Options{Public: true, Game: "kittens"})
	priv := m.Create(Options{Public: false, Game: "kittens"})
	joinN(t, pub, 1)
	joinN(t, priv, 1)

	codes := codesOf(m.List())
	if !contains(codes, pub.Code) {
		t.Errorf("public room %s missing from %v", pub.Code, codes)
	}
	if contains(codes, priv.Code) {
		t.Errorf("private room %s leaked into the listing %v", priv.Code, codes)
	}
}

func TestListHidesEmptyRooms(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	// Created but nobody has connected yet: there is no host to join.
	empty := m.Create(Options{Public: true, Game: "kittens"})
	if codes := codesOf(m.List()); contains(codes, empty.Code) {
		t.Errorf("an empty room was offered for joining: %v", codes)
	}

	joinN(t, empty, 1)
	if codes := codesOf(m.List()); !contains(codes, empty.Code) {
		t.Errorf("room with a host present is missing from %v", codes)
	}
}

func TestListHidesFullRooms(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	full := m.Create(Options{Public: true, Game: "kittens"})
	joinN(t, full, game.MaxPlayers)

	if codes := codesOf(m.List()); contains(codes, full.Code) {
		t.Errorf("a full room was offered for joining: %v", codes)
	}
}

func TestListHidesGamesInProgress(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	r := m.Create(Options{Public: true, Game: "kittens"})
	recs := joinN(t, r, 2)
	if codes := codesOf(m.List()); !contains(codes, r.Code) {
		t.Fatalf("room not listed before dealing: %v", codes)
	}

	// Deal, then it must drop out of the browser.
	r.Submit("p1", ClientMsg{Type: "start"})
	waitFor(t, func() bool {
		v, _ := recs[0].snapshot()
		return v != nil && v.Started
	}, "game to start")

	if codes := codesOf(m.List()); contains(codes, r.Code) {
		t.Errorf("a game in progress was offered for joining: %v", codes)
	}
}

func TestSummaryReportsWhoIsWaiting(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	r := m.Create(Options{Public: true, Game: "kittens"})
	joinN(t, r, 3)

	list := m.List()
	if len(list) != 1 {
		t.Fatalf("listed %d rooms, want 1", len(list))
	}
	s := list[0]
	if s.Players != 3 {
		t.Errorf("players = %d, want 3", s.Players)
	}
	if s.Capacity != game.MaxPlayers {
		t.Errorf("capacity = %d, want %d", s.Capacity, game.MaxPlayers)
	}
	if s.Host != "PA" {
		t.Errorf("host = %q, want the first player to join", s.Host)
	}
	if len(s.Names) != 3 {
		t.Errorf("names = %v, want all three", s.Names)
	}
	if !s.Joinable {
		t.Error("a three-player lobby should be joinable")
	}
}

// A room everybody has left must not linger in the browser as a dead row: the
// reaper only sweeps every minute, so the listing has to filter on its own.
func TestListHidesRoomsEverybodyLeft(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	r := m.Create(Options{Public: true, Game: "kittens"})
	rec := &recorder{}
	id, _, err := r.Join("", "Solo", rec)
	if err != nil {
		t.Fatal(err)
	}
	if codes := codesOf(m.List()); !contains(codes, r.Code) {
		t.Fatalf("room not listed while occupied: %v", codes)
	}

	r.Leave(id, rec)
	waitFor(t, func() bool {
		return !contains(codesOf(m.List()), r.Code)
	}, "abandoned room to drop out of the listing")
}

func TestGetStillFindsPrivateRooms(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Shutdown)

	priv := m.Create(Options{Public: false, Game: "kittens"})
	if _, err := m.Get(priv.Code); err != nil {
		t.Errorf("a private room must still be reachable by code: %v", err)
	}
}
