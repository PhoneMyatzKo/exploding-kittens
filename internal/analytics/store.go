// Package analytics records who plays what, and from where, so it can be
// reported on later — to an admin, or to the Odoo module that pulls the report.
//
// It is the one part of the server that touches a database. Live game state
// (hands, deck order, phase) stays in memory in package room as before; only
// facts about rooms and joins are written here, and only after the fact.
package analytics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the Postgres side of analytics. A pool rather than a single
// connection, because a pgx.Conn is not safe to share between goroutines and
// reports can be read while the recorder is writing.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to url (a postgres:// URL or key=value DSN) and makes sure the
// tables exist.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("analytics: parsing database url: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("analytics: connecting: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.pool.Close() }

// Room codes are recycled once a room is reaped, so a room is identified by its
// own id and the code is only a label. A player is recorded once per room: a
// reconnect presents the same player id and is not a second join.
const schema = `
CREATE TABLE IF NOT EXISTS rooms (
	id         BIGSERIAL PRIMARY KEY,
	code       TEXT        NOT NULL,
	game_slug  TEXT        NOT NULL,
	is_public  BOOLEAN     NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS rooms_code_idx       ON rooms (code, id DESC);
CREATE INDEX IF NOT EXISTS rooms_created_at_idx ON rooms (created_at);

CREATE TABLE IF NOT EXISTS room_players (
	id           BIGSERIAL PRIMARY KEY,
	room_id      BIGINT      NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
	player_id    TEXT        NOT NULL,
	country_code TEXT        NOT NULL,
	country_name TEXT        NOT NULL,
	joined_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE (room_id, player_id)
);
CREATE INDEX IF NOT EXISTS room_players_joined_at_idx ON room_players (joined_at);

CREATE TABLE IF NOT EXISTS game_suggestions (
	id           BIGSERIAL PRIMARY KEY,
	game_name    TEXT        NOT NULL,
	category     TEXT        NOT NULL CHECK (category IN ('card', 'family')),
	description  TEXT        NOT NULL,
	suggested_by TEXT        NOT NULL,
	country_code TEXT        NOT NULL,
	country_name TEXT        NOT NULL,
	created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("analytics: creating tables: %w", err)
	}
	return nil
}
