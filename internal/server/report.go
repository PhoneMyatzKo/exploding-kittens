package server

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"boardgame/kittens/internal/analytics"
)

// Analytics is the optional reporting side of the server. The zero value turns
// it off: nothing is recorded and the report endpoint answers 403.
type Analytics struct {
	Store    *analytics.Store
	Recorder *analytics.Recorder
	// Geo places a suggestion's sender in a country. Nil is fine: LAN or Unknown.
	Geo *analytics.Locator
	// Token guards the report. The Odoo module sends it as
	// "Authorization: Bearer <token>". Empty means nobody may read the report.
	Token string
	// TrustProxy believes X-Forwarded-For for the player's address. Only set it
	// when a reverse proxy in front of this server writes that header.
	TrustProxy bool
}

const (
	dateLayout       = "2006-01-02"
	defaultDays      = 30
	defaultRoomLimit = 200
	maxRoomLimit     = 5000
)

// reportHandler serves GET /api/report?from=2026-10-01&to=2026-10-31&limit=200.
// Both dates are inclusive days in UTC; with neither, it covers the last 30 days.
func reportHandler(an Analytics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, an.Token) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if an.Store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "analytics is not enabled"})
			return
		}

		q := r.URL.Query()
		today := time.Now().UTC().Truncate(24 * time.Hour)
		to := today
		if s := q.Get("to"); s != "" {
			t, err := time.Parse(dateLayout, s)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to must be YYYY-MM-DD"})
				return
			}
			to = t
		}
		from := to.AddDate(0, 0, -(defaultDays - 1))
		if s := q.Get("from"); s != "" {
			t, err := time.Parse(dateLayout, s)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be YYYY-MM-DD"})
				return
			}
			from = t
		}
		if from.After(to) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from is after to"})
			return
		}
		limit := defaultRoomLimit
		if s := q.Get("limit"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 || n > maxRoomLimit {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "limit must be 0–" + strconv.Itoa(maxRoomLimit),
				})
				return
			}
			limit = n
		}

		// "to" names a whole day, so the query runs up to the start of the next.
		rep, err := an.Store.Report(r.Context(), from, to.AddDate(0, 0, 1), limit)
		if err != nil {
			log.Printf("report: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "report failed"})
			return
		}
		rep.To = to
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, rep)
	}
}

func authorized(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	// Constant time, so the token can't be guessed a byte at a time from how
	// quickly wrong ones are refused.
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
