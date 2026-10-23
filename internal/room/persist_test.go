package room

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Surviving a restart, from the outside: a game in progress, the process gone,
// and the players' browsers reconnecting with the tokens they already hold.
//
// The reconnect path itself is not new — it is what already handles a phone going
// to sleep — so what these tests are really pinning is that a restored room is
// indistinguishable from the one that was saved. A restore that loses a hand, a
// seat token or the play-by-play would leave every player looking at a table they
// do not recognise.

func TestAGameInProgressSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// A table, dealt, with a few turns played into it.
	before := NewManager()
	r := before.Create(Options{Public: true, Game: "kittens"})
	code := r.Code

	recs := make([]*recorder, 3)
	tokens := make([]string, 3)
	ids := make([]string, 3)
	for i := range recs {
		recs[i] = &recorder{}
		id, tok, err := r.Join("", "P"+string(rune('A'+i)), recs[i])
		if err != nil {
			t.Fatal(err)
		}
		ids[i], tokens[i] = id, tok
	}
	r.Submit(ids[0], ClientMsg{Type: "start"})
	drain(t, r)

	wantView := recs[1].await(t, 0)
	if wantView == nil || !wantView.Started {
		t.Fatal("the game did not start")
	}
	wantHand := len(wantView.Me.Hand)
	wantDeck := wantView.DeckCount
	if wantHand == 0 || wantDeck == 0 {
		t.Fatalf("nothing was dealt: %d in hand, %d in the deck", wantHand, wantDeck)
	}

	n, err := before.SaveTo(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("saved %d rooms, want 1", n)
	}
	before.Shutdown()

	// A new process. Nothing in memory, one file on disk.
	after := NewManager()
	defer after.Shutdown()
	restored, err := after.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Fatalf("restored %d rooms, want 1", restored)
	}

	// The room is reachable by the code on everybody's invite link.
	back, err := after.Get(code)
	if err != nil {
		t.Fatalf("the room did not come back under %s: %v", code, err)
	}

	// And the second player reconnects with the token their browser is holding.
	rec := &recorder{}
	gotID, gotTok, err := back.Join(tokens[1], "PB", rec)
	if err != nil {
		t.Fatalf("reconnecting: %v", err)
	}
	if gotID != ids[1] {
		t.Errorf("came back as %s, was %s — that is somebody else's seat", gotID, ids[1])
	}
	if gotTok != tokens[1] {
		t.Error("the seat token changed across the restart, so the next reconnect would fail")
	}

	v := rec.await(t, 0)
	if !v.Started {
		t.Fatal("the restored room thinks it is still in its lobby")
	}
	if got := len(v.Me.Hand); got != wantHand {
		t.Errorf("came back holding %d cards, had %d", got, wantHand)
	}
	if v.DeckCount != wantDeck {
		t.Errorf("the deck has %d cards, had %d", v.DeckCount, wantDeck)
	}
	if v.CurrentID != wantView.CurrentID {
		t.Errorf("it is %s's turn, was %s's", v.CurrentID, wantView.CurrentID)
	}
	// The play-by-play too: coming back to an empty log would read as a new game.
	if len(v.Log) == 0 {
		t.Error("the play-by-play was lost")
	}
	// The other two are still in their seats, just not connected yet.
	if len(v.Seats) != 3 {
		t.Errorf("%d seats came back, want 3", len(v.Seats))
	}
}

// A file that was read successfully is kept. It is a checkpoint, not a farewell
// note: deleting it on load would leave a window right after startup in which a
// crash loses everything, and autosave overwrites it within the interval anyway.
func TestAGoodStateFileIsKeptAsACheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	mgr, _ := savedManager(t, path)
	mgr.Shutdown()

	after := NewManager()
	defer after.Shutdown()
	if _, err := after.LoadFrom(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the checkpoint was deleted on load: %v", err)
	}
}

// Autosave is what makes the file a checkpoint rather than something only a
// polite shutdown produces.
func TestAutosaveWritesWithoutBeingAskedToShutDown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	mgr := NewManager()
	defer mgr.Shutdown()

	r := mgr.Create(Options{Public: true, Game: "kittens"})
	for i := 0; i < 2; i++ {
		if _, _, err := r.Join("", "P"+string(rune('A'+i)), &recorder{}); err != nil {
			t.Fatal(err)
		}
	}
	drain(t, r)

	mgr.StartAutosave(path, 20*time.Millisecond)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return // written with nobody having shut anything down
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("autosave never wrote the file")
}

func TestAnEmptyRoomIsNotWorthSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	mgr := NewManager()
	defer mgr.Shutdown()
	mgr.Create(Options{Public: true, Game: "kittens"}) // nobody joins

	n, err := mgr.SaveTo(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("saved %d rooms with nobody in them", n)
	}
	// And no file left lying around to restore dead tables from.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a file was written for a room with nobody in it")
	}
}

// A file from yesterday is litter, not a game in progress — restoring it would
// put dead rooms in the public lobby browser.
func TestAStaleFileIsNotRestored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	mgr, _ := savedManager(t, path)
	mgr.Shutdown()

	// Age it past the cutoff.
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file stateFile
	if err := json.Unmarshal(blob, &file); err != nil {
		t.Fatal(err)
	}
	file.SavedAt = time.Now().Add(-maxRestoreAge - time.Minute)
	aged, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, aged, 0o600); err != nil {
		t.Fatal(err)
	}

	after := NewManager()
	defer after.Shutdown()
	n, err := after.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("restored %d stale rooms", n)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the stale file was left behind to be read again next time")
	}
}

func TestAFileFromAnotherVersionIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	blob, err := json.Marshal(stateFile{Version: stateFileVersion + 1, SavedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager()
	defer mgr.Shutdown()
	n, err := mgr.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("restored %d rooms from a file this build cannot read", n)
	}
}

// Corruption must not stop the server starting. Losing the games in the file is
// bad; losing the server as well is worse.
func TestARubbishFileDoesNotStopTheServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager()
	defer mgr.Shutdown()
	if _, err := mgr.LoadFrom(path); err == nil {
		t.Error("rubbish was accepted silently")
	} else if !strings.Contains(err.Error(), "state file") {
		t.Errorf("unhelpful error: %v", err)
	}
	// Whatever happened, the manager is usable.
	if r := mgr.Create(Options{Game: "kittens"}); r == nil {
		t.Error("the manager stopped working")
	}
}

// Turning it off means writing nothing and reading nothing, not writing to a file
// called "".
func TestNoPathMeansNoPersistence(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()
	if n, err := mgr.SaveTo(""); err != nil || n != 0 {
		t.Errorf("SaveTo(\"\") = %d, %v", n, err)
	}
	if n, err := mgr.LoadFrom(""); err != nil || n != 0 {
		t.Errorf("LoadFrom(\"\") = %d, %v", n, err)
	}
}

// Restoring a room whose game this build no longer has must skip that room and
// keep the others, rather than failing the whole load.
func TestAnUnknownGameSkipsOnlyItsOwnRoom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	good, _ := savedManager(t, path)
	good.Shutdown()

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file stateFile
	if err := json.Unmarshal(blob, &file); err != nil {
		t.Fatal(err)
	}
	// A second room, playing something that does not exist.
	phantom := file.Rooms[0]
	phantom.Code = "ZZZZ"
	phantom.Game = "buckaroo"
	file.Rooms = append(file.Rooms, phantom)
	out, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}

	after := NewManager()
	defer after.Shutdown()
	n, err := after.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("restored %d rooms, want only the one this build can deal", n)
	}
	if _, err := after.Get("ZZZZ"); err == nil {
		t.Error("a room playing an unknown game was restored anyway")
	}
}

// ─────────────────────────────────────────────────────────────────── helpers

// savedManager deals a table, plays a little, and writes it to path. Returns the
// manager still holding it, for the caller to shut down.
func savedManager(t *testing.T, path string) (*Manager, string) {
	t.Helper()
	mgr := NewManager()
	r := mgr.Create(Options{Public: true, Game: "kittens"})
	var first string
	for i := 0; i < 2; i++ {
		id, _, err := r.Join("", "P"+string(rune('A'+i)), &recorder{})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = id
		}
	}
	r.Submit(first, ClientMsg{Type: "start"})
	drain(t, r)

	if _, err := mgr.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	return mgr, r.Code
}

// drain waits for everything queued at the room to have run, the same trick the
// harness in room_test.go uses: a reply to a command queued now proves every
// earlier command has already been handled.
func drain(t *testing.T, r *Room) {
	t.Helper()
	reply := make(chan bool, 1)
	select {
	case r.cmds <- cmdIdleCheck{reply: reply}:
	case <-time.After(3 * time.Second):
		t.Fatal("the room stopped accepting commands")
	}
	select {
	case <-reply:
	case <-time.After(3 * time.Second):
		t.Fatal("the room stopped answering commands")
	}
}
