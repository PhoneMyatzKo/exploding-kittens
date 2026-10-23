// The tokens on the board, and the one thing that makes them able to move.
//
// They used to be rebuilt on every state — `.tile-tokens` emptied and a fresh
// `<i>` appended to whichever square each player stood on. That is correct and
// it is also why nothing could ever be animated: a transition needs the *same*
// DOM node to survive the update, and every node was being thrown away sixty
// times a game.
//
// So the tokens live here instead: one absolutely-positioned layer over the
// board, one element per player created once and kept, moved with `transform`.
// Transform rather than left/top because it is composited rather than laid out,
// and a 40-cell grid is an expensive thing to ask the browser to re-layout
// mid-animation.
//
// Two positions exist, and keeping them apart is the whole design:
//
//   target  where the server says a player is. Arrives with every state.
//   shown   where their token is drawn right now.
//
// They are equal at rest. A walk is the gap between them being closed one
// square at a time. Nothing here decides *when* to walk — index.js reads the
// events and asks — and nothing here blocks: the server is authoritative and a
// Roll button that waits for a token to stop moving makes the game feel broken.

// Forty squares, which is the modulus every walk is taken under. Mirrors
// BoardSize in internal/games/monopoly/game/board.go; the board the client draws
// comes from the server, but the arithmetic for walking round it is here.
const BoardSize = 40;

// How long one square takes, and the ceiling on a whole move.
//
// Paced to be *followed* rather than merely noticed: at 85ms a square the piece
// crossed half the board before anybody had focused on it, which is the same as
// not animating at all. 165ms is about the speed somebody moves a piece by hand,
// and it is slow enough to count the squares along with the dice.
//
// The cap matters at the top of the range: twelve squares would otherwise run
// to two seconds, so a long throw quickens a little rather than dragging.
const STEP_MS = 165;
const WALK_CAP_MS = 1700;
// A trip to jail is not a walk. It is the board picking you up, so it is quicker
// than a step-by-step move and it does not trace the route.
const JUMP_MS = 520;

const state = {
  layer: null,
  nodes: new Map(),  // playerId -> element
  shown: new Map(),  // playerId -> square being drawn
  geom: null,        // per-square { x, y } centre, relative to the board
};

// reduced reports whether this viewer has asked for less motion. Checked at the
// moment of use rather than cached: it is also the switch the browser tests flip
// (page.emulateMedia), and a value read once at mount would miss it.
export const reduced = () =>
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// mountTokens prepares the layer over an existing board grid. Safe to call again
// — rebuilding the board drops the layer with it.
export function mountTokens(grid) {
  if (!grid) return;
  state.nodes.clear();
  state.shown.clear();
  state.geom = null;

  const layer = document.createElement("div");
  layer.className = "token-layer";
  layer.id = "token-layer";
  grid.append(layer);
  state.layer = layer;
}

export function unmountTokens() {
  state.layer = null;
  state.nodes.clear();
  state.shown.clear();
  state.geom = null;
}

// forgetGeometry drops the cached square positions. The board is sized from the
// viewport, so every one of them changes when the window does.
export function forgetGeometry() {
  state.geom = null;
}

// squareCentres measures where each square is, once, relative to the board.
//
// Read from the DOM rather than computed from the grid definition: the track
// sizes are `fr` units and the corners are 1.55 of an edge square, so the only
// honest source of "where is square 27" is the browser that laid it out.
function squareCentres(grid) {
  if (state.geom) return state.geom;
  const out = new Array(BoardSize).fill(null);
  for (const el of grid.querySelectorAll(".tile")) {
    const i = Number(el.dataset.tile);
    if (!Number.isInteger(i)) continue;
    // offsetLeft/Top are relative to .board, which is positioned — see the
    // stylesheet. Cheaper than getBoundingClientRect and not affected by the
    // page having been scrolled.
    out[i] = { x: el.offsetLeft + el.offsetWidth / 2, y: el.offsetTop + el.offsetHeight / 2 };
  }
  state.geom = out;
  return out;
}

// fanOut spreads the tokens that share a square so six pieces on GO read as six
// pieces rather than as one. A ring, because a row would run off a corner
// square and the corners are where everybody starts.
function fanOut(index, total) {
  if (total <= 1) return { dx: 0, dy: 0 };
  const r = total <= 4 ? 7 : 9;
  const angle = (index / total) * Math.PI * 2 - Math.PI / 2;
  return { dx: Math.cos(angle) * r, dy: Math.sin(angle) * r };
}

// transformFor is where one token sits when it is standing on `square`.
function transformFor(grid, square, index, total) {
  const centres = squareCentres(grid);
  const c = centres[((square % BoardSize) + BoardSize) % BoardSize];
  if (!c) return "translate(-50%, -50%)";
  const { dx, dy } = fanOut(index, total);
  // The element is sized by CSS and centred on the point by its own half-size,
  // which is what the -50% pair does — so this stays correct as the board
  // resizes without the token's dimensions appearing in this arithmetic.
  return `translate(calc(${c.x + dx}px - 50%), calc(${c.y + dy}px - 50%))`;
}

