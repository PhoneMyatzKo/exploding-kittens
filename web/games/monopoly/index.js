// Monopoly Myanmar — the client half of one game.
//
// The shell (../../app.js) owns the socket, the menu, the lobby and the screens
// every game shares. This module owns everything from the deal onward: the board,
// the tokens, the dice, the prompts and the rules text. It never touches the
// socket directly and never reads the shell's state — both arrive through the ctx
// handed to mount().
//
// The server is authoritative for every rule. Nothing here decides whether a move
// is legal; it renders `view`, sends intents, and re-renders what comes back. The
// only thing it computes is where on screen a square goes.

import { $ } from "../../core/dom.js";
import { logOpen, setStoredLogOpen, storedLang, setStoredLang } from "../../core/store.js";
import { openModal, closeModal, modalKind } from "../../core/modal.js";
import { renderLog, freshEvents, resetFeed, scrollLogToEnd } from "../../core/feed.js";

// ────────────────────────────────────────────────────────── state

let ctx = null;

// The board: forty squares of names and prices, fetched once at mount. Static for
// the whole game, so it is not in the state payload — see GET /api/board.
const board = {
  tiles: [], cards: [], startingCash: 0, passGo: 0, jailFine: 0,
  houseSupply: 0, hotelSupply: 0,
};

const ui = {
  // The square whose detail card is open, or -1. Tapping a square opens it,
  // because the tiles are far too small to carry a full name and a rent table.
  inspecting: -1,
  // Which language the drawn card was built in. A card is built once and left
  // alone while it is being read — so without this, switching language leaves it
  // in the language it was drawn in, which is the one place on screen that would
  // not follow the toggle.
  cardLang: "",
};

// ────────────────────────────────────────────────────────── money and language

const lang = () => storedLang();

// Kyat, grouped. Written out in full wherever there is room, because "how much
// have I got" is the question the whole game is about.
const money = (n) => `K${Number(n || 0).toLocaleString("en-US")}`;

// The tile version. A square is ~40px wide on a phone and a seven-digit number
// will not fit, so thousands become "60k" and millions "1.5m".
function moneyShort(n) {
  const v = Number(n || 0);
  if (v >= 1_000_000) return `${+(v / 1_000_000).toFixed(1)}m`;
  if (v >= 1_000) return `${Math.round(v / 1_000)}k`;
  return String(v);
}

// UI strings that are not a square's name. Small enough to sit here rather than
// in a loader; the board's own names come from the server in both languages.
const STRINGS = {
  en: {
    roll: "Roll", rolling: "Rolling…", waiting: "Waiting…",
    yourTurn: "Your turn", turnOf: (n) => `${n}'s turn`,
    buy: (name, price) => `Buy ${name} for ${money(price)}`,
    decline: "No thanks", cash: "Cash", deeds: "Squares",
    out: "Out", won: (n) => `🏆 ${n} wins!`, youWon: "🏆 You win!",
    rentFor: "Rent", price: "Price", owner: "Owner", bank: "The bank",
    close: "Close", yours: "Yours", set: "Whole set — rent doubles",
    jailed: "You are in jail", tryDoubles: "Roll for a double",
    payFine: (n) => `Pay the ${money(n)} fine`, usePardon: "Use your free pardon",
    attempts: (n) => `${n} attempt${n === 1 ? "" : "s"} left`,
    houseCost: "House", houses: "Buildings", hotel: "Hotel",
    build: (n) => `Build for ${money(n)}`,
    buildHotel: (n) => `Build a hotel for ${money(n)}`,
    sell: (n) => `Sell one back for ${money(n)}`,
    needSet: "You need the whole colour set to build here",
    buildEven: "Build evenly — the rest of the set first",
    bankEmpty: "The bank has none left",
    stock: (houses, hotels) => `Bank: ${houses} houses, ${hotels} hotels`,
  },
  my: {
    roll: "လှိမ့်", rolling: "လှိမ့်နေသည်…", waiting: "စောင့်ဆိုင်းနေသည်…",
    yourTurn: "သင့်အလှည့်", turnOf: (n) => `${n} ၏ အလှည့်`,
    buy: (name, price) => `${name} ကို ${money(price)} ဖြင့် ဝယ်ရန်`,
    decline: "မဝယ်ပါ", cash: "ငွေ", deeds: "ကွက်",
    out: "ပွဲထွက်", won: (n) => `🏆 ${n} အောင်ပွဲရသည်!`, youWon: "🏆 သင် အောင်ပွဲရသည်!",
    rentFor: "အခွန်", price: "တန်ဖိုး", owner: "ပိုင်ရှင်", bank: "ဘဏ်",
    close: "ပိတ်", yours: "သင့်ပိုင်", set: "အစုံလိုက် — အခွန် နှစ်ဆ",
    jailed: "သင် အချုပ်ထဲ ရှိသည်", tryDoubles: "အံစာတူ လှိမ့်ကြည့်ပါ",
    payFine: (n) => `${money(n)} ဒဏ်ငွေ ပေးပါ`, usePardon: "အခမဲ့ထွက်ခွင့်ကဒ် အသုံးပြုပါ",
    attempts: (n) => `ကြိုးစားခွင့် ${n} ကြိမ် ကျန်သည်`,
    houseCost: "အိမ်တန်ဖိုး", houses: "အဆောက်အအုံ", hotel: "ဟိုတယ်",
    build: (n) => `${money(n)} ဖြင့် အိမ်ဆောက်ရန်`,
    buildHotel: (n) => `${money(n)} ဖြင့် ဟိုတယ်ဆောက်ရန်`,
    sell: (n) => `တစ်လုံး ${money(n)} ဖြင့် ပြန်ရောင်းရန်`,
    needSet: "ဆောက်ရန် အရောင်တစ်မျိုးလုံး ပိုင်ရမည်",
    buildEven: "အညီအမျှ ဆောက်ပါ — အစုံအတွင်း အခြားကွက်များ အရင်",
    bankEmpty: "ဘဏ်တွင် မကျန်တော့ပါ",
    stock: (houses, hotels) => `ဘဏ်: အိမ် ${houses}၊ ဟိုတယ် ${hotels}`,
  },
};
const t = () => STRINGS[lang()];

