// Board Games — the shell.
//
// This file is everything that is not one game: the socket and its reconnect,
// the menu, the name and visibility choices, the public lobby browser, the lobby
// and its portraits, the palette, and the mounting of a game on top of all that.
// It knows a room has seats and a log; it does not know what a card is.
//
// One game's own client lives in web/games/<slug>/index.js and is loaded on
// demand. The contract between the two is small and is documented at mountGame()
// below. See TODO.md, "Direction: the web client, per game".

import { $ } from "./core/dom.js";
import {
  storedName, setStoredName, tokenFor, setTokenFor, storedPublic, setStoredPublic,
} from "./core/store.js";
import {
  register as registerSound, setTrack, armAudio, mountMuteButtons,
} from "./core/sound.js";
import { closeModal } from "./core/modal.js";
import { resetFeed } from "./core/feed.js";
import { toast } from "./core/toast.js";

// The hub's own music: it loops over the title, the menu and the lobby, while
// people are arriving. A game registers its own tracks on top of this as it
// mounts — see core/sound.js for why music only plays on the shared screens.
const SHELL_TRACKS = {
  intro: { src: "/audio/intro.mp3", volume: 0.55, loop: true },
};

// Mute-button slots index.html carries. A game's markup may add its own.
const SHELL_SLOTS = ["sound-menu", "sound-home", "sound-lobby"];

const app = {
  ws: null,
  view: null,          // latest server state
  me: "",              // our player id
  code: "",
  name: "",
  game: "",            // catalogue slug chosen on the menu, or learnt from a room
  reconnectDelay: 500,
  closingOnPurpose: false,
};

// ────────────────────────────────────────────────────────── avatars

// Fetched rather than listed here, so the embedded PNGs stay the one catalogue.
// Portraits are the shell's: they are shared by every game, like the audio.
const avatars = { ids: [] };

const avatarURL = (id) => `/avatars/${encodeURIComponent(id)}.png`;
const avatarLabel = (id) =>
  id.split("-").map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ");

async function loadAvatars() {
  try {
    const res = await fetch("/api/avatars");
    if (!res.ok) return;
    const body = await res.json();
    avatars.ids = body.avatars || [];
  } catch {
    return; // the picker simply never appears; names still tell people apart
  }
  // The lobby may already be on screen: this finishes after the socket opens.
  if (app.view && !app.view.started) renderLobby(app.view);
}

// ────────────────────────────────────────────────────────── networking

