// Shared plumbing for the browser checks.
//
// These tests exist because `go test` cannot see the things that have actually
// broken here: a bar covering the hand, a toast burying a modal title, a screen
// missing the shared background rule. Everything below drives the real client in
// a real Chromium, one browser context per player, exactly as a party would.
//
// The client is compiled into the binary with go:embed, so a running server
// serves the *old* assets after any edit under web/. Always rebuild and restart
// before believing a result — see the README.

import { chromium } from "playwright";

export const BASE = process.env.BASE || "http://localhost:8099";
export const HEADLESS = process.env.HEADED !== "1";
export const SLOWMO = Number(process.env.SLOWMO || 0);

// Phones are the target device, so the mobile layout is worth exercising too.
export const PHONE = { width: 390, height: 844, isMobile: true, hasTouch: true };

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ─────────────────────────────────────────────────────────── assertions

export function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

let passed = 0;
let failed = 0;

// step is one link in a flow: if it fails the rest of the script is meaningless,
// so it re-throws.
export async function step(name, fn) {
  try {
    await fn();
    passed++;
    console.log(`  ✓ ${name}`);
  } catch (e) {
    failed++;
    console.log(`  ✗ ${name}\n      ${e.message}`);
    throw e;
  }
}

// check is an independent assertion: a failure is recorded and the script keeps
// going, so one broken case does not hide the other five.
export async function check(name, fn) {
  try {
    await fn();
    passed++;
    console.log(`  ✓ ${name}`);
  } catch (e) {
    failed++;
    console.log(`  ✗ ${name}\n      ${e.message}`);
  }
}

// skip records a check the random deal did not give us the cards to make. It is
// counted separately so a run that quietly proved less than usual says so
// instead of showing a tick.
let skipped = 0;
export function skip(name, why) {
  skipped++;
  console.log(`  – ${name} (${why})`);
}

export function report(title) {
  const note = skipped ? `, ${skipped} skipped` : "";
  if (failed) {
    console.log(`\n${failed} failed, ${passed} passed${note} — ${title}`);
    process.exitCode = 1;
  } else {
    console.log(`\nall ${passed} checks passed${note} — ${title}`);
  }
  return failed === 0;
}

// ─────────────────────────────────────────────────────────── browser

export async function launch() {
  return chromium.launch({ headless: HEADLESS, slowMo: SLOWMO });
}

// Each player gets their own context, not just their own tab: localStorage holds
// the seat token, and sharing it would make two tabs fight over one seat.
export async function seat(browser, name, { phone = false } = {}) {
  const ctx = await browser.newContext(phone ? { viewport: PHONE, ...PHONE } : {});
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log(`  ! ${name} page error: ${e.message}`));
  return new Player(page, name);
}

// safeClick tolerates the element vanishing between the visibility check and the
// click. In a multi-tab game that is legitimate: another player's move can close
// the window you were about to press a button in.
export async function safeClick(locator, timeout = 2000) {
  try {
    await locator.click({ timeout });
    return true;
  } catch {
    return false;
  }
}

export async function visible(locator) {
  try {
    return await locator.isVisible();
  } catch {
    return false;
  }
}

// waitFor polls a predicate. Used instead of a fixed sleep wherever the thing
// being waited on is a server round-trip.
export async function waitFor(fn, { timeout = 8000, interval = 100, what = "condition" } = {}) {
  const until = Date.now() + timeout;
  for (;;) {
    if (await fn()) return true;
    if (Date.now() > until) throw new Error(`timed out waiting for ${what}`);
    await sleep(interval);
  }
}

// ─────────────────────────────────────────────────────────── the client

export class Player {
  constructor(page, name) {
    this.page = page;
    this.name = name;
  }

  $(sel) {
    return this.page.locator(sel);
  }

  // home lands on the menu first — that is the front page now — and picks a game
  // to get to the room screen. Every check that is not about the menu itself
  // wants to be past it.
  async home(slug = "kittens") {
    await this.page.goto(`${BASE}/`);
    await this.menu();
    await this.pickGame(slug);
  }

  async menu() {
    await this.page.waitForSelector("#menu", { state: "visible" });
    await this.page.waitForSelector(".game-tile", { state: "visible" });
  }

  async pickGame(slug = "kittens") {
    await this.$(`.game-tile[data-slug="${slug}"]`).click();
    await this.page.waitForSelector("#create-btn", { state: "visible" });
    this.game = slug;
  }

  async games() {
    return this.page.$$eval(".game-tile", (els) =>
      els.map((e) => ({
        slug: e.dataset.slug || "",
        locked: e.classList.contains("locked"),
        text: e.textContent.replace(/\s+/g, " ").trim(),
      })),
    );
  }

  async onScreen(id) {
    return visible(this.$(`#${id}`));
  }

  async waitForScreen(id, timeout = 8000) {
    await this.page.waitForSelector(`#${id}`, { state: "visible", timeout });
  }

