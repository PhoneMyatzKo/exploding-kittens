# CLAUDE.md

Self-hosted multiplayer card games (Exploding Kittens, Imploding Kittens, UNO)
in Go. One binary serves the rules, the rooms and the browser client. Players
join from their phones with a room code. Go module: `boardgame/kittens`.

## Commands

```sh
go run ./cmd/server                 # http://localhost:8080 (-addr :80 to change)
go build -o kittens ./cmd/server
go test ./...
go test -race ./...
ANALYTICS_TEST_DATABASE_URL=postgres://odoo20:123@localhost:5432/postgres \
  go test ./internal/server        # also runs the real-Postgres analytics tests
cd web/testing && npm test         # Playwright; needs a server on :8099
```

The web client has no build step. `web/` and `static/` are embedded into the
binary, so rebuild after changing them.

## Layout

```
cmd/server         main: flags, env, signals; openAnalytics() reads the env vars below
internal/server    HTTP routes (server.go), report/export/suggestion handlers
internal/ws        WebSocket: one goroutine writes (writePump), the handler reads
internal/room      one goroutine per room owns all its state; others send commands on a channel
internal/view      per-player redaction of game state
internal/core      the game interface; adapters kittens.go / uno.go
internal/games     catalogue.go = the menu (slugs, playable flags); rules under <slug>/game
internal/analytics Postgres (pgxpool): rooms, room_players, game_suggestions; GeoIP lookup
web/               shell (app.js, core/*.js) + games/<slug>/ loaded by mountGame()
static/            embedded media: /cards, /media, /avatars, /audio, /video
```

`readme.md` explains features in depth. `TODO.md` has the roadmap ("Direction:
multi-game core" and "Direction: the web client, per game") and the rules
below.

## Rules to keep

- Live game state (hands, deck order, phase) stays in memory. Postgres only
  stores analytics facts. If `DATABASE_URL` is missing or the DB is down, games
  must still work.
- Never change a room's state from outside its goroutine. Send a command on
  `r.cmds`.
- Analytics writes go through the async `Recorder` (buffered channel, drops
  events when full), so the database never slows down a game. The exception is
  `AddSuggestion`, which is synchronous because the player waits for the answer.
- The game list lives only in `internal/games/catalogue.go`. The client reads
  it from `/api/games`.
- A shell file in `web/` must not reach into a game's markup, and a game must
  not reach into the shell's.

## Analytics, suggestions and Odoo

Env vars: `DATABASE_URL`, `GEOIP_DB` (.mmdb country file), `REPORT_TOKEN`.
Flag: `-trust-proxy`.

| Route | Access | Use |
|---|---|---|
| `GET /api/report` | Bearer token | summary JSON (countries, games, rooms) |
| `GET /api/export?rooms_after&players_after&limit` | Bearer token | raw rows for Odoo, paged by id |
| `GET /api/suggestions?after&limit` | Bearer token | game ideas for Odoo |
| `POST /api/suggestions` | public, 5 per IP per 10 min | the menu's "Suggest a game" form (`web/core/suggest.js`) |

Suggestions accept only `card` or `family` games. Poker and other gambling
words are refused in `analytics.SuggestionInput.Clean`.

The **Odoo 20 module `kittens_report`** lives outside this repo, in
`~/Projects/odoo20_test/kittens_report` (created with
`odev scaffold kittens_report -p odoo20_test -v 20`). It has the models
`kittens.room`, `kittens.player`, `kittens.suggestion` and `kittens.sync`, an
hourly cron, and settings under *Settings → Board Games* (config parameters
`kittens_report.server_url` / `kittens_report.token`).

Odoo 20 differs from older versions. Use:
- `security/ir.access.csv` (`id,name,model_id,group_id/id,operation,domain`),
  not `ir.model.access.csv`;
- `ir.config_parameter.get_str` / `set_str`, not `get_param` / `set_param`;
- `models.Constraint(...)`, not `_sql_constraints`;
- `<list>` views; no `target="inline"` on actions.

Run the module's tests on a throwaway database:

```sh
cd ~/odoo/20o && ~/.python-env/20env/bin/python odoo-bin \
  -c ~/Projects/odoo20_test/config/odoo20.conf \
  --addons-path=$HOME/odoo/20o/addons,$HOME/odoo/20o/odoo/addons,$HOME/Projects/odoo20_test \
  -d kittens_report_test -i kittens_report --test-tags /kittens_report --stop-after-init --http-port 8121
```

Or run it with odev: `odev run -p odoo20_test -v 20 -d <db> -u kittens_report`.

## Working with the owner

They are learning Go. English is not their first language, so write plain
English with short sentences. Ask each time whether they want to write the code
themselves with guidance, or have it written for them to review.
