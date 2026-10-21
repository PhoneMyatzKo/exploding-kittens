package analytics

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Recorder writes analytics events without making anyone wait for Postgres.
// Callers hand it an event and return at once; a single goroutine does the
// writes in order, the same one-owner pattern rooms use for their state. That
// order matters: a join is written after the room it belongs to.
//
// A nil *Recorder is valid and records nothing, so the server runs the same
// with analytics switched off.
type Recorder struct {
	pool   *pgxpool.Pool
	geo    *Locator
	events chan event
	done   chan struct{}
}

type event func(ctx context.Context, pool *pgxpool.Pool) error

// The queue is generous so that a slow database only costs data once it has
// fallen thousands of events behind, never game latency.
const queueSize = 4096

// NewRecorder starts the writer. Close it to flush what is queued.
func (s *Store) NewRecorder(geo *Locator) *Recorder {
	r := &Recorder{
		pool:   s.pool,
		geo:    geo,
		events: make(chan event, queueSize),
		done:   make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *Recorder) run() {
	defer close(r.done)
	for ev := range r.events {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := ev(ctx, r.pool); err != nil {
			log.Printf("analytics: %v", err)
		}
		cancel()
	}
}

// Close stops taking events and waits for the queued ones to be written.
func (r *Recorder) Close() {
	if r == nil {
		return
	}
	close(r.events)
	<-r.done
}

func (r *Recorder) push(ev event) {
	if r == nil {
		return
	}
	select {
	case r.events <- ev:
	default:
		log.Printf("analytics: queue full, event dropped")
	}
}

// RoomCreated records a new table.
func (r *Recorder) RoomCreated(code, gameSlug string, public bool) {
	r.push(func(ctx context.Context, pool *pgxpool.Pool) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO rooms (code, game_slug, is_public) VALUES ($1, $2, $3)`,
			code, gameSlug, public)
		return err
	})
}

// PlayerJoined records a player taking a seat in the room with this code. A
// reconnect into the same seat is ignored by the unique constraint.
func (r *Recorder) PlayerJoined(code, playerID string, ip net.IP) {
	if r == nil {
		return
	}
	// Looked up here rather than in the writer so the address, not the
	// country, is all that has to be carried; it is a local file read either way.
	c := r.geo.Country(ip)
	r.push(func(ctx context.Context, pool *pgxpool.Pool) error {
		// The newest room with this code is the live one: codes are only
		// reused after the old room is gone.
		_, err := pool.Exec(ctx, `
			INSERT INTO room_players (room_id, player_id, country_code, country_name)
			SELECT id, $2, $3, $4 FROM rooms WHERE code = $1 ORDER BY id DESC LIMIT 1
			ON CONFLICT (room_id, player_id) DO NOTHING`,
			code, playerID, c.Code, c.Name)
		return err
	})
}
