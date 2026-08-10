// Exploding Kittens — browser client.
//
// The server is authoritative for every rule. This file does no rules checking
// beyond deciding which buttons to light up: it renders `view`, sends intents,
// and re-renders whatever comes back.

const $ = (id) => document.getElementById(id);

const GLYPHS = {
  exploding: "💥", defuse: "✂️", nope: "🚫", attack: "⚔️", skip: "⏭️",
  favor: "🙏", shuffle: "🔀", future: "🔮",
  "cat-taco": "🌮", "cat-rainbow": "🌈", "cat-melon": "🍉",
  "cat-potato": "🥔", "cat-beard": "🧔",
};

// Scanned card faces, served from /cards. Slugs missing here fall back to the
// emoji glyph above, so partial art is fine.
const ART = {
  exploding: "Exploding-Kitten-Alien.jpg",
  defuse: "Defuse-Via-3AM-Flatulence.jpg",
  nope: "Nope-A-Jackanope-Bounds-into-the-Room.jpg",
  attack: "Attack-Bear-o-Dactyl.jpg",
  skip: "Skip-Commandeer-a-Bunnyraptor.jpg",
  favor: "Favor-Fall-So-Deeply-in-Love.jpg",
  "cat-beard": "Beard-Cat.jpg",
};

const artURL = (slug) => (ART[slug] ? `/cards/${encodeURIComponent(ART[slug])}` : "");

const app = {
  ws: null,
  view: null,          // latest server state
  me: "",              // our player id
  code: "",
  name: "",
  selected: new Set(), // card ids picked out of our hand
  awaitingTarget: false,
  modal: null,         // which modal is open, so render() doesn't reopen it
  windowEndsAt: 0,     // performance.now() deadline for the Nope countdown
  reconnectDelay: 500,
  closingOnPurpose: false,
};

// ────────────────────────────────────────────────────────── storage

const storedName = () => localStorage.getItem("ek:name") || "";
const setStoredName = (n) => localStorage.setItem("ek:name", n);
const tokenFor = (code) => localStorage.getItem(`ek:token:${code}`) || "";
const setTokenFor = (code, t) => localStorage.setItem(`ek:token:${code}`, t);

// ────────────────────────────────────────────────────────── networking

function connect(code) {
  app.code = code;
  app.closingOnPurpose = false;
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const url = new URL(`${proto}//${location.host}/ws`);
  url.searchParams.set("code", code);
  url.searchParams.set("name", app.name);
  url.searchParams.set("token", tokenFor(code));

  const ws = new WebSocket(url);
  app.ws = ws;

  ws.onopen = () => {
    app.reconnectDelay = 500;
    $("conn-warning").hidden = true;
  };
  ws.onmessage = (e) => {
    let msg;
    try { msg = JSON.parse(e.data); } catch { return; }
    handle(msg);
  };
  ws.onclose = () => {
    if (app.closingOnPurpose) return;
    $("conn-warning").hidden = false;
    setTimeout(() => connect(code), app.reconnectDelay);
    app.reconnectDelay = Math.min(app.reconnectDelay * 2, 8000);
  };
}

function send(msg) {
  if (app.ws && app.ws.readyState === WebSocket.OPEN) {
    app.ws.send(JSON.stringify(msg));
  }
}

function handle(msg) {
  switch (msg.type) {
    case "joined":
      app.me = msg.playerId;
      setTokenFor(msg.code, msg.token);
      location.hash = msg.code;
      break;
    case "state":
      app.view = msg;
      if (msg.pending) app.windowEndsAt = performance.now() + msg.pending.remainingMs;
      render();
      break;
    case "private":
      handlePrivate(msg.event);
      break;
    case "error":
      toast(msg.message);
      break;
    case "fatal":
      app.closingOnPurpose = true;
      showHome(msg.message);
      break;
  }
}

// Private events carry information only this player is entitled to see.
function handlePrivate(e) {
  const card = e.cards && e.cards[0];
  switch (e.kind) {
    case "future":
      openModal("future", {
        title: "🔮 The next three cards",
        body: "Top of the deck first. Nobody else saw this.",
        cards: e.cards,
        ok: "Got it",
      });
      break;
    case "drew":
      if (card) toast(`You drew ${card.name}`);
      break;
    case "stole":
      toast(e.actorId === app.me
        ? `You stole ${card.name} from ${nameOf(e.targetId)}`
        : `${nameOf(e.actorId)} stole your ${card.name}`);
      break;
    case "gave":
      toast(e.actorId === app.me
        ? `You gave away ${card.name}`
        : `${nameOf(e.actorId)} gave you ${card.name}`);
      break;
  }
}

// ────────────────────────────────────────────────────────── screens