// A square's name in the language on screen. Both come from the server, so this
// never has to guess and never falls back to a slug.
const tileName = (tile) => (lang() === "my" && tile.nameMy ? tile.nameMy : tile.name);

// ────────────────────────────────────────── the board's geometry

// Where square i sits on an 11×11 grid, and which side of the board it is on.
// GO is the bottom-right corner and play runs clockwise on the board, which on
// screen means leftwards along the bottom, up the left side, rightwards along the
// top and down the right.
//
// The one piece of real arithmetic in this file, and the reason it is a function
// rather than forty hand-written positions: a hand-written table is forty chances
// to put a square in the wrong place, and the mistake looks like a rendering bug.
//
// The side matters as much as the position. It decides which edge the colour band
// is printed on and which way the name reads — both of which the stylesheet keys
// off data-side, so this is the only place that knows.
function gridPos(i) {
  const n = ((i % 40) + 40) % 40;
  if (n <= 10) return { row: 11, col: 11 - n, side: "bottom" }; // right to left
  if (n <= 20) return { row: 21 - n, col: 1, side: "left" };    // bottom to top
  if (n <= 30) return { row: 1, col: n - 19, side: "top" };     // left to right
  return { row: n - 29, col: 11, side: "right" };               // top to bottom
}

const isCorner = (i) => i % 10 === 0;

// The fifth building on a square is a hotel. The server sends it as a fifth
// level rather than a separate flag, because rent looks it up in the same slice
// — see HotelLevel in build.go.
const HOTEL_LEVEL = 5;

// The glyph a square without a colour band is known by. Properties are read by
// their band and get none — an icon on all forty would be noise.
const TILE_ICONS = {
  go: "➜",
  jail: "👮",
  parking: "🌴",
  gotojail: "🚔",
  chance: "❓",
  chest: "🎁",
  tax: "💸",
  station: "🚉",
};

// The two utilities are told apart by name rather than by kind, because "which
// utility" is the only thing that decides what it looks like.
function tileIcon(tile) {
  if (tile.kind === "utility") return /water|ရေ/i.test(tile.name + tile.nameMy) ? "💧" : "⚡";
  return TILE_ICONS[tile.kind] || "";
}

// ────────────────────────────────────────────────────────── the module

const mod = {
  slots: ["sound-table"],

  async mount(context) {
    ctx = context;
    ui.inspecting = -1;

    await loadBoard();
    buildBoard();
    applyLang(lang());
    // mount() is not awaited by the shell, and the board is a fetch — so a state
    // that arrived while it was in flight has already been rendered against an
    // empty grid. Draw it again now that there is a board to draw on.
    if (ctx && ctx.view()) ctx.rerender();

    $("lang-en").onclick = () => applyLang("en");
    $("lang-my").onclick = () => applyLang("my");
    $("log-collapse").onclick = () => setLogOpen(!logOpen());
    $("rules-table").onclick = () => showRules(true);
    $("rules-close").onclick = () => showRules(false);
    $("roll-btn").onclick = () => ctx.send({ type: "roll" });
    $("buy-btn").onclick = () => ctx.send({ type: "buy" });
    $("pass-btn").onclick = () => ctx.send({ type: "pass" });
    $("fine-btn").onclick = () => ctx.send({ type: "fine" });
    $("pardon-btn").onclick = () => ctx.send({ type: "pardon" });
    setLogOpen(logOpen());
  },

  unmount() {
    ctx = null;
  },

  render(v) {
    $("table").hidden = false;
    renderTable(v);
  },

  leaveTable() {
    $("table").hidden = true;
    if (modalKind() === "tile") closeModal();
    ui.inspecting = -1;
  },

  // No cinematics in this slice — a token sliding round the board is the beat
  // worth animating, and it belongs with the rest of the movement work rather
  // than bolted on now. The call still has to happen: freshEvents advances the
  // sequence counters the log diffing runs on.
  onState(v) {
    freshEvents(v);
  },

  onPrivate() {},

  onEscape() {
    if (rulesOpen()) { showRules(false); return true; }
    if (modalKind() === "tile") { closeModal(); ui.inspecting = -1; return true; }
    return false;
  },

  showRules,
};

