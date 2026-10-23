# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A self-hosted board-game server: several games (Exploding Kittens, its Imploding
Kittens expansion, UNO, Monopoly Myanmar) hosted from one binary, played from a
phone or laptop by room code. Go on the server, plain ES modules in the browser,
no database and **no frontend build step**. Module path is `boardgame/kittens` —
a leftover from when this hosted one game; do not rename it while the migration
notes in `TODO.md` are still open.

## Commands

```sh
go run ./cmd/server                      # http://localhost:8080
go build -o kittens.exe ./cmd/server     # single-file distribution

go test ./...                            # everything
go test ./internal/games/uno/game/       # one package
go test ./internal/games/monopoly/game/ -run TestPassingGoPays
go vet ./...

# Also runs the analytics tests that need a real Postgres.
ANALYTICS_TEST_DATABASE_URL=postgres://odoo20:123@localhost:5432/postgres \
  go test ./internal/server
```

`-party-key` (default `partytest123`) reveals the catalogue's **preview** games:
ones that work but are not finished enough to show an ordinary visitor. Monopoly
is one. The menu's quiet `PTR` button is where the password is typed; the server
both filters `/api/games` and refuses `POST /api/rooms` for a preview game
without it, because hiding a tile in the browser alone is a curtain with no wall
behind it. `""` hides them from everybody — a gate that opens when it is
misconfigured is not a gate. It is a velvet rope, not authentication.

`go run ./cmd/scaffold <slug> "<Name>" <emoji> "<tagline>" <min> <max>` writes a
new game's Go adapter, rules stub, catalogue entry and client stub. It leaves
`internal/games/registry.go` alone on purpose — see "Adding a game" below.

### -race needs a specific toolchain

The `gcc` on PATH is MSYS2's and cgo fails with it (`cgo.exe: exit status 2`).
Point at the WinLibs one instead:

```powershell
$mingw = "C:\Users\ACER\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin"
$env:Path = "$mingw;$env:Path"
go test -race ./...
```

### gofmt reports false positives

The checkout is CRLF and `gofmt -l` flags nearly every file for it. Normalise
before believing either answer:

```sh
mkdir -p /tmp/fm
for f in $(git ls-files '*.go'); do tr -d '\r' < "$f" > "/tmp/fm/$(echo $f | tr '/' '_')"; done
gofmt -l /tmp/fm/
```

`gofmt -w` on a real hit is safe — it preserves the file's line endings.

### Browser tests

`web/testing/` drives real Chromium against a running server and is **not part of
`go test`**. It exists because every UI regression this project has had was
invisible to Go: a bar covering the hand, a modal running off screen, a button
that renders enabled and does nothing.

```powershell
go build -o kittens.exe ./cmd/server
Start-Process -FilePath .\kittens.exe -ArgumentList "-addr",":8099" -WindowStyle Hidden
cd web/testing; npm install; node run.js     # or: node monopoly.js
Get-Process kittens | Stop-Process -Force
```

**Rebuild before running it.** `web/` and `static/` are `go:embed`ed, so a running
server serves the assets from the last build — editing a `.js` file changes
nothing until you rebuild. This wastes more time than any other trap here.

`web/testing/README.md` documents the traps specific to these scripts. Read it
before adding one; several of them have bitten more than once.

## Architecture

### The seam: `internal/core`

`core.Game` is the whole vocabulary between a room and the rules it hosts.
`internal/room` owns sockets, seats, reconnects and timers and **must not import
any game's rules** — if you find yourself adding such an import, the logic belongs
behind `core.Game` instead. Each game supplies an adapter (`internal/games/<slug>/<slug>.go`)
that translates; the rules underneath it never learn what a WebSocket is.

Two shared structs carry everything on the wire:

- `core.ClientMsg` — one struct for every game, all scalars. A game reads the
  fields its own moves use. Adding a move that needs new data means adding a
  field here, with a comment saying whose it is.
- `core.Entry` — one line of the play-by-play, and the only shape in which a game
  reports what happened. `OnlyFor` restricts an entry to a single player, which
  is how private information reaches exactly one client.

`core.Game.Window()` is a timed window any player may act into — built for the
Nope window, and the right mechanism for anything else of that shape (Monopoly's
auctions, when they land).

### Rooms are single-writer

One goroutine per room owns all its mutable state. WebSocket readers never mutate
anything; they post commands over a channel. That is what makes the server
race-free without a mutex around game logic, and it is why a `core.Game`
implementation needs no locking and may hand out pointers into its own state.

### Randomness is state, and state is saved

