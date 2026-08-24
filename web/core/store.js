// Everything the client remembers between sessions, in one place.
//
// All five keep the "ek:" prefix they were written with, from when this server
// hosted one game. Renaming them would be a cosmetic change that logs everybody
// out of their seat and forgets their name, so they stay as they are: the prefix
// is now just a namespace, and none of these are per-game anyway.
//
// A seat token is the exception to "not per-game" only in that it is per-room,
// which is why it is keyed by code.

const key = {
  name: "ek:name",
  muted: "ek:muted",
  visibility: "ek:visibility",
  log: "ek:log",
  token: (code) => `ek:token:${code}`,
};

export const storedName = () => localStorage.getItem(key.name) || "";
export const setStoredName = (n) => localStorage.setItem(key.name, n);

export const tokenFor = (code) => localStorage.getItem(key.token(code)) || "";
export const setTokenFor = (code, t) => localStorage.setItem(key.token(code), t);

export const storedMuted = () => localStorage.getItem(key.muted) === "1";
export const setStoredMuted = (m) => localStorage.setItem(key.muted, m ? "1" : "0");

// Remembered so a host who always plays privately isn't re-picking every time.
export const storedPublic = () => localStorage.getItem(key.visibility) !== "private";
export const setStoredPublic = (pub) =>
  localStorage.setItem(key.visibility, pub ? "public" : "private");

// The log panel lives in a game's markup, but the preference outlives any one
// game, so it is the shell that remembers it.
export const logOpen = () => localStorage.getItem(key.log) !== "closed";
export const setStoredLogOpen = (open) =>
  localStorage.setItem(key.log, open ? "open" : "closed");
