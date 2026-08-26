// Exploding Kittens — the client half of one game.
//
// The shell (../../app.js) owns the socket, the menu, the lobby and the screens
// every game shares. This module owns everything from the moment the cards are
// dealt: the table, the hand, the Nope window, the cinematics, the rules text and
// the prompts. It never touches the socket directly and never reads the shell's
// state — both arrive through the ctx handed to mount().
//
// The server is authoritative for every rule. Nothing here does rules checking
// beyond deciding which buttons to light up: it renders `view`, sends intents,
// and re-renders whatever comes back.

import { $ } from "../../core/dom.js";
import { groupByCategory } from "../../core/sort.js";
import { logOpen, setStoredLogOpen } from "../../core/store.js";
import { register as registerSound, setTrack, playSfx } from "../../core/sound.js";
import { openModal, closeModal, modalKind } from "../../core/modal.js";
import { renderLog, logPrivate, freshEvents, scrollLogToEnd } from "../../core/feed.js";
import {
  configureCinema, pushCinematic, runCinema, cinemaPlaying, resetCinema,
} from "../../core/cinema.js";

// ────────────────────────────────────────────────────────── assets

const GLYPHS = {
  exploding: "💥", defuse: "✂️", nope: "🚫", attack: "⚔️", skip: "⏭️",
  favor: "🙏", shuffle: "🔀", future: "🔮",
  "cat-taco": "🌮", "cat-rainbow": "🌈", "cat-melon": "🍉",
  "cat-potato": "🥔", "cat-beard": "🧔",
  // Imploding Kittens expansion.
  imploding: "🌀", reverse: "🔄", bottom: "⬇️", "feral-cat": "🐈",
  alter: "🃏", "targeted-attack": "🎯",
};

// Every card arrives carrying the face the server drew for it, out of the scans
// in the binary — so the six Defuses in a game are six different Defuses, the
// way the printed deck works. The server picks rather than the client because
// everyone at the table has to be looking at the same picture.
//
// A card with no art — nothing registered, or a slug whose directory went away —
// falls back to the emoji glyph above, so partial art is fine.
const artURL = (card) =>
  card.art ? `/cards/${card.art.split("/").map(encodeURIComponent).join("/")}` : "";

// A card lying face up on the deck gets a picture of its own. The printed scan
// is what the Imploding Kitten looks like in a hand; this is what it looks like
// sitting on the pile with the whole table watching it. Keyed by slug rather
// than hardcoded, because "face up in the deck" is a property of the card.
const FACE_UP_ART = { imploding: "/cards/imploding_kitten_faceup.gif" };
const faceUpArtURL = (card) => FACE_UP_ART[card.slug] || artURL(card);

// Warmed once per page, off the browser cache thereafter. Kept out of mount/
// unmount state on purpose: the cache outlives the module, so a second game in
// the same tab should not fetch it again.
const prefetched = new Set();
function prefetchFaceUp(slug) {
  const src = FACE_UP_ART[slug];
  if (!src || prefetched.has(src)) return;
  prefetched.add(src);
  new Image().src = src;
}

// The game-over payoff. The title music belongs to the hub and is the shell's.
const TRACKS = {
  theme: { src: "/audio/theme_song1.mp3", volume: 0.6, loop: false },
};

// offset skips the dead air at the head of a clip, in seconds, so the sound
// lands on the beat that triggered it instead of a third of a second later —
// long enough to read as lag rather than as a card being drawn. The numbers are
// where each file actually starts making noise, rounded down a hair so the
// attack transient survives:
//
//   draw.mp3       silent to 0.32     explosion.mp3  silent to 0.05
//   nope.mp3       silent to 0.06
//
// Re-cut a file and this is the line to revisit. What the onset is:
//   ffmpeg -i f.mp3 -af silencedetect=noise=-40dB:d=0.03 -f null -
const SFX = {
  draw: { src: "/audio/draw.mp3", volume: 0.5, offset: 0.3 },
  boom: { src: "/audio/explosion.mp3", volume: 0.8, offset: 0.03 },
  nope: { src: "/audio/nope.mp3", volume: 0.9, offset: 0.04 },
};

// Read off the shared log, so every player sees the same bang at the same point.
// Must match the CSS animation delays.
//
// The explosion runs longest because it has real footage behind it, and it is
// the one beat in the game worth waiting through. Everything that would open a
// modal waits for it — see renderPrompts.
const CINEMA_MS = { exploded: 2200, defused: 1300, eliminated: 1400 };

// Anything spent on the offset comes out of the clip's 2.6s, and the beat needs
// CINEMA_MS.exploded (2.2s) of footage — so past ~0.4s the fire runs out before
// the flash does. Zero because the clip was already cut to the ignition: frame
// one is the flash, so there is nothing to skip.
const BOOM_VFX = { src: "/video/explosion.mp4", offset: 0 };

// ────────────────────────────────────────────────────────── state

// Everything here is this game's own, and is reset by mount() so a second round
// or a second room never inherits a stale selection.
const me = {
  selected: new Set(),  // card ids picked out of our hand
  coverHand: false,     // hand shown face-down, for nosy neighbours
  peeked: new Set(),    // cards turned back over while covered
  flipping: 0,          // the card id whose flip animation should run
  awaitingTarget: false,
  blockedHint: "",      // why the last card tap was refused
  windowEndsAt: 0,      // performance.now() deadline for the Nope countdown
  windowTotalMs: 0,     // full window length, as the server reported it
};

// The kinds a Three of a Kind may demand, fetched so card names live only in Go.
const catalogue = { demandable: [], kinds: [] };

