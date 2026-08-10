// Package server wires the HTTP surface together: static client, room API and
// the WebSocket endpoint. It is separated from main so tests can stand the whole
// stack up in-process.
package server

import (
	"encoding/json"
	"log"
	"net/http"

	"boardgame/kittens/internal/room"
	"boardgame/kittens/internal/ws"
	"boardgame/kittens/static"
	"boardgame/kittens/web"
)

// New returns the complete handler for the game server.
func New(mgr *room.Manager) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/", noCache(http.FileServer(http.FS(web.Assets))))

	// Card art is immutable once built into the binary, so unlike the client it
	// is worth caching hard — it is by far the heaviest thing we serve.
	mux.Handle("GET /cards/", http.StripPrefix("/cards/",
		immutable(http.FileServer(http.FS(static.CardArt())))))

	mux.HandleFunc("POST /api/rooms", func(w http.ResponseWriter, r *http.Request) {
		rm := mgr.Create()
		log.Printf("room %s created", rm.Code)
		writeJSON(w, http.StatusOK, map[string]string{"code": rm.Code})
	})

	mux.HandleFunc("GET /api/rooms/{code}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := mgr.Get(r.PathValue("code")); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]int{"rooms": mgr.Count()})
	})

	mux.Handle("GET /ws", ws.Handler(mgr))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// noCache keeps browsers from serving a stale client after a rebuild. The whole
// app is a few kilobytes, so there is nothing to gain from caching it.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

// immutable marks a response as safe to keep forever, for assets that only ever
// change alongside the binary serving them.
func immutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}
