# TODO

## Done

- ~~"Nope" action windows timer need longer (+20s)~~ — now 20s. The server sends
  the window length with every open window, so the countdown bar can't drift out
  of step with it again.
- ~~"Mute" Sound button in lobby and "Back to menu" button in lobby.~~ — Back to
  menu leaves the room properly: the seat is released, and the room code is
  cleared from the URL so a refresh doesn't walk straight back in.
- ~~Add Three of a Kind Rule.~~ — three matching cats name a card and take it.
  The demand is public (everyone hears what was asked for and whether it landed);
  if the target hasn't got one, the three cats are spent for nothing. Nopeable
  like any other play.
- ~~Card Selection constraints.~~ — one non-cat card at a time; cats only stack
  with *matching* cats, up to three. Incompatible cards are dimmed, and tapping
  one explains why instead of doing nothing.
- ~~Log chat can hide by drop down arrow.~~ — the whole "Play by play" header is
  the toggle. Collapsed state is remembered across reloads. Sideways on desktop
  it shrinks to a narrow spine rather than leaving a 300px empty bar.

- ~~Create Room with Private or Public setting~~ — a Public/Private toggle on the
  home screen, public by default and remembered between sessions. The lobby shows
  which one you got, under the room code.
- ~~Users can see public Room~~ — **🌍 Public lobby** lists joinable public rooms
  with host, seat count and who's waiting; tap Join and you're in without ever
  seeing a code. Only rooms you can actually enter are listed, so a room
  disappears once it's dealt or full.

- ~~Main menu listing every game, chosen before anything else~~ — the front page
  is now a picker. Exploding Kittens is playable; UNO and UNO No Mercy are shown
  locked with a "soon" badge rather than hidden, because an empty-looking hub
  reads as broken. The catalogue is served from `/api/games`, so the list is
  written down once in `internal/games` rather than again in JavaScript. Rooms
  carry a `game` slug from creation through to the client's view, unbuilt games
  are refused at `POST /api/rooms`, and an invite link still goes straight to the
  table — the sender already chose the game.

- ~~Move each game's files into a folder of its own~~ — `internal/games/kittens/`
  and `static/src/games/kittens/`. What stayed at the root is what a second game
  would share: the transport, the room loop, the portraits, the audio. The card
  art is served through a rooted FS, so the `/cards/…` URLs did not change.

- ~~Browser tests back in the repo~~ — `web/testing/`, six scripts driving real
  Chromium. They had been living in a scratch directory and were lost between
  sessions, which meant every UI change was unverifiable. Not part of `go test`:
  they need a running server.

## Ideas not started

- Five Different Cards combo (take any card from the discard pile) — the only
  base-game combo still missing.
- Imploding Kittens expansion: Reverse, Draw From The Bottom, Alter the Future,
  Feral Cat, and 6-player support.
