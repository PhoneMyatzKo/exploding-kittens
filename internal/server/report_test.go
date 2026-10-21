package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"boardgame/kittens/internal/analytics"
	"boardgame/kittens/internal/room"

	"github.com/jackc/pgx/v5"
)

func getReport(t *testing.T, base, token, query string) (*http.Response, *analytics.Report) {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/report"+query, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	var rep analytics.Report
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		t.Fatal(err)
	}
	return resp, &rep
}

func TestReportRefusesWithoutToken(t *testing.T) {
	for _, tc := range []struct {
		name, serverToken, sent string
	}{
		{"no token configured", "", "anything"},
		{"none sent", "s3cret", ""},
		{"wrong token", "s3cret", "guess"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := room.NewManager()
			srv := httptest.NewServer(New(mgr, Analytics{Token: tc.serverToken}))
			t.Cleanup(func() { srv.Close(); mgr.Shutdown() })

			resp, _ := getReport(t, srv.URL, tc.sent, "")
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content type = %q, want JSON", ct)
			}
		})
	}
}

// testStore opens analytics in a schema of its own, dropped afterwards. It
// needs a real Postgres: point ANALYTICS_TEST_DATABASE_URL at any database.
func testStore(t *testing.T, schema string) *analytics.Store {
	t.Helper()
	dbURL := os.Getenv("ANALYTICS_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ANALYTICS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE; CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		admin.Close(ctx)
	})

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := analytics.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestReportCountsRoomsPlayersAndCountries(t *testing.T) {
	store := testStore(t, "ek_report_test")
	rec := store.NewRecorder(nil)

	const token = "test-token"
	mgr := room.NewManager()
	srv := httptest.NewServer(New(mgr, Analytics{Store: store, Recorder: rec, Token: token}))
	t.Cleanup(func() { srv.Close(); mgr.Shutdown() })

	code := createRoom(t, srv.URL)
	a := dial(t, srv.URL, code, "Ann", "")
	a.await(t, 0)
	b := dial(t, srv.URL, code, "Bob", "")
	b.await(t, 0)
	settle(t, []*player{a, b})

	// A reconnect into the same seat is not a new player. Only a dealt game
	// keeps the seat; in the lobby a leaver is dropped and rejoins as new.
	act(t, []*player{a, b}, 0, room.ClientMsg{Type: "start"})
	a.mu.Lock()
	tok := a.token
	a.mu.Unlock()
	a.conn.Close()
	dial(t, srv.URL, code, "Ann", tok).await(t, 0)

	createRoom(t, srv.URL) // created, never joined

	// Flush the queue so everything above is in the database.
	rec.Close()

	_, rep := getReport(t, srv.URL, token, "?limit=10")
	if rep == nil {
		t.Fatal("report was refused")
	}
	if rep.Totals.Rooms != 2 || rep.Totals.Players != 2 {
		t.Fatalf("totals = %+v, want 2 rooms and 2 players", rep.Totals)
	}
	if len(rep.Countries) != 1 || rep.Countries[0].CountryCode != "LAN" || rep.Countries[0].Players != 2 {
		t.Fatalf("countries = %+v, want 2 players on LAN", rep.Countries)
	}
	if len(rep.Games) != 1 {
		t.Fatalf("games = %+v, want one game", rep.Games)
	}
	g := rep.Games[0]
	if g.RoomsCreated != 2 || g.RoomsPlayed != 1 || g.Players != 2 || g.AvgPlayers != 2 {
		t.Fatalf("game = %+v, want 2 created, 1 played, 2 players, avg 2", g)
	}
	if len(rep.Rooms) != 2 {
		t.Fatalf("rooms = %+v, want 2", rep.Rooms)
	}
	for _, r := range rep.Rooms {
		want := 0
		if r.Code == code {
			want = 2
		}
		if r.Players != want {
			t.Fatalf("room %s has %d players, want %d", r.Code, r.Players, want)
		}
	}

	if resp, _ := getReport(t, srv.URL, token, "?from=2026-13-01"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad date: status = %d, want 400", resp.StatusCode)
	}
}