function connect(code) {
  app.code = code;
  app.closingOnPurpose = false;
  resetFeed();
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
      // Before render(), so a game can start a flash that defers the modal which
      // would otherwise open underneath it. Skipped while nothing is mounted: the
      // first state a game sees is a catch-up and is deliberately silent anyway.
      if (mounted.mod) mounted.mod.onState(msg);
      render();
      break;
    case "private":
      if (mounted.mod) mounted.mod.onPrivate(msg.event);
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

// nameOf is the shell's because seats are: every game has players with names, and
// "You" rather than your own name is a courtesy that should not be re-decided.
function nameOf(id) {
  const s = app.view && app.view.seats.find((x) => x.id === id);
  if (!s) return "Someone";
  return s.id === app.me ? "You" : s.name;
}

// ────────────────────────────────────────────────────────── screens

// leaveRoom walks out of the room entirely and returns to the front page. In the
// lobby the server drops the seat outright, so this really is leaving rather than
// a disconnect to be reconnected — hence closingOnPurpose.
function leaveRoom() {
  app.closingOnPurpose = true;
  if (app.ws) app.ws.close();
  app.ws = null;
  app.view = null;
  app.me = "";
  app.code = "";
  resetFeed();
  // Unmounted rather than merely hidden: the game holds a round's worth of state
  // — a selection, a covered hand, a countdown — and none of it should survive
  // into the next room. Re-entering re-fetches a template the browser has cached.
  unmountGame();
  $("conn-warning").hidden = true;
  // Otherwise a refresh would walk straight back into the room just left.
  history.replaceState(null, "", location.pathname);
  // Back to the front page, not to one game's room screen: the button says
  // "menu", and after leaving a table the next choice really is which game.
  showMenu();
}

// ─────────────────────────── the game menu ───────────────────────────

// The catalogue comes from the server so the list of games is written down once,
// the same bargain the avatar picker makes. Until it arrives the menu says so
// rather than showing an empty page that looks broken.
const catalogueOfGames = { list: [], loaded: false };

async function loadGames() {
  try {
    const res = await fetch("/api/games");
    if (!res.ok) throw new Error("server said no");
    catalogueOfGames.list = (await res.json()).games || [];
    catalogueOfGames.loaded = true;
  } catch {
    $("menu-status").textContent =
      "Couldn't reach the server. Is it still running?";
    return;
  }
  renderGameList();
  // A deep link may already have chosen for us before this landed.
  if (app.game) applyGameChrome(app.game);
}

function gameInfo(slug) {
  return catalogueOfGames.list.find((g) => g.slug === slug) || null;
}

function renderGameList() {
  const games = catalogueOfGames.list;
  $("menu-status").textContent = games.some((g) => g.playable)
    ? ""
    : "No games are playable on this server yet.";
  $("menu-status").hidden = !$("menu-status").textContent;

  $("game-list").replaceChildren(...games.map((g) => {
    const li = document.createElement("li");
    // A locked tile is a real button that says why, not a dead div: tapping it
    // should answer the question it just raised.
    const tile = document.createElement("button");
    tile.type = "button";
    tile.className = "game-tile" + (g.playable ? "" : " locked");
    tile.dataset.slug = g.slug;

    const emoji = document.createElement("span");
    emoji.className = "game-emoji";
    emoji.textContent = g.emoji || "🎲";

    const text = document.createElement("span");
    text.className = "game-text";
    const name = document.createElement("b");
    name.textContent = g.name;
    const tag = document.createElement("small");
    // The player range is the thing somebody standing in a room with friends
    // actually needs, so every tile carries it, built or not.
    tag.textContent = `${g.tagline} · ${g.min}–${g.max} players`;
    text.append(name, tag);

    tile.append(emoji, text);
    if (!g.playable) {
      const soon = document.createElement("span");
      soon.className = "soon";
      soon.textContent = "Soon";
      tile.append(soon);
      // Deliberately not disabled: a disabled control cannot be focused or
      // tapped, so it can never answer the question it raises. Same bargain as a
      // blocked card in the hand — it refuses, and says why.
      tile.onclick = () => toast(`${g.name} isn't built yet.`);
    } else {
      // A chevron so the one tile you can press looks pressable next to the ones
      // you cannot — opacity alone reads as a rendering glitch.
      const go = document.createElement("span");
      go.className = "game-go";
      go.textContent = "›";
      go.setAttribute("aria-hidden", "true");
      tile.append(go);
      tile.onclick = () => chooseGame(g.slug);
    }

    li.append(tile);
    return li;
  }));
}

// chooseGame commits to a game and moves on to the room screen. The choice is
// deliberately not remembered: the menu is the front page, and silently skipping
// it would make the hub feel like it only has one game.
function chooseGame(slug) {
  app.game = slug;
  applyGameChrome(slug);
  showHome();
}

// applyGameChrome dresses the room screen as the chosen game, so the title is not
// a lie once there is more than one.
function applyGameChrome(slug) {
  setTheme(slug);
  mountGame(slug); // async; render() waits on it rather than this function
  // Below the theme and the mount, not above them: the catalogue may not have
  // landed (or may have failed), and a room we are already in still knows its own
  // slug. Neither the palette nor the table may wait on a fetch that might never
  // finish.
  const g = gameInfo(slug);
  if (!g) return;
  $("home-logo").textContent = `${g.emoji} ${g.name}`;
  $("home-tagline").textContent = `${g.tagline} ${g.min}–${g.max} players, one phone each.`;
  document.title = g.name;
}

// setTheme swaps the whole palette by naming the game on <html>; style.css does
// the rest. It is the root element rather than <body> because the `html` rule
// reads --bg, and the root is what paints the overscroll gutter on a phone.
//
// An unknown slug is left in place rather than guessed at: style.css falls back
// to the Exploding Kittens palette on a bare :root, so there is always a theme.
function setTheme(slug) {
  const root = document.documentElement;
  if (slug) root.dataset.game = slug;
  else delete root.dataset.game;
  // The browser chrome is part of the theme on a phone, where the address bar
  // sits directly against the app ground. Read back rather than duplicated here,
  // so the value stays written down once in the stylesheet.
  const meta = document.querySelector('meta[name="theme-color"]');
  const bg = getComputedStyle(root).getPropertyValue("--bg").trim();
  if (meta && bg) meta.content = bg;
}

// ─────────────────────── mounting a game ───────────────────────
//
// A game ships two files, both fetched on demand:
//
//   games/<slug>/table.html   its screens, injected into #game-root
//   games/<slug>/index.js     its client, default-exporting this shape:
//
//     slots        mute-button slot ids its markup carries
//     mount(ctx)   wire the injected markup; ctx is the shell handle below
//     unmount()    drop timers, listeners and anything still animating
//     onState(v)   every state, before render() — for sounds and cinematics
//     render(v)    every state once started, and shows its own table; the shell
//                  handles the lobby itself
//     leaveTable() take the table and any overlay off screen, without unmounting
//     onPrivate(e) a message only this player is entitled to see
//     onEscape()   true if it used the key, so the shell can stop
//     showRules(on)  because the lobby's button is the shell's
//
// render() and leaveTable() are the pair that keeps the shell from naming a
// game's ids: the shell decides *which* screen is up, the game decides what its
// own screen is made of.
//
// Two consequences worth knowing before adding to this:
//
//   * The controls inside that markup cannot be wired at module load, the way the
//     shell's are at the bottom of this file. mount() runs on every mount.
//   * render() gates on the mount and mountGame() calls it back once the markup
//     is in. A socket message can easily arrive first — an invite link learns the
//     game from the room's own state, so the very first state triggers the fetch.

const mounted = { slug: "", loading: "", mod: null };

// ctx is the entire surface a game has onto the shell. Deliberately functions
// rather than values: a game must not hold a copy of state that the next server
// message replaces.
const gameCtx = {
  send,
  toast,
  nameOf,
  avatarChip,
  rerender: () => render(),
  me: () => app.me,
  view: () => app.view,
};

async function mountGame(slug) {
  if (!slug || mounted.slug === slug || mounted.loading === slug) return;
  // The slug reaches here from the catalogue or from a room's state, but it ends
  // up in two URLs, one of them a module path — so it is checked rather than
  // trusted.
  if (!/^[a-z0-9][a-z0-9-]*$/.test(slug)) return;
  mounted.loading = slug;

  let html, mod;
  try {
    const [res, module] = await Promise.all([
      fetch(`games/${slug}/table.html`),
      import(`./games/${slug}/index.js`),
    ]);
    if (!res.ok) throw new Error(`server said ${res.status}`);
    html = await res.text();
    mod = module.default;
  } catch {
    mounted.loading = "";
    // Nothing can be played without them, and silence would leave the player on a
    // blank screen wondering whether the tap registered.
    showHome("Couldn't load the game screen. Reload the page and try again.");
    return;
  }
  // Another game may have been chosen while that was in flight.
  if (mounted.loading !== slug) return;

  unmountGame();
  $("game-root").innerHTML = html;
  mounted.slug = slug;
  mounted.mod = mod;
  mounted.loading = "";

  mod.mount(gameCtx);
  mountMuteButtons([...SHELL_SLOTS, ...(mod.slots || [])]);

  // The state that was waiting for this, if one beat the fetch here.
  if (app.view) render();
}

function unmountGame() {
  if (!mounted.slug) return;
  if (mounted.mod) mounted.mod.unmount();
  $("game-root").replaceChildren();
  mounted.slug = "";
  mounted.mod = null;
}

// The shell is moving to one of its own screens, so whatever the game had on
// screen must come off. It is asked rather than reached into: the table and any
// overlay above it are the game's markup, and only the game knows which of its
// ids those are. A no-op while nothing is mounted, which is the common case on
// the menu.
//
// Not the same as unmounting — the rules panel is still reachable from the lobby,
// and a countdown that is merely paused is cheaper to resume than to rebuild.
function leaveTable() {
  if (mounted.mod) mounted.mod.leaveTable();
}

function showMenu() {
  closeBrowser();
  closeModal();
  // The menu belongs to the hub rather than to any one game, so it drops back to
  // the default palette instead of keeping the colours of the game just left.
  setTheme("");
  document.title = "Board Games";
  $("menu").hidden = false;
  $("home").hidden = true;
  $("lobby").hidden = true;
  leaveTable();
  setTrack("intro");
}

function showHome(error) {
  closeBrowser();
  closeModal();
  $("menu").hidden = true;
  $("home").hidden = false;
  $("lobby").hidden = true;
  leaveTable();
  const box = $("home-error");
  box.hidden = !error;
  box.textContent = error || "";
  setTrack("intro"); // the title screen by definition, whatever app.view holds
}

// ─────────────────────── public lobby browser ───────────────────────

// Polled rather than pushed: the browser is not in a room yet, so there is no
// socket to be told on. Polling only runs while the screen is actually up.
const REFRESH_MS = 4000;
const browser = { timer: 0, open: false };

function showBrowser() {
  $("menu").hidden = true;
  $("home").hidden = true;
  $("lobby").hidden = true;
  leaveTable();
  $("browser").hidden = false;
  browser.open = true;
  $("browser-status").textContent = "Looking for rooms…";
  refreshBrowser();
  clearInterval(browser.timer);
  browser.timer = setInterval(refreshBrowser, REFRESH_MS);
}

function closeBrowser() {
  browser.open = false;
  clearInterval(browser.timer);
  browser.timer = 0;
  $("browser").hidden = true;
}

async function refreshBrowser() {
  let rooms;
  try {
    const res = await fetch("/api/rooms");
    if (!res.ok) throw new Error("server said no");
    rooms = (await res.json()).rooms || [];
  } catch {
    if (browser.open) $("browser-status").textContent = "Couldn't reach the server.";
    return;
  }
  // The screen may have been left while the request was in flight.
  if (browser.open) renderRoomList(rooms);
}

function renderRoomList(rooms) {
  // The listing spans every game on the server, but the player has already chosen
  // one — so it shows the rooms they could actually walk into, and says which
  // game those are.
  const mine = app.game ? rooms.filter((r) => r.game === app.game) : rooms;
  const named = gameInfo(app.game);
  const label = named ? named.name : "this game";

  $("browser-status").textContent = mine.length
    ? `${mine.length} ${label} room${mine.length === 1 ? "" : "s"} waiting for players`
    : `No public ${label} rooms right now. Create one and it will show up here for everybody else.`;

  $("browser-list").replaceChildren(...mine.map((r) => {
    const li = document.createElement("li");

    const info = document.createElement("div");
    info.className = "room-info";
    const code = document.createElement("b");
    code.className = "room-tag";
    code.textContent = r.code;
    const who = document.createElement("span");
    who.className = "room-who";
    const names = (r.names || []).join(", ");
    who.textContent = `${r.players}/${r.capacity} · ${names || "waiting"}`;
    info.append(code, who);

    const join = document.createElement("button");
    join.className = "btn primary small";
    join.textContent = "Join";
    join.onclick = () => {
      app.name = currentName();
      closeBrowser();
      connect(r.code);
    };

    li.append(info, join);
    return li;
  }));
}

// ────────────────────────────────────────────────────────── rendering

function render() {
  const v = app.view;
  if (!v) return;
  closeBrowser(); // a live room always wins the screen

  // An invite link drops you into a room without passing the menu, so the room
  // is where the client finds out which game it is rendering.
  if (v.game && v.game !== app.game) {
    app.game = v.game;
    applyGameChrome(v.game);
  }

  // Nothing to render into until the game is mounted. Deliberately before the
  // menu is hidden: the fetch is same-origin and small, and leaving the menu up
  // for it beats blanking the screen.
  //
  // `!mounted.slug` is not redundant with the comparison: it also catches a state
  // that names no game at all, which would otherwise fall through to a game
  // module that isn't there. Sitting on the menu is a bad outcome; throwing on
  // every state that arrives is a worse one.
  if (!mounted.slug || mounted.slug !== app.game) {
    mountGame(app.game);
    return;
  }

  $("menu").hidden = true;
  if (!v.started) {
    $("home").hidden = true;
    leaveTable();
    $("lobby").hidden = false;
    renderLobby(v);
    setTrack("intro"); // the lobby is still people arriving
    return;
  }
  $("home").hidden = true;
  $("lobby").hidden = true;
  mounted.mod.render(v); // shows its own table; the shell does not know its ids
}

function renderLobby(v) {
  $("lobby-code").textContent = v.code;
  // Whether the room is advertised matters most to whoever is about to invite
  // people, so it sits right under the code they are reading out.
  $("lobby-visibility").textContent = v.public
    ? "🌍 Listed in the public lobby"
    : "🔒 Private — only this code gets in";
  renderAvatarPicker(v);

  const list = $("lobby-players");
  list.replaceChildren(...v.seats.map((s) => {
    const li = document.createElement("li");
    const dot = document.createElement("i");
    dot.className = "dot" + (s.connected ? "" : " off");
    const nm = document.createElement("span");
    nm.textContent = s.name + (s.id === app.me ? " (you)" : "");
    li.append(avatarChip(s), dot, nm);
    if (s.host) {
      const tag = document.createElement("span");
      tag.className = "tag";
      tag.textContent = "host";
      li.append(tag);
    }
    return li;
  }));

  // The seat range comes from the catalogue rather than from a constant here, so
  // a game for ten is not held to a game for five. The server re-checks it; this
  // only decides whether the button lights up.
  const info = gameInfo(app.game);
  const n = v.seats.filter((s) => s.connected).length;
  const startable = n >= (info ? info.min : 2) && n <= (info ? info.max : Infinity);
  $("start-btn").hidden = !v.me.host;
  $("start-btn").disabled = !startable;
  $("lobby-hint").textContent = v.me.host
    ? (startable ? `${n} players ready.` : "Waiting for at least one more player…")
    : "Waiting for the host to deal…";
}

// A picked portrait stops being a choice — greyed and disabled, your own
// included, which you leave by picking another. Picking at all is optional.
function renderAvatarPicker(v) {
  const box = $("avatar-picker");
  box.hidden = avatars.ids.length === 0;
  if (box.hidden) return;

  const heldBy = new Map(v.seats.filter((s) => s.avatar).map((s) => [s.avatar, s]));

  $("avatar-grid").replaceChildren(...avatars.ids.map((id) => {
    const holder = heldBy.get(id);
    const mine = holder && holder.id === app.me;
    const label = avatarLabel(id);

    const b = document.createElement("button");
    b.type = "button";
    b.className = "avatar-pick" + (mine ? " mine" : holder ? " taken" : "");
    b.disabled = Boolean(holder);
    b.setAttribute("aria-pressed", String(Boolean(mine)));
    b.setAttribute("aria-label", mine ? `${label}, yours`
      : holder ? `${label}, taken by ${holder.name}`
      : `Play as ${label}`);

    const img = document.createElement("img");
    img.className = "avatar-art";
    img.src = avatarURL(id);
    img.alt = "";
    img.decoding = "async";

    const cap = document.createElement("span");
    cap.className = "avatar-cap";
    cap.textContent = mine ? "You" : holder ? holder.name : label;

    b.append(img, cap);
    if (!holder) b.onclick = () => send({ type: "avatar", avatar: id });
    return b;
  }));
}

// The portrait at name size, beside a player in the lobby and at the table. Handed
// to games through ctx, since the portraits are shared.
function avatarChip(seat) {
  const el = document.createElement("span");
  el.className = "avatar-chip";
  if (!seat.avatar) {
    el.textContent = "🐱";
    return el;
  }
  const img = document.createElement("img");
  img.src = avatarURL(seat.avatar);
  img.alt = "";
  img.decoding = "async";
  el.append(img);
  return el;
}

// ────────────────────────────────────────────────────────── wiring

$("name-input").value = storedName();

function currentName() {
  const n = $("name-input").value.trim() || "Player";
  setStoredName(n);
  return n;
}

function setCreateVisibility(pub) {
  setStoredPublic(pub);
  for (const [id, on] of [["vis-public", pub], ["vis-private", !pub]]) {
    $(id).classList.toggle("on", on);
    $(id).setAttribute("aria-checked", String(on));
  }
}

$("vis-public").onclick = () => setCreateVisibility(true);
$("vis-private").onclick = () => setCreateVisibility(false);
setCreateVisibility(storedPublic());

$("create-btn").onclick = async () => {
  app.name = currentName();
  try {
    const res = await fetch("/api/rooms", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ public: storedPublic(), game: app.game }),
    });
    if (!res.ok) throw new Error("server said no");
    const { code } = await res.json();
    connect(code);
  } catch {
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

$("leave-btn").onclick = leaveRoom;
$("home-back").onclick = () => showMenu();
$("browse-btn").onclick = showBrowser;
$("browser-refresh").onclick = refreshBrowser;
$("browser-back").onclick = () => showHome();

// The trigger is the shell's and the panel is the game's, so this asks rather
// than reaching for an element that may not be mounted yet.
$("rules-lobby").onclick = () => { if (mounted.mod) mounted.mod.showRules(true); };

document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  // The game gets first refusal: it may have a panel open or a target to pick.
  if (mounted.mod && mounted.mod.onEscape()) return;
});

registerSound({ tracks: SHELL_TRACKS });
mountMuteButtons(SHELL_SLOTS);
loadGames();
loadAvatars();
armAudio();          // must be installed before the first play() attempt below
setTrack("intro");   // the title screen is already up

// A shared link (…/#ABCD) drops you straight into that room, menu and all: the
// person who sent it already chose the game, and the room will say which.
const hash = location.hash.replace("#", "").toUpperCase();
if (hash) {
  $("code-input").value = hash;
  if (storedName()) {
    app.name = storedName();
    connect(hash);
  } else {
    // No name stored, so we still need the room screen to ask for one — but the
    // menu would be a detour, since the game is already decided.
    showHome();
  }
}
