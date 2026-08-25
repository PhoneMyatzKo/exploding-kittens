// What is new since the last server message, and the play-by-play built from it.
//
// Every game gets this rather than writing its own, because the logic is subtle
// and already debugged, and because both halves are driven off the same log
// sequence numbers:
//
//   seenSeq  the highest event animated — drives cinematics and one-shot sounds
//   logSeq   the highest event written into the log box
//
// They are separate counters because they advance at different moments: a bang
// may still be playing when the next state lands, but the line describing it is
// written immediately either way.
//
// The hard cases are all resync cases, and all detectable from the numbers
// alone — a fresh connection, a new round, and a gap meaning events happened
// while we were away. Getting them wrong is what makes an explosion fire twice
// or a log go silently stale, so they are handled here once.

import { $ } from "./dom.js";

const feed = {
  seenSeq: null,      // highest seq animated; null until the first state
  logSeq: null,       // highest seq written into the log box
  privateQueue: [],   // private notes waiting for the state they belong to
};

// Called when a socket opens, including a reconnect: the next state is a fresh
// start and must be caught up in silence rather than replayed as news.
export function resetFeed() {
  feed.seenSeq = null;
  feed.logSeq = null;
  feed.privateQueue.length = 0;
}

// freshEvents advances the animation counter and returns what has not been seen.
// The first state of a connection returns nothing: it is a catch-up, and playing
// twenty minutes of somebody else's game at once is not a welcome.
export function freshEvents(view) {
  const log = view.log || [];
  const newest = log.reduce((max, e) => Math.max(max, e.seq || 0), 0);
  if (feed.seenSeq === null) {
    feed.seenSeq = newest;
    return [];
  }
  const fresh = log.filter((e) => (e.seq || 0) > feed.seenSeq);
  feed.seenSeq = Math.max(feed.seenSeq, newest);
  return fresh;
}

// The log is a chat window, so it is appended to rather than rebuilt: that is
// what lets a player scroll back through the round without the next card played
// yanking them to the bottom, and what keeps private lines (which the server
// never replays) from being wiped on the next state.
//
// A full rebuild is still right in three cases, all detectable from the seq
// numbers: the first state of a connection, a new round (only a fresh buffer is
// ever headed by "started"), and a gap meaning we missed events while away.
//
// lineFor turns one event into an <li>, or null to leave it out. It is the game's,
// because only the game knows what its events mean.
export function renderLog(view, lineFor, { startsRound = (e) => e.kind === "started" } = {}) {
  const box = $("log");
  if (!box) return;
  const entries = view.log || [];
  const newest = entries.length ? entries[entries.length - 1].seq : 0;

  if (feed.logSeq === null || newest < feed.logSeq ||
      (entries.length && entries[0].seq > feed.logSeq + 1)) {
    rebuild(box, entries, lineFor);
    return;
  }

  const fresh = entries.filter((e) => e.seq > feed.logSeq);
  if (!fresh.length) {
    flushPrivateLog();
    return;
  }
  feed.logSeq = newest;

  if (fresh.some(startsRound)) {
    rebuild(box, entries, lineFor);
    return;
  }
  appendLogLines(fresh.map(lineFor).filter(Boolean));
  flushPrivateLog();
}

function rebuild(box, entries, lineFor) {
  box.replaceChildren(...entries.map(lineFor).filter(Boolean));
  feed.logSeq = entries.length ? entries[entries.length - 1].seq : 0;
  feed.privateQueue.length = 0; // belongs to a round we just replaced
  box.scrollTop = box.scrollHeight;
}

// appendLogLines follows the newest line only when the reader was already at the
// bottom; someone reading back through the history is left where they were.
function appendLogLines(lines) {
  if (!lines.length) return;
  const box = $("log");
  if (!box) return;
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
export function logPrivate(text) {
  feed.privateQueue.push(text);
}

function flushPrivateLog() {
  if (!feed.privateQueue.length) return;
  const lines = feed.privateQueue.map((text) => {
    const li = document.createElement("li");
    li.className = "mine";
    li.textContent = text;
    return li;
  });
  feed.privateQueue.length = 0;
  appendLogLines(lines);
}

// Scrolls the log to the newest line — for reopening a collapsed panel, where
// the reader's old position is no longer meaningful.
export function scrollLogToEnd() {
  const box = $("log");
  if (box) box.scrollTop = box.scrollHeight;
}
