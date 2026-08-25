// Package server wires the HTTP surface together: static client, room API and
// the WebSocket endpoint. It is separated from main so tests can stand the whole
// stack up in-process.
package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"boardgame/kittens/internal/games"
	"boardgame/kittens/internal/games/kittens/game"
	"boardgame/kittens/internal/room"
	"boardgame/kittens/internal/ws"
	"boardgame/kittens/static"
	"boardgame/kittens/web"
)

// New returns the complete handler for the game server.
func New(mgr *room.Manager) http.Handler {
	// Card faces are drawn per deal out of the scans that shipped in this binary,
	// so the engine has to be told what is on offer before the first deck is
	// built. Here rather than in main so that anything standing the stack up —
	// the end-to-end test included — deals the same decks the binary does.
	game.SetCardArt(static.CardArtVariants())

	mux := http.NewServeMux()

	mux.Handle("/", noCache(http.FileServer(http.FS(web.Assets))))

	// Media is immutable once built into the binary, so unlike the client it is
	// worth caching hard — it is by far the heaviest thing we serve.
	mux.Handle("GET /cards/", http.StripPrefix("/cards/",
		immutable(http.FileServer(http.FS(static.CardArt())))))
	// Per-game artwork: /media/uno/uno-background.jpg and whatever a later game
	// brings with it.
	mux.Handle("GET /media/", http.StripPrefix("/media/",
		immutable(http.FileServer(http.FS(static.GameMedia())))))
	mux.Handle("GET /avatars/", http.StripPrefix("/avatars/",
		immutable(http.FileServer(http.FS(static.Avatars())))))
	mux.Handle("GET /audio/", http.StripPrefix("/audio/",
		immutable(http.FileServer(http.FS(static.Audio())))))
	mux.Handle("GET /video/", http.StripPrefix("/video/",
		immutable(http.FileServer(http.FS(static.Video())))))

	// The portrait catalogue. The lobby builds its picker from this, so the
	// embedded files stay the only place the set is written down.
	mux.HandleFunc("GET /api/avatars", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string][]string{"avatars": static.AvatarIDs()})
	})

	// The kinds a Three of a Kind may demand. Served rather than duplicated in
	// JavaScript so card names are written down in exactly one place. It depends
	// on the deck, so the game is a query parameter — the expansion adds five more
	// kinds somebody could be holding.
	mux.HandleFunc("GET /api/cards", func(w http.ResponseWriter, r *http.Request) {
		slug := r.URL.Query().Get("game")
		if slug == "" {
			slug = games.Kittens
		}
		variant, ok := games.VariantFor(slug)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no such game"})
			return
		}
		type kind struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
			// Face is one representative scan, for the how-to-play sheet. A card
			// with several printed faces gets its first; a card with none gets an
			// empty string and the sheet falls back to its emoji.
			Face string `json:"face,omitempty"`
		}

		faces := static.CardArtVariants()
		build := func(types []game.CardType) []kind {
			out := make([]kind, 0, len(types))
			for _, t := range types {
				k := kind{Name: t.String(), Slug: t.Slug()}
				if v := faces[t.Slug()]; len(v) > 0 {
					k.Face = v[0]
				}
				out = append(out, k)
			}
			return out
		}

		writeJSON(w, http.StatusOK, map[string][]kind{
			"demandable": build(game.DemandableTypes(variant)),
			// Every kind in the deck, so the rules sheet can show a real card
			// instead of an emoji — and only the cards this deck contains.
			"kinds": build(game.AllTypes(variant)),
		})
	})

	// The menu is built from this, so which games exist is written down once, in
	// the catalogue, rather than again in JavaScript.
	mux.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string][]games.Info{"games": games.All()})
	})

	mux.HandleFunc("POST /api/rooms", func(w http.ResponseWriter, r *http.Request) {
		// Public unless the body says otherwise, so an empty POST still behaves
		// like the button most people press.
		body := struct {
			Public *bool  `json:"public"`
			Game   string `json:"game"`
		}{}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body)
		public := body.Public == nil || *body.Public

		// Kittens unless asked otherwise, so a client that predates the menu still
		// gets a game rather than an error.
		slug := body.Game
		if slug == "" {
			slug = games.Kittens
		}
		// An announced-but-unbuilt game is refused here rather than in the room:
		// nothing downstream knows how to deal a table it cannot name.
		if !games.Playable(slug) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "that game is not playable on this server yet",
			})
			return
		}
		// The slug is all the room is given: games.New turns it into rules, and the
		// slug-to-deck mapping stays in the catalogue where the menu reads it.
		rm := mgr.Create(room.Options{Public: public, Game: slug})
		visibility := "private"
		if public {
			visibility = "public"
		}
		log.Printf("room %s created (%s, %s)", rm.Code, slug, visibility)
		writeJSON(w, http.StatusOK, map[string]any{
			"code": rm.Code, "public": public, "game": slug,
		})
	})

	// The lobby browser. Only rooms somebody could actually join are listed.
	mux.HandleFunc("GET /api/rooms", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string][]room.Summary{"rooms": mgr.List()})
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
