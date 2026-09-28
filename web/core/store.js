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
  lang: "ek:lang",
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

// English or Burmese. Set from the Exploding Kittens rules sheet and read by
// Monopoly's board, which names every square in both — so a table that reads
// Burmese picks it once rather than per game.
//
// One key, and a *fallback* per game rather than a key per game. The difference
// matters because the two are not the same question: "what has this person
// chosen" is shared, and "what should they see if they have chosen nothing" is
// the game's. Monopoly Myanmar opens in Burmese because that is what it is;
// Exploding Kittens opens in English. Either way an actual choice, once made,
// travels with the person.
//
// Which means an unrecognised value has to read as *unset* rather than as
// English — otherwise there is nothing for a game to have an opinion about. And
// games must not write the language back on mount: doing so turns "I have not
// chosen" into "I chose English" the first time anybody opens a table, and would
// silently defeat every fallback but the first one seen.
export const storedLang = (fallback = "en") => {
  const saved = localStorage.getItem(key.lang);
  return saved === "my" || saved === "en" ? saved : fallback;
};
export const setStoredLang = (lang) =>
  localStorage.setItem(key.lang, lang === "my" ? "my" : "en");
