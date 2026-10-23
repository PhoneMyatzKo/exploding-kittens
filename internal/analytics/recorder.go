package analytics

import (
	"context"
	"log"
	"net"
	"time"
)

// Sink is where recorded events end up: the Postgres Store, or Odoo through
// its webhook. Calls come from the Recorder's one goroutine, in order.
type Sink interface {
	RoomCreated(ctx context.Context, ev RoomEvent) error
	PlayerJoined(ctx context.Context, ev JoinEvent) error
}

// RoomEvent is a room being opened.
type RoomEvent struct {
	Code     string    `json:"code"`
	GameSlug string    `json:"game_slug"`
	Public   bool      `json:"public"`
	At       time.Time `json:"at"`
}

// JoinEvent is a player taking a seat. Code names the room: the newest room
// with that code is the live one, because codes are only reused after the old
// room is gone.
type JoinEvent struct {
	Code        string    `json:"code"`
	PlayerID    string    `json:"player_id"`
	CountryCode string    `json:"country_code"`
	CountryName string    `json:"country_name"`
	At          time.Time `json:"at"`
}

// Recorder writes analytics events without making anyone wait for the sink.
// Callers hand it an event and return at once; a single goroutine does the
// writes in order, the same one-owner pattern rooms use for their state. That
// order matters: a join is written after the room it belongs to.
//
// A nil *Recorder is valid and records nothing, so the server runs the same
// with analytics switched off.
type Recorder struct {
	sink   Sink
	geo    *Locator
	events chan event
	done   chan struct{}
}

type event func(ctx context.Context, s Sink) error

// The queue is generous so that a slow sink only costs data once it has
// fallen thousands of events behind, never game latency.
const queueSize = 4096

// NewRecorder starts the writer. Close it to flush what is queued.
func NewRecorder(sink Sink, geo *Locator) *Recorder {
	r := &Recorder{
		sink:   sink,
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := ev(ctx, r.sink); err != nil {
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
	ev := RoomEvent{Code: code, GameSlug: gameSlug, Public: public, At: time.Now().UTC()}
	r.push(func(ctx context.Context, s Sink) error { return s.RoomCreated(ctx, ev) })
}

// PlayerJoined records a player taking a seat in the room with this code. A
// reconnect into the same seat is recorded once.
func (r *Recorder) PlayerJoined(code, playerID string, ip net.IP) {
	if r == nil {
		return
	}
	// Looked up here rather than in the writer so the address, not the
	// country, is all that has to be carried; it is a local file read either way.
	c := r.geo.Country(ip)
	ev := JoinEvent{
		Code: code, PlayerID: playerID,
		CountryCode: c.Code, CountryName: c.Name,
		At: time.Now().UTC(),
	}
	r.push(func(ctx context.Context, s Sink) error { return s.PlayerJoined(ctx, ev) })
}
