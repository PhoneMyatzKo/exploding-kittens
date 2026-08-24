// UNO — the client half of one game.
//
// The shell (../../app.js) owns the socket, the menu, the lobby and the screens
// every game shares. This module owns everything from the moment the cards are
// dealt: the table, the hand, the colour picker, the challenge prompt and the
// rules text. It never touches the socket directly and never reads the shell's
// state — both arrive through the ctx handed to mount().
//
// The server is authoritative for every rule. Nothing here checks a rule beyond
// deciding which buttons to light up: `view.me.playable` says what may be laid
// down, and everything else is sent as an intent and re-rendered from whatever
// comes back.

import { $ } from "../../core/dom.js";
import { logOpen, setStoredLogOpen } from "../../core/store.js";
import { openModal, closeModal, modalKind } from "../../core/modal.js";
import { renderLog, logPrivate, freshEvents, scrollLogToEnd } from "../../core/feed.js";
import * as cards from "./cards.js";

// ────────────────────────────────────────────────────────── state

// Everything here is this game's own, and is reset by mount() so a second round
// or a second room never inherits a stale choice.
const me = {
  // A wild waiting for its colour: the card is chosen, the colour is not, and
  // nothing has been sent yet.
  pendingWild: null,
  // Armed by the "Say UNO" button before the play that empties you down to one
  // card, so the shout goes out with the card rather than a round-trip later.
  armed: false,
};

// Handed in by the shell at mount. Declared here so every function below can
// reach it without threading it through a dozen signatures.
let ctx = null;

const nameOf = (id) => ctx.nameOf(id);
const send = (msg) => ctx.send(msg);
const rerender = () => ctx.rerender();

// ────────────────────────────────────────────────────────── the module

