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
| Cat cards ×20 | Worthless alone. Two matching cats steal a random card from a player you choose. |

Setup follows the printed rules: kittens and defuses come out of the deck,
everyone gets 7 cards plus a guaranteed Defuse, the leftover Defuses go back in,
and exactly **one fewer kitten than there are players** is shuffled in. Last
player standing wins.

Not included: the Imploding Kittens expansion, and the advanced cat combos
(three-of-a-kind and five-different).

### Nope windows

When someone plays an action card it sits on the table for 7 seconds while
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
```

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