// The slug matters: the expansion adds five more kinds a trio could demand, and
// asking without it would offer the base game's list at an Imploding table.
async function loadCardKinds(slug) {
  try {
    const res = await fetch(`/api/cards?game=${encodeURIComponent(slug || "kittens")}`);
    if (!res.ok) return;
    const body = await res.json();
    catalogue.demandable = body.demandable || [];
    catalogue.kinds = body.kinds || [];
  } catch {
    // Left empty: the demand picker falls back to explaining why it can't open,
    // and the rules sheet keeps its emoji.
    return;
  }
  paintRulesArt();
}

// paintRulesArt puts the real scanned card in the how-to-play sheet, where the
// markup ships an emoji. A picture of the card is worth more than a glyph when
// what you are trying to do is recognise it in your hand.
//
// The emoji stays in the markup as the fallback: a card with no art registered,
// or a face that fails to load, keeps it rather than leaving a blank.
function paintRulesArt() {
  const faces = new Map((catalogue.kinds || []).map((k) => [k.slug, k.face]));

  for (const dt of document.querySelectorAll(".rules-cards dt[data-slug]")) {
    // Split the shipped label once and remember both halves, so repainting for
    // another deck does not eat the text.
    if (dt.dataset.label === undefined) {
      const raw = dt.textContent.trim();
      const cut = raw.indexOf(" ");
      dt.dataset.glyph = cut > 0 ? raw.slice(0, cut) : "";
      dt.dataset.label = cut > 0 ? raw.slice(cut + 1) : raw;
    }

    const glyph = () => {
      const s = document.createElement("span");
      s.className = "rules-glyph";
      s.textContent = dt.dataset.glyph;
      return s;
    };

    dt.replaceChildren();
    const art = faces.get(dt.dataset.slug);
    if (art) {
      const img = document.createElement("img");
      img.className = "rules-art";
      img.src = artURL({ art });
      img.alt = "";
      // Eager, unlike the cards on the table: the sheet is one long scroll of
      // sixteen faces, and a lazy one below the fold is still an empty box by the
      // time you have scrolled to it — which is the whole thing this replaced.
      img.decoding = "async";
      img.onerror = () => img.replaceWith(glyph());
      dt.append(img);
    } else if (dt.dataset.glyph) {
      dt.append(glyph());
    }
    dt.append(document.createTextNode(dt.dataset.label));
  }
}

// ────────────────────────────────────────────────── rules language

// Burmese is offered because this is a party game played out loud, and the
// person reading the rules aloud is not always the person who set the server up.
// Only the how-to-play sheet is translated; the table itself is mostly card
// names and numbers.
const rulesLang = () => (localStorage.getItem("ek:lang") === "my" ? "my" : "en");

function setRulesLang(lang) {
  localStorage.setItem("ek:lang", lang);
  for (const s of document.querySelectorAll(".rules-lang")) {
    s.hidden = s.dataset.lang !== lang;
  }
  for (const [id, on] of [["lang-en", lang === "en"], ["lang-my", lang === "my"]]) {
    $(id).classList.toggle("on", on);
    $(id).setAttribute("aria-checked", String(on));
  }
  document.querySelector(".rules-title").textContent =
    lang === "my" ? "ကစားနည်း" : "How to play";
  $("rules-close").textContent = lang === "my" ? "ပိတ်" : "Close";
}

// Handed in by the shell at mount. Declared here so every function below can
// reach it without threading it through a dozen signatures.
let ctx = null;

const view = () => ctx.view();
const nameOf = (id) => ctx.nameOf(id);
const send = (msg) => ctx.send(msg);
const rerender = () => ctx.rerender();

// ────────────────────────────────────────────────────────── the module

export default {
  // Mute-button slots this game's markup carries, beyond the shell's own.
  slots: ["sound-table"],

  mount(context) {
    ctx = context;
    me.selected.clear();
    me.peeked.clear();
    me.coverHand = false;
    me.flipping = 0;
    me.awaitingTarget = false;
    me.blockedHint = "";

    registerSound({ tracks: TRACKS, sfx: SFX });
    configureCinema({ vfx: BOOM_VFX, onIdle: onCinemaIdle });
    loadCardKinds(ctx.game());
    // The expansion's rules ship with this panel but are hidden at the base
    // game's table, so a player is not told about cards their deck does not
    // contain.
    for (const el of document.querySelectorAll(".rules-expansion")) {
      el.hidden = ctx.game() !== "kittens-imploding";
    }

    $("hand-cover").onclick = toggleCoverHand;
    $("target-cancel").onclick = () => { me.awaitingTarget = false; rerender(); };
    $("log-collapse").onclick = () => setLogOpen(!logOpen());
    $("rules-table").onclick = () => showRules(true);
    $("rules-close").onclick = () => showRules(false);
    $("lang-en").onclick = () => setRulesLang("en");
    $("lang-my").onclick = () => setRulesLang("my");
    setRulesLang(rulesLang());
    $("deck").onclick = () => {
      me.selected.clear();
      me.awaitingTarget = false;
      send({ type: "draw" });
    };
    setLogOpen(logOpen());
  },

  unmount() {
    cancelAnimationFrame(countdownRaf);
    clearTimeout(blockedTimer);
    resetCinema();
    document.documentElement.style.removeProperty("--nope-h");
    ctx = null;
  },

  // Called for every state the server sends, once the cards are dealt. The shell
  // handles the lobby itself; this is only ever the table.
  render(v) {
    $("table").hidden = false;
    renderTable(v);
    // The one place that knows this game's music policy: quiet during play,
    // because five phones playing one bed drift apart in a single room.
    setTrack(v.phase === "game_over" ? "theme" : null);
  },

  // The shell has moved to one of its own screens. The Nope bar goes with the
  // table rather than staying put, because it is fixed to the bottom of the
  // viewport and would otherwise float over the lobby.
  //
  // The rules panel deliberately survives this: its button in the lobby is the
  // shell's, and closing the panel because the table went away would make that
  // button useless exactly where it is offered.
  leaveTable() {
    $("table").hidden = true;
    $("nope-bar").hidden = true;
    cancelAnimationFrame(countdownRaf);
  },

  // Beats worth animating, and the one-shot sounds that go with them. Split from
  // render() because it must run *before* it, so a starting flash can defer the
  // modal that would otherwise open underneath it.
  onState(v) {
    if (v.pending) {
      me.windowEndsAt = performance.now() + v.pending.remainingMs;
      me.windowTotalMs = v.pending.totalMs || 0;
    }
    for (const e of freshEvents(v)) {
      // Sounds that belong to the moment the event landed. The bang is not one of
      // them: it goes off with its picture, from the cinema queue, which may be a
      // beat later if something is already playing.
      if (e.kind === "drew") playSfx("draw");
      if (e.kind === "noped") playSfx("nope");
      const item = cinematicFor(e);
      if (item) pushCinematic(item);
    }
    runCinema();
  },

  onPrivate(e) { handlePrivate(e); },

  // Escape closes the rules, then backs out of picking a target. Returns whether
  // it used the key, so the shell can fall through to its own handling.
  onEscape() {
    if (rulesOpen()) { showRules(false); return true; }
    if (me.awaitingTarget) { me.awaitingTarget = false; rerender(); return true; }
    return false;
  },

  // The lobby's "How to play" button is the shell's, but the panel it opens is
  // this game's, so the shell asks through here.
  showRules,
};