export default {
  // The cat portraits are Exploding Kittens' own, and a UNO table wearing them
  // looks like the wrong game. No picker in the lobby, no cat beside a name.
  avatars: false,

  // Mute-button slots this game's markup carries, beyond the shell's own.
  slots: ["sound-table"],

  mount(context) {
    ctx = context;
    me.pendingWild = null;
    me.armed = false;

    $("deck").onclick = () => {
      me.pendingWild = null;
      send({ type: "draw" });
    };
    $("uno-pass").onclick = () => send({ type: "pass" });
    $("log-collapse").onclick = () => setLogOpen(!logOpen());
    $("rules-table").onclick = () => showRules(true);
    $("rules-close").onclick = () => showRules(false);
    setLogOpen(logOpen());
  },

  unmount() {
    ctx = null;
  },

  // Called for every state the server sends, once the cards are dealt. The shell
  // handles the lobby itself; this is only ever the table.
  render(v) {
    $("table").hidden = false;
    renderTable(v);
  },

  // The shell has moved to one of its own screens. The prompt row goes with the
  // table; the rules panel deliberately does not, because the button that opens
  // it in the lobby is the shell's.
  leaveTable() {
    $("table").hidden = true;
  },

  // Runs before render(), which is what lets it defer anything that would open
  // over an animation. UNO has no cinematics — it advances the feed's counter so
  // a reconnect is still caught up in silence rather than replayed.
  onState(v) {
    freshEvents(v);
  },

  onPrivate(e) { handlePrivate(e); },

  // Escape closes the rules, then backs out of a colour choice that has not been
  // sent. Returns whether it used the key, so the shell can fall through to its
  // own handling.
  onEscape() {
    if (!$("rules-panel").hidden) { showRules(false); return true; }
    if (me.pendingWild !== null) { me.pendingWild = null; rerender(); return true; }
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
  renderPrompt(v);
  renderLog(v, logLine);
  renderGameOver(v);
}

function renderDeck(v) {
  $("deck-count").textContent = `${v.deckCount} left`;
  $("deck").disabled = !v.me.canDraw;

  const box = $("uno-colour");
  if (v.phase === "game_over" || !v.colour) {
    // No colour is in force between a wild landing and its colour being named,
    // which is exactly when the picker is up and saying so would be noise.
    box.replaceChildren();
    return;
  }
  const dot = document.createElement("i");
  dot.style.background = cards.INKS[v.colour] || "transparent";
  box.replaceChildren(dot, document.createTextNode(v.colour));
}

function renderTurnBanner(v) {
  const el = $("turn-banner");
  el.classList.toggle("mine", v.currentId === ctx.me());
  if (v.phase === "game_over") {
    const won = v.winnerId === ctx.me();
    el.textContent = `🏆 ${won ? "You" : nameOf(v.winnerId)} won with ${v.points} points`;
    return;
  }
  // The arrow is worth showing even at two players, where a Reverse is a Skip:
  // it is the only way to tell that the Reverse did anything at all.
  const arrow = v.direction < 0 ? " ↺" : " ↻";
  el.textContent = (v.currentId === ctx.me() ? "Your turn" : `${nameOf(v.currentId)}'s turn`) + arrow;
}

function renderSeats(v) {
  $("seats").replaceChildren(...v.seats.map((s) => {
    const div = document.createElement("div");
    div.className = "seat";
    div.dataset.seat = s.id;
    div.classList.toggle("current", s.current);
    div.classList.toggle("uno-one", !!s.uno);

    const nm = document.createElement("div");
    nm.className = "seat-name";
    const dot = document.createElement("i");
    dot.className = "dot" + (s.connected ? "" : " off");
    nm.append(ctx.avatarChip(s), dot,
      document.createTextNode(s.name + (s.id === ctx.me() ? " (you)" : "")));
    if (s.uno) {
      const badge = document.createElement("span");
      // Said out loud, or on one card and suspiciously quiet — the difference is
      // whether anybody may pounce, so it is the difference the badge shows.
      badge.className = "uno-badge" + (s.called ? "" : " quiet");
      badge.textContent = s.called ? "UNO!" : "1 card";
      nm.append(badge);
    }

    const meta = document.createElement("div");
    meta.className = "seat-meta";
    meta.textContent = `${s.handCount} card${s.handCount === 1 ? "" : "s"}`;
    div.append(nm, meta);
    return div;
  }));
}

function renderDiscard(v) {
  const box = $("discard");
  box.replaceChildren();
  if (!v.discardTop) return;
  const el = cards.element(v.discardTop);
  // A wild on top wears the colour it named, so the pile says what to play on
  // rather than only what was played.
  if (v.colour && cards.INKS[v.colour]) {
    el.style.boxShadow = `0 0 0 4px ${cards.INKS[v.colour]}, 0 6px 14px rgba(0,0,0,.45)`;
  }
  box.append(el);
}

function renderHand(v) {
  const playable = new Set(v.me.playable || []);
  $("hand").replaceChildren(...v.me.hand.map((c) => {
    const el = cards.element(c);
    const canPlay = playable.has(c.id);
    el.classList.toggle("playable", canPlay);
    el.classList.toggle("dim", !canPlay);
    el.classList.toggle("selected", me.pendingWild === c.id);
    el.tabIndex = 0;
    el.onclick = () => tapCard(v, c, canPlay);
    el.onkeydown = (e) => {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); tapCard(v, c, canPlay); }
    };
    return el;
  }));
}

// tapCard is the whole of playing a card. A wild stops here for its colour; a
// refused tap explains itself rather than doing nothing, which is the difference
// between a rule and a bug as far as the player can tell.
function tapCard(v, card, canPlay) {
  if (!canPlay) {
    hint(v.me.canPass
      ? "You've drawn — that one card is all you may play now."
      : v.me.myTurn
        ? `Doesn't match ${v.colour || "the pile"} or ${faceName(v.discardTop)}.`
        : "Not your turn yet.");
    return;
  }
  if (card.rank === "wild" || card.rank === "wild-draw-four") {
    me.pendingWild = card.id;
    rerender();
    return;
  }
  playCard(v, card.id);
}