function showHome(error) {
  $("home").hidden = false;
  $("lobby").hidden = true;
  $("table").hidden = true;
  $("nope-bar").hidden = true;
  closeModal();
  const box = $("home-error");
  box.hidden = !error;
  box.textContent = error || "";
}

function render() {
  const v = app.view;
  if (!v) return;

  if (!v.started) {
    $("home").hidden = true;
    $("table").hidden = true;
    $("lobby").hidden = false;
    renderLobby(v);
    return;
  }
  $("home").hidden = true;
  $("lobby").hidden = true;
  $("table").hidden = false;
  renderTable(v);
}

function renderLobby(v) {
  $("lobby-code").textContent = v.code;
  const list = $("lobby-players");
  list.replaceChildren(...v.seats.map((s) => {
    const li = document.createElement("li");
    const dot = document.createElement("i");
    dot.className = "dot" + (s.connected ? "" : " off");
    const nm = document.createElement("span");
    nm.textContent = s.name + (s.id === app.me ? " (you)" : "");
    li.append(dot, nm);
    if (s.host) {
      const tag = document.createElement("span");
      tag.className = "tag";
      tag.textContent = "host";
      li.append(tag);
    }
    return li;
  }));

  const n = v.seats.filter((s) => s.connected).length;
  const startable = n >= 2 && n <= 5;
  $("start-btn").hidden = !v.me.host;
  $("start-btn").disabled = !startable;
  $("lobby-hint").textContent = v.me.host
    ? (startable ? `${n} players ready.` : "Waiting for at least one more player…")
    : "Waiting for the host to deal…";
}

function renderTable(v) {
  $("table-code").textContent = v.code;
  $("deck-count").textContent = `${v.deckCount} left`;

  renderTurnBanner(v);
  renderSeats(v);
  renderDiscard(v);
  renderHand(v);
  renderActions(v);
  renderNopeBar(v);
  renderLog(v);
  renderPrompts(v);
}

function renderTurnBanner(v) {
  const el = $("turn-banner");
  el.classList.toggle("mine", v.currentId === app.me);
  if (v.phase === "game_over") {
    el.textContent = `🏆 ${nameOf(v.winnerId)} survived!`;
    return;
  }
  const extra = v.turnsRemaining > 1 ? ` · ${v.turnsRemaining} turns to take` : "";
  el.textContent = (v.currentId === app.me ? "Your turn" : `${nameOf(v.currentId)}'s turn`) + extra;
}