// ────────────────────────────────────────────────────────── the table

function renderTable(v) {
  $("table-code").textContent = v.code;

  renderDeck(v);
  renderTurnBanner(v);
  renderSeats(v);
  renderDiscard(v);
  renderHand(v);
  renderActions(v);
  renderNopeBar(v);
  renderLog(v, logLine);
  renderPrompts(v);
}

// The odds of the next draw being a Kitten — arithmetic on two public numbers.
function renderDeck(v) {
  $("deck-count").textContent = `${v.deckCount} left`;
  renderDeckFace(v);

  const kittens = v.kittensLeft || 0;
  const pct = v.deckCount > 0 ? Math.round((kittens / v.deckCount) * 100) : 0;
  const risk = $("deck-risk");
  // The armed Imploding Kitten is worth calling out on its own: it is the one
  // card in the deck a Defuse cannot answer, so the plain percentage understates
  // what is at stake.
  const glyph = v.implodingArmed ? "🌀" : "💥";
  // Once the kitten is actually on top there is nothing left to estimate — a
  // percentage next to a picture of the card would read as a hedge. The
  // pointer still goes all the way to the red end: the next draw is certain.
  risk.textContent =
    v.phase === "game_over" ? ""
    : v.deckTop ? `${glyph} next draw`
    : `${glyph} ${pct}%`;
  risk.title = v.deckTop
    ? `${v.deckTop.name} is face up on top — whoever draws is out, and no Defuse stops it`
    : v.implodingArmed
    ? `${kittens} kitten${kittens === 1 ? "" : "s"} in ${v.deckCount} cards — one of them is the armed Imploding Kitten, and no Defuse stops it`
    : `${kittens} Exploding Kitten${kittens === 1 ? "" : "s"} in ${v.deckCount} cards`;
  risk.classList.toggle("hot", pct >= 25 || Boolean(v.implodingArmed));
  risk.classList.toggle("armed", Boolean(v.implodingArmed));

  $("risk-meter").hidden = v.phase === "game_over";
  const pointer = $("risk-pointer");
  pointer.style.bottom = `${v.deckTop ? 100 : pct}%`;
  pointer.classList.toggle("armed", Boolean(v.implodingArmed));
}

// The face-up card on top of the deck, when there is one. Only the armed
// Imploding Kitten is ever face up, and the server sends it only while it is
// actually on top — so the picture appearing means the very next draw is fatal,
// not merely that the kitten is somewhere in the pile. Everyone sees the same
// thing at the same time, which is the point of putting it back face up.
function renderDeckFace(v) {
  const img = $("deck-face");
  const deck = $("deck");
  const top = v.deckTop;

  if (!top) {
    // Armed but still buried: fetch the animation now, while nobody is waiting
    // for it. It is a few megabytes, and the moment it surfaces is the worst
    // possible time to be staring at a half-loaded card.
    if (v.implodingArmed) prefetchFaceUp("imploding");
    img.hidden = true;
    img.removeAttribute("src"); // stops the gif animating behind a hidden element
    img.alt = "";
    deck.classList.remove("face-up");
    deck.removeAttribute("aria-label");
    return;
  }

  const src = faceUpArtURL(top);
  // Guarded: reassigning the same src restarts the animation on every state, and
  // states arrive on every play anybody makes.
  if (img.getAttribute("src") !== src) img.setAttribute("src", src);
  img.alt = top.name;
  img.hidden = false;
  deck.classList.add("face-up");
  deck.setAttribute("aria-label", `Draw — the ${top.name} is face up on top`);
}

function renderTurnBanner(v) {
  const el = $("turn-banner");
  el.classList.toggle("mine", v.currentId === ctx.me());
  if (v.phase === "game_over") {
    el.textContent = `🏆 ${nameOf(v.winnerId)} survived!`;
    return;
  }
  const extra = v.turnsRemaining > 1 ? ` · ${v.turnsRemaining} turns to take` : "";
  // Only mentioned once it is not the obvious answer: saying "going forwards" on
  // every turn of every game would be noise.
  const way = v.direction < 0 ? " · 🔄 reversed" : "";
  el.textContent = (v.currentId === ctx.me() ? "Your turn" : `${nameOf(v.currentId)}'s turn`) + extra + way;
}