export default mod;

// ────────────────────────────────────────────────────────── the board

async function loadBoard() {
  try {
    const res = await fetch(`/api/board?game=${encodeURIComponent(ctx.game() || "monopoly")}`);
    if (!res.ok) throw new Error(`server said ${res.status}`);
    const body = await res.json();
    board.tiles = body.tiles || [];
    board.cards = body.cards || [];
    board.startingCash = body.startingCash || 0;
    board.passGo = body.passGo || 0;
    board.jailFine = body.jailFine || 0;
    board.houseSupply = body.houseSupply || 0;
    board.hotelSupply = body.hotelSupply || 0;
  } catch {
    // Without the board there is nothing to draw, and a blank square grid would
    // look like a loading state that never ends.
    board.tiles = [];
    board.cards = [];
  }
}

// buildBoard writes the forty squares once. Everything that changes during a
// game — owners, tokens, the language — is set on these nodes by renderBoard
// rather than by rebuilding them, so a token move does not throw the board away.
function buildBoard() {
  const grid = $("board");
  if (!grid) return;

  grid.replaceChildren(...board.tiles.map((tile, i) => {
    const pos = gridPos(i);
    const el = document.createElement("button");
    el.type = "button";
    el.className = "tile";
    el.dataset.tile = String(i);
    el.dataset.kind = tile.kind;
    el.dataset.side = pos.side;
    if (tile.group) el.dataset.group = tile.group;
    el.classList.toggle("corner", isCorner(i));
    el.style.gridRow = String(pos.row);
    el.style.gridColumn = String(pos.col);

    // The colour band is what people actually read a Monopoly board by. Stations
    // and utilities get one too — they are bought and they charge rent, so they
    // are not scenery — and the stylesheet decides its colour from the kind.
    if (tile.group || tile.kind === "station" || tile.kind === "utility") {
      const band = document.createElement("span");
      band.className = "tile-band";
      el.append(band);
    }

    // Everything that reads as text goes inside one wrapper, because that is the
    // piece the stylesheet stands on its end for the side columns.
    const body = document.createElement("span");
    body.className = "tile-body";

    const icon = tileIcon(tile);
    if (icon) {
      const glyph = document.createElement("span");
      glyph.className = "tile-icon";
      glyph.textContent = icon;
      body.append(glyph);
    }

    const name = document.createElement("span");
    name.className = "tile-name";
    body.append(name);

    if (tile.price) {
      const price = document.createElement("span");
      price.className = "tile-price";
      price.textContent = moneyShort(tile.price);
      body.append(price);
    }
    el.append(body);

    // Buildings sit on the band, which is where they are printed on a real
    // board — and it is the one strip of a square that carries no text, so a row
    // of houses costs nothing that was being read.
    if (tile.group) {
      const houses = document.createElement("span");
      houses.className = "tile-houses";
      el.append(houses);
    }

    // Tokens are painted over the top rather than in the flow, so a piece
    // standing on a square does not push its name out of the way.
    const tokens = document.createElement("span");
    tokens.className = "tile-tokens";
    el.append(tokens);

    // Tapping a square opens its detail card. Even at this size a square cannot
    // carry a full name and a rent table, and "what does this cost" is the
    // question asked most often — so it is one tap rather than a guess.
    el.onclick = () => inspect(i);
    return el;
  }));
}

function renderTable(v) {
  $("table-code").textContent = v.code;
  renderBanner(v);
  renderSeats(v);
  renderBoard(v);
  renderCentre(v);
  renderLog(v, logLine);
  renderCard(v);
  if (ui.inspecting >= 0 && modalKind() === "tile") inspect(ui.inspecting, v);
}

// renderCard puts the drawn card up, or takes it down once it has been read.
// Guarded on the kind so it is not rebuilt on every state while somebody is
// reading it — and it must not fight the detail card, which is why inspecting is
// dropped when a card lands.
function renderCard(v) {
  const showing = modalKind() === "monopoly-card";
  if (v.drawn >= 0 && v.phase === "card") {
    // Rebuilt when the language changes, and otherwise left alone: it is being
    // read, and redrawing it on every state would flicker.
    if (!showing || ui.cardLang !== lang()) {
      ui.inspecting = -1;
      ui.cardLang = lang();
      openCardModal(v);
    }
    return;
  }
  if (showing) closeModal();
  ui.cardLang = "";
}

