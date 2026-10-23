package games_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games"
	"boardgame/kittens/internal/prng"
)

// Saving and resuming, for every game in the catalogue at once.
//
// Written against core.Game rather than against each engine on purpose: a game
// added later is covered the day it becomes playable, without anybody
// remembering to write this again. That is the same bargain
// TestEveryPlayableGameHasRules makes.
//
// What it is really checking is the thing that is easy to get wrong and silent
// when you do: a field that does not survive the round trip. Exploding Kittens
// hides a card's *type* from the client payload, so a JSON snapshot would restore
// a deck of nameless cards and the engine would deal them happily — a bug that
// shows up several turns later as the wrong card being drawn.

// playable is the games this test drives, with a legal seat count for each.
func playable(t *testing.T) []games.Info {
	t.Helper()
	var out []games.Info
	for _, g := range games.All() {
		if g.Playable {
			out = append(out, g)
		}
	}
	if len(out) < 3 {
		t.Fatalf("only %d playable games — has the catalogue shrunk?", len(out))
	}
	return out
}

func seatsFor(n int) []core.Seat {
	var out []core.Seat
	for i := 0; i < n; i++ {
		id := string(rune('a' + i))
		out = append(out, core.Seat{ID: "p" + id, Name: "P" + id})
	}
	return out
}

// A game part-way through, snapshotted, restored into a fresh instance, and then
// asked to keep playing. Both halves have to agree on what every player can see
// and on every move that follows — the second is what catches a lost RNG, since
// two generators in different places diverge on the next shuffle rather than
// immediately.
func TestEveryGameSurvivesASnapshot(t *testing.T) {
	for _, info := range playable(t) {
		t.Run(info.Slug, func(t *testing.T) {
			seats := seatsFor(info.Min)

			live, err := games.New(info.Slug, prng.New(4242))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := live.Deal(seats); err != nil {
				t.Fatal(err)
			}

			// Far enough in that there is real state to lose: cards played, money
			// moved, somebody's turn part-resolved.
			playOut(t, live, 40)

			blob, err := live.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if len(blob) == 0 {
				t.Fatal("a dealt game snapshotted to nothing")
			}

			// A fresh instance, as a restarted server would build it: no deal, then
			// the bytes.
			restored, err := games.New(info.Slug, prng.New(999999))
			if err != nil {
				t.Fatal(err)
			}
			if err := restored.Restore(blob); err != nil {
				t.Fatalf("restoring: %v", err)
			}

			if !restored.Started() {
				t.Fatal("a restored game does not think it has started")
			}

			// The sharpest check available, and the reason it is here rather than
			// only comparing views: a snapshot has to round-trip to the same bytes.
			// The views agreeing proves the *visible* state came back, but the
			// generator is invisible — a restore that quietly started a fresh one
			// would pass every other assertion in this test until the next shuffle,
			// which the driver below may never reach.
			again, err := restored.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(blob, again) {
				t.Fatalf("a snapshot did not round-trip: %d bytes in, %d out%s",
					len(blob), len(again), firstDifference(blob, again))
			}
			if live.Over() != restored.Over() {
				t.Errorf("over = %v restored, %v live", restored.Over(), live.Over())
			}
			for _, s := range seats {
				if a, b := viewOf(t, live, s.ID), viewOf(t, restored, s.ID); a != b {
					t.Fatalf("%s sees a different table after the restore\n live: %s\n back: %s", s.ID, a, b)
				}
			}

			// And it plays on identically. A restore that dropped the generator
			// looks perfect until the next shuffle or the next throw.
			for i := 0; i < 60; i++ {
				a := step(live)
				b := step(restored)
				if (a == nil) != (b == nil) {
					t.Fatalf("move %d: one of them had a move and the other did not", i)
				}
				if a == nil {
					break
				}
				for _, s := range seats {
					if x, y := viewOf(t, live, s.ID), viewOf(t, restored, s.ID); x != y {
						t.Fatalf("move %d: %s diverged\n live: %s\n back: %s", i, s.ID, x, y)
					}
				}
			}
		})
	}
}