function renderSeats(v) {
  $("seats").replaceChildren(...v.seats.map((s) => {
    const div = document.createElement("div");
    div.className = "seat";
    div.dataset.seat = s.id; // where an explosion goes off
    div.classList.toggle("current", s.current);
    div.classList.toggle("dead", !s.alive);
    if (me.awaitingTarget && s.alive && s.id !== ctx.me()) {
      div.classList.add("selectable");
      div.onclick = () => playSelection(s.id);
    }
    const nm = document.createElement("div");
    nm.className = "seat-name";
    const dot = document.createElement("i");
    dot.className = "dot" + (s.connected ? "" : " off");
    nm.append(ctx.avatarChip(s), dot,
      document.createTextNode(s.name + (s.id === ctx.me() ? " (you)" : "")));
    const meta = document.createElement("div");
    meta.className = "seat-meta";
    meta.textContent = s.alive ? `${s.handCount} cards` : "exploded";
    div.append(nm, meta);
    return div;
  }));
}

function renderDiscard(v) {
  const box = $("discard");
  box.replaceChildren();
  if (v.discardTop) box.append(cardEl(v.discardTop, { static: true }));
}

function renderHand(v) {
  // Drop selections for cards we no longer hold (stolen, given, noped away).
  const held = new Set(v.me.hand.map((c) => c.id));
  for (const id of [...me.selected]) if (!held.has(id)) me.selected.delete(id);
  for (const id of [...me.peeked]) if (!held.has(id)) me.peeked.delete(id);

  const sel = selectedCards(v);
  // Grouped by slug so, say, all three Cat Tacos sit together rather than
  // wherever they happened to be drawn — the whole point of a hand of cards.
  const hand = groupByCategory(v.me.hand, (c) => c.slug);

  $("hand").replaceChildren(...hand.map((c) => {
    // While the hand is covered, the first tap turns a card over and the next
    // one picks it, so a glance over your shoulder gets nothing.
    if (me.coverHand && !me.peeked.has(c.id)) {
      const back = backEl();
      back.onclick = () => { me.peeked.add(c.id); me.flipping = c.id; rerender(); };
      return back;
    }
    const el = cardEl(c);
    const picked = me.selected.has(c.id);
    el.classList.toggle("selected", picked);
    // Dimmed means "can't join what you've already picked" — visible before the
    // tap, so a refusal is never a surprise.
    if (!picked && !selectableWith(c, sel).ok) el.classList.add("blocked");
    if (me.flipping === c.id) el.classList.add("flipping");
    el.onclick = () => toggleSelect(c.id);
    return el;
  }));
  me.flipping = 0;

  const btn = $("hand-cover");
  btn.hidden = !v.me.alive || v.me.hand.length === 0;
  btn.textContent = me.coverHand ? "👀 Show hand" : "🙈 Hide hand";
  btn.setAttribute("aria-pressed", String(me.coverHand));
}

function toggleCoverHand() {
  me.coverHand = !me.coverHand;
  me.peeked.clear();
  me.selected.clear();
  me.awaitingTarget = false;
  rerender();
}

function renderActions(v) {
  $("deck").disabled = !v.me.myTurn;

  const sel = selectedCards(v);
  const plan = selectionPlan(sel);
  const playBtn = $("play-btn");
  playBtn.disabled = !(v.me.myTurn && plan.ok);
  playBtn.onclick = () => {
    if (plan.needsTarget) { me.awaitingTarget = true; rerender(); }
    else playSelection("");
  };

  // A refused tap outranks the standing advice, since it is answering something
  // the player just did.
  $("hint").textContent = me.blockedHint
    ? me.blockedHint
    : !v.me.alive ? "You're out — enjoy the show."
    : v.phase === "game_over" ? ""
    : !v.me.myTurn ? "Waiting…"
    : sel.length === 0 ? "Play a card, or draw to end your turn."
    : plan.ok ? (plan.needsTarget ? "Press Play, then pick a player." : "")
    : plan.why;
  $("hint").classList.toggle("warn", Boolean(me.blockedHint));

  const row = $("target-row");
  row.hidden = !me.awaitingTarget;
  if (me.awaitingTarget) {
    $("target-buttons").replaceChildren(...v.seats
      .filter((s) => s.alive && s.id !== ctx.me())
      .map((s) => {
        const b = document.createElement("button");
        b.className = "btn small";
        b.textContent = s.name;
        b.onclick = () => playSelection(s.id);
        return b;
      }));
  }
}

// ────────────────────────────────────────────────────────── the nope window

// reserveForNopeBar keeps the hand clear of the fixed bottom bar by telling the
// layout exactly how tall it currently is.
function reserveForNopeBar(bar) {
  const h = bar.hidden ? 0 : bar.offsetHeight;
  document.documentElement.style.setProperty("--nope-h", `${h}px`);
}

