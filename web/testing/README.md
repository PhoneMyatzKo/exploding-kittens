# Browser checks

`go test ./...` covers the rules and the room. It cannot see the things that
have actually broken in this project: a Nope bar sitting on top of the hand, a
toast burying a modal title, a new screen missing the shared background rule, a
log that yanks you to the bottom while you are reading it. These scripts drive
the real client in a real Chromium, one browser context per player, and assert
on what a player can actually see.

They are **not** part of `go test` — they need a running server and a browser.

## Running them

The client is compiled into the binary with `go:embed`, so **a running server
serves the old assets after any edit under `web/`**. Always rebuild first.

```powershell
$env:Path = "C:\Program Files\Go\bin;$env:Path"

go build -o kittens.exe ./cmd/server
Start-Process -FilePath .\kittens.exe -ArgumentList "-addr",":8099" -WindowStyle Hidden

cd web/testing
npm install          # first time only
node run.js
```

Stop the server afterwards with
`Get-Process kittens -ErrorAction SilentlyContinue | Stop-Process -Force`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `BASE` | `http://localhost:8099` | Server to drive |
| `HEADED` | unset | `1` shows the browsers |
| `SLOWMO` | `0` | Milliseconds between actions, for watching |
| `MOBILE` | unset | `1` runs `play.js` on a phone viewport |

Screenshots land in `shots/` (gitignored).

## The scripts

| Script | What it holds to |
| --- | --- |
| `smoke.js` | Three people get from a cold load to a dealt table; eight cards each; one player on turn; no opponent's cards rendered; the card art actually loads |
| `menu.js` | The front page is a game picker: every catalogued game gets a tile, unbuilt ones are shown locked and say so when tapped, the server refuses to open a room for them, and an invite link skips the menu entirely |
| `selection.js` | One non-cat at a time, cats stack only with matching cats, a refused tap explains itself, the hand can be covered; lobby mute and leaving the room properly |
| `logtest.js` | The log behaves like a chat box: always on screen, does not overlap the hand, does not yank a scrolled reader, follows when pinned, collapses and remembers it |
| `publiclobby.js` | Only joinable rooms are listed — private, dealt and full ones drop out; joining from the list; visibility remembered |
| `modals.js` | The Favor picker fits on screen and every card in it can be reached, on a laptop, a small window and two phone sizes — plus a hand twice the size a deal can produce |
| `play.js` | A full three-player game to a winner by real clicks, asserting the invariants on every path and reporting which mechanics it hit |

`play.js` is a fuzz test with a browser attached: the deal is random, so it
prints the coverage it got (`nope`, `defuse`, `catTrio`, …). A run that never
reached a defuse is a weaker run than one that did, and it says so.

Because the deal is random, a check that needs particular cards has to be
possible on nearly every run or it is not worth having. `selection.js` seats a
full table of five for exactly this reason — with three hands a matching cat
pair only turns up about half the time. Two rules are deliberately *not* checked
here for the same reason: the cap at three cats, and building a trio. Both need
three or four of one cat in a single hand, so the check would be skipped almost
every run, and a check that never runs is worse than none — it teaches you to
skim past the skip line. Both are enforced by the engine and covered in
`internal/games/kittens/game`.

## Writing more

**Desktop is not the easy case.** `--card-w` clamps at its *maximum* on a wide
screen, so a list of cards stacks two-per-row into a very tall box — the Favor
picker running off the bottom of the screen was reported on a PC, and the phone
viewports alone would not have found it. Anything sized in `vw` needs checking at
both ends of the range, not just the small one.

Three traps that have caught this harness before:

- **Elements vanish between the check and the click.** In a multi-tab game
  another player's move can legitimately close the window you were about to
  press a button in. Use `safeClick`, which swallows the timeout and returns
  `false`.
- **An empty `<ul>` has zero size**, so Playwright treats it as hidden. Wait on
  a sibling that always has text — `#browser-status`, not `#browser-list`.
- **`offsetTop` is relative to the nearest positioned ancestor**, not to the
  scroll box an element sits in. Arithmetic on it to decide "can this be
  scrolled to" quietly measures nothing. Scroll to the element and ask the
  browser where it ended up instead.

The client keeps its state in a module scope with nothing on `window`. That is
correct, and the tests read the DOM instead. Resist the urge to export state for
testing: an assertion against an internal variable can pass while the screen is
broken, which is the entire failure mode these scripts exist to catch.