function playCard(v, cardId, colour) {
  const msg = { type: "play", cardId };
  if (colour) msg.colour = colour;
  // Down to one card once this lands, and the button was armed: say it now, with
  // the card, exactly as you would at a table.
  if (v.me.hand.length === 2 && me.armed) msg.sayUno = true;
  me.pendingWild = null;
  me.armed = false;
  send(msg);
}

function renderActions(v) {
  const pass = $("uno-pass");
  pass.hidden = !v.me.canPass;

  const call = $("uno-call");
  if (v.me.canCallUno) {
    // The card is already down and the shout is late but still in time.
    call.hidden = false;
    call.textContent = "UNO!";
    call.onclick = () => send({ type: "uno" });
  } else if (v.me.hand.length === 2 && v.me.myTurn) {
    call.hidden = false;
    call.textContent = me.armed ? "UNO! ✓" : "Say UNO";
    call.onclick = () => { me.armed = !me.armed; rerender(); };
  } else {
    call.hidden = true;
    me.armed = false;
  }

  const catchBtn = $("uno-catch");
  if (v.me.catchId) {
    catchBtn.hidden = false;
    catchBtn.textContent = `Caught ${nameOf(v.me.catchId)}!`;
    catchBtn.onclick = () => send({ type: "catch" });
  } else {
    catchBtn.hidden = true;
  }

  if (v.phase === "game_over") hint("");
  else if (v.me.mustName) hint("Name a colour.");
  else if (v.me.mustAnswer) hint("Take the four, or call the bluff.");
  else if (v.me.canPass) hint("Play what you drew, or keep it and pass.");
  else if (v.me.myTurn && !(v.me.playable || []).length) hint("Nothing matches — draw a card.");
  else if (v.me.myTurn) hint("Play a card, or draw.");
  else hint("");
}

function hint(text) { $("hint").textContent = text; }

// renderPrompt drives the one row that blocks the table, in priority order:
// a colour for a wild in the hand, a colour for one already down, then a Draw
// Four waiting to be answered.
function renderPrompt(v) {
  const row = $("uno-prompt");
  const label = $("uno-prompt-label");
  const actions = $("uno-prompt-actions");

  // A wild we were about to play may have gone — a reconnect, or a state that
  // crossed with the tap.
  if (me.pendingWild !== null && !v.me.hand.some((c) => c.id === me.pendingWild)) {
    me.pendingWild = null;
  }

  if (me.pendingWild !== null) {
    label.textContent = "Play it as which colour?";
    actions.replaceChildren(
      ...colourButtons((colour) => playCard(v, me.pendingWild, colour)),
      cancelButton(() => { me.pendingWild = null; rerender(); }),
    );
    row.hidden = false;
    return;
  }

  if (v.me.mustName) {
    label.textContent = "Your wild is down. Which colour?";
    actions.replaceChildren(...colourButtons((colour) => send({ type: "colour", colour })));
    row.hidden = false;
    return;
  }

  if (v.me.mustAnswer) {
    const from = v.challenge ? nameOf(v.challenge.actorId) : "somebody";
    label.textContent = `${from} played a Wild Draw Four on you.`;
    const take = document.createElement("button");
    take.className = "btn primary";
    take.textContent = "Take 4";
    take.onclick = () => send({ type: "accept" });
    const call = document.createElement("button");
    call.className = "btn ghost";
    call.textContent = "Challenge!";
    call.title = "If they held the colour, they draw four instead — if not, you draw six";
    call.onclick = () => send({ type: "challenge" });
    actions.replaceChildren(take, call);
    row.hidden = false;
    return;
  }

  row.hidden = true;
  actions.replaceChildren();
}

function colourButtons(choose) {
  return ["red", "yellow", "green", "blue"].map((colour) => {
    const b = document.createElement("button");
    b.className = "uno-swatch";
    b.title = colour;
    b.setAttribute("aria-label", colour);
    b.innerHTML = cards.swatch(colour);
    b.onclick = () => choose(colour);
    return b;
  });
}

function cancelButton(onClick) {
  const b = document.createElement("button");
  b.className = "btn ghost small";
  b.textContent = "Cancel";
  b.onclick = onClick;
  return b;
}

