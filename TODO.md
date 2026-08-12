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
- Persistence, so a server restart doesn't wipe a game in progress.