function renderBanner(v) {
  const el = $("turn-banner");
  el.classList.toggle("mine", v.currentId === ctx.me());
  if (v.phase === "gameOver") {
    el.textContent = v.winnerId === ctx.me() ? t().youWon : t().won(nameOf(v, v.winnerId));
    return;
  }
  el.textContent = v.currentId === ctx.me() ? t().yourTurn : t().turnOf(nameOf(v, v.currentId));
}

function renderSeats(v) {
  $("seats").replaceChildren(...(v.seats || []).map((s) => {
    const div = document.createElement("div");
    div.className = "seat";
    div.dataset.seat = s.id;
    div.classList.toggle("current", Boolean(s.current));
    div.classList.toggle("dead", !s.alive);

    const nm = document.createElement("div");
    nm.className = "seat-name";
    const dot = document.createElement("i");
    dot.className = "dot" + (s.connected ? "" : " off");
    // The token colour is the seat's, and it is the same colour drawn on the
    // board — which is the only way to tell whose piece is whose at that size.
    const pip = document.createElement("i");
    pip.className = "seat-pip";
    pip.style.background = tokenColour(v, s.id);
    nm.append(pip, ctx.avatarChip(s), dot,
      document.createTextNode(s.name + (s.id === ctx.me() ? " (you)" : "")));

    const meta = document.createElement("div");
    meta.className = "seat-meta";
    let line = s.alive ? `${money(s.cash)} · ${s.deeds} ${t().deeds}` : t().out;
    if (s.alive && s.buildings > 0) line = `${line} · 🏠${s.buildings}`;
    if (s.alive && s.jailed) line = `🔒 ${line}`;
    if (s.alive && s.pardons > 0) line = `${line} · 🎫${s.pardons > 1 ? s.pardons : ""}`;
    meta.textContent = line;

    div.append(nm, meta);
    return div;
  }));
}

function renderBoard(v) {
  const grid = $("board");
  if (!grid) return;

  // Owners first, then tokens, so a square that changed hands and a piece that
  // moved onto it are both right after one pass.
  for (const el of grid.querySelectorAll(".tile")) {
    const i = Number(el.dataset.tile);
    const owner = (v.owner || [])[i] || "";
    el.classList.toggle("owned", Boolean(owner));
    el.classList.toggle("mine", owner === ctx.me());
    // The whole square is tinted in the owner's colour, so the stylesheet needs
    // the colour rather than an element to paint.
    if (owner) el.style.setProperty("--own", tokenColour(v, owner));
    else el.style.removeProperty("--own");
    el.title = owner ? `${tileName(board.tiles[i] || {})} — ${nameOf(v, owner)}` : "";

    const name = el.querySelector(".tile-name");
    name.textContent = tileName(board.tiles[i] || {}) || "";
    // Tagged per element, not on the document: the stylesheet gives Burmese its
    // own size and line height, and a square is the smallest thing that has to
    // get that right.
    name.lang = lang();
    el.querySelector(".tile-tokens").replaceChildren();

    // Buildings: four little houses, or one hotel. Drawn as elements rather
    // than as an emoji count, because 🏠🏠🏠🏠 will not fit across a square at
    // this size and a hotel has to be visibly a different thing.
    const houses = el.querySelector(".tile-houses");
    if (houses) {
      const level = Number((v.houses || [])[i] || 0);
      houses.replaceChildren();
      houses.dataset.level = String(level);
      if (level >= HOTEL_LEVEL) {
        const hotel = document.createElement("i");
        hotel.className = "hotel";
        houses.append(hotel);
      } else {
        for (let n = 0; n < level; n++) houses.append(document.createElement("i"));
      }
    }
  }

  for (const s of v.seats || []) {
    if (!s.alive) continue;
    const el = grid.querySelector(`.tile[data-tile="${s.pos}"]`);
    if (!el) continue;
    const token = document.createElement("i");
    token.className = "token";
    token.style.background = tokenColour(v, s.id);
    token.title = s.name;
    if (s.id === ctx.me()) token.classList.add("me");
    el.querySelector(".tile-tokens").append(token);
  }
}

// The pips a die face is made of, as positions on a three-by-three grid. Drawn
// rather than written with the ⚀-⚅ characters, which come out at wildly
// different weights across platforms and are illegible at this size.
const PIPS = {
  1: [5],
  2: [1, 9],
  3: [1, 5, 9],
  4: [1, 3, 7, 9],
  5: [1, 3, 5, 7, 9],
  6: [1, 3, 4, 6, 7, 9],
};

function dieEl(face) {
  const d = document.createElement("span");
  d.className = "die";
  d.dataset.face = String(face);
  d.setAttribute("aria-label", String(face));
  const spots = PIPS[face] || [];
  for (let cell = 1; cell <= 9; cell++) {
    const pip = document.createElement("i");
    if (!spots.includes(cell)) pip.style.visibility = "hidden";
    d.append(pip);
  }
  return d;
}

