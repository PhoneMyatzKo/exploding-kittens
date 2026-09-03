// Surviving a restart, with real browsers.
//
// The Go tests prove a room round-trips through the state file. What they cannot
// see is the half that matters to a player: the browser is holding a seat token
// in localStorage, the socket drops when the server goes away, and the client has
// to reconnect on its own and land back in the same seat looking at the same
// table. Nobody should have to do anything.
//
// This script owns the server process, unlike every other script here — it has to
// stop and start it. It uses its own port and its own state file so a suite run
// cannot trip over it.

import { spawn } from "node:child_process";
import { rm } from "node:fs/promises";
import path from "node:path";
import {
  launch, seat, step, check, report, assert, waitFor, sleep, safeClick,
} from "./lib.js";

const PORT = 8123;
const BASE = `http://localhost:${PORT}`;
const ROOT = path.resolve(import.meta.dirname, "../..");
const STATE = path.join(ROOT, "restart-test-state.json");
const EXE = path.join(ROOT, "kittens.exe");

console.log("restart");

let server = null;
const browser = await launch();

try {
  await rm(STATE, { force: true });
  await startServer();
  await theGameComesBack();
} catch (e) {
  console.log(`  ! ${e && e.message}`);
} finally {
  await browser.close();
  await stopServer();
  await rm(STATE, { force: true });
}

report("restart");

async function theGameComesBack() {
  const players = [
    await seat(browser, "Aung", { base: BASE }),
    await seat(browser, "Bo", { base: BASE }),
  ];
  const [host] = players;

  let code = "";
  let before = null;

  await step("a game is dealt and played into", async () => {
    code = await host.create({ game: "kittens" });
    await players[1].join(code);
    for (const p of players) {
      await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
    }
    await host.deal();
    for (const p of players) await p.waitForScreen("table");

    // A few turns, so there is a game worth losing rather than a fresh deal.
    for (let i = 0; i < 6; i++) {
      for (const p of players) {
        if (await p.isMyTurn()) {
          await safeClick(p.$("#deck"), 1500);
          break;
        }
      }
      await sleep(150);
      for (const p of players) await safeClick(p.$("#pass-btn"), 300);
    }
    before = await tableOf(host);
    assert(before.hand > 0, "nothing was dealt");
  });

  await step("the server is restarted underneath them", async () => {
    // No page reloads, no clicks: exactly what a deploy does to a live game.
    await stopServer();
    await sleep(600);
    await startServer();
  });

  await check("the table comes back on its own", async () => {
    // Waiting on the *connection*, not on #table being visible. The table stays
    // on screen the whole time the socket is down — the first version of this
    // check passed against leftover DOM while both clients were disconnected,
    // which is exactly the false pass worth avoiding.
    await waitFor(async () => (await tableOf(host)).connected, {
      timeout: 30_000, interval: 500, what: "the socket to come back",
    });

    const after = await tableOf(host);
    assert(after.code === before.code, `the room code changed from ${before.code} to ${after.code}`);
    assert(after.hand === before.hand, `came back holding ${after.hand} cards, had ${before.hand}`);
    assert(after.deck === before.deck, `the deck has ${after.deck}, had ${before.deck}`);
    assert(after.seats === before.seats, `${after.seats} seats came back, had ${before.seats}`);
    // The *shared* play-by-play is what makes it feel like the same game rather
    // than a new one that happens to have the same cards in it.
    //
    // Only the shared part, and that is not a shortcut: lines about what only you
    // saw — "You drew Nope" — are sent privately and were never in the log the
    // room keeps, because the room keeps the log every client replays. They do
    // not survive *any* reconnect, restart or not, so demanding them here would
    // be asserting the redaction boundary is broken.
    assert(
      after.shared >= before.shared,
      `the shared log shrank from ${before.shared} to ${after.shared} lines`,
    );
    assert(after.dealt, "the restored log does not mention the deal, so it reads as a new game");
  });

  await check("both players are back in their own seats", async () => {
    for (const p of players) {
      await waitFor(async () => {
        const t = await tableOf(p);
        return t.connected && t.started;
      }, { timeout: 30_000, interval: 500, what: `${p.name}'s table` });
      const t = await tableOf(p);
      assert(t.youNamed, `${p.name} cannot see which seat is theirs`);
      assert(t.hand > 0, `${p.name} came back with an empty hand`);
    }
  });

  await check("and the game can still be played", async () => {
    // The point of all of it: the restored table takes moves. A state that
    // reloads but refuses input would pass every check above.
    //
    // Driven until the *log* grows, not until a click lands. Straight after a
    // reconnect a client can still be showing the turn it had before the socket
    // dropped, so the first click may be aimed at the wrong seat and refused —
    // which is a race in this script, not a bug in the game, and stopping at the
    // first click made it look like the table was dead.
    const logBefore = (await tableOf(host)).log;
    let grew = false;
    for (let i = 0; i < 60 && !grew; i++) {
      for (const p of players) {
        if (await p.isMyTurn()) {
          await safeClick(p.$("#deck"), 1500);
          break;
        }
      }
      await sleep(200);
      for (const p of players) await safeClick(p.$("#pass-btn"), 300);
      grew = (await tableOf(host)).log > logBefore;
    }
    assert(grew, "no move after the restart ever reached the play-by-play");
  });
}

