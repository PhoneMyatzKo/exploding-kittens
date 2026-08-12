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

// Every card arrives carrying the face the server drew for it, out of the scans
// in the binary — so the six Defuses in a game are six different Defuses, the
// way the printed deck works. The server picks rather than the client because
// everyone at the table has to be looking at the same picture.
//
// A card with no art — nothing registered, or a slug whose directory went away —
// falls back to the emoji glyph above, so partial art is fine.
const artURL = (card) =>
  card.art ? `/cards/${card.art.split("/").map(encodeURIComponent).join("/")}` : "";

// ────────────────────────────────────────────────────────── avatars

// Fetched rather than listed here, so the embedded PNGs stay the one catalogue.
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

// The kinds a Three of a Kind may demand, fetched so card names live only in Go.
const catalogue = { demandable: [] };

async function loadCardKinds() {
  try {
    const res = await fetch("/api/cards");
    if (!res.ok) return;
    const body = await res.json();
    catalogue.demandable = body.demandable || [];
  } catch {
    // Left empty: the demand picker falls back to explaining why it can't open.
  }
}

const app = {
  ws: null,
  view: null,          // latest server state
  me: "",              // our player id
  code: "",
  name: "",
  selected: new Set(), // card ids picked out of our hand
  coverHand: false,    // hand shown face-down, for nosy neighbours
  peeked: new Set(),   // cards turned back over while covered
  flipping: 0,         // the card id whose flip animation should run
  awaitingTarget: false,
  blockedHint: "",     // why the last card tap was refused
  modal: null,         // which modal is open, so render() doesn't reopen it
  windowEndsAt: 0,     // performance.now() deadline for the Nope countdown
  windowTotalMs: 0,    // full window length, as the server reported it
  seenSeq: null,       // highest log seq animated; null until the first state
  logSeq: null,        // highest log seq written into the chat box
  privateQueue: [],    // private notes waiting for the state they belong to
  reconnectDelay: 500,
  closingOnPurpose: false,
};

// ────────────────────────────────────────────────────────── storage

const storedName = () => localStorage.getItem("ek:name") || "";
const setStoredName = (n) => localStorage.setItem("ek:name", n);
const tokenFor = (code) => localStorage.getItem(`ek:token:${code}`) || "";
const setTokenFor = (code, t) => localStorage.setItem(`ek:token:${code}`, t);
const storedMuted = () => localStorage.getItem("ek:muted") === "1";
const setStoredMuted = (m) => localStorage.setItem("ek:muted", m ? "1" : "0");

// ────────────────────────────────────────────────────────── sound

// Two tracks, and deliberately nothing during a turn.
//
// The reason is the one-phone-each format: a continuous bed becomes five copies
// of the same track drifting out of sync in one room, which sounds far worse
// than silence. So music only plays where every device is on the same screen at
// the same moment — the intro loops over the title and lobby while people are
// arriving, and the theme plays once over the game-over screen. Turns are quiet.
//
// Both files are optional. A missing asset has to leave the game silent rather
// than noisy, and browsers refuse to start audio before the page has been
// interacted with, so a rejected play() arms a one-shot gesture listener and
// retries instead of giving up.

const TRACKS = {
  intro: { src: "/audio/intro.mp3", volume: 0.55, loop: true },
  theme: { src: "/audio/theme_song1.mp3", volume: 0.6, loop: false },
};

// Effects are not music, and deliberately outside the mute toggle: that switch
// is about not having five phones playing the same track, which a card flick, a
// bang or a shouted NOPE is not — those are the table reacting, and they are
// short.
//
// Each is optional in the same way the tracks are: a file that fails to load
// takes its own effect out and leaves the rest of the game alone.
//
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

const sfx = {}; // name → HTMLAudioElement, created on first use; null once broken

function sfxEl(name) {
  if (name in sfx) return sfx[name];
  const s = SFX[name];
  if (!s) return null;
  const a = new Audio(s.src);
  a.preload = "auto";
  a.volume = s.volume;
  a.addEventListener("error", () => { sfx[name] = null; }, { once: true });
  sfx[name] = a;
  cueSfx(a, s.offset); // parked on the onset, ready for the first play
  return a;
}

// Winds an effect back to where its sound starts. Seeking needs the metadata, and
// an element built moments ago may not have it yet — assigning currentTime then
// is either dropped on the floor or throws, depending on the engine, so it is
// retried once the duration arrives. In practice unlockTracks() has primed every
// element on the first tap, long before anything explodes.
function cueSfx(a, offset) {
  const at = offset || 0;
  const seek = () => { try { a.currentTime = at; } catch { /* play from the head */ } };
  if (a.readyState >= 1 /* HAVE_METADATA */) seek();
  else a.addEventListener("loadedmetadata", seek, { once: true });
}

