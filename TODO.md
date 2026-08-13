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
- ~~Users can see public Room~~ — **🌍 Public lobby** on the menu lists joinable
  public rooms with host, seat count and who's waiting; tap Join and you're in
  without ever seeing a code. Only rooms you can actually enter are listed, so a
  room disappears once it's dealt or full.

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

### Split

Share `internal/game`, `internal/ws`, `internal/view`, `static` — never fork
them. `internal/room` (675 lines) is the only real duplicate. The v2 kittens
"game" is a thin adapter over the existing engine, not a rewrite of it.

    internal/room/          v1, frozen once core exists
    internal/core/          v2 generic room loop, registry, deadlines
    internal/games/kittens/ adapter wrapping game.Apply + view.For
    internal/hub/           accounts, entitlements, browse — HTTP + DB only

Serve `/ws` + `/` from v1 and `/v2/ws` + `/v2/` from v2 out of the same binary.

### Two changes that are cheap now and painful later

- **Serializable state.** `State.rng *rand.Rand` is the only field in `State`
  that won't marshal; everything else is already plain data. Replace it with a
  seed plus a draw counter. That one change unblocks snapshot-on-shutdown,
  running more than one server process, and dumping exact state from a bug
  report. Today a deploy kills every live game.
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

1. Add `Game string` to `view.View` so the client can pick a renderer per room.
2. Replace `State.rng` with seed + counter.
3. Pull the room test scenarios (`room_test.go`, `e2e_test.go` — reconnect,
   token reclaim, host transfer, idle takeover) behind an interface and make v1
   pass them unchanged. This is the conformance suite; if v2 passes the same
   cases the cutover is a non-event.
4. Auth, accounts, `userID → seatID` mapping in the room layer.
5. `internal/core` + kittens adapter, green against the same suite.
6. React client. Shell (ws, reconnect, lobby, log, audio, mute — roughly
   `app.js` 140–500, none of it mentions cards) split from per-game renderers
   from the first commit. Vite → `dist/` → `//go:embed all:dist` keeps the
   single-executable property; flip `immutable` onto hashed assets and leave
   `noCache` on `index.html` only.
7. Point new rooms at v2, let v1 drain, delete it, then rename the module off
   `boardgame/kittens`.
8. Hub and payments.
9. UNO, then UNO No Mercy.

### Watch out

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