// ─────────────────────────────────────────────────────────────────── helpers

function tableOf(p) {
  return p.page.evaluate(() => {
    const table = document.getElementById("table");
    const seats = [...document.querySelectorAll(".seats .seat")];
    return {
      started: Boolean(table) && !table.hidden,
      code: (document.getElementById("table-code") || {}).textContent || "",
      hand: document.querySelectorAll("#hand .card").length,
      deck: Number(((document.getElementById("deck-count") || {}).textContent || "0").replace(/\D/g, "")),
      seats: seats.length,
      log: document.querySelectorAll("#log li").length,
      youNamed: seats.some((s) => /\(you\)/.test(s.textContent)),
      // The shell shows this whenever the socket is down, so its absence is the
      // one honest signal that the client is talking to a server again.
      connected: Boolean(document.getElementById("conn-warning")?.hidden),
      // Lines everybody can see, as opposed to the private "You drew …" ones.
      shared: [...document.querySelectorAll("#log li")]
        .filter((l) => !/^You /.test(l.textContent)).length,
      dealt: [...document.querySelectorAll("#log li")]
        .some((l) => /cards are dealt/i.test(l.textContent)),
    };
  });
}

function startServer() {
  return new Promise((resolve, reject) => {
    server = spawn(EXE, ["-addr", `:${PORT}`, "-state", STATE, "-state-every", "500ms"], {
      cwd: ROOT, stdio: ["ignore", "pipe", "pipe"],
    });
    server.on("error", reject);

    // Poll rather than trusting a log line: the binary prints its addresses
    // before the listener is necessarily accepting.
    (async () => {
      for (let i = 0; i < 100; i++) {
        try {
          const res = await fetch(`${BASE}/api/games`);
          if (res.ok) return resolve();
        } catch {}
        await sleep(100);
      }
      reject(new Error(`the server never came up on ${BASE}`));
    })();
  });
}

// Killed outright, not asked nicely — and that is the point rather than a
// shortcut. Node cannot send a real SIGINT on Windows (every signal maps to
// TerminateProcess), so the graceful save is not reachable from here at all; and
// it is the *un*graceful exit that a crash or a power cut looks like. What this
// tests is therefore the checkpoint, not the farewell note.
async function stopServer() {
  if (!server) return;
  const dead = new Promise((r) => server.on("exit", r));
  server.kill();
  await Promise.race([dead, sleep(6000)]);
  server = null;
}