function playSfx(name) {
  const a = sfxEl(name);
  if (!a) return;
  cueSfx(a, SFX[name].offset);
  a.volume = SFX[name].volume;
  a.play().catch(() => armAudio());
}

const sound = {
  el: {},           // track name → HTMLAudioElement, created on first use
  broken: {},       // track name → true once its asset has failed to load
  muted: storedMuted(),
  want: null,       // which track should be sounding, or null for none
  playing: null,    // which track we have actually started for this episode
};

function trackEl(name) {
  if (sound.broken[name]) return null;
  if (sound.el[name]) return sound.el[name];
  const t = TRACKS[name];
  const a = new Audio(t.src);
  a.loop = t.loop;
  a.preload = "auto";
  a.volume = 0;
  a.addEventListener("error", () => {
    sound.broken[name] = true;
    delete sound.el[name];
  }, { once: true });
  sound.el[name] = a;
  return a;
}

// The single entry point. Idempotent, because render() calls it on every server
// message: naming the track already sounding must not restart it or restack
// fades, and a finished one-shot must not be re-triggered.
function setTrack(name) {
  if (sound.want !== name) {
    sound.want = name;
    sound.playing = null;
    for (const other of Object.keys(TRACKS)) if (other !== name) fadeOut(other);
  }
  if (name && sound.playing !== name) startTrack(name);
}

function startTrack(name) {
  if (sound.muted) return;
  const a = trackEl(name);
  if (!a) return;
  sound.playing = name;
  if (a.currentTime) a.currentTime = 0;
  a.play()
    .then(() => fadeTo(a, TRACKS[name].volume, 700))
    .catch((err) => {
      sound.playing = null;
      // Nothing surfaces to the player, but leave a trace: a refused play() is
      // otherwise indistinguishable from a missing file or a policy we did not
      // anticipate. NotAllowedError means blocked.
      console.debug(`sound: ${name} did not start —`, err && err.name);
      // Re-arming matters most for the theme: it is started from a socket
      // message, so if that attempt is refused the next tap — dismissing the
      // game-over modal, usually — is what recovers it.
      armAudio();
    });
}

function fadeOut(name) {
  const a = sound.el[name];
  if (a && !a.paused) fadeTo(a, 0, 450);
}

// Autoplay is blocked until the page has been interacted with, and browsers do
// not agree on how they say so: some reject the play() promise, others leave it
// pending until media data arrives. Waiting to be told therefore loses the
// track entirely on the second kind, so the hook goes on at boot and retries on
// the first interaction, whatever that turns out to be.
let gestureHook = null;

function armAudio() {
  if (gestureHook) return;
  const go = (e) => {
    // The sound toggle owns its own click. Counting it as the unblocking
    // gesture too would start the track on pointerdown and then mute it on
    // click, so the first press would look like it did nothing.
    if (e.target instanceof Element && e.target.closest(".mute")) return;
    disarmAudio();
    unlockTracks();
    if (sound.want && sound.playing !== sound.want) startTrack(sound.want);
  };
  gestureHook = go;
  document.addEventListener("pointerdown", go);
  document.addEventListener("keydown", go);
}

function disarmAudio() {
  if (!gestureHook) return;
  document.removeEventListener("pointerdown", gestureHook);
  document.removeEventListener("keydown", gestureHook);
  gestureHook = null;
}

// WebKit grants playback per *element*, and wants that element's first play() to
// happen inside a user gesture — sticky activation is not enough. The theme's
// element would otherwise be created and played much later, from the socket
// message that ends the game, with no gesture anywhere in the stack: blocked on
// iOS while the intro plays fine. So every track is primed here, silently,
// while we do have a gesture to spend.
function unlockTracks() {
  // The effects need the same per-element blessing as the music, and the bang in
  // particular is fired from a socket message with no gesture in the stack.
  for (const name of Object.keys(SFX)) {
    const s = sfxEl(name);
    if (!s || !s.paused) continue;
    s.volume = 0;
    const p = s.play();
    if (p) p.then(() => { s.pause(); cueSfx(s, SFX[name].offset); }).catch(() => {});
  }
  for (const name of Object.keys(TRACKS)) {
    const a = trackEl(name);
    if (!a || !a.paused) continue;
    a.volume = 0;
    const p = a.play();
    // startTrack() runs straight after this and claims sound.playing
    // synchronously, so the guard keeps the primer from pausing the one track
    // we actually want.
    if (p) p.then(() => { if (sound.playing !== name) a.pause(); }).catch(() => {});
  }
}