function renderCentre(v) {
  const dice = $("dice");
  const [a, b] = v.dice || [0, 0];
  dice.replaceChildren();
  if (a > 0 && b > 0) {
    dice.append(dieEl(a), dieEl(b));
    const total = document.createElement("span");
    total.className = "die-total";
    total.textContent = String(a + b);
    dice.append(total);
  }

  const jailed = Boolean(v.me && v.me.jailed) && v.phase === "jail";

  const roll = $("roll-btn");
  // In jail the same button throws for a double instead of moving — same
  // message, so it is the same button rather than a second one that does
  // almost the same thing.
  roll.textContent = jailed ? t().tryDoubles : t().roll;
  roll.disabled = !(v.me && v.me.canRoll);
  roll.hidden = v.phase === "gameOver";

  const offer = v.me ? v.me.offer : -1;
  const buy = $("buy-btn");
  const pass = $("pass-btn");
  const open = offer >= 0 && offer < board.tiles.length;
  buy.hidden = !open;
  pass.hidden = !open;
  if (open) {
    const tile = board.tiles[offer];
    buy.textContent = t().buy(tileName(tile), tile.price);
    buy.disabled = (v.me.cash || 0) < tile.price;
    pass.textContent = t().decline;
  }

  const fine = $("fine-btn");
  const pardonBtn = $("pardon-btn");
  fine.hidden = !(v.me && v.me.canPayFine);
  pardonBtn.hidden = !(v.me && v.me.canPardon);
  if (!fine.hidden) fine.textContent = t().payFine(board.jailFine);
  if (!pardonBtn.hidden) pardonBtn.textContent = t().usePardon;

  $("board-status").textContent =
    v.phase === "gameOver" ? ""
    : jailed ? t().jailed
    : open ? ""
    : v.me && v.me.canRoll ? t().yourTurn
    : t().waiting;
}

// tokenColour is a seat's colour, taken from its position in the seat list rather
// than stored on the server: it is presentation, and every client builds the same
// answer from the same list.
const TOKENS = ["#e0393e", "#2e9e4f", "#3572b0", "#f5a623", "#8e44ad", "#00a3a3"];
function tokenColour(v, id) {
  const i = (v.seats || []).findIndex((s) => s.id === id);
  return i < 0 ? "#888" : TOKENS[i % TOKENS.length];
}

const nameOf = (v, id) => {
  const seat = (v.seats || []).find((s) => s.id === id);
  return seat ? seat.name : "somebody";
};

// ────────────────────────────────────────── the two decks

// Burmese digits. The card text was written with them — "၁ လှည့်နားပါ" — so the
// amounts and counts this file generates have to match rather than sitting in
// Latin numerals beside them.
const MY_DIGITS = "၀၁၂၃၄၅၆၇၈၉";
const burmeseNumerals = (s) => String(s).replace(/[0-9]/g, (d) => MY_DIGITS[+d]);

// localNumber writes a number the way the language on screen writes numbers.
const localNumber = (n) => {
  const grouped = Number(n || 0).toLocaleString("en-US");
  return lang() === "my" ? burmeseNumerals(grouped) : grouped;
};

// One line of plain language per effect, so a player is told what a card does
// rather than left to infer it from their cash going down. Generated here rather
// than written into the pack: the pack is the joke, this is the rule.
function effectLine(e) {
  const my = lang() === "my";
  const money = `${my ? "ကျပ် " : "K"}${localNumber(Math.abs(e.money || 0))}`;
  const squares = localNumber(Math.abs(e.steps || 0));
  const turns = localNumber(e.turns || 1);

  switch (e.kind) {
    case "money":
      if ((e.money || 0) >= 0) return my ? `${money} ယူပါ။` : `Collect ${money}.`;
      return my
        ? `${money} ပေးပါ။${e.orJail ? " မပေးနိုင်လျှင် အချုပ်သို့ သွားပါ။" : ""}`
        : `Pay ${money}.${e.orJail ? " If you cannot, go to jail." : ""}`;
    case "move":
      if ((e.steps || 0) > 0) return my ? `ရှေ့ ${squares} ကွက် တိုးပါ။` : `Move forward ${squares} squares.`;
      return my ? `နောက် ${squares} ကွက် ဆုတ်ပါ။` : `Go back ${squares} squares.`;
    case "skip":
      return my ? `${turns} လှည့် နားပါ။` : `Miss ${turns} turn${(e.turns || 1) > 1 ? "s" : ""}.`;
    case "jail":
      return my ? "အချုပ်သို့ သွားပါ။" : "Go to jail.";
    case "pardon":
      return my
        ? "ဒီကဒ်ကို သိမ်းထားပါ။ အချုပ်ကျလျှင် အခမဲ့ထွက်နိုင်သည်။"
        : "Keep this card. It gets you out of jail once, free.";
    case "release":
      return my ? "အချုပ်မှ ချက်ချင်း ထွက်ပါ။" : "Out of jail, straight away.";
    default:
      return "";
  }
}

