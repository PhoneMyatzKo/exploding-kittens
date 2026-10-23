package room

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games"
	"boardgame/kittens/internal/prng"
)

// Saving rooms across a restart.
//
// Rooms live in memory, which until now meant a deploy or a crash ended every
// game in progress. For a ten-minute hand of Exploding Kittens that is a shrug;
// for a Monopoly session it is the difference between a game people finish and
// one they abandon. So the manager writes every room down on the way out and
// reads them back on the way in.
//
// What is *not* saved: sockets. Everybody comes back disconnected, holding the
// seat token their browser already has, and reclaims their seat through the
// ordinary reconnect path — the same one that already handles a phone going to
// sleep. Nothing new had to be built for that, which is the main reason this is
// as small as it is.

// stateFileVersion guards against reading a file written by a different shape of
// this struct. A mismatch is not an error worth shouting about: the games in it
// are lost either way, and refusing to start would be worse than starting empty.
const stateFileVersion = 1

// maxRestoreAge is how stale a state file may be and still be worth reading. A
// table nobody has come back to in six hours is not a game in progress, it is
// litter — and restoring it would put dead rooms in the public lobby browser.
const maxRestoreAge = 6 * time.Hour

// SavedMember is one seat, without its connection.
type SavedMember struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	// Token is what the browser presents to reclaim this seat. It is the only
	// thing in this file that is remotely sensitive, which is why the file is
	// written 0600.
	Token string `json:"token"`
	Host  bool   `json:"host,omitempty"`
}

// SavedRoom is one table, written down.
type SavedRoom struct {
	Code   string `json:"code"`
	Public bool   `json:"public"`
	Game   string `json:"game"`
	// Seq is the counter player ids are minted from, so a rejoining player is not
	// handed an id somebody else already holds.
	Seq     int           `json:"seq"`
	LogSeq  int           `json:"logSeq"`
	Members []SavedMember `json:"members"`
	Log     []core.Entry  `json:"log,omitempty"`
	// State is the game's own snapshot, opaque here. Empty for a room still in
	// its lobby.
	State []byte `json:"state,omitempty"`
}

type stateFile struct {
	Version int         `json:"version"`
	SavedAt time.Time   `json:"savedAt"`
	Rooms   []SavedRoom `json:"rooms"`
}

// cmdSave asks a room to describe itself for the state file. It goes through the
// command channel like everything else: the room's own goroutine owns the members
// and the game, and this is the one place that would be tempting to read them
// from outside.
type cmdSave struct{ reply chan SavedRoom }

func (cmdSave) isCommand() {}

func (r *Room) handleSave(c cmdSave) {
	out := SavedRoom{
		Code: r.Code, Public: r.Public, Game: r.Game,
		Seq: r.seq, LogSeq: r.logSeq,
		Log: append([]core.Entry(nil), r.logbuf...),
	}
	for _, m := range r.members {
		out.Members = append(out.Members, SavedMember{
			ID: m.ID, Name: m.Name, Avatar: m.Avatar, Token: m.Token, Host: m.Host,
		})
	}
	// A game that cannot describe itself is logged and saved as a lobby rather
	// than taking the whole file down with it.
	if blob, err := r.game.Snapshot(); err != nil {
		log.Printf("room %s: snapshot failed, saving the lobby only: %v", r.Code, err)
	} else {
		out.State = blob
	}
	c.reply <- out
}

// Save asks the room to describe itself, giving up rather than blocking if the
// room is wedged — one stuck table must not stop the others being written.
func (r *Room) Save(timeout time.Duration) (SavedRoom, bool) {
	reply := make(chan SavedRoom, 1)
	select {
	case r.cmds <- cmdSave{reply: reply}:
	case <-r.done:
		return SavedRoom{}, false
	case <-time.After(timeout):
		return SavedRoom{}, false
	}
	select {
	case s := <-reply:
		return s, true
	case <-time.After(timeout):
		return SavedRoom{}, false
	}
}