// Fade generations are per element so that fading one track out cannot cancel
// the ramp bringing the other one in.
function fadeTo(el, to, ms) {
  const gen = (el.fadeGen = (el.fadeGen || 0) + 1);
  const from = el.volume;
  const t0 = performance.now();
  const step = (now) => {
    if (gen !== el.fadeGen) return;
    const k = Math.min(1, (now - t0) / ms);
    el.volume = Math.min(1, Math.max(0, from + (to - from) * k));
    if (k < 1) requestAnimationFrame(step);
    else if (to === 0) el.pause();
  };
  requestAnimationFrame(step);
}

// The one place that knows the music policy. Driven by the state we just
// rendered rather than by which section ended up hidden: the DOM is downstream
// of this, so reading it back would quietly make the music depend on the order
// of the assignments in render().
//
// showHome() does not come through here — it is the title screen by definition,
// while app.view may still hold the game we just dropped out of.
function syncSound() {
  const v = app.view;
  if (!v || !v.started) return setTrack("intro");        // title and lobby
  setTrack(v.phase === "game_over" ? "theme" : null);    // otherwise: quiet
}

function setMuted(m) {
  sound.muted = m;
  setStoredMuted(m);
  if (m) {
    for (const a of Object.values(sound.el)) { a.pause(); a.volume = 0; }
    sound.playing = null;
  } else if (sound.want) {
    startTrack(sound.want);
  }
  renderMuteButtons();
}

function renderMuteButtons() {
  for (const b of document.querySelectorAll(".mute")) {
    b.classList.toggle("off", sound.muted);
    b.setAttribute("aria-pressed", String(sound.muted));
    b.setAttribute("aria-label", sound.muted ? "Unmute the theme" : "Mute the theme");
    b.title = sound.muted ? "Sound off" : "Sound on";
  }
}

// Drawn with currentColor so the one icon works on both surface families, and
// built here rather than in the HTML so it is written once for both slots.
const SPEAKER_SVG = `
  <svg class="ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
    <path d="M4 9.4h3.3L12 5.4v13.2L7.3 14.6H4z" fill="currentColor"/>
    <g fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
      <g class="wave"><path d="M15.2 9.5a3.7 3.7 0 0 1 0 5"/><path d="M17.9 7.1a7.1 7.1 0 0 1 0 9.8"/></g>
      <g class="slash"><path d="M15.6 9.6l4.8 4.8"/><path d="M20.4 9.6l-4.8 4.8"/></g>
    </g>
  </svg>`;

function mountMuteButtons() {
  for (const slot of ["sound-home", "sound-lobby", "sound-table"]) {
    const host = $(slot);
    if (!host) continue;
    const b = document.createElement("button");
    b.type = "button";
    b.className = "btn ghost small mute";
    b.innerHTML = SPEAKER_SVG;
    b.onclick = () => setMuted(!sound.muted);
    host.replaceChildren(b);
  }
  renderMuteButtons();
}

// ────────────────────────────────────────────────────────── networking

function connect(code) {
  app.code = code;
  app.closingOnPurpose = false;
  app.seenSeq = null;
  app.logSeq = null;
  app.privateQueue.length = 0;
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
      if (msg.pending) {
        app.windowEndsAt = performance.now() + msg.pending.remainingMs;
        app.windowTotalMs = msg.pending.totalMs || 0;
      }
      playNewEvents(msg); // before render(), so a starting flash defers the modal
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
        cards: e.cards,
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
      note = e.actorId === app.me
        ? `You stole ${card.name} from ${nameOf(e.targetId)}`
        : `${nameOf(e.actorId)} stole your ${card.name}`;
      break;
    case "gave":
      note = e.actorId === app.me
        ? `You gave away ${card.name}`
        : `${nameOf(e.actorId)} gave you ${card.name}`;
      break;
    default:
      return;
  }

  toast(note);
  logPrivate(note);
}

// ────────────────────────────────────────────────────────── screens