function renderNopeBar(v) {
  const bar = $("nope-bar");
  if (v.phase !== "nope" || !v.pending) {
    bar.hidden = true;
    reserveForNopeBar(bar);
    return;
  }
  bar.hidden = false;

  const p = v.pending;
  const names = p.cards.map((c) => c.name).join(" + ");
  // A demand reads better as what is being asked for than as "on <player>".
  const on = p.named
    ? `, demanding ${nameOf(p.targetId)}'s ${p.named.name}`
    : p.targetId ? ` on ${nameOf(p.targetId)}` : "";
  const stack = p.nopes > 0
    ? ` — ${p.nopes} Nope${p.nopes > 1 ? "s" : ""} stacked, so it ${p.cancelled ? "will NOT happen" : "WILL happen"}`
    : "";
  $("nope-text").textContent = `${nameOf(p.actorId)} played ${names}${on}${stack}`;

  const nopeBtn = $("nope-btn");
  nopeBtn.hidden = !v.me.canNope;
  nopeBtn.onclick = () => send({ type: "nope" });

  const passBtn = $("pass-btn");
  passBtn.hidden = !v.me.canPass;
  passBtn.onclick = () => send({ type: "pass" });

  reserveForNopeBar(bar);
  tickCountdown();
}

// The window length comes from the server with every open window, so there is no
// constant here to fall out of step with internal/room. The fallback only covers
// an older server that doesn't send it.
let countdownRaf = 0;
function tickCountdown() {
  cancelAnimationFrame(countdownRaf);
  const total = me.windowTotalMs || 20000;
  const step = () => {
    const left = Math.max(0, me.windowEndsAt - performance.now());
    $("nope-fill").style.width = `${Math.min(100, (left / total) * 100)}%`;
    if (left > 0 && !$("nope-bar").hidden) countdownRaf = requestAnimationFrame(step);
  };
  step();
}

// ────────────────────────────────────────────────────────── the log panel

function setLogOpen(open) {
  setStoredLogOpen(open);
  $("log-panel").classList.toggle("collapsed", !open);
  $("log-caret").textContent = open ? "▾" : "▸";
  $("log-collapse").setAttribute("aria-expanded", String(open));
  $("log-collapse").title = open ? "Hide the log" : "Show the log";
  if (open) scrollLogToEnd(); // reopen at the newest line
}

function logLine(e) {
  const cards = (e.cards || []).map((c) => c.name).join(" + ");
  const who = nameOf(e.actorId);
  const li = document.createElement("li");
  let text, big = false;

  switch (e.kind) {
    case "joined":    text = `${who} joined`; break;
    case "started":   text = "The cards are dealt."; big = true; break;
    case "turn":      return null; // the banner already says whose turn it is
    case "nope_window": return null; // a UI state, not a log line
    case "resolved":  text = `↳ ${cards} takes effect`; break;
    case "played":
      text = `${who} played ${cards}` + (e.targetId ? ` on ${nameOf(e.targetId)}` : "");
      break;
    case "noped":     text = `${who} said NOPE`; break;
    case "cancelled": text = `✖ ${cards} was noped away`; break;
    // Events you were part of arrive privately too, naming the actual card, so
    // the vaguer public line would only duplicate them.
    case "drew":
      if (e.actorId === ctx.me()) return null;
      text = `${who} drew a card`;
      break;
    case "shuffled":  text = `${who} shuffled the deck`; break;
    // Which way play is going is shared information, so the log says so rather
    // than leaving people to work it out from whose turn came next.
    case "reversed":  text = `🔄 ${who} reversed the order of play`; big = true; break;
    // Where it went is secret; that it is live is not.
    case "armed":
      text = e.text ? `🌀 ${who} ${e.text}` : `🌀 ${who} drew the Imploding Kitten`;
      big = true;
      break;
    case "altered":   text = `🃏 ${who} rearranged the top of the deck`; break;
    // Both kittens end a game the same way, but naming the wrong one would be a
    // lie — and with the expansion in, the Imploding one is the notable death.
    case "exploded":
      text = (e.cards || []).some((c) => c.slug === "imploding")
        ? `🌀 ${who} drew the armed Imploding Kitten!`
        : `💥 ${who} drew an Exploding Kitten!`;
      big = true;
      break;
    case "defused":   text = e.text ? `${who} ${e.text}` : `${who} defused it`; break;
    case "eliminated":text = `☠️ ${who} ${who === "You" ? "are" : "is"} out`; big = true; break;
    // A demand is announced out loud, so unlike a random steal it is logged in
    // full for everybody — including the card named and whether it landed.
    case "demanded":
      text = `${who} demanded ${nameOf(e.targetId)}'s ${e.text}`;
      break;
    case "missed":
      text = `…${nameOf(e.targetId)} had no ${e.text}. Three cats wasted.`;
      break;
    case "stole":
      // The trio's transfer names the card, so it is public and worth showing.
      if (e.text) {
        text = `${who} took ${nameOf(e.targetId)}'s ${e.text}`;
        break;
      }
      if (e.actorId === ctx.me() || e.targetId === ctx.me()) return null;
      text = `${who} stole a card from ${nameOf(e.targetId)}`;
      break;
    case "gave":
      if (e.actorId === ctx.me() || e.targetId === ctx.me()) return null;
      text = `${who} gave a card to ${nameOf(e.targetId)}`;
      break;
    case "game_over": text = `🏆 ${who} wins!`; big = true; break;
    case "auto":      text = `${who} was away, so the table moved on`; break;
    default:          return null;
  }
  li.textContent = text;
  if (big) li.className = "big";
  return li;
}

// ────────────────────────────────────────────────────────── private events