// restoreRoom rebuilds a room from a saved one. It mirrors newRoom, and the two
// have to stay in step — anything newRoom initialises has to be initialised here
// as well, or a restored table behaves differently from a fresh one.
func restoreRoom(s SavedRoom) (*Room, error) {
	rng := prng.NewSeeded()
	g, err := games.New(s.Game, rng)
	if err != nil {
		return nil, fmt.Errorf("room %s: %w", s.Code, err)
	}
	if err := g.Restore(s.State); err != nil {
		return nil, fmt.Errorf("room %s: restoring %s: %w", s.Code, s.Game, err)
	}

	r := &Room{
		Code:   s.Code,
		Public: s.Public,
		Game:   s.Game,
		game:   g,
		seq:    s.Seq,
		logSeq: s.LogSeq,
		logbuf: s.Log,
		cmds:   make(chan command, 32),
		done:   make(chan struct{}),
		rng:    rng,
		// Restored rooms start their reaping clock now: whoever was playing gets
		// the usual grace period to come back, and a table nobody returns to is
		// cleaned up on the same schedule as any other empty one.
		lastEmpty: time.Now(),
	}
	for _, m := range s.Members {
		r.members = append(r.members, &member{
			ID: m.ID, Name: m.Name, Avatar: m.Avatar, Token: m.Token, Host: m.Host,
			// No socket, and so not connected. The browser reconnects with its
			// token and lands back in this seat.
			Connected: false,
		})
	}

	r.windowTimer = time.NewTimer(time.Hour)
	r.windowTimer.Stop()
	r.idleTimer = time.NewTimer(time.Hour)
	r.idleTimer.Stop()
	go r.run()
	return r, nil
}

// StartAutosave writes the state file on a timer for as long as the manager
// lives.
//
// Saving only on the way out is not enough, and finding that out is what put this
// here: a graceful shutdown is the one exit that can be relied on least. A crash
// takes no signal at all, a power cut takes no signal at all, and on Windows a
// process killed by anything other than Ctrl+C in its own console never sees
// os.Interrupt either. A game two hours in should not depend on the server being
// asked politely.
//
// So the file is a checkpoint rather than a farewell note, and the most anybody
// loses to a hard kill is the last interval.
func (m *Manager) StartAutosave(path string, every time.Duration) {
	if path == "" || every <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-t.C:
				if _, err := m.SaveTo(path); err != nil {
					log.Printf("autosave to %s failed: %v", path, err)
				}
			}
		}
	}()
}

// SaveTo writes every room with somebody in it to path, atomically.
//
// Atomically because the alternative is a half-written file, and a half-written
// file is indistinguishable from a corrupt one on the way back in. Written to a
// temporary name in the same directory and renamed over the top, which is atomic
// on every filesystem this runs on.
func (m *Manager) SaveTo(path string) (int, error) {
	if path == "" {
		return 0, nil
	}

	m.mu.RLock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.RUnlock()

	file := stateFile{Version: stateFileVersion, SavedAt: time.Now()}
	for _, r := range rooms {
		s, ok := r.Save(2 * time.Second)
		if !ok {
			log.Printf("room %s did not answer in time; not saved", r.Code)
			continue
		}
		// A room with nobody in it has nobody to come back, and an empty room
		// restored into the lobby browser is just a dead listing.
		if len(s.Members) == 0 {
			continue
		}
		file.Rooms = append(file.Rooms, s)
	}

	if len(file.Rooms) == 0 {
		// Nothing worth keeping. Remove any older file rather than leaving one
		// that would resurrect finished games on the next start.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		return 0, nil
	}

	blob, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return 0, err
	}
	tmp := path + ".tmp"
	// 0600: the file contains seat tokens, which are bearer credentials for a
	// seat at a table.
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return len(file.Rooms), nil
}

// LoadFrom reads rooms back. A missing file is not an error — it is the normal
// case on a first run.
//
// A file that *was* read is left where it is, on purpose: it is a checkpoint, and
// deleting it would leave a window right after startup in which a crash loses
// everything. Autosave overwrites it within the interval, and SaveTo removes it
// once there is nothing left worth keeping. A file that could *not* be read is
// deleted, so a bad one is not retried on every start forever.
func (m *Manager) LoadFrom(path string) (int, error) {
	if path == "" {
		return 0, nil
	}
	blob, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	var file stateFile
	if err := json.Unmarshal(blob, &file); err != nil {
		// Unreadable: drop it rather than failing to start on every boot.
		_ = os.Remove(path)
		return 0, fmt.Errorf("%s is not a state file: %w", filepath.Base(path), err)
	}
	if file.Version != stateFileVersion {
		log.Printf("%s was written by version %d, this build reads %d — ignoring it",
			filepath.Base(path), file.Version, stateFileVersion)
		return 0, os.Remove(path)
	}
	if age := time.Since(file.SavedAt); age > maxRestoreAge {
		log.Printf("%s is %s old — too stale to restore", filepath.Base(path), age.Round(time.Minute))
		return 0, os.Remove(path)
	}

	restored := 0
	for _, s := range file.Rooms {
		r, err := restoreRoom(s)
		if err != nil {
			// One unreadable room does not cost the others theirs.
			log.Printf("skipping a saved room: %v", err)
			continue
		}
		m.mu.Lock()
		if _, taken := m.rooms[r.Code]; taken {
			m.mu.Unlock()
			r.close()
			continue
		}
		m.rooms[r.Code] = r
		m.mu.Unlock()
		restored++
	}
	return restored, nil
}
