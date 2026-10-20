// "Suggest a game": the menu's way for players to ask for the next game.
//
// The shell's, not any game's, because it is about which games exist. It reuses
// the one modal rather than bringing its own, and posts to /api/suggestions,
// which the Odoo module later reads. Card games and family games only — the
// server refuses poker and other gambling games, and the form says so up front
// so nobody types a long description to be turned away.

import { $ } from "./dom.js";
import { openModal, closeModal } from "./modal.js";
import { toast } from "./toast.js";
import { storedName } from "./store.js";

export function mountSuggest() {
  $("suggest-btn").onclick = openSuggest;
}

function field(label, control) {
  const wrap = document.createElement("label");
  wrap.className = "field";
  const span = document.createElement("span");
  span.textContent = label;
  wrap.append(span, control);
  return wrap;
}

function openSuggest() {
  const name = document.createElement("input");
  name.maxLength = 80;
  name.placeholder = "e.g. Sushi Go, Ludo";
  name.autocomplete = "off";

  const category = document.createElement("select");
  for (const [value, text] of [["card", "🃏 Card game"], ["family", "👨‍👩‍👧 Family game"]]) {
    const opt = document.createElement("option");
    opt.value = value;
    opt.textContent = text;
    category.append(opt);
  }

  const description = document.createElement("textarea");
  description.maxLength = 1000;
  description.rows = 4;
  description.placeholder = "How is it played? Why do you like it?";

  const by = document.createElement("input");
  by.maxLength = 40;
  by.placeholder = "Optional";
  by.autocomplete = "nickname";
  by.value = storedName() || "";

  const error = document.createElement("p");
  error.className = "error";
  error.hidden = true;

  const form = document.createElement("div");
  form.className = "suggest-form";
  form.append(
    field("Game name", name),
    field("Type", category),
    field("Tell us about it", description),
    field("Your name", by),
    error,
  );

  const { ok, alt } = openModal("suggest", {
    title: "Suggest a game",
    body: "Which card game or family game should we add? No poker or other gambling games, please.",
    extra: [form],
    ok: "Send",
    alt: "Cancel",
  });
  name.focus();

  ok.onclick = async () => {
    error.hidden = true;
    if (!name.value.trim()) {
      error.textContent = "Please name the game.";
      error.hidden = false;
      name.focus();
      return;
    }
    ok.disabled = true;
    try {
      const res = await fetch("/api/suggestions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          game_name: name.value,
          category: category.value,
          description: description.value,
          suggested_by: by.value,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || "Something went wrong.");
      closeModal();
      toast(`Thanks! We got your idea: ${name.value.trim()}`);
    } catch (e) {
      // The server's messages are written for players, so show them as they are.
      const msg = e instanceof TypeError ? "Couldn't reach the server." : e.message;
      error.textContent = msg.charAt(0).toUpperCase() + msg.slice(1);
      error.hidden = false;
    } finally {
      ok.disabled = false;
    }
  };
  alt.onclick = closeModal;
}