// openCardModal turns the drawn card face up on everybody's screen. Public on
// purpose: at a table the card is read out, and watching somebody else's luck is
// half of why the deck is there. Only the player who drew it gets the button.
function openCardModal(v) {
  const card = board.cards[v.drawn];
  if (!card) return;
  const my = lang() === "my";

  const face = document.createElement("div");
  face.className = "draw-card";
  face.dataset.deck = card.deck;

  const emoji = document.createElement("span");
  emoji.className = "draw-emoji";
  emoji.textContent = card.emoji;

  const title = document.createElement("strong");
  title.className = "draw-title";
  title.textContent = my ? card.titleMy : card.title;
  title.lang = lang();

  const flavour = document.createElement("p");
  flavour.className = "draw-flavour";
  flavour.textContent = my ? card.flavourMy : card.flavour;
  flavour.lang = lang();

  const effects = document.createElement("p");
  effects.className = "draw-effect";
  effects.textContent = (card.effects || []).map(effectLine).filter(Boolean).join(" ");
  effects.lang = lang();

  face.append(emoji, title, flavour, effects);

  const deckName = card.deck === "chest"
    ? (my ? "ရပ်ရွာရန်ပုံငွေ" : "Community Chest")
    : (my ? "ကံစမ်း" : "Chance");

  const { ok } = openModal("monopoly-card", {
    title: deckName,
    // Whose card it is, since everybody is looking at it.
    body: v.currentId === ctx.me()
      ? ""
      : (my ? `${nameOf(v, v.currentId)} ၏ ကဒ်` : `${nameOf(v, v.currentId)}'s card`),
    extra: [face],
    ok: v.me && v.me.mustRead ? (my ? "ကောင်းပြီ" : "Got it") : "",
  });
  // Reading it is what applies it, so the button sends rather than just closing.
  if (v.me && v.me.mustRead) ok.onclick = () => ctx.send({ type: "read" });
}

// ────────────────────────────────────────────────────────── the detail card

// inspect opens one square's card: the full name in both languages, what it
// costs, what it charges, and who holds it. Re-called on every state while open
// so an owner changing under you is visible without closing it.
function inspect(i, view) {
  const tile = board.tiles[i];
  if (!tile) return;
  ui.inspecting = i;
  const v = view || ctx.view();

  const rows = document.createElement("dl");
  rows.className = "tile-detail";

  const add = (label, value) => {
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.textContent = value;
    rows.append(dt, dd);
  };

  const level = Number((v && v.houses ? v.houses[i] : 0) || 0);

  if (tile.price) add(t().price, money(tile.price));
  if (tile.kind === "property") {
    // The whole ladder, not just what it charges today. "What is this worth if I
    // finish the set" is the question building exists to answer, and a player
    // who can only see the current figure cannot answer it.
    add(t().rentFor, rentLadder(tile, level));
    if (tile.house) add(t().houseCost, money(tile.house));
    if (level > 0) add(t().houses, level >= HOTEL_LEVEL ? `🏨 ${t().hotel}` : "🏠".repeat(level));
  }
  if (tile.tax) add(t().rentFor, money(tile.tax));

  const owner = v && v.owner ? v.owner[i] : "";
  if (tile.price) {
    add(t().owner, !owner ? t().bank : owner === ctx.me() ? t().yours : nameOf(v, owner));
  }

  const { alt } = openModal("tile", {
    // Both names, always: the one on the board is in the language on screen, and
    // the other is the one somebody at the table is using.
    title: tileName(tile),
    body: tile.nameMy && tile.name !== tile.nameMy
      ? (lang() === "my" ? tile.name : tile.nameMy)
      : "",
    extra: [rows, buildingControls(v, i, tile, level)],
    alt: t().close,
  });
  alt.onclick = () => { ui.inspecting = -1; closeModal(); };
}

// rentLadder is the rent at every level, with the one in force marked. Written
// as one line rather than six rows because the detail card has to stay short
// enough to read on a phone without scrolling.
function rentLadder(tile, level) {
  const rents = tile.rent || [];
  if (!rents.length) return money(0);
  return rents
    .map((r, n) => (n === level ? `[${money(r)}]` : money(r)))
    .join(" · ");
}