// leaveRoom walks out of the room entirely and returns to the title screen. In
// the lobby the server drops the seat outright, so this really is leaving rather
// than a disconnect to be reconnected — hence closingOnPurpose.
function leaveRoom() {
  app.closingOnPurpose = true;
  if (app.ws) app.ws.close();
  app.ws = null;
  app.view = null;
  app.me = "";
  app.code = "";
  app.selected.clear();
  app.awaitingTarget = false;
  app.privateQueue.length = 0;
  app.logSeq = null;
  app.seenSeq = null;
  $("conn-warning").hidden = true;
  // Otherwise a refresh would walk straight back into the room just left.
  history.replaceState(null, "", location.pathname);
  showHome();
  syncSound(); // app.view is null again, so this returns to the title track
}

// ─────────────────────── public lobby browser ───────────────────────

// Polled rather than pushed: the browser is not in a room yet, so there is no
// socket to be told on. Polling only runs while the screen is actually up.
const REFRESH_MS = 4000;
const browser = { timer: 0, open: false };

function showBrowser() {
  $("home").hidden = true;
  $("lobby").hidden = true;
  $("table").hidden = true;
  $("browser").hidden = false;
  browser.open = true;
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
  $("browser-status").textContent = rooms.length
    ? `${rooms.length} room${rooms.length === 1 ? "" : "s"} waiting for players`
    : "No public rooms right now. Create one and it will show up here for everybody else.";

  $("browser-list").replaceChildren(...rooms.map((r) => {
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

function showHome(error) {
  closeBrowser();
  $("home").hidden = false;
  $("lobby").hidden = true;
  $("table").hidden = true;
  $("nope-bar").hidden = true;
  closeModal();
  const box = $("home-error");
  box.hidden = !error;
  box.textContent = error || "";
  setTrack("intro"); // not syncSound(): app.view may still hold a live game
}

function render() {
  const v = app.view;
  if (!v) return;
  closeBrowser(); // a live room always wins the screen

  if (!v.started) {
    $("home").hidden = true;
    $("table").hidden = true;
    $("lobby").hidden = false;
    renderLobby(v);
    syncSound();
    return;
  }
  $("home").hidden = true;
  $("lobby").hidden = true;
  $("table").hidden = false;
  renderTable(v);
  syncSound();
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

  const n = v.seats.filter((s) => s.connected).length;
  const startable = n >= 2 && n <= 5;
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

// The portrait at name size, beside a player in the lobby and at the table.
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

function renderTable(v) {
  $("table-code").textContent = v.code;

  renderDeck(v);
  renderTurnBanner(v);
  renderSeats(v);
  renderDiscard(v);
  renderHand(v);
  renderActions(v);
  renderNopeBar(v);
  renderLog(v);
  renderPrompts(v);
}

// The odds of the next draw being a Kitten — arithmetic on two public numbers.
function renderDeck(v) {
  $("deck-count").textContent = `${v.deckCount} left`;

  const kittens = v.kittensLeft || 0;
  const pct = v.deckCount > 0 ? Math.round((kittens / v.deckCount) * 100) : 0;
  const risk = $("deck-risk");
  risk.textContent = v.phase === "game_over" ? "" : `💥 ${pct}%`;
  risk.title = `${kittens} Exploding Kitten${kittens === 1 ? "" : "s"} in ${v.deckCount} cards`;
  risk.classList.toggle("hot", pct >= 25);
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
    div.dataset.seat = s.id; // where an explosion goes off
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
    nm.append(avatarChip(s), dot, document.createTextNode(s.name + (s.id === app.me ? " (you)" : "")));
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
  for (const id of [...app.peeked]) if (!held.has(id)) app.peeked.delete(id);

  const sel = selectedCards(v);

  $("hand").replaceChildren(...v.me.hand.map((c) => {
    // While the hand is covered, the first tap turns a card over and the next
    // one picks it, so a glance over your shoulder gets nothing.
    if (app.coverHand && !app.peeked.has(c.id)) {
      const back = backEl();
      back.onclick = () => { app.peeked.add(c.id); app.flipping = c.id; render(); };
      return back;
    }
    const el = cardEl(c);
    const picked = app.selected.has(c.id);
    el.classList.toggle("selected", picked);
    // Dimmed means "can't join what you've already picked" — visible before the
    // tap, so a refusal is never a surprise.
    if (!picked && !selectableWith(c, sel).ok) el.classList.add("blocked");
    if (app.flipping === c.id) el.classList.add("flipping");
    el.onclick = () => toggleSelect(c.id);
    return el;
  }));
  app.flipping = 0;

  const btn = $("hand-cover");
  btn.hidden = !v.me.alive || v.me.hand.length === 0;
  btn.textContent = app.coverHand ? "👀 Show hand" : "🙈 Hide hand";
  btn.setAttribute("aria-pressed", String(app.coverHand));
}

function toggleCoverHand() {
  app.coverHand = !app.coverHand;
  app.peeked.clear();
  app.selected.clear();
  app.awaitingTarget = false;
  render();
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

  // A refused tap outranks the standing advice, since it is answering something
  // the player just did.
  $("hint").textContent = app.blockedHint
    ? app.blockedHint
    : !v.me.alive ? "You're out — enjoy the show."
    : v.phase === "game_over" ? ""
    : !v.me.myTurn ? "Waiting…"
    : sel.length === 0 ? "Play a card, or draw to end your turn."
    : plan.ok ? (plan.needsTarget ? "Press Play, then pick a player." : "")
    : plan.why;
  $("hint").classList.toggle("warn", Boolean(app.blockedHint));

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
  // A demand reads better as what is being asked for than as "on <player>".
  const on = p.named
    ? `, demanding ${nameOf(p.targetId)}'s ${p.named.name}`
    : p.targetId ? ` on ${nameOf(p.targetId)}` : "";
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

// The window length comes from the server with every open window, so there is no
// constant here to fall out of step with internal/room. The fallback only covers
// an older server that doesn't send it.
let countdownRaf = 0;
function tickCountdown() {
  cancelAnimationFrame(countdownRaf);
  const total = app.windowTotalMs || 20000;
  const step = () => {
    const left = Math.max(0, app.windowEndsAt - performance.now());
    $("nope-fill").style.width = `${Math.min(100, (left / total) * 100)}%`;
    if (left > 0 && !$("nope-bar").hidden) countdownRaf = requestAnimationFrame(step);
  };
  step();
}

// The log is a chat window, so it is appended to rather than rebuilt: that is
// what lets a player scroll back through the round without the next card played
// yanking them to the bottom, and what keeps private lines (which the server
// never replays) from being wiped on the next state.
//
// A full rebuild is still right in three cases, all detectable from the seq
// numbers: the first state of a connection, a new round (only a fresh buffer is
// ever headed by "started"), and a gap meaning we missed events while away.
function renderLog(v) {
  const box = $("log");
  const entries = v.log || [];
  const newest = entries.length ? entries[entries.length - 1].seq : 0;

  if (app.logSeq === null || newest < app.logSeq ||
      (entries.length && entries[0].seq > app.logSeq + 1)) {
    box.replaceChildren(...entries.map(logLine).filter(Boolean));
    app.logSeq = newest;
    app.privateQueue.length = 0; // belongs to a round we just replaced
    box.scrollTop = box.scrollHeight;
    return;
  }

  const fresh = entries.filter((e) => e.seq > app.logSeq);
  if (!fresh.length) {
    flushPrivateLog();
    return;
  }
  app.logSeq = newest;

  if (fresh.some((e) => e.kind === "started")) {
    box.replaceChildren(...entries.map(logLine).filter(Boolean));
    app.privateQueue.length = 0;
    box.scrollTop = box.scrollHeight;
    return;
  }
  appendLogLines(fresh.map(logLine).filter(Boolean));
  flushPrivateLog();
}

// appendLogLines follows the newest line only when the reader was already at the
// bottom; someone reading back through the history is left where they were.
function appendLogLines(lines) {
  if (!lines.length) return;
  const box = $("log");
  const pinned = box.scrollHeight - box.scrollTop - box.clientHeight < 48;
  for (const li of lines) {
    li.classList.add("fresh");
    box.append(li);
  }
  if (pinned) box.scrollTop = box.scrollHeight;
}

// Private events never reach the shared log, so they are written in from here.
// They are queued rather than appended on arrival: the server sends them just
// ahead of the state broadcast that carries the public half of the same move, so
// appending immediately would file them above lines that happened first.
function logPrivate(text) {
  app.privateQueue.push(text);
}

function flushPrivateLog() {
  if (!app.privateQueue.length) return;
  const lines = app.privateQueue.map((text) => {
    const li = document.createElement("li");
    li.className = "mine";
    li.textContent = text;
    return li;
  });
  app.privateQueue.length = 0;
  appendLogLines(lines);
}

// renderPrompts opens the modal a phase demands, and closes one that no longer
// applies (an action can be noped out from under you).
//
// Modals wait for a flash to finish rather than opening underneath it; runCinema
// calls back here once the screen is clear.
function renderPrompts(v) {
  if (cinema.playing) return;

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

// ────────────────────────────────────────────────────────── cinematics

// Read off the shared log, so every player sees the same bang at the same point.
// Must match the CSS animation delays.
//
// The explosion runs longest because it has real footage behind it, and it is
// the one beat in the game worth waiting through. Everything that would open a
// modal waits for it — see renderPrompts.
const CINEMA_MS = { exploded: 2200, defused: 1300, eliminated: 1400 };

// The explosion plate, laid over the whole table. Muted and inline is what lets
// it start without a gesture on iOS; the bang itself is a separate mp3, which is
// also why a device that refuses the video still gets the noise.
//
// offset trims the head the same way the effects above do, except it is spent as
// a media fragment on the URL — browsers honour #t= on a media element natively,
// which beats waiting on readyState to seek. Zero because the clip was already
// cut to the ignition: frame one is the flash, so there is nothing to skip.
//
// Anything spent here comes out of the clip's 2.6s, and the beat needs
// CINEMA_MS.exploded (2.2s) of footage — so past ~0.4s the fire runs out before
// the flash does.
const BOOM_VFX = { src: "/video/explosion.mp4", offset: 0 };

const cinema = { queue: [], playing: false, played: false };

// The bang and the skull go off over the seat they belong to; the defuse is the
// whole table's news, so it plays in the middle.
function cinematicFor(e) {
  const who = nameOf(e.actorId);
  switch (e.kind) {
    case "exploded":
      return { kind: "exploded", glyph: "💥", text: `${who} drew an Exploding Kitten!`, seat: e.actorId };
    case "defused":
      if (e.text) return null; // the reinsertion, which has its own modal
      return { kind: "defused", glyph: "✂️", text: `${who} defused it` };
    case "eliminated":
      return { kind: "eliminated", glyph: "☠️", text: `${who} ${who === "You" ? "are" : "is"} out`, seat: e.actorId };
    default:
      return null;
  }
}

function playNewEvents(v) {
  const log = v.log || [];
  const newest = log.reduce((max, e) => Math.max(max, e.seq || 0), 0);
  if (app.seenSeq === null) {
    app.seenSeq = newest; // first state of this connection: catch up in silence
    return;
  }
  const fresh = log.filter((e) => (e.seq || 0) > app.seenSeq);
  app.seenSeq = Math.max(app.seenSeq, newest);

  for (const e of fresh) {
    // Sounds that belong to the moment the event landed. The bang is not one of
    // them: it goes off with its picture, from runCinema, which may be a beat
    // later if something is already playing.
    if (e.kind === "drew") playSfx("draw");
    if (e.kind === "noped") playSfx("nope");
    const item = cinematicFor(e);
    if (item) cinema.queue.push(item);
  }
  // Never build a backlog nobody is waiting for.
  if (cinema.queue.length > 3) cinema.queue.splice(0, cinema.queue.length - 3);
  runCinema();
}

// Lays the flash over a seat's own box. A seat scrolled out of the row is
// brought into view first, otherwise the bang goes off where nobody can see it.
function anchorToSeat(flash, seatId) {
  if (!seatId) return;
  const seat = document.querySelector(`[data-seat="${seatId}"]`);
  if (!seat) return;
  seat.scrollIntoView({ behavior: "instant", inline: "center", block: "nearest" });
  const r = seat.getBoundingClientRect();
  if (!r.width) return;
  flash.classList.add("at-seat");
  flash.style.left = `${r.left}px`;
  flash.style.top = `${r.top}px`;
  flash.style.width = `${r.width}px`;
  flash.style.height = `${r.height}px`;
}

function runCinema() {
  if (cinema.playing) return;
  const box = $("cinema");
  const item = cinema.queue.shift();
  if (!item) {
    box.hidden = true;
    box.replaceChildren();
    clearBoomVfx();
    if (cinema.played) {
      cinema.played = false;
      if (app.view && app.view.started) renderPrompts(app.view);
    }
    return;
  }
  cinema.playing = true;
  cinema.played = true;

  const flash = document.createElement("div");
  flash.className = `flash ${item.kind}`;
  anchorToSeat(flash, item.seat);
  const glyph = document.createElement("span");
  glyph.className = "flash-glyph";
  glyph.textContent = item.glyph;
  const text = document.createElement("p");
  text.className = "flash-text";
  text.textContent = item.text;
  flash.append(glyph, text);

  box.replaceChildren(flash);
  if (item.kind === "exploded") {
    playSfx("boom");
    playBoomVfx();
  }
  box.hidden = false;
  setTimeout(() => { cinema.playing = false; runCinema(); }, CINEMA_MS[item.kind]);
}

// The plate goes on <body>, not into #cinema — see .flash-vfx in the stylesheet
// for why that placement is what makes the blending work at all.
//
// Built fresh each time rather than rewound: a <video> that failed to decode
// once stays broken, and this way the next bang gets a clean try.
function playBoomVfx() {
  clearBoomVfx();
  const v = document.createElement("video");
  v.className = "flash-vfx";
  v.src = BOOM_VFX.offset ? `${BOOM_VFX.src}#t=${BOOM_VFX.offset}` : BOOM_VFX.src;
  v.muted = true;
  v.defaultMuted = true; // Safari reads this attribute, not the property
  v.playsInline = true;
  v.preload = "auto";
  v.setAttribute("aria-hidden", "true");
  // Never leave a black rectangle over the table: the plate is only additive
  // because of how it blends, so a codec we cannot play must take itself out.
  v.addEventListener("error", clearBoomVfx, { once: true });
  document.body.append(v);
  const p = v.play();
  if (p) p.catch(clearBoomVfx);
}

function clearBoomVfx() {
  for (const v of document.querySelectorAll(".flash-vfx")) v.remove();
}

// ────────────────────────────────────────────────────────── selection

function selectedCards(v) {
  return v.me.hand.filter((c) => app.selected.has(c.id));
}

const isCat = (slug) => slug.startsWith("cat-");

// selectableWith says whether a card may join the current selection. Only two
// shapes of play exist, so anything else is refused at the point of tapping
// rather than allowed to build up into something the server would reject:
// a single non-cat card, or two-to-three cats of one kind.
function selectableWith(card, sel) {
  if (sel.length === 0) return { ok: true };
  const first = sel[0];
  if (!isCat(first.slug)) {
    return { ok: false, why: `Only one card at a time — deselect ${first.name} first.` };
  }
  if (!isCat(card.slug)) {
    return { ok: false, why: `${card.name} can't join a cat set — deselect the cats first.` };
  }
  if (card.slug !== first.slug) {
    return { ok: false, why: `A set has to match: only another ${first.name}.` };
  }
  if (sel.length >= 3) return { ok: false, why: "Three is the biggest set." };
  return { ok: true };
}

let blockedTimer;
// flashBlocked explains a refused tap in the hint line, then clears itself, so a
// dimmed card is never silently unresponsive.
function flashBlocked(why) {
  app.blockedHint = why;
  clearTimeout(blockedTimer);
  blockedTimer = setTimeout(() => { app.blockedHint = ""; render(); }, 2600);
  render();
}

function toggleSelect(id) {
  const v = app.view;
  if (!v) return;

  if (app.selected.has(id)) {
    app.selected.delete(id);
    app.blockedHint = "";
    app.awaitingTarget = false;
    render();
    return;
  }

  const card = v.me.hand.find((c) => c.id === id);
  if (!card) return;
  const check = selectableWith(card, selectedCards(v));
  if (!check.ok) {
    flashBlocked(check.why);
    return;
  }
  app.selected.add(id);
  app.blockedHint = "";
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
    if (isCat(s)) return { ok: false, why: "Add a second matching cat to steal, or a third to demand." };
    return { ok: false, why: "" };
  }
  if (sel.length === 2) {
    if (sel[0].slug === sel[1].slug && isCat(sel[0].slug)) {
      return { ok: true, needsTarget: true };
    }
    return { ok: false, why: "Two cats only work if they match." };
  }
  if (sel.length === 3) {
    const same = sel.every((c) => c.slug === sel[0].slug);
    if (same && isCat(sel[0].slug)) return { ok: true, needsTarget: true, needsName: true };
    return { ok: false, why: "Three cats only work if all three match." };
  }
  return { ok: false, why: sel.length ? "That's too many cards." : "" };
}

// playSelection is the last step for everything except a three-cat set, which
// still needs a card named before it can be sent.
function playSelection(targetId) {
  const plan = selectionPlan(selectedCards(app.view));
  if (plan.needsName) {
    app.awaitingTarget = false;
    openDemandModal(targetId);
    render();
    return;
  }
  sendPlay(targetId, "");
}

function sendPlay(targetId, named) {
  send({ type: "play", cardIds: [...app.selected], targetId, named });
  app.selected.clear();
  app.awaitingTarget = false;
  app.blockedHint = "";
  render();
}

// ────────────────────────────────────────────────────────── modals

function openModal(kind, { title, body = "", cards = [], ok = "", alt = "", onCard = null }) {
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
  // Secondary action, for the modals that offer a genuine second choice.
  const altBtn = $("modal-alt");
  altBtn.hidden = !alt;
  altBtn.textContent = alt;
  altBtn.onclick = closeModal;
}

function closeModal() {
  app.modal = null;
  $("modal").hidden = true;
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
  openModal("demand", {
    title: "Demand a card",
    body: `Name the card you want from ${nameOf(targetId)}. If they have one they must hand it over; if not, the three cats are spent for nothing.`,
    alt: "Cancel",
  });

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
  $("modal-cards").replaceChildren(grid);

  // Cancelling has to put the cats back in play, not silently eat the turn.
  $("modal-alt").onclick = () => { closeModal(); render(); };
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
  const outcome = won ? "Everyone else exploded. Well played." : "Better luck next round.";
  openModal("over", {
    title: won ? "🏆 You survived!" : `🏆 ${nameOf(v.winnerId)} survived`,
    // Only the host can act, so everyone else is told what they are waiting for
    // rather than being left on a dead table with a Close button.
    body: v.me.host
      ? outcome
      : `${outcome} Waiting for ${nameOf(hostId(v))} to deal again or reopen the lobby.`,
    ok: v.me.host ? "Deal again" : "Close",
    alt: v.me.host ? "Back to lobby" : "",
  });
  $("modal-ok").onclick = () => {
    if (v.me.host) send({ type: "start" });
    closeModal();
  };
  // The lobby is the only way to pick up players who arrived mid-round: dealing
  // again seats whoever is present right now and drops everybody else.
  $("modal-alt").onclick = () => {
    send({ type: "lobby" });
    closeModal();
  };
}

function hostId(v) {
  const h = v.seats.find((s) => s.host);
  return h ? h.id : "";
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
    // Events you were part of arrive privately too, naming the actual card, so
    // the vaguer public line would only duplicate them.
    case "drew":
      if (e.actorId === app.me) return null;
      text = `${who} drew a card`;
      break;
    case "shuffled":  text = `${who} shuffled the deck`; break;
    case "exploded":  text = `💥 ${who} drew an Exploding Kitten!`; big = true; break;
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
      if (e.actorId === app.me || e.targetId === app.me) return null;
      text = `${who} stole a card from ${nameOf(e.targetId)}`;
      break;
    case "gave":
      if (e.actorId === app.me || e.targetId === app.me) return null;
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

// Remembered so a host who always plays privately isn't re-picking every time.
const storedPublic = () => localStorage.getItem("ek:visibility") !== "private";

function setCreateVisibility(pub) {
  localStorage.setItem("ek:visibility", pub ? "public" : "private");
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
      body: JSON.stringify({ public: storedPublic() }),
    });
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

$("hand-cover").onclick = toggleCoverHand;
$("target-cancel").onclick = () => { app.awaitingTarget = false; render(); };

$("leave-btn").onclick = leaveRoom;
$("browse-btn").onclick = showBrowser;
$("browser-refresh").onclick = refreshBrowser;
$("browser-back").onclick = () => showHome();

// The log stays where you left it between rounds and page loads.
const logOpen = () => localStorage.getItem("ek:log") !== "closed";

function setLogOpen(open) {
  localStorage.setItem("ek:log", open ? "open" : "closed");
  $("log-panel").classList.toggle("collapsed", !open);
  $("log-caret").textContent = open ? "▾" : "▸";
  $("log-collapse").setAttribute("aria-expanded", String(open));
  $("log-collapse").title = open ? "Hide the log" : "Show the log";
  if (open) {
    const box = $("log");
    box.scrollTop = box.scrollHeight; // reopen at the newest line
  }
}

$("log-collapse").onclick = () => setLogOpen(!logOpen());
setLogOpen(logOpen());

const showRules = (on) => { $("rules-panel").hidden = !on; };
$("rules-lobby").onclick = () => showRules(true);
$("rules-table").onclick = () => showRules(true);
$("rules-close").onclick = () => showRules(false);

document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!$("rules-panel").hidden) showRules(false);
  else if (app.awaitingTarget) { app.awaitingTarget = false; render(); }
});

mountMuteButtons();
loadAvatars();
loadCardKinds();
armAudio();  // must be installed before the first play() attempt below
syncSound(); // the title screen is already up, so this starts the intro

// A shared link (…/#ABCD) drops you straight into that room.
const hash = location.hash.replace("#", "").toUpperCase();
if (hash) {
  $("code-input").value = hash;
  if (storedName()) {
    app.name = storedName();
    connect(hash);
  }
}