- Spectators for eliminated players (right now you watch the table you're on).
- Persistence, so a server restart doesn't wipe a game in progress. — see
  "Serializable state" below; the seed change is the part that unblocks this.

## Direction: multi-game core

The plan is to turn this into a hub that hosts several board games (UNO, UNO No
Mercy, later Monopoly and others) on one shared core, with accounts and a paid tier.

**Nothing gets deleted up front.** A second stack goes in beside the first, new
rooms are pointed at it once it passes the same tests, and the old one is
removed only after it is carrying no traffic. Rooms are in-memory and reaped
15 minutes after they empty, so this costs nothing: there is no data to migrate
and no dual-write. Old rooms simply drain.

### What is already right

`game.Apply(state, action) ([]Event, error)` is a pure reducer with no I/O, and
`room.applyAction` is its only caller. `internal/ws` and the single-goroutine
room loop don't know what a card is. `internal/view` already does per-viewer
redaction. Those are the pieces a multi-game core needs, and they exist.

### What is kitten-shaped in `internal/room`

- `toAction` hardcodes the six message types → action payload becomes opaque
  bytes the game decodes
- `rearmNope` treats the Nope window as a room concept → `Deadline(state)`
- `waitingOn` switches on kitten phases → `BlockedOn(state)`
- `actForAbsentPlayer` picks kitten moves → `AutoMove(state, playerID)`
- `game.MaxPlayers` and `static.HasAvatar` reach into game-specific packages

### What is kitten-shaped in `internal/view`

Not just the room. `view.View` imports `game` directly and carries
`KittensLeft`, `MustPlace`, `MustGive`, `CanNope` and a `Pending` with a Nope
count. **Only the envelope is generic** — `Code`, `Seats`, `Log`, `Seq`,
`Connected`, `Host`, `Game`, `Public`. So `internal/view` splits into a shared
shell plus a per-game payload; it does not get shared whole. Reading it as
"never fork view" would end with UNO's fields bolted onto this struct.

### Split

Share `internal/ws`, `static`, and the *shell* of `internal/view` — never fork
them. `internal/room` (748 lines) is the only real duplicate. The v2 kittens
"game" is a thin adapter over the existing engine, not a rewrite of it.

    internal/room/                v1, frozen once core exists
    internal/core/                v2 generic room loop, registry, deadlines
    internal/games/kittens/       adapter wrapping game.Apply + view.For
    internal/games/kittens/game/  the rules, untouched by any of this
    internal/hub/                 accounts, entitlements, browse — HTTP + DB only

Serve `/ws` + `/` from v1 and `/v2/ws` + `/v2/` from v2 out of the same binary.

### Two changes that are cheap now and painful later

- **Serializable state.** `State.rng *rand.Rand` is the only field in `State`
  that won't marshal. But this is bigger than swapping a field: the rng is also
  consumed by `fullDeck` picking card faces, three shuffles in setup, the
  Shuffle card, the cat-pair steal and the starting player — and `rng.Shuffle`
  eats a variable number of values, so a seed plus a *draw counter* cannot
  reproduce it. It needs a counter-based generator (`hash(seed, n)`) with our
  own Fisher-Yates pulling from it; then seed + n really is the whole state.
  Still cheap, but it is "replace the randomness layer", not "replace a field".

  What it buys: snapshot-on-shutdown, and exact reproduction from a bug report.
  It does **not** buy running more than one server process — that needs room
  ownership and routing, which is a separate and larger problem. Today a deploy
  kills every live game.
- **Identity before the room.** Right now the room mints the token and the seat
  is anonymous. With login, the user exists first and must be verified before
  the socket upgrade. Keep seat IDs room-scoped (`p1`, `p2`) and map
  `userID → seatID` in the room layer only — the engine must never learn what a
  user account is, or the reducer stops being pure and testable on its own.

### Rules to hold to

- Once `internal/core` exists, new features land in v2 only. Building twice
  teaches nothing about whether v2 is any good.
- Live game state stays in memory. The database gets accounts, entitlements and
  finished-match rows — never hands, deck order or phase.
- Entitlement checks happen at room create/join and are passed in as resolved
  capabilities. If `game.Apply` ever asks whether someone paid, back it out.
- v1 is deleted when UNO ships on v2. If that hasn't happened, the migration is
  stalled and should be finished before anything else starts.

### Order

Done already, ahead of the rest:

- ~~`Game string` on `view.View`~~, plus the slug through `POST /api/rooms`,
  `room.Options`, `Summary` and the menu. Unbuilt games are refused at the door.
- ~~Per-game folders~~ (`internal/games/kittens/`, `static/src/games/kittens/`).
- ~~Browser tests in the repo~~ (`web/testing/`), which the rest of this depends
  on more than it looks — see the warning below.

Then:

1. **Conformance suite first.** Pull the room scenarios (`room_test.go`,
   `e2e_test.go` — reconnect, token reclaim, host transfer, idle takeover)
   behind an interface and make v1 pass them unchanged. This was third; it
   belongs first. It is pure test refactoring, touches no production behaviour,
   and every step after it is safer once it exists. If v2 passes the same cases
   the cutover is a non-event.
2. Replace `State.rng` with a counter-based generator (see above — this is the
   randomness layer, not one field).
3. Auth, accounts, `userID → seatID` mapping in the room layer.
4. `internal/core` + kittens adapter, green against the same suite.
5. React client. Shell (ws, reconnect, lobby, log, audio, mute — roughly
   `app.js` 140–500, none of it mentions cards) split from per-game renderers
   from the first commit. Vite → `dist/` → `//go:embed all:dist` keeps the
   single-executable property; flip `immutable` onto hashed assets and leave
   `noCache` on `index.html` only.
6. Point new rooms at v2, let v1 drain, delete it, then rename the module off
   `boardgame/kittens`.
7. Hub and payments.
8. UNO, then UNO No Mercy.

### Watch out

- **The client rewrite has the thinnest safety net here.** `web/app.js` is 1600+
  lines and no Go test can see any of it. Every UI regression this project has
  had was caught by looking at a real browser: a Nope bar covering the hand,
  toasts burying a modal title, a new screen missing the shared background rule.
  `web/testing/` is that net, and it has to grow to cover a flow *before* that
  flow is rewritten in React — not after. Rewriting the client while also moving
  rooms onto a new core is the one place the "never break v1 and v2 at the same
  time" rule is likely to go.
- **VFX/SFX in React.** The animation cues are driven by diffing log sequence
  numbers against the previous snapshot (`app.seenSeq`, `app.logSeq`), including
  the resync cases. That logic is correct and already debugged — keep it as a
  plain module and let a thin hook drive it, rather than rewriting it in React
  idioms. StrictMode double-invokes effects in dev, so the explosion will fire
  twice until that's handled.
- **Monopoly does not fit this model** and shouldn't be forced into it.
  Simultaneous auctions, N-way trade negotiation and multi-hour sessions all
  break the one-actor-per-turn phase assumption. It shares the transport and the
  hub, not the turn engine. Decide that after UNO, not before.
- Don't rename the Go module until v1 is gone — it rewrites every import in both
  stacks at once, which is the one big-bang change this plan exists to avoid.
- The engine work and the hub/payments work are independent. Doing both halves
  at the same time is how this stalls; pick which one is being tested first.

### Prompt to resume this

    Read TODO.md, section "Direction: multi-game core". I'm turning this
    Exploding Kittens server into a multi-game hub. The plan is additive: build
    internal/core alongside internal/room, never fork internal/game, point new
    rooms at v2 once it passes the same conformance tests, delete v1 only after
    it's carrying no traffic.

    Work on step N. Before changing anything, confirm which steps are already
    done by reading the code rather than trusting this file. Keep the existing
    tests green at every commit — no step should require v1 and v2 to be broken
    at the same time.

## Direction: the web client, per game

UNO needs its own palette (`#f8d92b` yellow, `#ec2229` red, `#000000` black), its
own music, SFX and VFX, and its own table layout. `web/app.js` is 1771 lines,
`web/style.css` 742, `web/index.html` 285, and all three are single files written
for one game. This section is how a second game gets added to them without
tripling their length.

It is scoped to the browser only. It is independent of the core work above and
can be done first, because it touches no Go beyond one `go:embed` line.

### What is already right

More than it looks:

- `internal/games/catalogue.go` already carries `uno` and `uno-no-mercy` with
  `Playable: false`, and `/api/games` already serves them. The tiles exist.
- `applyGameChrome(slug)` (`web/app.js:599`) already swaps the logo, tagline and
  document title per game. It is the hook the rest of this hangs off.
- `static/src/games/kittens/` already namespaces media per game, and the card art
  is served through a rooted FS, so `static/src/games/uno/` needs no URL changes.
- **The CSS is genuinely token-driven.** All 13 hex literals in `style.css` live
  in the header comment (lines 5–6), `:root` (21–33) and the `.panel` paper
  family (59–61). Not one component rule names a hue — they all read `--ink`,
  `--muted`, `--line`, `--accent`, `--bg`. Retheming is therefore a token swap,
  not a sweep.

### What is kitten-shaped in the client

- `index.html` — the whole `#table` section and every word of `#rules-panel`
- `app.js` — `GLYPHS`, `artURL`, `TRACKS`, the SFX table, `renderHand`,
  `renderSeats`, `renderDiscard`, the deck-risk readout, the Nope bar, the
  cinema/VFX cues
- `style.css` — `.deck`, `.discard`, `.hand`, `.card`, `.seats`, `.nope-bar`,
  `.flash`, and the palette values themselves

Everything else — menu, home, public-lobby browser, lobby, avatar picker, modal,
toasts, socket, reconnect, token reclaim, storage, the mute engine — is already
game-agnostic and must not be forked. That is roughly `app.js` 1–660.

### Options considered

**Fork the files** (`uno.html`, `uno.js`, `uno.css`, served as a second page).
Fastest to ship and it cannot break Exploding Kittens, but it duplicates ~660
lines of shell, so every lobby and reconnect bug then gets fixed twice, and going
menu → UNO becomes a page load that drops the socket. Rejected: the duplicated
part is exactly the part that took the longest to get right.

**Branch inline** (`if (app.game === "uno") …` through the existing functions).
No restructuring, but `app.js` goes past 3500 lines of two interleaved games, and
UNO No Mercy makes it three. Rejected — this is the option that actually hurts,
and it hurts later rather than now, which is why it is tempting.

**Per-game ES modules, no build step** — chosen. `index.html` already loads
`app.js` as `type="module"`, so `await import()` works natively today.

**Vite + React/Svelte.** This is step 5 of the multi-game plan above and is not
in conflict with the modules approach: the module boundary drawn here is the same
seam React would need (`shell` vs `per-game renderer`), so doing it in vanilla
first makes the React port a port rather than a redesign. Do not do both at once.

### Layout

    web/
      index.html          shell markup only — menu, home, browser, lobby, modal, toasts
      app.js              shell — socket, routing, storage, sound engine, mountGame()
      core/               sound.js, ui.js, theme.js — shared helpers
      games/
        kittens/          index.js, table.html, rules.html, theme.css
        uno/              index.js, table.html, rules.html, theme.css

Each game module default-exports one small contract:

    export default {
      tracks: { intro: "/audio/uno/intro.mp3", … },
      sfx:    { … },
      async mount(root) { … },   // inject table.html, wire listeners
      render(view)      { … },   // called on every server state
      unmount()         { … },   // stop audio, clear timers, drop listeners
    };

and the shell loads it inside the existing `chooseGame()`:

    const mod = (await import(`./games/${slug}/index.js`)).default;

`unmount()` is not optional. Leaving a room and picking another game must stop
the previous game's audio and kill its timers, or the second game plays over the
first — the same class of bug the mute toggle exists to prevent.

### Order

1. ~~**Theme tokens first.**~~ Done. `--crimson`/`--gold`/`--paper`/`--grey` are
   now `--brand`/`--accent`/`--surface`/`--muted-ink`, declared as channel
   triples (`--brand-rgb: 130 10 18`) because most are also needed at partial
   alpha; the solid forms are derived with `rgb(var(…))`, so each colour has one
   source. Structural tokens (`--radius`, `--card-w`, `--card-h`) stayed in a
   shared `:root`; the palettes are `:root` (Exploding Kittens, also the
   fallback) and `:root[data-game="uno"]`. `setTheme()` in `app.js` sets the
   attribute and rewrites `<meta name="theme-color">` by reading back the
   computed `--bg`, so the value is still written down only in the stylesheet.

   Verified with a 130-element probe page rendered in headless Chrome against the
   old and new stylesheets, comparing every colour-bearing computed property:
   byte-identical for Exploding Kittens. The same probe with `data-game="uno"`
   repalettes 180 properties with no unresolved `var()`.

   Four things the plan had wrong, worth knowing before step 2:

   - **The attribute goes on `<html>`, not `<body>`.** The `html, body` rule
     reads `--bg`, and the root element is what paints the overscroll gutter. On
     `<body>` the palette would apply to the app but not to the rubber-band strip
     behind it.
   - **The shadow token had to split in two.** `--shadow-rgb` lifts an element
     off the ground and becomes the yellow under UNO; `--scrim-rgb` darkens the
     card scans so white labels survive on top of them and stays black in every
     palette. One token could not do both — a yellow modal backdrop is not a
     backdrop. Verified: under UNO the card-art and deck-strip gradients are
     still `rgba(0,0,0,…)` while the card-lift and toast shadows are yellow.
   - **The rename exposed four rules that named `--brand` for a role that is not
     the brand.** `.btn.primary` and `.conn-warning` wanted "ink on the accent"
     (`--accent-ink`); `.logo`, `.room-code` and the range thumb wanted the
     surface family's `--ink`. They were indistinguishable while crimson happened
     to be all three. Under UNO the old spelling gave red-on-yellow at 3.1:1.
   - **`.error` needed a token of its own.** It was crimson-on-paper at 10.5:1;
     `#ec2229` on white is 4.4:1 and fails AA. Hence `--danger`, which UNO sets
     to `#c1121a` (6.1:1). This is the general shape of the problem: kittens'
     brand is dark enough to double as ink, and UNO's is not.

   Also folded in, since the hub is now what the landing screen is: the document
   title and favicon in `index.html` are the hub's (Board Games, 🎲), and
   `applyGameChrome()` overwrites both halves per game while `showMenu()` puts
   them back.
2. ~~**Extract the table.**~~ Done. `web/games/kittens/table.html` now carries the
   table, the rules panel, the cinema layer and the Nope bar — all four, not just
   the two the plan named, because the last two are Exploding Kittens mechanics
   as much as the table is. `index.html` is down from 285 lines to 164 and holds
   only the shell: menu, home, public lobby browser, lobby, modal, toasts,
   connection warning, plus a `<div id="game-root">` to inject into.

   `mountGame(slug)` fetches and injects; `unmountGame()` drops the markup *and*
   the things still running against it (the countdown frame, the explosion plate,
   the queued cinema, the `--nope-h` custom property). `wireGame()` binds the
   controls inside the injected markup and runs per mount, because those bindings
   can no longer sit in the module-load block at the bottom of `app.js`.

   Verified with the full browser suite: 6 scripts, 53 checks, all passing, with
   `play.js` reaching defuse, Nope, cat pairs, Favor and See the Future — so the
   modals, the cinema and the Nope bar all work through injected DOM.

   Three things worth knowing:

   - **`render()` gates on the mount.** An invite link learns its game from the
     room's own first state, so a socket message routinely arrives before the
     table exists. The gate sits *before* the menu is hidden — the fetch is
     same-origin and small, and leaving the menu up for it beats blanking the
     screen. `mountGame()` calls `render()` back with the same state.
   - **`web/embed.go` must stay an explicit list.** The plan said `//go:embed
     all:.`; that is wrong now that `web/testing/node_modules` exists — a wildcard
     would compile several thousand Playwright files into the server. It reads
     `index.html app.js style.css games`.
   - **`#modal` stayed in the shell**, including the `#modal-place` "bury it"
     slider, which is Exploding Kittens' alone. The modal is generic chrome and
     splitting the game-specific body out of it is step 3's job, not step 2's.

   Also: `web/testing/lib.js` now honours a `CHROME` env var pointing at a system
   browser, because `npx playwright install chromium` would not complete here.
3. ~~**Split `app.js`.**~~ Done. 1771 lines became a 720-line shell, an 889-line
   game and 697 lines of shared modules:

       core/dom.js      $ and hide — the two lookups everything is written on
       core/store.js    every localStorage key, still "ek:"-prefixed on purpose
       core/sound.js    the audio engine; games register their own assets
       core/toast.js    passing notices
       core/modal.js    the one blocking prompt, with no game's content in it
       core/feed.js     the log-sequence diffing and the play-by-play
       core/cinema.js   the one-at-a-time flash player
       app.js           socket, menu, name, browser, lobby, palette, mounting
       games/kittens/index.js   the table, hand, Nope window, prompts, rules

   `app.js` no longer contains the word "card". The game is reached only through
   the contract documented at `mountGame()`, and reaches the shell only through
   the `ctx` handed to `mount()` — which is deliberately all functions, so a game
   cannot hold a copy of state the next server message replaces.

   The whole suite passed on the first run of the split: 6 scripts, 53 checks,
   with `play.js` hitting attack, cat pairs, defuse, draw, Favor, See the Future,
   Nope, shuffle and skip.

   Five things worth knowing:

   - **The log-sequence diffing survived intact**, as `core/feed.js`, with the two
     counters kept separate for the reason they always were: a bang may still be
     playing when the next state lands, but the line describing it is written
     immediately either way. `freshEvents()` returning `[]` on the first state of
     a connection is what keeps a reconnect from replaying somebody else's whole
     game at once.
   - **`unmountGame()` is now exercised on every "back to menu".** Leaving a room
     unmounts rather than merely hiding, because the game holds a round's worth of
     state — a selection, a covered hand, a countdown — and none of it should
     survive into the next room. `selection.js` already leaves, reloads, rejoins
     and plays on, so the mount → unmount → remount path is covered for free. That
     closes the gap step 2 left open.
   - **`#modal-place` left the shell.** The modal now has two content slots that
     take elements a game built, so the kitten-burying slider is constructed in
     JS rather than sitting in `index.html`. `core/modal.js` has no idea what a
     card is, which was the point.
   - **Registering audio late needed a fix.** WebKit grants playback per element,
     and a game registers its tracks at mount — usually *after* the tap that
     unblocked audio. Anything registered late has therefore never been blessed,
     so `register()` re-primes when a gesture has already been spent. Without it
     the bang is silently refused on iOS while the intro plays fine.
   - **The lobby's seat range comes from the catalogue now**, not from `n >= 2 &&
     n <= 5`. That constant was one of the last two Exploding Kittens rules left
     in the shell, and a game for ten would have been held to a game for five.
   - **The other one was `hide("nope-bar")`.** The shell was hiding a game's
     overlay by id, on two screens. That is now `render(v)` / `leaveTable()` on the
     module: the shell decides which screen is up, the game decides what its own
     screen is made of. "Does `app.js` name an id from a game's template?" is the
     test for whether this split is real, and the answer is now no.

   Still untested, as before the split and for the same reason: the three-cat
   demand modal. `play.js` will exercise it the moment a trio turns up, but the
   deal is random and one rarely does — see the note in `web/testing/README.md`
   about why a check that almost never runs is worse than none.
4. **UNO's own module**, once 1–3 are done and Exploding Kittens still passes
   `web/testing/`. All three are done and it does, so this is the next step: a
   `games/uno/` directory with a `table.html` and an `index.js` exporting the
   shape `mountGame()` documents, plus the palette block that is already in
   `style.css`. Nothing in the shell should need to change — if it does, that is
   the finding, and it belongs in this file.

### Watch out

- **The UNO palette has three real contrast failures.** Exploding Kittens got
  away with a token-only recolour because `#820a12` is dark (white on it is
  10.5:1). UNO's red is bright:
  - white on `#ec2229` is **4.4:1** — fails AA for body text. Either black ink on
    red, or reserve red for fills and borders and never body copy.
  - `#f8d92b` on `#ec2229` is **3.1:1** — the obvious yellow-on-red headline does
    not pass. Large text only, if at all.
  - `--grey #5f5d5c` on `#000000` is **3.2:1** — the muted token needs a lighter
    value for UNO. It is the one that leaks everywhere (`.muted`, taglines, deck
    counts), so it will look fine on the panels and fail on the ground.
  - black on `#f8d92b` is 15:1, so yellow is the safe surface for text. Build the
    UNO palette around that rather than around red.