// ────────────────────────────────────────────────────────── the log

function logLine(e) {
  const li = document.createElement("li");
  const who = nameOf(e.actorId);
  const them = nameOf(e.targetId);
  const played = (e.cards || []).map((c) => c.name).join(" + ");
  let text;
  let big = false;

  switch (e.kind) {
    case "joined":       text = `${who} joined`; break;
    case "started":      text = `The cards are dealt. ${played} is turned over.`; big = true; break;
    case "turn":         return null; // the banner already says whose turn it is
    case "played":       text = `${who} played ${played}`; break;
    case "colour":       text = `${who} called ${e.colour}`; break;
    case "drew":         text = `${who} drew ${e.count} card${e.count === 1 ? "" : "s"}${e.text ? ` — ${e.text}` : ""}`; break;
    case "passed":       text = `${who} passed`; break;
    case "skipped":      text = `${who} was skipped`; break;
    case "reversed":     text = "↩ Play turns around"; break;
    case "reshuffled":   text = `The pile was shuffled back into the deck (${e.count})`; break;
    case "uno_called":   text = `${who}: UNO!`; big = true; break;
    case "uno_caught":   text = `${who} caught ${them} silent on one card`; big = true; break;
    case "challenge":    return null; // the prompt row is the whole story
    case "bluffed":      text = `${who} called the bluff — and was right`; big = true; break;
    case "bluff_failed": text = `${who} challenged and was wrong`; break;
    case "revealed":     return null; // private, and handled as a modal
    case "round_over":   text = `${who} went out for ${e.points} points`; big = true; break;
    case "game_over":    text = `🏆 ${who} wins with ${e.points}`; big = true; break;
    case "auto":         text = `${who} was away — the table played for them`; break;
    default:             text = `${who} ${e.kind}`;
  }

  li.textContent = text;
  if (big) li.classList.add("big");
  return li;
}

// Private entries reach one player only: the cards you drew, and a hand a
// challenge entitled you to see.
function handlePrivate(e) {
  const names = (e.cards || []).map((c) => c.name);
  if (e.kind === "drew") {
    logPrivate(`You drew ${names.join(", ")}`);
    return;
  }
  if (e.kind === "revealed") {
    logPrivate(`${nameOf(e.actorId)}'s hand: ${names.join(", ")}`);
    openModal("revealed", {
      title: `${nameOf(e.actorId)}'s hand`,
      body: "What the challenge entitled you to see. Nobody else is shown this.",
      cards: (e.cards || []).map((c) => cards.element(c)),
      ok: "Close",
    });
  }
}

// ────────────────────────────────────────────────────────── odds and ends

function faceName(card) {
  if (!card) return "the pile";
  return card.rank === "wild" || card.rank === "wild-draw-four" ? "a wild" : card.rank;
}

function renderGameOver(v) {
  if (v.phase !== "game_over") {
    if (modalKind() === "uno-over") closeModal();
    return;
  }
  if (modalKind() === "uno-over") return;

  const won = v.winnerId === ctx.me();
  const outcome = won
    ? `You went out first, for ${v.points} points.`
    : `${nameOf(v.winnerId)} went out first, for ${v.points} points.`;
  const { ok, alt } = openModal("uno-over", {
    title: won ? "🏆 You won!" : `🏆 ${nameOf(v.winnerId)} won`,
    // Only the host can act, so everyone else is told what they are waiting for
    // rather than being left on a dead table with a Close button.
    body: v.me.host ? outcome : `${outcome} Waiting for ${nameOf(hostId(v))} to deal again.`,
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

function showRules(on) {
  $("rules-panel").hidden = !on;
}

function setLogOpen(open) {
  setStoredLogOpen(open);
  const panel = $("log-panel");
  panel.classList.toggle("collapsed", !open);
  $("log-collapse").setAttribute("aria-expanded", String(open));
  $("log-caret").textContent = open ? "▾" : "▸";
  $("log").hidden = !open;
  if (open) scrollLogToEnd();
}