// buildingControls is the Build and Sell buttons for one square, and the reason
// a refusal is explained rather than left as a dead button.
//
// What is offered comes straight from the server's canBuild/canSell lists — the
// even-build rule and the bank's stock are its to work out, and a client that
// decided for itself would be a second copy of the rule that could disagree.
// What is *explained* is worked out here, because a reason is presentation: the
// server has already said no by leaving the square out of the list.
function buildingControls(v, i, tile, level) {
  const box = document.createElement("div");
  box.className = "build-row";
  if (tile.kind !== "property" || !v || !v.me) return box;

  const mine = (v.owner || [])[i] === ctx.me();
  const canBuild = (v.me.canBuild || []).includes(i);
  const canSell = (v.me.canSell || []).includes(i);

  if (canBuild) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.id = "build-btn";
    btn.className = "btn primary small";
    btn.textContent = level === HOTEL_LEVEL - 1
      ? t().buildHotel(tile.house)
      : t().build(tile.house);
    btn.onclick = () => ctx.send({ type: "build", tile: i });
    box.append(btn);
  }
  if (canSell) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.id = "sell-btn";
    btn.className = "btn ghost small";
    btn.textContent = t().sell(Math.floor((tile.house || 0) / 2));
    btn.onclick = () => ctx.send({ type: "sell", tile: i });
    box.append(btn);
  }

  // Why not, when it is your square and there is no button on it. Only for the
  // owner: telling somebody else's landlord what they are missing is noise.
  if (mine && !canBuild && !canSell) {
    const why = document.createElement("p");
    why.className = "build-why";
    why.textContent = whyNotBuildable(v, i, tile, level);
    why.lang = lang();
    if (why.textContent) box.append(why);
  }

  // The bank's stock, whenever this square could otherwise take a building. It
  // is a rule that it runs out, so it has to be visible before it does.
  if (mine && level < HOTEL_LEVEL) {
    const stock = document.createElement("p");
    stock.className = "build-stock";
    stock.textContent = t().stock(v.housesLeft || 0, v.hotelsLeft || 0);
    stock.lang = lang();
    box.append(stock);
  }
  return box;
}

function whyNotBuildable(v, i, tile, level) {
  if (level >= HOTEL_LEVEL) return "";
  // A set is complete when every square sharing this colour is the viewer's.
  const wholeSet = board.tiles.every((other, n) =>
    other.group !== tile.group || (v.owner || [])[n] === ctx.me());
  if (!wholeSet) return t().needSet;
  if (level === HOTEL_LEVEL - 1 ? !v.hotelsLeft : !v.housesLeft) return t().bankEmpty;
  // Level across the set, or somebody else's turn — the first is the rule worth
  // naming, and it is the one that surprises people.
  const lowest = board.tiles.reduce(
    (low, other, n) => (other.group === tile.group
      ? Math.min(low, Number((v.houses || [])[n] || 0)) : low),
    HOTEL_LEVEL);
  if (level > lowest) return t().buildEven;
  return "";
}

// ────────────────────────────────────────────────────────── chrome

function applyLang(next) {
  setStoredLang(next);
  for (const [id, on] of [["lang-en", next === "en"], ["lang-my", next === "my"]]) {
    const el = $(id);
    if (!el) continue;
    el.classList.toggle("on", on);
    el.setAttribute("aria-checked", String(on));
  }
  for (const s of document.querySelectorAll(".rules-lang")) {
    s.hidden = s.dataset.lang !== next;
  }
  document.documentElement.lang = next === "my" ? "my" : "en";

  // The log is the one thing a re-render does not fix. It is built by appending
  // only what is new, so every line already on screen keeps the language it was
  // written in — switching mid-game left a Burmese board above an English
  // play-by-play. Resetting the sequence counters makes the next render rebuild
  // the whole list from the entries, which the server still has in full.
  resetFeed();

  // Everything else is drawn from the view, so a re-render is what applies it.
  if (ctx && ctx.view()) ctx.rerender();
}

const rulesOpen = () => { const p = $("rules-panel"); return Boolean(p) && !p.hidden; };
function showRules(on) { const p = $("rules-panel"); if (p) p.hidden = !on; }

function setLogOpen(open) {
  setStoredLogOpen(open);
  $("log-panel").classList.toggle("collapsed", !open);
  $("log-caret").textContent = open ? "▾" : "▸";
  $("log-collapse").setAttribute("aria-expanded", String(open));
  if (open) scrollLogToEnd();
}