// Private events carry information only this player is entitled to see. Each one
// gets a toast for the moment and a log line for the record — a toast is gone in
// three seconds, and what you drew is exactly the sort of thing you want to
// scroll back for later.
function handlePrivate(e) {
  const card = e.cards && e.cards[0];
  let note = "";

  switch (e.kind) {
    case "future":
      openModal("future", {
        title: "🔮 The next three cards",
        body: "Top of the deck first. Nobody else saw this.",
        cards: (e.cards || []).map((c) => cardEl(c, { static: true })),
        ok: "Got it",
      });
      note = `🔮 You saw: ${(e.cards || []).map((c) => c.name).join(", ")}`;
      logPrivate(note);
      return; // the modal is the announcement; no toast on top of it
    case "drew":
      if (!card) return;
      note = `You drew ${card.name}`;
      break;
    case "stole":
      note = e.actorId === ctx.me()
        ? `You stole ${card.name} from ${nameOf(e.targetId)}`
        : `${nameOf(e.actorId)} stole your ${card.name}`;
      break;
    case "gave":
      note = e.actorId === ctx.me()
        ? `You gave away ${card.name}`
        : `${nameOf(e.actorId)} gave you ${card.name}`;
      break;
    default:
      return;
  }

  ctx.toast(note);
  logPrivate(note);
}

// ────────────────────────────────────────────────────────── cinematics

// The bang and the skull go off over the seat they belong to; the defuse is the
// whole table's news, so it plays in the middle.
function cinematicFor(e) {
  const who = nameOf(e.actorId);
  const at = (id) => `[data-seat="${id}"]`;
  switch (e.kind) {
    case "exploded":
      return {
        kind: "exploded", glyph: "💥", text: `${who} drew an Exploding Kitten!`,
        anchor: at(e.actorId), ms: CINEMA_MS.exploded, sfx: "boom", vfx: true,
      };
    case "defused":
      if (e.text) return null; // the reinsertion, which has its own modal
      return { kind: "defused", glyph: "✂️", text: `${who} defused it`, ms: CINEMA_MS.defused };
    case "eliminated":
      return {
        kind: "eliminated", glyph: "☠️", text: `${who} ${who === "You" ? "are" : "is"} out`,
        anchor: at(e.actorId), ms: CINEMA_MS.eliminated,
      };
    default:
      return null;
  }
}

// The screen is clear again, so any prompt held back behind a flash may open.
function onCinemaIdle() {
  const v = ctx && ctx.view();
  if (v && v.started) renderPrompts(v);
}

// ────────────────────────────────────────────────────────── selection

function selectedCards(v) {
  return v.me.hand.filter((c) => me.selected.has(c.id));
}

// A Feral Cat is not one of the five collectible cats, but it plays as any of
// them — so for stacking purposes it counts, and for matching it is a wildcard.
const FERAL = "feral-cat";
const isCat = (slug) => slug.startsWith("cat-") || slug === FERAL;
const isWild = (slug) => slug === FERAL;

// kindOf is what a selection has committed to being: the slug of the first
// non-wild cat in it, or "" while it is still all Feral Cats and could become
// anything.
const kindOf = (sel) => (sel.find((c) => !isWild(c.slug)) || {}).slug || "";

// selectableWith says whether a card may join the current selection. Only two
// shapes of play exist, so anything else is refused at the point of tapping
// rather than allowed to build up into something the server would reject:
// a single non-cat card, or two-to-three cats that agree on a kind.
function selectableWith(card, sel) {
  if (sel.length === 0) return { ok: true };
  const first = sel[0];
  if (!isCat(first.slug)) {
    return { ok: false, why: `Only one card at a time — deselect ${first.name} first.` };
  }
  if (!isCat(card.slug)) {
    return { ok: false, why: `${card.name} can't join a cat set — deselect the cats first.` };
  }
  if (sel.length >= 3) return { ok: false, why: "Three is the biggest set." };

  const kind = kindOf(sel);
  if (!isWild(card.slug) && kind && card.slug !== kind) {
    const named = sel.find((c) => c.slug === kind);
    return { ok: false, why: `A set has to match: only another ${named.name} — or a Feral Cat.` };
  }
  return { ok: true };
}

let blockedTimer;
// flashBlocked explains a refused tap in the hint line, then clears itself, so a
// dimmed card is never silently unresponsive.
function flashBlocked(why) {
  me.blockedHint = why;
  clearTimeout(blockedTimer);
  blockedTimer = setTimeout(() => { me.blockedHint = ""; rerender(); }, 2600);
  rerender();
}

function toggleSelect(id) {
  const v = view();
  if (!v) return;

  if (me.selected.has(id)) {
    me.selected.delete(id);
    me.blockedHint = "";
    me.awaitingTarget = false;
    rerender();
    return;
  }

  const card = v.me.hand.find((c) => c.id === id);
  if (!card) return;
  const check = selectableWith(card, selectedCards(v));
  if (!check.ok) {
    flashBlocked(check.why);
    return;
  }
  me.selected.add(id);
  me.blockedHint = "";
  me.awaitingTarget = false;
  rerender();
}

// selectionPlan decides only whether the Play button lights up. The server
// re-checks everything; this exists so the UI can explain itself.
function selectionPlan(sel) {
  if (sel.length === 1) {
    const s = sel[0].slug;
    if (["skip", "attack", "shuffle", "future", "reverse", "bottom", "alter"].includes(s)) {
      return { ok: true, needsTarget: false };
    }
    if (s === "favor" || s === "targeted-attack") return { ok: true, needsTarget: true };
    if (s === "nope") return { ok: false, why: "Nope is played from the purple bar, not on your turn." };
    if (s === "defuse") return { ok: false, why: "Defuse is used automatically when you draw a kitten." };
    if (isCat(s)) return { ok: false, why: "Add a second matching cat to steal, or a third to demand." };
    return { ok: false, why: "" };
  }
  // A set agrees if every non-wild card in it is the same kind; Feral Cats stand
  // in for whatever the rest of the set is, so an all-Feral set works too.
  const agrees = () => {
    const kind = kindOf(sel);
    return sel.every((c) => isCat(c.slug) && (isWild(c.slug) || c.slug === kind));
  };
  if (sel.length === 2) {
    if (agrees()) return { ok: true, needsTarget: true };
    return { ok: false, why: "Two cats only work if they match, or one of them is Feral." };
  }
  if (sel.length === 3) {
    if (agrees()) return { ok: true, needsTarget: true, needsName: true };
    return { ok: false, why: "Three cats only work if they all match." };
  }
  return { ok: false, why: sel.length ? "That's too many cards." : "" };
}