// crowd is who else is standing where this player is standing, so the fan is
// stable rather than jumping as other people move.
function crowd(seats, square, playerID) {
  const here = seats
    .filter((s) => s.alive && displayedSquare(s) === square)
    .map((s) => s.id);
  const index = Math.max(0, here.indexOf(playerID));
  return { index, total: Math.max(1, here.length) };
}

// displayedSquare is where a seat's token is *drawn*, which during a walk is
// behind where the server says they are.
function displayedSquare(seat) {
  const shown = state.shown.get(seat.id);
  return shown === undefined ? seat.pos : shown;
}

// syncTokens makes the layer match the seat list: a node per living player, in
// their colour, and gone once they are out. It does not move anybody who is
// already placed — that is placeAll's job, and during a walk it must not be.
export function syncTokens(grid, seats, { me, colourOf }) {
  if (!state.layer || !grid) return;

  const living = new Set();
  for (const s of seats) {
    if (!s.alive) continue;
    living.add(s.id);

    let el = state.nodes.get(s.id);
    if (!el) {
      el = document.createElement("i");
      el.className = "token";
      el.dataset.seat = s.id;
      state.layer.append(el);
      state.nodes.set(s.id, el);
      // A token that has never been drawn starts where the server says, with no
      // animation. This is also the reconnect path: coming back to a game in
      // progress should not replay somebody's lap.
      state.shown.set(s.id, s.pos);
    }
    el.style.background = colourOf(s.id);
    el.title = s.name;
    el.classList.toggle("me", s.id === me);
  }

  for (const [id, el] of state.nodes) {
    if (living.has(id)) continue;
    el.remove();
    state.nodes.delete(id);
    state.shown.delete(id);
  }
}

// placeAll draws every token where it is currently *shown*, with no animation.
// Called on every render, so a resize, a language switch or a rebuilt board puts
// the pieces back exactly where they were.
export function placeAll(grid, seats) {
  if (!state.layer || !grid) return;
  for (const s of seats) {
    const el = state.nodes.get(s.id);
    if (!el) continue;
    const square = displayedSquare(s);
    const { index, total } = crowd(seats, square, s.id);
    el.style.transform = transformFor(grid, square, index, total);
    // Which square this piece is drawn on. Written to the DOM because that is
    // where it belongs — it is what the token is showing, not a private
    // variable — and because "whose piece is on Bagan" is otherwise unanswerable
    // from the outside once position became a transform. The browser suite reads
    // it; so does anybody with dev tools open.
    el.dataset.square = String(square);
  }
}

// snapAll abandons any pretence and puts every token where the server says.
// The catch-up half of the design: when there is nothing queued, shown is truth.
export function snapAll(grid, seats) {
  for (const s of seats) state.shown.set(s.id, s.pos);
  placeAll(grid, seats);
}

// walkToken moves one token square by square, the long way round the board.
//
// Square by square and not in a straight line, because the straight line is a
// diagonal across the middle of the board and says nothing. Walking is what
// makes passing GO legible: you watch the corner go by.
export async function walkToken(grid, seats, playerID, from, steps) {
  const el = state.nodes.get(playerID);
  if (!el || !grid || steps === 0) return;

  const forward = steps > 0;
  const count = Math.abs(steps);
  const path = [];
  for (let n = 1; n <= count; n++) {
    const sq = from + (forward ? n : -n);
    path.push(((sq % BoardSize) + BoardSize) % BoardSize);
  }
  const land = path[path.length - 1];

  if (reduced()) {
    state.shown.set(playerID, land);
    placeAll(grid, seats);
    return;
  }

  const { index, total } = crowd(seats, land, playerID);
  const frames = [
    { transform: transformFor(grid, from, index, total) },
    ...path.map((sq) => ({ transform: transformFor(grid, sq, index, total) })),
  ];

  el.classList.add("walking");
  const anim = el.animate(frames, {
    duration: Math.min(count * STEP_MS, WALK_CAP_MS),
    easing: "linear",
    fill: "forwards",
  });
  await settled(anim);
  el.classList.remove("walking");

  // The animation is thrown away and the position written properly, so the next
  // render agrees with the screen. A `fill: forwards` left in place would fight
  // every later style change on this element.
  anim.cancel();
  state.shown.set(playerID, land);
  placeAll(grid, seats);
}

// jumpToken is for a square somebody did not walk to — the trip to jail. No
// path, because there was no journey.
export async function jumpToken(grid, seats, playerID, to) {
  const el = state.nodes.get(playerID);
  if (!el || !grid) return;
  if (reduced()) {
    state.shown.set(playerID, to);
    placeAll(grid, seats);
    return;
  }

  const { index, total } = crowd(seats, to, playerID);
  const anim = el.animate(
    [{ transform: el.style.transform || "none" },
     { transform: transformFor(grid, to, index, total) }],
    { duration: JUMP_MS, easing: "cubic-bezier(.4,0,.2,1)", fill: "forwards" },
  );
  await settled(anim);
  anim.cancel();
  state.shown.set(playerID, to);
  placeAll(grid, seats);
}

// settled waits for an animation and never rejects. A cancelled animation
// rejects its finished promise, and a cancel is a perfectly ordinary thing to
// happen here — the board can be rebuilt out from under a walk.
function settled(anim) {
  return anim.finished.catch(() => {});
}
