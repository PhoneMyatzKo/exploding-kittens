# 💥 Exploding Kittens

A self-hosted web version of Exploding Kittens (Original Edition) for 2–5 people.
One person runs the server, everybody else joins from their own phone or laptop
with a four-letter room code.

Written in Go with no database and no frontend build step. The whole thing —
rules, rooms, and browser client — compiles into a single executable.

## Quick start

```sh
go run ./cmd/server            # then open http://localhost:8080
go run ./cmd/server -addr :80  # if you'd rather not type a port
```

On start it prints every address guests can reach it on:

```
Exploding Kittens ready
  http://localhost:8080
  http://192.168.1.26:8080  (Wi-Fi)
```

Give people the `192.168.…` one and the room code. The first time you run it,
Windows will ask whether to let it through the firewall — say yes for private
networks, or nobody else can connect.

To hand out a single binary instead:

```sh
go build -o kittens.exe ./cmd/server
```

The client is embedded in the binary, so `kittens.exe` is the only file you need.

## Playing

1. Someone clicks **Create a room** and reads out the code (or shares the invite
   link, which drops people straight in).
2. Everyone else types the code and joins.
3. The host clicks **Deal the cards**.

On your turn you may play as many cards as you like, then you **draw to end your
turn**. Drawing is what passes play on — so is Skip, and so is Attack.

Rules implemented (Original Edition, 56 cards):

| Card | Effect |
| --- | --- |
| Exploding Kitten ×4 | Draw one and you're out, unless you play a Defuse. |
| Defuse ×6 | Played automatically when you explode; you then choose, in secret, where in the deck the kitten goes back. |
| Nope ×5 | Cancels the action just played, even out of turn. A Nope can be Noped. |
| Attack ×4 | End your turn without drawing; the next player takes 2 turns. Stacks: attacking while attacked passes on your remaining turns **plus** 2. |
| Skip ×4 | End one turn without drawing. Under Attack it burns only one of the two. |
| Favor ×4 | Pick a player; **they** choose which card to hand you. |
| Shuffle ×4 | Shuffle the draw pile. |
| See the Future ×5 | Privately look at the top three cards. |
| Cat cards ×20 | Worthless alone. **Two** matching cats steal a random card from a player you choose. **Three** matching cats let you name the card you want — they hand it over if they have one, and the whole table hears what you asked for. |

Setup follows the printed rules: kittens and defuses come out of the deck,
everyone gets 7 cards plus a guaranteed Defuse, the leftover Defuses go back in,
and exactly **one fewer kitten than there are players** is shuffled in. Last
player standing wins.

Not included: the Imploding Kittens expansion, and the five-different-cards combo
that takes a card from the discard pile.

### Nope windows

When someone plays an action card it sits on the table for 20 seconds while
anyone holding a Nope can cancel it. Play a second Nope on top and the action is
back on. The window only opens if somebody actually holds a Nope, so the game
doesn't stall for no reason.

### Dropping out and coming back

Your seat is tied to a token in the browser's local storage, so refreshing the
page or losing Wi-Fi puts you back in the same seat with the same hand. If the
table is waiting on someone who has gone offline, it plays the least destructive
legal move for them after 25 seconds rather than stalling forever.

## How it fits together

```
cmd/server        main: flags, signals, prints LAN addresses
internal/server   HTTP routes and static client
internal/ws       WebSocket read/write pumps
internal/room     one goroutine per table; owns all mutable state
internal/view     redaction: turns the game into a per-player payload
internal/game     the rules, as a pure function
web/              browser client (no build step), embedded into the binary
static/           card scans (/cards), portraits (/avatars), music (/audio)
```

Cards without a scan in `static/src/images` fall back to the emoji glyph in
`web/app.js`, so the art set can stay incomplete without anything breaking.

## Portraits