// playSelection is the last step for everything except a three-cat set, which
// still needs a card named before it can be sent.
function playSelection(targetId) {
  const plan = selectionPlan(selectedCards(view()));
  if (plan.needsName) {
    me.awaitingTarget = false;
    openDemandModal(targetId);
    rerender();
    return;
  }
  sendPlay(targetId, "");
}

function sendPlay(targetId, named) {
  send({ type: "play", cardIds: [...me.selected], targetId, named });
  me.selected.clear();
  me.awaitingTarget = false;
  me.blockedHint = "";
  rerender();
}

// ────────────────────────────────────────────────────────── prompts

// renderPrompts opens the modal a phase demands, and closes one that no longer
// applies (an action can be noped out from under you).
//
// Modals wait for a flash to finish rather than opening underneath it; the cinema
// calls onCinemaIdle back here once the screen is clear.
function renderPrompts(v) {
  if (cinemaPlaying()) return;

  if (v.me.mustPlace && modalKind() !== "place") {
    openPlaceModal(v);
  } else if (v.me.mustGive && modalKind() !== "give") {
    openGiveModal(v);
  } else if (v.me.mustAlter && modalKind() !== "alter") {
    openAlterModal(v);
  } else if (v.phase === "game_over" && modalKind() !== "over") {
    openGameOverModal(v);
  } else if ((modalKind() === "place" && !v.me.mustPlace) ||
             (modalKind() === "give" && !v.me.mustGive) ||
             (modalKind() === "alter" && !v.me.mustAlter)) {
    closeModal();
  }
}

// openDemandModal is the second half of a Three of a Kind: the target is already
// chosen, now name the card. Everyone will hear the demand, so there is no need
// to keep the choice quiet.
function openDemandModal(targetId) {
  const kinds = catalogue.demandable;
  if (!kinds.length) {
    flashBlocked("Couldn't load the card list — reload the page and try again.");
    return;
  }

  const grid = document.createElement("div");
  grid.className = "demand-grid";
  grid.replaceChildren(...kinds.map((k) => {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "demand-pick";
    b.dataset.slug = k.slug;
    const g = document.createElement("span");
    g.className = "demand-glyph";
    g.textContent = GLYPHS[k.slug] || "🐱";
    const n = document.createElement("span");
    n.textContent = k.name;
    b.append(g, n);
    b.onclick = () => { closeModal(); sendPlay(targetId, k.slug); };
    return b;
  }));

  const { alt } = openModal("demand", {
    title: "Demand a card",
    body: `Name the card you want from ${nameOf(targetId)}. If they have one they must hand it over; if not, the three cats are spent for nothing.`,
    cards: [grid],
    alt: "Cancel",
  });
  // Cancelling has to put the cats back in play, not silently eat the turn.
  alt.onclick = () => { closeModal(); rerender(); };
}

function openGiveModal(v) {
  // Favor doesn't end the requester's turn, so the current player *is* the
  // person asking for the card.
  openModal("give", {
    title: "Hand one over",
    body: `${nameOf(v.currentId)} played Favor. You choose which card to give.`,
    cards: v.me.hand.map((c) => {
      const el = cardEl(c);
      el.onclick = () => { send({ type: "give", cardIds: [c.id] }); closeModal(); };
      return el;
    }),
  });
}

// The quick buttons and the slider are two ways at the same number, because "top"
// and "bottom" are what people actually want and hunting for either end of a
// range is fiddly on a phone.
function openPlaceModal(v) {
  const quick = document.createElement("div");
  quick.className = "place-quick";
  const PLACES = [
    ["top", "Top"], ["second", "2nd from top"], ["middle", "Middle"],
    ["random", "Random"], ["bottom", "Bottom"],
  ];

  const range = document.createElement("input");
  range.id = "place-range";
  range.type = "range";
  range.min = "0";
  range.max = String(v.deckCount);
  range.value = "0";

  const readoutEl = document.createElement("span");
  readoutEl.id = "place-readout";
  const readout = () => {
    const i = Number(range.value);
    readoutEl.textContent =
      i === 0 ? "Right on top — the next player gets it"
      : i >= v.deckCount ? "Very bottom of the deck"
      : `${i} card${i > 1 ? "s" : ""} down`;
  };
  range.oninput = readout;

  quick.replaceChildren(...PLACES.map(([key, label]) => {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "btn small";
    b.dataset.place = key;
    b.textContent = label;
    b.onclick = () => {
      const max = v.deckCount;
      range.value = String({
        top: 0,
        second: Math.min(1, max),
        middle: Math.floor(max / 2),
        random: Math.floor(Math.random() * (max + 1)),
        bottom: max,
      }[key]);
      readout();
    };
    return b;
  }));

  const slider = document.createElement("label");
  slider.className = "place-slider";
  slider.append(range, readoutEl);

  const confirm = document.createElement("button");
  confirm.id = "place-confirm";
  confirm.className = "btn primary";
  confirm.textContent = "Bury it";
  confirm.onclick = () => {
    send({ type: "place", index: Number(range.value) });
    closeModal();
  };

  const place = document.createElement("div");
  place.id = "modal-place";
  place.className = "place";
  place.append(quick, slider, confirm);

  // Both kittens are put back the same way; what differs is what it means. A
  // defused Exploding Kitten is one hazard among several. The Imploding Kitten
  // goes back face up, and the next player to reach it is simply out.
  const imploding = v.placing === "imploding";
  openModal("place", {
    title: imploding ? "🌀 Imploding Kitten!" : "💥 Defused!",
    body: imploding
      ? "It goes back face up. Whoever draws it next is out, and no Defuse will stop it. Nobody sees where you put it."
      : "Slide the kitten back into the deck wherever you like. Nobody sees where it goes.",
    extra: [place],
  });
  readout();
}

