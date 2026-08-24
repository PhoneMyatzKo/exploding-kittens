// Music and effects, for whichever game is on.
//
// The engine is here; the files are not. Games register their own with
// register(), so this module never names an asset and a second game does not
// inherit the first one's theme. Two roles are conventional and the shell asks
// for them by name:
//
//   intro   loops over the title and lobby, while people are arriving
//   theme   plays once over the game-over screen
//
// and deliberately nothing during a turn. The reason is the one-phone-each
// format: a continuous bed becomes five copies of the same track drifting out of
// sync in one room, which sounds far worse than silence. So music only plays
// where every device is on the same screen at the same moment. Turns are quiet.
//
// Effects are not music, and are deliberately outside the mute toggle: that
// switch is about not having five phones playing the same track, which a card
// flick, a bang or a shouted NOPE is not — those are the table reacting, and they
// are short.
//
// Every file is optional. A missing asset has to leave the game silent rather
// than noisy, and browsers refuse to start audio before the page has been
// interacted with, so a rejected play() arms a one-shot gesture listener and
// retries instead of giving up.

import { $ } from "./dom.js";
import { storedMuted, setStoredMuted } from "./store.js";

// name → { src, volume, loop } for music, { src, volume, offset } for effects.
const TRACKS = {};
const SFX = {};

const sfx = {}; // name → HTMLAudioElement, created on first use; null once broken

const sound = {
  el: {},           // track name → HTMLAudioElement, created on first use
  broken: {},       // track name → true once its asset has failed to load
  muted: storedMuted(),
  want: null,       // which track should be sounding, or null for none
  playing: null,    // which track we have actually started for this episode
};

// register merges a set of assets in. Called by the shell for the hub's own
// music and by each game as it mounts; a game may override a name the shell
// registered, which is how it gets its own theme without a second mechanism.
//
// Registering a name that is already sounding does not restart it — the element
// is rebuilt lazily on the next play, so a re-register mid-track is harmless.
export function register({ tracks = {}, sfx: effects = {} } = {}) {
  for (const [name, t] of Object.entries(tracks)) {
    if (TRACKS[name] && TRACKS[name].src !== t.src) {
      delete sound.el[name];
      delete sound.broken[name];
    }
    TRACKS[name] = t;
  }
  for (const [name, s] of Object.entries(effects)) {
    if (SFX[name] && SFX[name].src !== s.src) delete sfx[name];
    SFX[name] = s;
  }
  // A game registers as it mounts, which is usually *after* the tap that
  // unblocked audio — and WebKit grants playback per element, so anything
  // registered late has never been blessed. Prime it now while the page is still
  // activated, or the bang from a socket message is silently refused on iOS.
  if (unlocked) unlockTracks();
}

// ────────────────────────────────────────────────────────── effects

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

export function playSfx(name) {
  const a = sfxEl(name);
  if (!a) return;
  cueSfx(a, SFX[name].offset);
  a.volume = SFX[name].volume;
  a.play().catch(() => armAudio());
}

// ────────────────────────────────────────────────────────── music

function trackEl(name) {
  if (sound.broken[name]) return null;
  if (sound.el[name]) return sound.el[name];
  const t = TRACKS[name];
  if (!t) return null;
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

// The single entry point. Idempotent, because a game calls it on every server
// message: naming the track already sounding must not restart it or restack
// fades, and a finished one-shot must not be re-triggered.
export function setTrack(name) {
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

// ────────────────────────────────────────────────────────── unblocking

// Autoplay is blocked until the page has been interacted with, and browsers do
// not agree on how they say so: some reject the play() promise, others leave it
// pending until media data arrives. Waiting to be told therefore loses the
// track entirely on the second kind, so the hook goes on at boot and retries on
// the first interaction, whatever that turns out to be.
let gestureHook = null;
let unlocked = false; // a gesture has been spent, so late arrivals can be primed

export function armAudio() {
  if (gestureHook) return;
  const go = (e) => {
    // The sound toggle owns its own click. Counting it as the unblocking
    // gesture too would start the track on pointerdown and then mute it on
    // click, so the first press would look like it did nothing.
    if (e.target instanceof Element && e.target.closest(".mute")) return;
    disarmAudio();
    unlocked = true;
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

// ────────────────────────────────────────────────────────── the toggle

export function setMuted(m) {
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
// built here rather than in the HTML so it is written once for every slot.
const SPEAKER_SVG = `
  <svg class="ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
    <path d="M4 9.4h3.3L12 5.4v13.2L7.3 14.6H4z" fill="currentColor"/>
    <g fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
      <g class="wave"><path d="M15.2 9.5a3.7 3.7 0 0 1 0 5"/><path d="M17.9 7.1a7.1 7.1 0 0 1 0 9.8"/></g>
      <g class="slash"><path d="M15.6 9.6l4.8 4.8"/><path d="M20.4 9.6l-4.8 4.8"/></g>
    </g>
  </svg>`;

// Re-run after a game mounts: its table carries a slot of its own, and the slots
// that were already filled are simply rebuilt.
export function mountMuteButtons(slots) {
  for (const slot of slots) {
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