Everyone picks a cat in the lobby, from the PNGs in `static/src/avatars`. Those
files are the whole catalogue: the client asks `GET /api/avatars` for the list
and turns each id into both a label and an image URL (`ninja-nala` → "Ninja
Nala", `/avatars/ninja-nala.png`), so adding a character is one file and a
rebuild, with no list to keep in step.

One portrait per table. A picked one goes grey and its button is disabled — for
the player holding it as much as for everybody else, so the only move the grid
offers you is a different cat. The room enforces that too, and refuses an id it
does not serve, since a portrait is public and a hand-written socket message
should not be able to leave a broken image on four other screens.

Picking is optional: skip it and you keep a placeholder chip, and the host can
still deal.

## Explosions, defuses, odds

Drawing a Kitten sets off a flash on every device: the bang and the skull go off
over the seat they belong to, while the defuse plays in the middle of the screen
as the whole table's news. They are driven off the shared log
rather than a message of their own, so everyone sees the same beat at the same
point in the story. Each log entry carries a `seq`, and a client only animates
entries past its own high-water mark; a reconnecting player primes that mark from
the log it is handed, which is what stops a refresh replaying a round of
explosions. Modals hold off until the flash is done — the defuse prompt is a
consequence of the bang, not something to answer over the top of it.

Under the draw pile is the chance the next card is a Kitten (`kittensLeft /
deckCount`). Both numbers are public — setup buries one fewer Kitten than there
are players, Kittens never sit in a hand, and eliminations are announced — so
this is arithmetic any table can do, and no more than they could work out for
themselves. It climbs as the deck thins and drops when a player takes a Kitten
out of the game with them.

`?` in the topbar and **How to play** in the lobby open the rules.

## Cards and the deck back

Cards are 3:4, sized off `--card-w`, and both the draw pile and a face-down card
in your own hand wear `static/src/images/background.jpeg` cropped to that ratio.
The pile's "Draw" label rides on a scrim tight to the bottom edge so it does not
sit over the wordmark.

**Hide hand** turns your own cards face-down for playing next to somebody nosy.
The first tap on a covered card flips it over, the next one selects it, so a
glance over your shoulder gets a card back. Covering clears whatever you had
selected.

## Theme

Five values, and nothing outside them:

```
crimson  #820a12    gold  #fbc240    paper  #fdfeff    grey  #5f5d5c
gradient #e72834 → #ffc942
```

Every other colour in `style.css` is one of those alpha-composited over
another, annotated with the mix it stands for. Two consequences worth knowing
before editing it:

- **Two surface families.** Crimson is the app ground; `.panel`, `.modal-box`,
  `.log-panel`, `.toast`, `.nope-bar` and `.card` switch to paper. Each family
  declares its own `--ink`/`--muted`/`--line`, so component rules never need to
  know which one they are on. The palette forces the split — grey is 1.9:1 on
  crimson but 6.5:1 on paper.
- **The gradient cannot carry text.** Paper drops to 1.6:1 against its gold
  end, and crimson to 2.4:1 against its red end, so no ink is readable across
  the whole sweep. It is therefore used only on decorative surfaces, or behind
  a crimson scrim (the deck back) that brings the label back to 6.2:1. Buttons
  and other labelled controls take solid gold with crimson ink, at 6.4:1.

Card type coding collapses to three families, since thirteen hues cannot fit
five colours: a gold band for the powerless cat cards, the gradient for cards
that do something, solid crimson for the Exploding Kitten.

## Music

Two tracks, both optional, both under `static/src/audio`:

- `intro.mp3` — loops over the title and lobby screens, fades out on the deal.
- `theme_song1.mp3` — plays once over the game-over screen, then stops.

Plus `draw.mp3`, a quarter-second card flick played whenever anybody draws. It
is an effect rather than music, and deliberately outside the mute toggle: that
switch exists so five phones do not play the same track out of sync, which a
single short flick is not.

**Nothing plays during a turn, on purpose.** The format is one phone per player,
so a continuous bed is really five copies of the same track drifting out of sync
in one room — worse than silence. Music is confined to the moments when every
device is on the same screen at the same time. The policy lives in one function,
`syncSound()` in `web/app.js`; change it there.

A missing file means silence — nothing errors, and the toggle still works. Mute
state is remembered per browser under `ek:muted`. Browsers will not start audio
before the page has been interacted with, so a rejected `play()` arms a one-shot
gesture listener and retries rather than giving up.

`static.go` embeds by extension (`src/images/*.jpg`, `src/avatars/*.png`,
`src/audio/*.mp3`) so that
working files sitting next to the assets — a rules PDF, a 46 MB download a clip
was cut from — never end up in the binary.

## After a game

The game-over screen gives the host two ways on:

- **Deal again** seats whoever is connected at that moment and starts a round.
- **Back to lobby** drops the finished game so the whole table returns to the
  lobby together. This is the only way to pick up players who joined or
  reconnected mid-round, since dealing again forgets everyone who was not
  present. Refused while a game is still in progress, so it cannot be used to
  wipe a losing position.

Two ideas carry most of the weight:

**One goroutine per room.** Each table has a single goroutine that exclusively
mutates its game state; sockets only post commands to it. There is no lock
anywhere near the rules, and no data race is possible by construction.

**Redaction at one boundary.** `internal/view` is the only code allowed to build
a client payload from a game state. A player sees their own hand in full,
everyone else's as a count, and the draw pile as a count — never its order. That
property is enforced by a test that serialises thousands of game states and
fails if any card ID reaches a player who shouldn't see it.

The rules in `internal/game` know nothing about JSON, sockets or players'
connections, which is why the interesting bugs are catchable with `go test`.

## Tests

```sh
go test ./...            # everything
go test -race ./...      # needs gcc on Windows
```

What they cover:

- **Rules** — every card's behaviour, Attack stacking, Skip under Attack, Nope
  parity, defuse placement, elimination and win detection. Plus 200 randomised
  full games asserting that cards are never duplicated or lost, the turn never
  lands on a dead player, and every game terminates with exactly one survivor.
- **Redaction** — the leak check described above.
- **Rooms** — join, capacity, host-only start, reconnect-into-your-old-hand, and
  a complete game driven through the real room goroutine.
- **End to end** — a full game played to a winner across three live WebSocket
  connections against the real HTTP server, with all three clients asked to
  agree on the winner.