`internal/prng` is the only source of randomness a game may use. It is
counter-based: the entire state is a seed and a count, so it marshals — which is
what lets a game be written to disk and resumed. `math/rand` cannot do that and
must not come back. Room codes and seat tokens are the exception and stay on
`crypto/rand`.

Each engine's `State` carries its own `*prng.Source` as an exported field tagged
`json:"-"`: exported so gob can save it, hidden so it never reaches a client.

Snapshots are **gob, not JSON**, and that is load-bearing: a game's json tags
belong to the *client* payload, and Exploding Kittens hides a card's `Type` from
the wire — a JSON snapshot would restore a deck of typeless cards and the engine
would deal them happily. `internal/games/snapshot_test.go` drives every playable
game through save/restore and asserts the snapshot round-trips to identical
bytes, which is the only assertion that catches a lost generator.

The room writes a checkpoint every 30s and on a clean exit (`-state`,
`-state-every`). On the way back in, players reconnect through the ordinary
token path — the one that already handles a phone going to sleep — so nothing
special happens on the client. Private log lines ("You drew Nope") are not in the
shared log and never survive a reconnect, restart or not.

### Rules are pure reducers

Each game's rules are `Apply(state, action) ([]Event, error)`, mutating state in
place, with no I/O. This is where the majority of bugs will be and where they are
cheapest to catch — prefer a table-driven test here over a browser check.

### Redaction happens in exactly one place per game

A game's view package (`internal/view` for kittens, `internal/games/<slug>/view.go`
for the others) is the only thing that builds a client payload from a state. The
server must never serialise raw state — it contains the deck order and every hand.

`internal/view/redact_test.go` walks many random games and asserts that the only
card IDs a player can see are their own. When a rule has to *show* somebody a card
without letting them name it later, use the `FaceCard` pattern: send name/slug/art
with **no ID**, and take positions back rather than IDs. That is what let Alter the
Future and Monopoly's face-up deck square exist without loosening the scan.

### The catalogue and the registry must agree

`internal/games/catalogue.go` is data — what the menu shows. `internal/games/registry.go`
is the only place a slug becomes code. `TestEveryPlayableGameHasRules` enforces
that a slug is `Playable: true` **iff** `registry.New` has a case for it, so an
unbuilt game is announced with a "soon" badge and refused at `POST /api/rooms`.

### The client: shell and game

`web/app.js` is the shell — socket, menu, lobby, palette, reconnect, mounting. A
game lives in `web/games/<slug>/` and is loaded on demand. **The contract between
them is documented at `mountGame()` in `app.js`; read it before touching either
side.** The test for whether the split is intact: `app.js` must not name an id
from a game's template.

`web/core/` holds what both halves share (`dom.js`, `store.js`, `sound.js`,
`modal.js`, `feed.js`, `cinema.js`, `toast.js`). `store.js` is the one place
localStorage keys live; keys keep their historical `ek:` prefix on purpose.

Styling is token-driven: every component rule reads a semantic token
(`--ink`, `--muted`, `--accent`, `--surface`, `--bg`) and never names a hue, so a
game is a palette block keyed off `<html data-game>`. When repointing tokens,
repoint at the *role* — an invalid `var()` computes as `unset` and fails silently,
which is how three dead tokens survived unnoticed in eleven rules.

### Static assets

`static/static.go` names embed patterns one directory at a time rather than
sweeping, because most of the card scans are for cards no engine implements. The
patterns must stay in step with `cardArtDirs`; each must match at least one file
or the build fails, which turns a renamed directory into a compile error instead
of a deck quietly wearing emoji.

## Adding a game

1. `go run ./cmd/scaffold …` — writes the module, the client stub and the
   catalogue entry, marked not-playable.
2. Implement the rules under `internal/games/<slug>/game/`.
3. Wire `registry.go` and flip `Playable: true` **together** (the test above).
4. Add a script to `web/testing/` and to `run.js`.

## Conventions worth keeping

- **Tests that cannot run are worse than no tests.** A check gated on a random
  deal that almost never comes up teaches you to skim past the skip line; a fuzz
  test that logs "no winner" every seed silently disables everything after it.
  If a check cannot fire, delete it or make it fire. Both mistakes are recorded
  in this repo with the comment explaining why.
- **Comments say why, not what.** Load-bearing decisions and the traps behind them
  are written down at the point of the decision; several files' header comments
  are the real documentation for their subsystem.
- `TODO.md` is the backlog and the design record. Items are struck through with a
  paragraph on what was actually built and what was learned. Its "Direction"
  sections carry the standing plan; the architecture notes in `readme.md` predate
  `internal/core` and are out of date where the two disagree.
- The Burmese in the rules sheets and the Monopoly board was written without a
  native reader. Flag it rather than treating it as settled.
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