function renderSeats(v) {
  $("seats").replaceChildren(...v.seats.map((s) => {
    const div = document.createElement("div");
    div.className = "seat";
    div.classList.toggle("current", s.current);
    div.classList.toggle("dead", !s.alive);
    if (app.awaitingTarget && s.alive && s.id !== app.me) {
      div.classList.add("selectable");
      div.onclick = () => playSelection(s.id);
    }
    const nm = document.createElement("div");
    nm.className = "seat-name";
    const dot = document.createElement("i");
    dot.className = "dot" + (s.connected ? "" : " off");
    nm.append(dot, document.createTextNode(s.name + (s.id === app.me ? " (you)" : "")));
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
  for (const id of [...app.selected]) if (!held.has(id)) app.selected.delete(id);

  $("hand").replaceChildren(...v.me.hand.map((c) => {
    const el = cardEl(c);
    el.classList.toggle("selected", app.selected.has(c.id));
    el.onclick = () => toggleSelect(c.id);
    return el;
  }));
}

function renderActions(v) {
  const deck = $("deck");
  deck.disabled = !v.me.myTurn;
  deck.onclick = () => { app.selected.clear(); app.awaitingTarget = false; send({ type: "draw" }); };

  const sel = selectedCards(v);
  const plan = selectionPlan(sel);
  const playBtn = $("play-btn");
  playBtn.disabled = !(v.me.myTurn && plan.ok);
  playBtn.onclick = () => {
    if (plan.needsTarget) { app.awaitingTarget = true; render(); }
    else playSelection("");
  };

  $("hint").textContent = !v.me.alive ? "You're out — enjoy the show."
    : v.phase === "game_over" ? ""
    : !v.me.myTurn ? "Waiting…"
    : sel.length === 0 ? "Play a card, or draw to end your turn."
    : plan.ok ? (plan.needsTarget ? "Press Play, then pick a player." : "")
    : plan.why;

  const row = $("target-row");
  row.hidden = !app.awaitingTarget;
  if (app.awaitingTarget) {
    $("target-buttons").replaceChildren(...v.seats
      .filter((s) => s.alive && s.id !== app.me)
      .map((s) => {
        const b = document.createElement("button");
        b.className = "btn small";
        b.textContent = s.name;
        b.onclick = () => playSelection(s.id);
        return b;
      }));
  }
}

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
  const on = p.targetId ? ` on ${nameOf(p.targetId)}` : "";
  const stack = p.nopes > 0 ? ` — ${p.nopes} Nope${p.nopes > 1 ? "s" : ""} stacked, so it ${p.cancelled ? "will NOT happen" : "WILL happen"}` : "";
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

// Must match nopeWindow in internal/room/room.go — it is only the width of the
// bar, so drifting slightly is cosmetic, not a correctness problem.
const NOPE_WINDOW_MS = 7000;

let countdownRaf = 0;
function tickCountdown() {
  cancelAnimationFrame(countdownRaf);
  const step = () => {
    const left = Math.max(0, app.windowEndsAt - performance.now());
    $("nope-fill").style.width = `${(left / NOPE_WINDOW_MS) * 100}%`;
    if (left > 0 && !$("nope-bar").hidden) countdownRaf = requestAnimationFrame(step);
  };
  step();
}

function renderLog(v) {
  const lines = v.log.map(logLine).filter(Boolean);
  $("log").replaceChildren(...lines);
  $("log").scrollTop = $("log").scrollHeight;
}

// renderPrompts opens the modal a phase demands, and closes one that no longer
// applies (an action can be noped out from under you).
function renderPrompts(v) {
  if (v.me.mustPlace && app.modal !== "place") {
    openPlaceModal(v);
  } else if (v.me.mustGive && app.modal !== "give") {
    openGiveModal(v);
  } else if (v.phase === "game_over" && app.modal !== "over") {
    openGameOverModal(v);
  } else if ((app.modal === "place" && !v.me.mustPlace) ||
             (app.modal === "give" && !v.me.mustGive)) {
    closeModal();
  }
}

// ────────────────────────────────────────────────────────── selection

function selectedCards(v) {
  return v.me.hand.filter((c) => app.selected.has(c.id));
}

function toggleSelect(id) {
  if (app.selected.has(id)) app.selected.delete(id);
  else app.selected.add(id);
  app.awaitingTarget = false;
  render();
}

// selectionPlan decides only whether the Play button lights up. The server
// re-checks everything; this exists so the UI can explain itself.
function selectionPlan(sel) {
  if (sel.length === 1) {
    const s = sel[0].slug;
    if (["skip", "attack", "shuffle", "future"].includes(s)) return { ok: true, needsTarget: false };
    if (s === "favor") return { ok: true, needsTarget: true };
    if (s === "nope") return { ok: false, why: "Nope is played from the purple bar, not on your turn." };
    if (s === "defuse") return { ok: false, why: "Defuse is used automatically when you draw a kitten." };
    if (s.startsWith("cat-")) return { ok: false, why: "Pick a second matching cat to steal a card." };
    return { ok: false, why: "" };
  }
  if (sel.length === 2) {
    const [a, b] = sel;
    if (a.slug === b.slug && a.slug.startsWith("cat-")) return { ok: true, needsTarget: true };
    return { ok: false, why: "Two cats only work if they match." };
  }
  return { ok: false, why: sel.length ? "That's too many cards." : "" };
}

function playSelection(targetId) {
  send({ type: "play", cardIds: [...app.selected], targetId });
  app.selected.clear();
  app.awaitingTarget = false;
  render();
}

// ────────────────────────────────────────────────────────── modals

function openModal(kind, { title, body = "", cards = [], ok = "", onCard = null }) {
  app.modal = kind;
  $("modal").hidden = false;
  $("modal-title").textContent = title;
  $("modal-body").textContent = body;
  $("modal-place").hidden = true;
  $("modal-cards").replaceChildren(...cards.map((c) => {
    const el = cardEl(c, { static: !onCard });
    if (onCard) el.onclick = () => onCard(c);
    return el;
  }));
  const okBtn = $("modal-ok");
  okBtn.hidden = !ok;
  okBtn.textContent = ok;
  okBtn.onclick = closeModal; // callers that need more override this afterwards
}

function closeModal() {
  app.modal = null;
  $("modal").hidden = true;
}

function openGiveModal(v) {
  // Favor doesn't end the requester's turn, so the current player *is* the
  // person asking for the card.
  openModal("give", {
    title: "Hand one over",
    body: `${nameOf(v.currentId)} played Favor. You choose which card to give.`,
    cards: v.me.hand,
    onCard: (c) => { send({ type: "give", cardIds: [c.id] }); closeModal(); },
  });
}

function openPlaceModal(v) {
  openModal("place", {
    title: "💥 Defused!",
    body: "Slide the kitten back into the deck wherever you like. Nobody sees where it goes.",
  });
  $("modal-place").hidden = false;

  const range = $("place-range");
  range.max = String(v.deckCount);
  range.value = "0";
  const readout = () => {
    const i = Number(range.value);
    $("place-readout").textContent =
      i === 0 ? "Right on top — the next player gets it"
      : i >= v.deckCount ? "Very bottom of the deck"
      : `${i} card${i > 1 ? "s" : ""} down`;
  };
  range.oninput = readout;
  readout();

  for (const b of document.querySelectorAll("[data-place]")) {
    b.onclick = () => {
      const max = v.deckCount;
      range.value = String({
        top: 0,
        second: Math.min(1, max),
        middle: Math.floor(max / 2),
        random: Math.floor(Math.random() * (max + 1)),
        bottom: max,
      }[b.dataset.place]);
      readout();
    };
  }
  $("place-confirm").onclick = () => {
    send({ type: "place", index: Number(range.value) });
    closeModal();
  };
}

function openGameOverModal(v) {
  const won = v.winnerId === app.me;
  openModal("over", {
    title: won ? "🏆 You survived!" : `🏆 ${nameOf(v.winnerId)} survived`,
    body: won ? "Everyone else exploded. Well played." : "Better luck next round.",
    ok: v.me.host ? "Deal again" : "Close",
  });
  $("modal-ok").onclick = () => {
    if (v.me.host) send({ type: "start" });
    closeModal();
  };
}

// ────────────────────────────────────────────────────────── bits

function cardEl(card, { static: isStatic = false } = {}) {
  const el = document.createElement(isStatic ? "div" : "button");
  el.className = "card" + (isStatic ? " static" : "");
  el.dataset.slug = card.slug;
  el.dataset.id = card.id;
  if (!isStatic) el.type = "button";

  const l = document.createElement("span");
  l.className = "label";
  l.textContent = card.name;

  const src = artURL(card.slug);
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

function glyphEl(card) {
  const g = document.createElement("span");
  g.className = "glyph";
  g.textContent = GLYPHS[card.slug] || "🐱";
  return g;
}

function nameOf(id) {
  const s = app.view && app.view.seats.find((x) => x.id === id);
  if (!s) return "Someone";
  return s.id === app.me ? "You" : s.name;
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
    case "drew":      text = `${who} drew a card`; break;
    case "shuffled":  text = `${who} shuffled the deck`; break;
    case "exploded":  text = `💥 ${who} drew an Exploding Kitten!`; big = true; break;
    case "defused":   text = e.text ? `${who} ${e.text}` : `${who} defused it`; break;
    case "eliminated":text = `☠️ ${who} ${who === "You" ? "are" : "is"} out`; big = true; break;
    case "stole":     text = `${who} stole a card from ${nameOf(e.targetId)}`; break;
    case "gave":      text = `${who} gave a card to ${nameOf(e.targetId)}`; break;
    case "game_over": text = `🏆 ${who} wins!`; big = true; break;
    case "auto":      text = `${who} was away, so the table moved on`; break;
    default:          return null;
  }
  li.textContent = text;
  if (big) li.className = "big";
  return li;
}

const MAX_TOASTS = 3;

function toast(text) {
  const el = document.createElement("div");
  el.className = "toast";
  el.textContent = text;
  const box = $("toasts");
  box.append(el);
  // A flurry of steals and gifts can arrive at once; keep only the newest few.
  while (box.children.length > MAX_TOASTS) box.firstElementChild.remove();
  setTimeout(() => el.remove(), 3200);
}

// ────────────────────────────────────────────────────────── wiring

$("name-input").value = storedName();

function currentName() {
  const n = $("name-input").value.trim() || "Player";
  setStoredName(n);
  return n;
}

$("create-btn").onclick = async () => {
  app.name = currentName();
  try {
    const res = await fetch("/api/rooms", { method: "POST" });
    if (!res.ok) throw new Error("server said no");
    const { code } = await res.json();
    connect(code);
  } catch (err) {
    showHome("Couldn't reach the server. Is it still running?");
  }
};

$("join-form").onsubmit = (e) => {
  e.preventDefault();
  const code = $("code-input").value.trim().toUpperCase();
  if (!code) return;
  app.name = currentName();
  connect(code);
};

$("start-btn").onclick = () => send({ type: "start" });

$("copy-link").onclick = async () => {
  const url = `${location.origin}/#${app.code}`;
  try {
    await navigator.clipboard.writeText(url);
    toast("Invite link copied");
  } catch {
    toast(url);
  }
};

$("log-toggle").onclick = () => { $("log-panel").hidden = false; };
$("log-close").onclick = () => { $("log-panel").hidden = true; };
$("target-cancel").onclick = () => { app.awaitingTarget = false; render(); };

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && app.awaitingTarget) { app.awaitingTarget = false; render(); }
});

// A shared link (…/#ABCD) drops you straight into that room.
const hash = location.hash.replace("#", "").toUpperCase();
if (hash) {
  $("code-input").value = hash;
  if (storedName()) {
    app.name = storedName();
    connect(hash);
  }
}
