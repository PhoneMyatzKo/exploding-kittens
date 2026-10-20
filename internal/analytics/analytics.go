package analytics

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func OpenDB(dbname string) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, fmt.Errorf("parsing DATABASE_URL: %w", err)
	}
	cfg.Database = dbname

	conn, err := pgx.ConnectConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)

	}
	return conn, nil
}

func CreateEditDB(conn *pgx.Conn) error {
	conn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS rooms (
		id SERIAL PRIMARY KEY,
		game_code TEXT NOT NULL,
		is_public BOOL NOT NULL,
		game_slug TEXT NOT NULL,
		members_count INT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
CREATE TABLE IF NOT EXISTS room_players (
    ip_address TEXT NOT NULL,
    geo_location TEXT NOT NULL,
    active_at TIMESTAMPTZ DEFAULT NOW()
    closed_at TIMESTAMPTZ DEFAULT NOW()
)
`)
	return nil
}
