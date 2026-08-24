// Drives a whole three-player game to a winner with real clicks, and reports
// which mechanics it happened to exercise on the way.
//
// The game is random, so this is a fuzz test with a browser attached rather than
// a fixed script: it asserts the invariants that must hold on every path (one
// player on turn, nobody ever sees another hand, the game ends with exactly one
// survivor) and prints the coverage it got so a run that never hit a defuse is
// visibly weaker than one that did.
//
// MOBILE=1 runs it on a phone viewport.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  safeClick, visible, clearNope, requireServer,
} from "./lib.js";

await requireServer();
console.log("full game");

// Cats are the only cards that stack, and they only stack with their own kind.
const isCat = (slug) => slug.startsWith("cat-");
// Cards that do something on their own when played from the hand.
const SINGLES = ["skip", "attack", "shuffle", "future", "favor"];

const hit = new Set();
const MAX_STEPS = 400;

// modals.js covers the Favor picker in detail. This catches the same fault in
// any other modal the game happens to open — the demand grid especially, which
// is twelve tiles and only shows up when somebody is dealt three matching cats.
// Declared up here rather than beside its helper: the checks below run before
// the bottom of the file is reached, and a `const` down there is not yet
// initialised when they do.
const overflowed = [];

const browser = await launch();
const phone = process.env.MOBILE === "1";
const players = [
  await seat(browser, "Alex", { phone }),
  await seat(browser, "Bea", { phone }),
  await seat(browser, "Cy", { phone }),
];
const [host] = players;

