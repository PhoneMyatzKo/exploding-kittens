// People talking to each other, as opposed to the table reporting itself.
//
// Shell-level, like the modal and the toasts, and that is the whole reason this
// file exists rather than a chat panel in each game's template:
//
//   * It works on the lobby screen as well as at the table. Half of what anybody
//     says in a party game is said before the deal.
//   * No game's markup, module or Go code is touched by it. The shell contract
//     says app.js must not name an id from a game's template — a chat panel
//     tabbed into a game's log would break that in both directions.
//   * It does not compete with the play-by-play for room. The log is a record
//     you scroll back through; this is a conversation. Merged, a chatty table
//     buries "Aung bought Bagan", and on a phone there is no space for two
//     stacked panels anyway.
//
// The server is the only source of a line. Nothing here echoes what you typed —
// your own message appears when it comes back, so what you see is what everyone
// else sees, and a message the server threw away does not sit on your screen
// looking sent.

import { $ } from "./dom.js";

// Matches chatMaxLen in internal/room/chat.go. The server truncates as a
// safeguard; this is what stops anybody reaching it.
const MAX_LEN = 300;

const chat = {
  open: false,
  unread: 0,
  lines: [],
  send: null, // supplied by the shell, which owns the socket
  me: "",     // our player id, for "said by you"
};

// configureChat hands the module its way of talking to the server. The shell
// owns the socket; this file never touches it.
export function configureChat({ send }) {
  chat.send = send;
}

// setChatViewer names who is looking, and repaints.
//
// Held here rather than passed in per message because of an ordering the server
// does not promise: the room replies to a join and *then* sends the history, so
// the conversation can land before the shell has learnt its own player id. Told
// separately, the lines already on screen get their attribution the moment it is
// known instead of at the next message.
export function setChatViewer(me) {
  chat.me = me || "";
  renderChat();
}

// mountChat wires the launcher, the panel and the composer. Called once.
export function mountChat() {
  const launcher = $("chat-launch");
  const form = $("chat-form");
  const input = $("chat-input");
  if (!launcher || !form || !input) return;

  input.maxLength = MAX_LEN;
  launcher.onclick = () => openChat(!chat.open);
  $("chat-close").onclick = () => openChat(false);

  form.onsubmit = (e) => {
    e.preventDefault();
    const text = input.value.trim();
    if (!text || !chat.send) return;
    chat.send({ type: "chat", text });
    // Cleared on send rather than on the echo: the box has to feel responsive,
    // and a refused message is reported by the shell's own error toast.
    input.value = "";
    input.focus();
  };
}

export const chatIsOpen = () => chat.open;

// openChat shows or hides the panel. Opening clears the unread badge and puts
// the cursor in the box, because opening it is what you do to say something.
export function openChat(on) {
  const panel = $("chat-panel");
  const launcher = $("chat-launch");
  if (!panel || !launcher) return;

  chat.open = Boolean(on);
  panel.hidden = !chat.open;
  launcher.classList.toggle("on", chat.open);
  launcher.setAttribute("aria-expanded", String(chat.open));

  if (!chat.open) return;
  chat.unread = 0;
  paintBadge();
  scrollToEnd();
  // Not on a touch screen: focusing raises the keyboard over the panel you just
  // opened, which is the opposite of helpful when you wanted to read it.
  if (!window.matchMedia("(pointer: coarse)").matches) $("chat-input").focus();
}

// showChatLauncher decides whether chat is reachable at all. On in a room —
// lobby or table — and off on the menu, the title screen and the lobby browser,
// where there is nobody to talk to yet.
export function showChatLauncher(on) {
  const launcher = $("chat-launch");
  if (!launcher) return;
  launcher.hidden = !on;
  if (!on) openChat(false);
}

// chatMessage takes one server message: either the whole conversation to replace
// what is held, or new lines to append.
//
// `reset` arrives on joining and on every reconnect. That matters more than it
// looks: unlike the play-by-play, which the server rebuilds into every state,
// chat is only ever sent as it happens — so without the replay a phone coming
// back from sleep would show an empty panel and the conversation would read as
// lost.
export function chatMessage(msg) {
  const lines = msg.lines || [];
  if (msg.reset) {
    chat.lines = lines.slice();
    renderChat();
    return;
  }
  if (!lines.length) return;

  chat.lines.push(...lines);
  // Somebody else talking while the panel is shut is what the badge is for. Your
  // own message coming back is not news.
  if (!chat.open) {
    chat.unread += lines.filter((l) => l.actorId !== chat.me).length;
  }
  renderChat();
}

export function resetChat() {
  chat.lines = [];
  chat.unread = 0;
  renderChat();
}

function renderChat() {
  const box = $("chat-log");
  if (!box) return;

  // Following the newest line only if the reader was already at the bottom —
  // the same courtesy the play-by-play extends. Somebody reading back through
  // the conversation is left where they were.
  const pinned = box.scrollHeight - box.scrollTop - box.clientHeight < 48;

  box.replaceChildren(...chat.lines.map((line) => {
    const li = document.createElement("li");
    li.className = "chat-line";
    if (chat.me && line.actorId === chat.me) li.classList.add("mine");

    const who = document.createElement("b");
    // The name comes down with the line rather than being looked up in the seat
    // list, so somebody who has left the room is still named.
    who.textContent = line.name || "…";
    const text = document.createElement("span");
    // textContent, never innerHTML. This is the one place in the app where a
    // stranger's typing reaches the DOM, and this is what makes it safe.
    text.textContent = line.text;

    li.append(who, text);
    return li;
  }));

  $("chat-empty").hidden = chat.lines.length > 0;
  paintBadge();
  if (pinned || chat.open) scrollToEnd();
}

function paintBadge() {
  const badge = $("chat-badge");
  if (!badge) return;
  badge.hidden = chat.unread === 0;
  badge.textContent = chat.unread > 9 ? "9+" : String(chat.unread);
}

function scrollToEnd() {
  const box = $("chat-log");
  if (box) box.scrollTop = box.scrollHeight;
}
