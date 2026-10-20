package analytics

import (
	"context"
	"fmt"
	"time"
)

// Report answers three questions for a period: which countries players come
// from, which game is played most, and how many players join each room.
// Field names are snake_case because the main reader is an Odoo module, and
// Python code reads them as dict keys.
type Report struct {
	From      time.Time     `json:"from"`
	To        time.Time     `json:"to"`
	Totals    Totals        `json:"totals"`
	Countries []CountryStat `json:"countries"`
	Games     []GameStat    `json:"games"`
	Rooms     []RoomStat    `json:"rooms"`
}

type Totals struct {
	Rooms   int `json:"rooms"`
	Players int `json:"players"`
}

type CountryStat struct {
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Players     int    `json:"players"`
	Rooms       int    `json:"rooms"`
}

// GameStat counts rooms that somebody actually joined as "played"; a room that
// was created and abandoned before anyone sat down is counted separately.
type GameStat struct {
	GameSlug     string  `json:"game_slug"`
	RoomsCreated int     `json:"rooms_created"`
	RoomsPlayed  int     `json:"rooms_played"`
	Players      int     `json:"players"`
	AvgPlayers   float64 `json:"avg_players_per_room"`
}

type RoomStat struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	GameSlug  string    `json:"game_slug"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"created_at"`
	Players   int       `json:"players"`
}

// Report builds the report for rooms created, and players who joined, in
// [from, to). Rooms are newest first and capped at roomLimit; the totals and
// per-game numbers always cover the whole period.
func (s *Store) Report(ctx context.Context, from, to time.Time, roomLimit int) (*Report, error) {
	rep := &Report{
		From: from, To: to,
		Countries: []CountryStat{}, Games: []GameStat{}, Rooms: []RoomStat{},
	}

	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM rooms        WHERE created_at >= $1 AND created_at < $2),
			(SELECT COUNT(*) FROM room_players WHERE joined_at  >= $1 AND joined_at  < $2)`,
		from, to).Scan(&rep.Totals.Rooms, &rep.Totals.Players)
	if err != nil {
		return nil, fmt.Errorf("analytics: totals: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT country_code, country_name, COUNT(*), COUNT(DISTINCT room_id)
		FROM room_players
		WHERE joined_at >= $1 AND joined_at < $2
		GROUP BY country_code, country_name
		ORDER BY COUNT(*) DESC, country_code`,
		from, to)
	if err != nil {
		return nil, fmt.Errorf("analytics: countries: %w", err)
	}
	for rows.Next() {
		var c CountryStat
		if err := rows.Scan(&c.CountryCode, &c.CountryName, &c.Players, &c.Rooms); err != nil {
			rows.Close()
			return nil, fmt.Errorf("analytics: countries: %w", err)
		}
		rep.Countries = append(rep.Countries, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics: countries: %w", err)
	}

	rows, err = s.pool.Query(ctx, `
		WITH per_room AS (
			SELECT r.id, r.game_slug, COUNT(p.id) AS players
			FROM rooms r LEFT JOIN room_players p ON p.room_id = r.id
			WHERE r.created_at >= $1 AND r.created_at < $2
			GROUP BY r.id, r.game_slug
		)
		SELECT game_slug,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE players > 0),
		       COALESCE(SUM(players), 0),
		       COALESCE(AVG(players) FILTER (WHERE players > 0), 0)::float8
		FROM per_room
		GROUP BY game_slug
		ORDER BY COUNT(*) FILTER (WHERE players > 0) DESC, SUM(players) DESC, game_slug`,
		from, to)
	if err != nil {
		return nil, fmt.Errorf("analytics: games: %w", err)
	}
	for rows.Next() {
		var g GameStat
		if err := rows.Scan(&g.GameSlug, &g.RoomsCreated, &g.RoomsPlayed, &g.Players, &g.AvgPlayers); err != nil {
			rows.Close()
			return nil, fmt.Errorf("analytics: games: %w", err)
		}
		rep.Games = append(rep.Games, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics: games: %w", err)
	}

	rows, err = s.pool.Query(ctx, `
		SELECT r.id, r.code, r.game_slug, r.is_public, r.created_at, COUNT(p.id)
		FROM rooms r LEFT JOIN room_players p ON p.room_id = r.id
		WHERE r.created_at >= $1 AND r.created_at < $2
		GROUP BY r.id
		ORDER BY r.created_at DESC
		LIMIT $3`,
		from, to, roomLimit)
	if err != nil {
		return nil, fmt.Errorf("analytics: rooms: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r RoomStat
		if err := rows.Scan(&r.ID, &r.Code, &r.GameSlug, &r.Public, &r.CreatedAt, &r.Players); err != nil {
			return nil, fmt.Errorf("analytics: rooms: %w", err)
		}
		rep.Rooms = append(rep.Rooms, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics: rooms: %w", err)
	}
	return rep, nil
}
