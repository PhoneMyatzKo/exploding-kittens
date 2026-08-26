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

`npm install` pulls the Playwright library; `npx playwright install chromium`
fetches the browser it drives. If that download will not complete — it is ~150MB
and some networks refuse it outright — point `CHROME` at a Chromium-based browser
already installed and skip it:

```bash
CHROME=/usr/bin/google-chrome node run.js
```

Stop the server afterwards with
`Get-Process kittens -ErrorAction SilentlyContinue | Stop-Process -Force`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `BASE` | `http://localhost:8099` | Server to drive |
| `CHROME` | unset | Path to a browser already on the machine, instead of Playwright's own |
| `HEADED` | unset | `1` shows the browsers |
| `SLOWMO` | `0` | Milliseconds between actions, for watching |
| `MOBILE` | unset | `1` runs `play.js` on a phone viewport |
| `GAME` | `kittens` | Which deck `play.js` deals — `kittens-imploding` for the expansion |

Screenshots land in `shots/` (gitignored).

## The scripts

| Script | What it holds to |
| --- | --- |
| `smoke.js` | Three people get from a cold load to a dealt table; eight cards each; one player on turn; no opponent's cards rendered; the card art actually loads |
| `menu.js` | The front page is a game picker: every catalogued game gets a tile, unbuilt ones are shown locked and say so when tapped, the server refuses to open a room for them, and an invite link skips the menu entirely |
| `selection.js` | One non-cat at a time, cats stack only with matching cats, a refused tap explains itself, the hand can be covered; lobby mute and leaving the room properly |
| `logtest.js` | The log behaves like a chat box: always on screen, does not overlap the hand, does not yank a scrolled reader, follows when pinned, collapses and remembers it |
| `publiclobby.js` | Only joinable rooms are listed — private, dealt and full ones drop out; joining from the list; visibility remembered |
| `modals.js` | Everything that appears over the table, on a laptop, a small window and two phone sizes: the Favor picker fits and every card in it can be reached (including a hand twice the size a deal can produce), a big hand never covers the deck or the discard, the Nope window is a centred prompt with reachable buttons and a draining clock, and the target dial spreads its candidates out inside the box |
| `rules.js` | The how-to-play sheet, per deck: it lists exactly the cards this deck contains, shows them as *loaded* card pictures rather than emoji, reads in Burmese with the art intact, and remembers the language |
| `imploding.js` | The expansion: the tile seats six and a six-player table deals; the demand list follows the deck; the rules sheet appears only for it; a Feral Cat stacks with a cat it does not match; and the Imploding Kitten goes back face up for free, arms visibly for everyone, stays hidden while buried but shows its own animated card once it surfaces, and then takes somebody with a Defuse in hand |
| `play.js` | A full three-player game to a winner by real clicks, asserting the invariants on every path and reporting which mechanics it hit |
| `uno.js` | The other game, the same way: a hand plays out to a winner, a Draw Four gets challenged, and every client agrees who won |

`play.js` is a fuzz test with a browser attached: the deal is random, so it
prints the coverage it got (`nope`, `defuse`, `catTrio`, …). A run that never
reached a defuse is a weaker run than one that did, and it says so. `run.js`
drives it twice, once per deck, because the expansion's cards only ever come up
in a game dealt with them.

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
both ends of the range, not just the small one. The same asymmetry hid the deck
sliding under the hand: the *width* clamps on a wide screen while the height does
not, so a 1440×640 window breaks where a phone does not.

**Windows scaling moves the goalposts.** A screenshot 1855px wide from a machine
at 125% is a 1484px viewport, and that is what the CSS sees. When a report comes
with a picture, divide before picking a viewport to reproduce in.

Three traps that have caught this harness before:

- **Elements vanish between the check and the click.** In a multi-tab game
  another player's move can legitimately close the window you were about to
  press a button in. Use `safeClick`, which swallows the timeout and returns
  `false`.
- **An empty `<ul>` has zero size**, so Playwright treats it as hidden. Wait on
  a sibling that always has text — `#browser-status`, not `#browser-list`.
- **A `const` declared below the top-level code is in its TDZ when that code
  runs.** These scripts execute top to bottom with their helpers underneath, so
  putting shared state next to the helper that uses it fails with "Cannot access
  X before initialization" — and only on the path that reads it, so it looks like
  a logic bug. Declare shared state at the top of the file.
- **`offsetTop` is relative to the nearest positioned ancestor**, not to the
  scroll box an element sits in. Arithmetic on it to decide "can this be
  scrolled to" quietly measures nothing. Scroll to the element and ask the
  browser where it ended up instead.

The client keeps its state in a module scope with nothing on `window`. That is
correct, and the tests read the DOM instead. Resist the urge to export state for
testing: an assertion against an internal variable can pass while the screen is
broken, which is the entire failure mode these scripts exist to catch.
