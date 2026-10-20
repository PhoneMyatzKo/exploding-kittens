package analytics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// A suggestion is a player asking for a game this server does not host yet.
// Only card games and family games are taken: the site is for playing with
// friends and family, so anything built around betting money is refused.
const (
	CategoryCard   = "card"
	CategoryFamily = "family"
)

const (
	maxGameName    = 80
	maxDescription = 1000
	maxSuggestedBy = 40
)

// gamblingWords are refused anywhere in a suggestion. Plain substrings on
// lower-cased text, so "Texas Hold'em" and "POKER night" are both caught.
var gamblingWords = []string{
	"poker", "hold'em", "holdem", "hold em", "blackjack", "baccarat",
	"roulette", "casino", "gambl", "betting", "slot machine", "pachinko",
}

// ErrGambling is returned for a suggestion of a gambling game.
var ErrGambling = errors.New("gambling games like poker are not accepted")

// SuggestionInput is what the player typed.
type SuggestionInput struct {
	GameName    string `json:"game_name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	SuggestedBy string `json:"suggested_by"`
}

// Clean trims every field and checks it, so the caller stores exactly what was
// validated. The error message is written for the player to read.
func (in *SuggestionInput) Clean() error {
	in.GameName = strings.TrimSpace(in.GameName)
	in.Category = strings.TrimSpace(in.Category)
	in.Description = strings.TrimSpace(in.Description)
	in.SuggestedBy = strings.TrimSpace(in.SuggestedBy)

	switch {
	case in.GameName == "":
		return errors.New("please name the game")
	case utf8.RuneCountInString(in.GameName) > maxGameName:
		return fmt.Errorf("the game name is too long (max %d characters)", maxGameName)
	case in.Category != CategoryCard && in.Category != CategoryFamily:
		return errors.New("pick card game or family game")
	case utf8.RuneCountInString(in.Description) > maxDescription:
		return fmt.Errorf("the description is too long (max %d characters)", maxDescription)
	case utf8.RuneCountInString(in.SuggestedBy) > maxSuggestedBy:
		return fmt.Errorf("your name is too long (max %d characters)", maxSuggestedBy)
	}

	text := strings.ToLower(in.GameName + " " + in.Description)
	for _, w := range gamblingWords {
		if strings.Contains(text, w) {
			return ErrGambling
		}
	}
	return nil
}

// Suggestion is a stored suggestion, as the Odoo module reads it.
type Suggestion struct {
	ID          int64     `json:"id"`
	GameName    string    `json:"game_name"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	SuggestedBy string    `json:"suggested_by"`
	CountryCode string    `json:"country_code"`
	CountryName string    `json:"country_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// AddSuggestion stores a suggestion that has already been through Clean.
// Unlike rooms and joins it is written straight away rather than through the
// Recorder: the player is waiting to hear whether it was saved.
func (s *Store) AddSuggestion(ctx context.Context, in SuggestionInput, c Country) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO game_suggestions (game_name, category, description, suggested_by, country_code, country_name)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		in.GameName, in.Category, in.Description, in.SuggestedBy, c.Code, c.Name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("analytics: saving suggestion: %w", err)
	}
	return id, nil
}

// Suggestions returns up to limit suggestions with an id above afterID, oldest
// first, so a reader can page through with the last id it has seen.
func (s *Store) Suggestions(ctx context.Context, afterID int64, limit int) ([]Suggestion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, game_name, category, description, suggested_by, country_code, country_name, created_at
		FROM game_suggestions
		WHERE id > $1
		ORDER BY id
		LIMIT $2`,
		afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("analytics: suggestions: %w", err)
	}
	defer rows.Close()
	out := []Suggestion{}
	for rows.Next() {
		var g Suggestion
		if err := rows.Scan(&g.ID, &g.GameName, &g.Category, &g.Description, &g.SuggestedBy,
			&g.CountryCode, &g.CountryName, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("analytics: suggestions: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics: suggestions: %w", err)
	}
	return out, nil
}
