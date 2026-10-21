package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"boardgame/kittens/internal/analytics"
)

const (
	defaultPageSize = 500
	maxPageSize     = 5000

	// A player may suggest a few games in a row, but not fill the database.
	suggestBurst  = 5
	suggestWindow = 10 * time.Minute
)

// suggestLimiter counts suggestions per address in fixed windows. Memory is
// bounded by how many addresses suggest within one window; old windows are
// dropped as soon as anyone is checked after they end.
type suggestLimiter struct {
	mu      sync.Mutex
	start   time.Time
	counts  map[string]int
	burst   int
	window  time.Duration
	nowFunc func() time.Time
}

func newSuggestLimiter() *suggestLimiter {
	return &suggestLimiter{
		counts: map[string]int{}, burst: suggestBurst, window: suggestWindow, nowFunc: time.Now,
	}
}

func (l *suggestLimiter) allow(addr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.nowFunc()
	if now.Sub(l.start) >= l.window {
		l.start = now
		clear(l.counts)
	}
	if l.counts[addr] >= l.burst {
		return false
	}
	l.counts[addr]++
	return true
}

// suggestHandler serves POST /api/suggestions, the "Suggest a game" form on
// the menu. Anyone may post; reading them back needs the report token.
func suggestHandler(an Analytics) http.HandlerFunc {
	limiter := newSuggestLimiter()
	return func(w http.ResponseWriter, r *http.Request) {
		if an.Store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "suggestions are not switched on for this server",
			})
			return
		}
		ip := analytics.ClientIP(r, an.TrustProxy)
		if !limiter.allow(ip.String()) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "thanks! that's a lot of ideas, try again in a few minutes",
			})
			return
		}

		var in analytics.SuggestionInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read the form"})
			return
		}
		if err := in.Clean(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		id, err := an.Store.AddSuggestion(r.Context(), in, an.Geo.Country(ip))
		if err != nil {
			log.Printf("suggest: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save it, please try later"})
			return
		}
		log.Printf("suggestion %d: %q (%s)", id, in.GameName, in.Category)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	}
}

// listSuggestionsHandler serves GET /api/suggestions?after=0&limit=500 for the
// Odoo module.
func listSuggestionsHandler(an Analytics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardAdmin(w, r, an) {
			return
		}
		after, limit, err := pageParams(r, "after")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		list, err := an.Store.Suggestions(r.Context(), after, limit)
		if err != nil {
			log.Printf("suggestions: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "suggestions failed"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string][]analytics.Suggestion{"suggestions": list})
	}
}

// exportHandler serves GET /api/export?rooms_after=0&players_after=0&limit=500,
// the raw rows the Odoo module copies and groups itself.
func exportHandler(an Analytics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardAdmin(w, r, an) {
			return
		}
		roomsAfter, limit, err := pageParams(r, "rooms_after")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		playersAfter, _, err := pageParams(r, "players_after")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		ex, err := an.Store.Export(r.Context(), roomsAfter, playersAfter, limit)
		if err != nil {
			log.Printf("export: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "export failed"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, ex)
	}
}

// guardAdmin answers 403 or 503 and returns false when the request may not
// read analytics.
func guardAdmin(w http.ResponseWriter, r *http.Request, an Analytics) bool {
	if !authorized(r, an.Token) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return false
	}
	if an.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "analytics is not enabled"})
		return false
	}
	return true
}

// pageParams reads an "after this id" cursor and the shared limit.
func pageParams(r *http.Request, afterName string) (after int64, limit int, err error) {
	q := r.URL.Query()
	if s := q.Get(afterName); s != "" {
		after, err = strconv.ParseInt(s, 10, 64)
		if err != nil || after < 0 {
			return 0, 0, errors.New(afterName + " must be a whole number")
		}
	}
	limit = defaultPageSize
	if s := q.Get("limit"); s != "" {
		limit, err = strconv.Atoi(s)
		if err != nil || limit < 1 || limit > maxPageSize {
			return 0, 0, errors.New("limit must be 1–" + strconv.Itoa(maxPageSize))
		}
	}
	return after, limit, nil
}