// A game nobody has dealt saves as nothing and comes back as a lobby, which is
// what a room snapshotted before anybody pressed start has to do.
func TestAnUndealtGameSnapshotsToNothing(t *testing.T) {
	for _, info := range playable(t) {
		t.Run(info.Slug, func(t *testing.T) {
			g, err := games.New(info.Slug, prng.New(1))
			if err != nil {
				t.Fatal(err)
			}
			blob, err := g.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if len(blob) != 0 {
				t.Errorf("an undealt game produced %d bytes", len(blob))
			}

			// And the empty snapshot is accepted rather than treated as corruption.
			if err := g.Restore(nil); err != nil {
				t.Errorf("restoring nothing: %v", err)
			}
			if g.Started() {
				t.Error("restoring nothing started a game")
			}
		})
	}
}

// Rubbish in the state file must be an error, not a panic and not a half-built
// table. The file is written by us, but it can be truncated by a machine losing
// power in the middle of the write.
func TestATruncatedSnapshotIsRefused(t *testing.T) {
	for _, info := range playable(t) {
		t.Run(info.Slug, func(t *testing.T) {
			g, err := games.New(info.Slug, prng.New(7))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := g.Deal(seatsFor(info.Min)); err != nil {
				t.Fatal(err)
			}
			playOut(t, g, 10)
			blob, err := g.Snapshot()
			if err != nil {
				t.Fatal(err)
			}

			for _, cut := range []int{1, len(blob) / 3, len(blob) / 2, len(blob) - 1} {
				fresh, err := games.New(info.Slug, prng.New(7))
				if err != nil {
					t.Fatal(err)
				}
				if err := fresh.Restore(blob[:cut]); err == nil {
					t.Errorf("%d of %d bytes was accepted as a whole game", cut, len(blob))
				}
			}
			// Not gob at all.
			fresh, _ := games.New(info.Slug, prng.New(7))
			if err := fresh.Restore([]byte("not a snapshot")); err == nil {
				t.Error("arbitrary bytes were accepted as a game")
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────── helpers

// step plays one legal move for whoever the table is waiting on, using the
// game's own idea of the least destructive thing to do. Returns nil when there is
// nothing left to play.
func step(g core.Game) []core.Entry {
	if g.Over() {
		return nil
	}
	who := g.BlockedOn()
	if who == "" {
		// Only Exploding Kittens has a window nobody is obliged to answer; closing
		// it is the move.
		if _, _, open := g.Window(); open {
			return g.WindowExpired()
		}
		return nil
	}
	entries, err := g.AutoMove(who)
	if err != nil {
		return nil
	}
	return entries
}

func playOut(t *testing.T, g core.Game, moves int) {
	t.Helper()
	for i := 0; i < moves; i++ {
		if step(g) == nil {
			return
		}
	}
}

// viewOf is a player's whole payload as a string, so two of them can be compared
// outright. Through JSON because that is what actually reaches a browser: if two
// tables serialise the same, no client can tell them apart.
func viewOf(t *testing.T, g core.Game, playerID string) string {
	t.Helper()
	shell := core.Shell{
		Code: "TEST", Public: true, Game: "x",
		Members: []core.Membership{
			{ID: "pa", Name: "Pa", Connected: true, Host: true},
			{ID: "pb", Name: "Pb", Connected: true},
			{ID: "pc", Name: "Pc", Connected: true},
			{ID: "pd", Name: "Pd", Connected: true},
		},
		ViewerID: playerID,
	}
	blob, err := json.Marshal(g.View(shell))
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}

// firstDifference locates where two snapshots part company, because "1841 bytes
// in, 1841 out" on its own says nothing about what moved.
func firstDifference(a, b []byte) string {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return fmt.Sprintf(" — first differing byte at %d of %d", i, len(a))
		}
	}
	if len(a) != len(b) {
		return fmt.Sprintf(" — identical for %d bytes, then one ends", min(len(a), len(b)))
	}
	return ""
}