// logLine turns one server entry into a line of the play-by-play. Every amount
// arrives as a number so it can be written in kyat here, in either language.
function logLine(e) {
  const v = ctx.view() || {};
  const who = e.actorId === ctx.me() ? (lang() === "my" ? "သင်" : "You") : nameOf(v, e.actorId);
  const square = e.tile !== undefined && e.tile !== null ? tileName(board.tiles[e.tile] || {}) : "";
  const li = document.createElement("li");
  let text = "";
  let big = false;

  switch (e.kind) {
    case "started": text = lang() === "my" ? "ကစားပွဲ စတင်သည်။" : "The game begins."; big = true; break;
    case "rolled":
      text = lang() === "my"
        ? `${who} ${(e.dice || []).join(" + ")} လှိမ့်သည်`
        : `${who} rolled ${(e.dice || []).join(" + ")}`;
      break;
    case "moved":
      text = lang() === "my" ? `↳ ${square} သို့` : `↳ to ${square}`;
      break;
    case "passedGo":
      text = lang() === "my"
        ? `${who} စတင်ကွက်ကို ဖြတ်၍ ${money(e.count)} ရသည်`
        : `${who} passed GO and collected ${money(e.count)}`;
      break;
    case "bought":
      text = lang() === "my"
        ? `${who} ${square} ကို ${money(e.count)} ဖြင့် ဝယ်သည်`
        : `${who} bought ${square} for ${money(e.count)}`;
      break;
    case "built": {
      // The level comes with the entry rather than being read off the board: by
      // the time somebody scrolls back to this line the square has moved on.
      const hotel = e.houses >= HOTEL_LEVEL;
      text = lang() === "my"
        ? `${hotel ? "🏨" : "🏠"} ${who} ${square} တွင် ${hotel ? "ဟိုတယ်" : "အိမ်"} ဆောက်သည် (${money(e.count)})`
        : `${hotel ? "🏨" : "🏠"} ${who} built a ${hotel ? "hotel" : "house"} on ${square} for ${money(e.count)}`;
      big = hotel;
      break;
    }
    case "sold":
      text = lang() === "my"
        ? `${who} ${square} မှ တစ်လုံး ${money(e.count)} ဖြင့် ပြန်ရောင်းသည်`
        : `${who} sold a building on ${square} back for ${money(e.count)}`;
      break;
    case "declined":
      text = lang() === "my" ? `${who} ${square} ကို မဝယ်ပါ` : `${who} passed on ${square}`;
      break;
    case "rent":
      text = lang() === "my"
        ? `${who} ${nameOf(v, e.targetId)} ကို ${square} အခွန် ${money(e.count)} ပေးသည်`
        : `${who} paid ${nameOf(v, e.targetId)} ${money(e.count)} for ${square}`;
      break;
    case "tax":
      text = lang() === "my"
        ? `${who} ${square} ${money(e.count)} ပေးသည်`
        : `${who} paid ${money(e.count)} — ${square}`;
      break;
    case "jailed":
      text = lang() === "my" ? `${who} အချုပ်သို့ ရောက်သည်` : `${who} was sent to jail`;
      big = true;
      break;
    case "card": {
      // Named rather than summarised: the joke is the card, and a log that said
      // only "drew a card" would throw away the half of the deck that is fun.
      const card = board.cards[e.card] || {};
      const title = lang() === "my" ? card.titleMy : card.title;
      const deck = card.deck === "chest"
        ? (lang() === "my" ? "ရပ်ရွာရန်ပုံငွေ" : "Community Chest")
        : (lang() === "my" ? "ကံစမ်း" : "Chance");
      text = lang() === "my"
        ? `${card.emoji || "🃏"} ${who} ${deck}: ${title || ""}`
        : `${card.emoji || "🃏"} ${who} drew ${deck}: ${title || ""}`;
      big = true;
      break;
    }
    case "cardPay":
      if ((e.count || 0) >= 0) {
        text = lang() === "my"
          ? `↳ ${who} ${money(e.count)} ရသည်`
          : `↳ ${who} collected ${money(e.count)}`;
      } else {
        text = lang() === "my"
          ? `↳ ${who} ${money(-e.count)} ပေးရသည်`
          : `↳ ${who} paid ${money(-e.count)}`;
      }
      break;
    case "missTurn":
      text = lang() === "my" ? `↳ ${who} အလှည့် နားရသည်` : `↳ ${who} misses a turn`;
      break;
    case "fine":
      text = lang() === "my"
        ? `${who} ဒဏ်ငွေ ${money(e.count)} ပေးပြီး အချုပ်မှ ထွက်သည်`
        : `${who} paid a ${money(e.count)} fine and left jail`;
      break;
    case "freed":
      text = lang() === "my" ? `🔓 ${who} အချုပ်မှ ထွက်သည်` : `🔓 ${who} is out of jail`;
      break;
    case "pardonUsed":
      text = lang() === "my"
        ? `🎫 ${who} အခမဲ့ထွက်ခွင့်ကဒ် အသုံးပြုသည်`
        : `🎫 ${who} used a get-out-of-jail card`;
      break;
    case "bankrupt":
      text = lang() === "my" ? `${who} ပွဲထွက်သွားသည်` : `${who} is out of the game`;
      big = true;
      break;
    case "won":
      text = lang() === "my" ? `🏆 ${who} အောင်ပွဲရသည်!` : `🏆 ${who} wins!`;
      big = true;
      break;
    case "turn":
      // Not worth a line of its own: the banner already says whose turn it is,
      // and one of these after every roll would be most of the log.
      return null;
    default:
      return null;
  }

  li.textContent = text;
  if (big) li.className = "big";
  if (e.actorId === ctx.me()) li.classList.add("mine");
  return li;
}
