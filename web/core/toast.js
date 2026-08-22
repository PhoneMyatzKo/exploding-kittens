// Passing notices, over whatever screen is up.
//
// Deliberately below the modal layer in the stylesheet: a prompt you have to
// answer must not be covered by a notice, and every toast the game raises is
// also written to the play-by-play, so nothing is lost by letting one hide
// behind a modal.

import { $ } from "./dom.js";

const MAX = 3;
const LIFE_MS = 3200;

export function toast(text) {
  const el = document.createElement("div");
  el.className = "toast";
  el.textContent = text;
  const box = $("toasts");
  box.append(el);
  // A flurry of steals and gifts can arrive at once; keep only the newest few.
  while (box.children.length > MAX) box.firstElementChild.remove();
  setTimeout(() => el.remove(), LIFE_MS);
}
