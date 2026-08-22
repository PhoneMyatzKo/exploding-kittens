// Big moments, played one at a time over whatever is on the table.
//
// The mechanism is here and the content is not: a game pushes items describing a
// glyph, a caption, how long to hold and what to sound, and this queues them so
// two never overlap. That matters because the beats arrive in bursts — an
// explosion, a defuse and an elimination can all land in one state — and playing
// them on top of each other reads as a rendering fault rather than as drama.
//
// Nothing here swallows a tap: the layer is pointer-events:none in the
// stylesheet, so a Nope stays reachable through the bang.

import { $ } from "./dom.js";
import { playSfx } from "./sound.js";

// Never build a backlog nobody is waiting for. Three is enough to show a chain
// of eliminations; past that the table has moved on and the player would be
// watching history.
const MAX_QUEUED = 3;

const cinema = {
  queue: [],
  playing: false,
  played: false,   // something ran since the queue last emptied
  onIdle: null,    // called once the screen is clear again
  vfx: null,       // { src, offset } for the full-screen plate, if the game has one
};

export const cinemaPlaying = () => cinema.playing;

export function configureCinema({ onIdle = null, vfx = null } = {}) {
  cinema.onIdle = onIdle;
  cinema.vfx = vfx;
}

// item: { glyph, text, ms, anchor?, sfx?, vfx? }
//
//   anchor  a selector to play over instead of the middle of the screen
//   sfx     an effect registered with core/sound, fired with the picture rather
//           than when the event arrived — which may be a beat earlier
//   vfx     whether to lay the configured plate over the whole table
export function pushCinematic(item) {
  cinema.queue.push(item);
  if (cinema.queue.length > MAX_QUEUED) {
    cinema.queue.splice(0, cinema.queue.length - MAX_QUEUED);
  }
}

export function resetCinema() {
  cinema.queue.length = 0;
  cinema.playing = false;
  cinema.played = false;
  clearVfx();
}

export function runCinema() {
  if (cinema.playing) return;
  const box = $("cinema");
  if (!box) return;

  const item = cinema.queue.shift();
  if (!item) {
    box.hidden = true;
    box.replaceChildren();
    clearVfx();
    // Only after something actually played: the callback is how a game reopens
    // the prompts it held back, and firing it on an already-idle screen would
    // reopen them on every state.
    if (cinema.played) {
      cinema.played = false;
      if (cinema.onIdle) cinema.onIdle();
    }
    return;
  }
  cinema.playing = true;
  cinema.played = true;

  const flash = document.createElement("div");
  flash.className = `flash ${item.kind}`;
  if (item.anchor) anchorTo(flash, item.anchor);

  const glyph = document.createElement("span");
  glyph.className = "flash-glyph";
  glyph.textContent = item.glyph;
  const text = document.createElement("p");
  text.className = "flash-text";
  text.textContent = item.text;
  flash.append(glyph, text);

  box.replaceChildren(flash);
  if (item.sfx) playSfx(item.sfx);
  if (item.vfx) playVfx();
  box.hidden = false;

  setTimeout(() => { cinema.playing = false; runCinema(); }, item.ms);
}

// Lays the flash over one element's own box — a seat, usually. An element
// scrolled out of view is brought back first, otherwise the bang goes off where
// nobody can see it.
function anchorTo(flash, selector) {
  const host = document.querySelector(selector);
  if (!host) return;
  host.scrollIntoView({ behavior: "instant", inline: "center", block: "nearest" });
  const r = host.getBoundingClientRect();
  if (!r.width) return;
  flash.classList.add("at-seat");
  flash.style.left = `${r.left}px`;
  flash.style.top = `${r.top}px`;
  flash.style.width = `${r.width}px`;
  flash.style.height = `${r.height}px`;
}

// Real footage, laid over the whole table and under the glyph.
//
// The plate goes on <body>, not into #cinema — see .flash-vfx in the stylesheet
// for why that placement is what makes the blending work at all.
//
// Built fresh each time rather than rewound: a <video> that failed to decode
// once stays broken, and this way the next beat gets a clean try.
//
// Muted and inline is what lets it start without a gesture on iOS; any bang is a
// separate effect, which is also why a device that refuses the video still gets
// the noise.
function playVfx() {
  clearVfx();
  if (!cinema.vfx) return;
  const { src, offset } = cinema.vfx;
  const v = document.createElement("video");
  v.className = "flash-vfx";
  // Browsers honour #t= on a media element natively, which beats waiting on
  // readyState to seek.
  v.src = offset ? `${src}#t=${offset}` : src;
  v.muted = true;
  v.defaultMuted = true; // Safari reads this attribute, not the property
  v.playsInline = true;
  v.preload = "auto";
  v.setAttribute("aria-hidden", "true");
  // Never leave a black rectangle over the table: the plate is only additive
  // because of how it blends, so a codec we cannot play must take itself out.
  v.addEventListener("error", clearVfx, { once: true });
  document.body.append(v);
  const p = v.play();
  if (p) p.catch(clearVfx);
}

export function clearVfx() {
  for (const v of document.querySelectorAll(".flash-vfx")) v.remove();
}