- ~~**Shadows disappear on black.**~~ Handled in step 1 by `--shadow-rgb` /
  `--scrim-rgb`; UNO sets the first to the yellow and leaves the second black.
- ~~**Three rgba literals sit outside the tokens.**~~ Now `rgb(var(--surface-rgb)
  / .78)` on the deck count and `--accent-rgb` / `--grad-from-rgb` in the blast
  ring. The blast is still a kittens VFX and should move to
  `games/kittens/theme.css` in step 2 rather than staying in the shared file.
- ~~**`web/embed.go` lists the three files explicitly.**~~ Now
  `index.html app.js style.css games`. Explicitly *not* `all:.`: that would sweep
  in `web/testing/node_modules`. A missing directory fails silently as a 404 at
  runtime rather than as a build error, so check the network tab, not the build.
- **The table teardown/remount path still has no check.** The existing scripts
  cover menu, selection, public lobby and play, and they caught nothing in step 2
  because they exercise one mount and never a second. `unmountGame()` is
  effectively untested today — with one game it only ever runs on a switch that
  cannot happen yet. It is the first thing to break in step 4, so write a check
  that goes game → menu → game before trusting it.

### Prompt to resume this

    Read TODO.md, section "Direction: the web client, per game". I'm adding UNO
    to the browser client. The shell (menu, home, browser, lobby, socket,
    reconnect, storage, mute) is shared and must not be forked; the table, rules,
    palette, audio and VFX are per game.

    Work on step N. Confirm what's already done by reading web/ rather than
    trusting this file. Exploding Kittens must still pass web/testing/ at every
    commit, and step 1 must leave it looking pixel-identical.
