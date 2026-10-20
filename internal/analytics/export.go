package analytics

import (
	"context"
	"fmt"
	"time"
)

// Export is the raw rows behind the report, for a reader that keeps its own
// copy (the Odoo module) and does its own grouping. Report is the summary;
// this is the ledger it is summed from.
type Export struct {
	Rooms   []ExportRoom   `json:"rooms"`
	Players []ExportPlayer `json:"players"`
}

type ExportRoom struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	GameSlug  string    `json:"game_slug"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportPlayer is one seat taken. The player id is left out: it means nothing
// outside the room, and the reader has no use for it.
type ExportPlayer struct {
	ID          int64     `json:"id"`
	RoomID      int64     `json:"room_id"`
	CountryCode string    `json:"country_code"`
	CountryName string    `json:"country_name"`
	JoinedAt    time.Time `json:"joined_at"`
}

// Export returns up to limit rooms with an id above roomsAfter, and up to limit
// players with an id above playersAfter, both oldest first.
//
// Rows are never updated once written, so paging by id is all a reader needs
// to stay in step: it remembers the last id of each kind it stored and asks
// for what came after. The players stop at the first one whose room is not in
// this page or an earlier one, so a reader never meets a player whose room it
// has not been given yet, and never steps over one it will need later.
func (s *Store) Export(ctx context.Context, roomsAfter, playersAfter int64, limit int) (*Export, error) {
	ex := &Export{Rooms: []ExportRoom{}, Players: []ExportPlayer{}}

	rows, err := s.pool.Query(ctx, `
		SELECT id, code, game_slug, is_public, created_at
		FROM rooms WHERE id > $1 ORDER BY id LIMIT $2`,
		roomsAfter, limit)
	if err != nil {
		return nil, fmt.Errorf("analytics: export rooms: %w", err)
	}
	for rows.Next() {
		var r ExportRoom
		if err := rows.Scan(&r.ID, &r.Code, &r.GameSlug, &r.Public, &r.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("analytics: export rooms: %w", err)
		}
		ex.Rooms = append(ex.Rooms, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics: export rooms: %w", err)
	}

	// The newest room the reader will hold after this page.
	maxRoom := roomsAfter
	if n := len(ex.Rooms); n > 0 {
		maxRoom = ex.Rooms[n-1].ID
	}

	rows, err = s.pool.Query(ctx, `
		SELECT id, room_id, country_code, country_name, joined_at
		FROM room_players WHERE id > $1 ORDER BY id LIMIT $2`,
		playersAfter, limit)
	if err != nil {
		return nil, fmt.Errorf("analytics: export players: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p ExportPlayer
		if err := rows.Scan(&p.ID, &p.RoomID, &p.CountryCode, &p.CountryName, &p.JoinedAt); err != nil {
			return nil, fmt.Errorf("analytics: export players: %w", err)
		}
		if p.RoomID > maxRoom {
			break
		}
		ex.Players = append(ex.Players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics: export players: %w", err)
	}
	return ex, nil
}