// openAlterModal is Alter the Future: you look at the top of the deck and put it
// back in whatever order you like.
//
// The cards arrive as faces with no identity — the server will not put a deck
// card's ID on the wire even for the player looking at it — so this works in
// positions throughout, and sends a permutation back.
function openAlterModal(v) {
  const faces = v.me.alter || [];
  if (!faces.length) return;

  // order[displayPosition] = the position that card started in.
  let order = faces.map((_, i) => i);
  let picked = -1;

  const grid = document.createElement("div");
  grid.className = "alter-grid";

  const draw = () => {
    grid.replaceChildren(...order.map((origin, pos) => {
      const face = faces[origin];
      const el = document.createElement("button");
      el.type = "button";
      el.className = "card alter-card" + (picked === pos ? " picked" : "");
      el.dataset.slug = face.slug;
      el.dataset.pos = String(pos);

      const label = document.createElement("span");
      label.className = "label";
      label.textContent = face.name;

      const src = artURL(face);
      if (src) {
        const img = document.createElement("img");
        img.className = "art";
        img.src = src;
        img.alt = "";
        img.onerror = () => { img.remove(); el.classList.remove("has-art"); el.prepend(glyphEl(face)); };
        el.classList.add("has-art");
        el.append(img, label);
      } else {
        el.append(glyphEl(face), label);
      }

      const seat = document.createElement("span");
      seat.className = "alter-pos";
      seat.textContent = pos === 0 ? "Next" : `#${pos + 1}`;
      el.append(seat);

      el.onclick = () => {
        if (picked === -1) picked = pos;
        else if (picked === pos) picked = -1;
        else {
          [order[picked], order[pos]] = [order[pos], order[picked]];
          picked = -1;
        }
        draw();
      };
      return el;
    }));
  };
  draw();

  const { ok } = openModal("alter", {
    title: "🃏 Alter the Future",
    body: faces.length > 1
      ? "The next card is on the left. Tap two to swap them, then put them back."
      : "One card left in the deck. Nothing to rearrange.",
    cards: [grid],
    ok: "Put them back",
  });
  ok.onclick = () => {
    send({ type: "alter", order });
    closeModal();
  };
}

function openGameOverModal(v) {
  const won = v.winnerId === ctx.me();
  const outcome = won ? "Everyone else exploded. Well played." : "Better luck next round.";
  const { ok, alt } = openModal("over", {
    title: won ? "🏆 You survived!" : `🏆 ${nameOf(v.winnerId)} survived`,
    // Only the host can act, so everyone else is told what they are waiting for
    // rather than being left on a dead table with a Close button.
    body: v.me.host
      ? outcome
      : `${outcome} Waiting for ${nameOf(hostId(v))} to deal again or reopen the lobby.`,
    ok: v.me.host ? "Deal again" : "Close",
    alt: v.me.host ? "Back to lobby" : "",
  });
  ok.onclick = () => {
    if (v.me.host) send({ type: "start" });
    closeModal();
  };
  // The lobby is the only way to pick up players who arrived mid-round: dealing
  // again seats whoever is present right now and drops everybody else.
  alt.onclick = () => {
    send({ type: "lobby" });
    closeModal();
  };
}

function hostId(v) {
  const h = v.seats.find((s) => s.host);
  return h ? h.id : "";
}

// ────────────────────────────────────────────────────────── the rules panel

const rulesOpen = () => { const p = $("rules-panel"); return Boolean(p) && !p.hidden; };
function showRules(on) { const p = $("rules-panel"); if (p) p.hidden = !on; }

// ────────────────────────────────────────────────────────── cards

function cardEl(card, { static: isStatic = false } = {}) {
  const el = document.createElement(isStatic ? "div" : "button");
  el.className = "card" + (isStatic ? " static" : "");
  el.dataset.slug = card.slug;
  el.dataset.id = card.id;
  if (!isStatic) el.type = "button";

  const l = document.createElement("span");
  l.className = "label";
  l.textContent = card.name;

  const src = artURL(card);
  if (src) {
    // The art carries the card's own title, but it is unreadable at hand size,
    // so the label stays on top as a legible strip.
    const img = document.createElement("img");
    img.className = "art";
    img.src = src;
    img.alt = "";
    img.loading = "lazy";
    img.decoding = "async";
    // A missing or broken file must not leave a blank card.
    img.onerror = () => { img.remove(); el.classList.remove("has-art"); el.prepend(glyphEl(card)); };
    el.classList.add("has-art");
    el.append(img, l);
  } else {
    el.append(glyphEl(card), l);
  }
  return el;
}

// A card seen from the back: the box art, same as the draw pile.
function backEl() {
  const el = document.createElement("button");
  el.type = "button";
  el.className = "card hidden-face";
  el.setAttribute("aria-label", "Face-down card — tap to look");
  return el;
}

function glyphEl(card) {
  const g = document.createElement("span");
  g.className = "glyph";
  g.textContent = GLYPHS[card.slug] || "🐱";
  return g;
}