try {
  await step("three players reach a dealt table", async () => {
    const code = await host.create();
    for (const p of players.slice(1)) await p.join(code);
    for (const p of players) {
      await waitFor(async () => (await p.lobbyNames()).length === 3, {
        what: `${p.name} to see everyone`,
      });
    }
    await host.deal();
    for (const p of players) await p.waitForScreen("table");
  });

  let winner = "";
  let steps = 0;

  await step("the game plays through to a winner", async () => {
    while (!winner && steps < MAX_STEPS) {
      steps++;

      // Anything sitting in a modal blocks the table, so it goes first.
      for (const p of players) {
        const done = await answerModal(p);
        if (done) { winner = done; break; }
      }
      if (winner) break;

      winner = await winnerFromLog(host);
      if (winner) break;

      // Twenty seconds is a long time to wait for a window nobody wants to use.
      await clearNope(players);

      const current = await whoseTurn();
      if (!current) { await sleep(120); continue; }

      await takeTurn(current);
    }
    assert(winner, `no winner after ${steps} steps`);
  });

  await check("exactly one player is left standing", async () => {
    const alive = await host.page.$$eval(
      ".seats .seat",
      (els) => els.filter((e) => !e.classList.contains("dead")).length,
    );
    assert(alive === 1, `expected one survivor, the table shows ${alive}`);
  });

  await check("every client agrees who won", async () => {
    // The loop above stops as soon as *one* client shows the result, so the
    // others may still have the broadcast in flight. Wait for them rather than
    // reading a snapshot that is a few milliseconds too early.
    for (const p of players) {
      await waitFor(
        async () => (await p.logLines()).some((l) => /wins!/.test(l)),
        { timeout: 5000, what: `${p.name} to see the winner` },
      );
    }
    // The log is written from each reader's point of view, so the winner's own
    // client says "You wins" where everyone else names them. That is the right
    // behaviour, and it means the agreement to check is: exactly one client sees
    // itself win, and the rest name the same player.
    const named = new Set();
    let sawItself = 0;
    for (const p of players) {
      const line = (await p.logLines()).find((l) => /wins!/.test(l));
      const who = line.replace(/[^A-Za-z ]/g, "").replace(/\s*wins\s*$/, "").trim();
      if (who === "You") sawItself++;
      else named.add(who);
    }
    assert(sawItself === 1, `${sawItself} clients think they won, want exactly 1`);
    assert(named.size === 1, `the losers disagree on who won: ${[...named]}`);
  });

  await check("no modal ran off the screen", async () => {
    assert(
      overflowed.length === 0,
      `modals taller than the window: ${overflowed.join("; ")}`,
    );
  });

  await check("no opponent's card was ever rendered", async () => {
    // Belt and braces next to the Go leak test: if the client ever drew a face
    // for another seat, the markup would show it.
    for (const p of players) {
      const faces = await p.page.$$eval(".seats .card[data-slug]", (els) => els.length);
      assert(faces === 0, `${p.name}'s table renders ${faces} opponent card faces`);
    }
  });

  console.log(`\n  played ${steps} steps; exercised: ${[...hit].sort().join(", ") || "nothing notable"}`);
  await host.screenshot(phone ? "gameover-phone" : "gameover");
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("full game");

// ─────────────────────────────────────────────────────────── the driver

// answerModal deals with whatever prompt is on this player's screen. It returns
// the winner's name if the modal it found was the end of the game, so the caller
// stops rather than clicking "Deal again" and starting a second match.
async function answerModal(p) {
  if (!(await visible(p.$("#modal")))) return "";

  await noteIfModalOverflows(p);
  const title = (await p.$("#modal-title").textContent()) || "";

  if (/survived/i.test(title)) {
    hit.add("gameover");
    return title.replace(/[^A-Za-z ]/g, "").trim() || "someone";
  }

  // Defuse: slide the kitten back in somewhere.
  if (await visible(p.$("#modal-place"))) {
    hit.add("defuse");
    const quick = ["top", "middle", "random", "bottom"];
    await safeClick(p.$(`[data-place="${quick[Math.floor(Math.random() * quick.length)]}"]`));
    await safeClick(p.$("#place-confirm"));
    return "";
  }

  // Three of a kind: name the card you want.
  if (await visible(p.$(".demand-pick").first())) {
    hit.add("demand");
    const picks = p.$(".demand-pick");
    const n = await picks.count();
    await safeClick(picks.nth(Math.floor(Math.random() * n)));
    return "";
  }

  // Favor: the target chooses which card to hand over.
  if (/hand one over/i.test(title)) {
    hit.add("favor");
    await safeClick(p.$("#modal-cards .card").first());
    return "";
  }

  if (/future/i.test(title)) hit.add("future");

  if (!(await safeClick(p.$("#modal-ok"), 800))) {
    await safeClick(p.$("#modal-alt"), 800);
  }
  return "";
}

async function noteIfModalOverflows(p) {
  const bad = await p.page.evaluate(() => {
    const box = document.querySelector(".modal-box");
    if (!box) return null;
    const r = box.getBoundingClientRect();
    const vh = window.innerHeight;
    if (r.top >= -1 && r.bottom <= vh + 1) return null;
    const kind = document.getElementById("modal").dataset.kind || "?";
    return `${kind}: ${Math.round(r.height)}px box in a ${vh}px viewport`;
  });
  if (bad && !overflowed.includes(bad)) overflowed.push(bad);
}

async function winnerFromLog(p) {
  const lines = await p.logLines();
  const win = lines.find((l) => /wins!/.test(l));
  return win ? win.replace(/[^A-Za-z ]/g, "").trim() : "";
}

// The draw pile is enabled for exactly one seat, which is how a player knows it
// is their go — so it is how the driver knows too.
async function whoseTurn() {
  const on = [];
  for (const p of players) if (await p.isMyTurn()) on.push(p);
  assert(on.length <= 1, `${on.length} players have the turn at once: ${on.map((p) => p.name)}`);
  return on[0] || null;
}

async function takeTurn(p) {
  // A modal can open between the sweep above and this call — Favor puts one on
  // the target's screen the moment it resolves. Anything still up owns the
  // screen, so hand the turn back and come round again.
  if (await p.modalOpen()) {
    await answerModal(p);
    return;
  }
  await p.reveal();

  // Mostly draw — a game where everybody plays every card never gets anywhere
  // near the deck, and drawing is what ends turns and eventually explodes people.
  if (Math.random() < 0.55 && (await playSomething(p))) return;

  hit.add("draw");
  await safeClick(p.$("#deck"));
}

async function playSomething(p) {
  const hand = await p.hand();

  // Matching cats, biggest set first: three is a demand, two is a random steal.
  const groups = {};
  for (const c of hand) if (isCat(c.slug)) (groups[c.slug] ||= []).push(c);
  const set = Object.values(groups).sort((a, b) => b.length - a.length)[0];

  if (set && set.length >= 2 && Math.random() < 0.6) {
    const take = set.length >= 3 && Math.random() < 0.5 ? 3 : 2;
    for (const c of set.slice(0, take)) {
      if (!(await p.selectCard(c.id))) return false;
    }
    hit.add(take === 3 ? "catTrio" : "catPair");
    return finishPlay(p, { needsTarget: true });
  }

  const single = hand.filter((c) => SINGLES.includes(c.slug));
  if (!single.length) return false;
  const c = single[Math.floor(Math.random() * single.length)];
  if (!(await p.selectCard(c.id))) return false;
  hit.add(c.slug);
  return finishPlay(p, { needsTarget: c.slug === "favor" });
}

async function finishPlay(p, { needsTarget }) {
  if (await p.$("#play-btn").isDisabled()) return false;
  await safeClick(p.$("#play-btn"));

  if (needsTarget) {
    const targets = p.$("#target-buttons .btn");
    await waitFor(async () => (await targets.count()) > 0, {
      timeout: 3000, what: "a target list",
    }).catch(() => {});
    const n = await targets.count();
    if (!n) return false;
    await safeClick(targets.nth(Math.floor(Math.random() * n)));
  }

  // Somebody occasionally uses the window it opened, which is the only way the
  // Nope path gets exercised at all.
  await maybeNope();
  return true;
}

async function maybeNope() {
  for (const p of players) {
    if (Math.random() > 0.25) continue;
    if (await safeClick(p.$("#nope-btn"), 400)) {
      hit.add("nope");
      return;
    }
  }
}