  // create returns the room code. Visibility is chosen on the home screen
  // because the room does not exist yet once you are in the lobby.
  async create({ visibility = "public", game = "kittens" } = {}) {
    await this.home(game);
    await this.$("#name-input").fill(this.name);
    await this.$(visibility === "private" ? "#vis-private" : "#vis-public").click();
    await this.$("#create-btn").click();
    await this.waitForScreen("lobby");
    const code = (await this.$("#lobby-code").textContent()).trim();
    assert(/^[A-Z0-9]{4,6}$/.test(code), `room code looks wrong: ${JSON.stringify(code)}`);
    this.code = code;
    return code;
  }

  async join(code) {
    await this.home();
    await this.$("#name-input").fill(this.name);
    await this.$("#code-input").fill(code);
    await this.$("#join-form button[type=submit]").click();
    await this.waitForScreen("lobby");
    this.code = code;
  }

  async lobbyNames() {
    return this.page.$$eval("#lobby-players li", (els) =>
      els.map((e) => e.textContent.replace(/\s+/g, " ").trim()),
    );
  }

  async deal() {
    await this.$("#start-btn").click();
    await this.waitForScreen("table");
  }

  // ── table state, read from the DOM only. The client keeps its state in a
  // module scope with nothing on window, which is correct — so the tests read
  // what a player can actually see, which is what they should be asserting on.

  async hand() {
    return this.page.$$eval("#hand .card", (els) =>
      els.map((e) => ({
        slug: e.dataset.slug || "",
        id: Number(e.dataset.id || 0),
        blocked: e.classList.contains("blocked"),
        selected: e.classList.contains("selected"),
        faceDown: e.classList.contains("hidden-face"),
      })),
    );
  }

  async isMyTurn() {
    // The draw pile is enabled for exactly the player whose turn it is.
    return this.page.$eval("#deck", (e) => !e.disabled).catch(() => false);
  }

  async logLines() {
    return this.page.$$eval("#log li", (els) =>
      els.map((e) => e.textContent.replace(/\s+/g, " ").trim()),
    );
  }

  async hint() {
    return (await this.$("#hint").textContent()) || "";
  }

  async turnBanner() {
    return (await this.$("#turn-banner").textContent()) || "";
  }

  // Turning the hand face-up again: the cover is a privacy feature, so a driver
  // has to peek before it can pick anything.
  async reveal() {
    for (const back of await this.page.$$("#hand .card.hidden-face")) {
      await back.click().catch(() => {});
    }
  }

  // Bounded on purpose: in a live game a card can leave your hand, or a modal
  // can open over it, between choosing it and tapping it. Callers decide what to
  // do about a refusal rather than hanging for the default thirty seconds.
  async selectCard(id, timeout = 2500) {
    return safeClick(this.$(`#hand .card[data-id="${id}"]`), timeout);
  }

  async modalOpen() {
    return visible(this.$("#modal"));
  }

  // pass closes an open Nope window from this player's side. With a twenty
  // second window, a driver that does not do this spends the whole game waiting.
  async passNope() {
    return safeClick(this.$("#pass-btn"), 500);
  }

  async screenshot(name) {
    await this.page.screenshot({ path: `shots/${name}.png`, fullPage: false });
  }
}

// clearNope gets every player to stand down so the pending action resolves at
// once instead of after the full window.
export async function clearNope(players) {
  for (const p of players) await p.passNope();
}

// clearModals dismisses whatever prompt each player is sitting on. Any test that
// clicks table furniture needs this first: an open modal covers the whole screen,
// so the click lands on the overlay and waits out its timeout instead. That has
// caught the log's collapse button more than once.
export async function clearModals(players) {
  for (const p of players) {
    if (!(await visible(p.$("#modal")))) continue;
    // In the order a player would: confirm a placement, hand a card over, then
    // whichever plain button is offered.
    if (await safeClick(p.$("#place-confirm"), 400)) continue;
    if (await safeClick(p.$("#modal-cards .card").first(), 400)) continue;
    if (await safeClick(p.$(".demand-pick").first(), 400)) continue;
    if (await safeClick(p.$("#modal-ok"), 400)) continue;
    await safeClick(p.$("#modal-alt"), 400);
  }
}

// ─────────────────────────────────────────────────────────── server

// Rooms are in-memory, so the tests talk to the same HTTP API the client does
// rather than reaching into the server.
export async function listRooms() {
  const res = await fetch(`${BASE}/api/rooms`);
  assert(res.ok, `GET /api/rooms returned ${res.status}`);
  // The endpoint wraps the list in an object so it has somewhere to grow.
  const body = await res.json();
  return body.rooms || [];
}

export async function listGames() {
  const res = await fetch(`${BASE}/api/games`);
  assert(res.ok, `GET /api/games returned ${res.status}`);
  return (await res.json()).games || [];
}

export async function serverUp() {
  try {
    const res = await fetch(`${BASE}/healthz`);
    return res.ok;
  } catch {
    return false;
  }
}

export async function requireServer() {
  if (await serverUp()) return;
  console.error(
    `No server on ${BASE}.\n` +
      `Build and start it first (the client is embedded, so a stale binary\n` +
      `serves stale assets):\n\n` +
      `  go build -o kittens.exe ./cmd/server\n` +
      `  ./kittens.exe -addr :8099\n`,
  );
  process.exit(2);
}
