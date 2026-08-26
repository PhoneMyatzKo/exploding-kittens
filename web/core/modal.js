// The one blocking prompt, shared by every game.
//
// Generic on purpose: it knows about a title, a body, two slots for content and
// up to two actions. It does not know what a card is — callers pass elements they
// built themselves, which is what keeps the shell free of any one game's markup.
//
// The two slots differ only in how they lay out, and both are in index.html
// because the sizing and the scrim are the shell's:
//
//   cards  a centred wrapping row — hands, the three cards off the top
//   extra  a plain column for a game's own controls, hidden when unused
//
// Which prompt is open is tracked here rather than in a game, because a game's
// render pass needs to ask "is my prompt already up?" without keeping a second
// copy of the answer that could drift out of step with the DOM.

import { $, markScrollable } from "./dom.js";

let current = null;

// modalKind names the open prompt, or null. Games use it to avoid reopening a
// prompt they have already put up, since render() runs on every server message.
export const modalKind = () => current;

// openModal returns its two action buttons so a caller can override what they do
// without reaching back into the document for them. Both default to closing.
export function openModal(kind, { title, body = "", cards = [], extra = [], ok = "", alt = "" }) {
  current = kind;
  $("modal").hidden = false;
  // Which prompt this is, exposed for styling: the give picker wants smaller
  // cards than the three See the Future shows you.
  $("modal").dataset.kind = kind;
  $("modal-title").textContent = title;
  $("modal-body").textContent = body;

  $("modal-cards").replaceChildren(...cards);
  markScrollable($("modal-cards"));

  const extraBox = $("modal-extra");
  extraBox.replaceChildren(...extra);
  // Hidden rather than empty: .modal-box is a grid, so an empty child would still
  // spend a row gap and open a visible hole under the body text.
  extraBox.hidden = extra.length === 0;

  const okBtn = $("modal-ok");
  okBtn.hidden = !ok;
  okBtn.textContent = ok;
  okBtn.onclick = closeModal;

  const altBtn = $("modal-alt");
  altBtn.hidden = !alt;
  altBtn.textContent = alt;
  altBtn.onclick = closeModal;

  return { ok: okBtn, alt: altBtn };
}

export function closeModal() {
  current = null;
  $("modal").hidden = true;
  delete $("modal").dataset.kind;
}

