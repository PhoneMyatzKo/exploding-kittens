// Drives a whole UNO game to a winner with real clicks, and reports which
// mechanics it happened to exercise on the way.
//
// The deal is random, so this is a fuzz test with a browser attached rather than
// a fixed script: it asserts the invariants that must hold on every path (one
// player on turn, nobody ever sees another hand, a winner is reached) and prints
// the coverage it got, so a run that never saw a Draw Four is visibly weaker
// than one that did.
//
// MOBILE=1 runs it on a phone viewport.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  safeClick, visible, requireServer,
} from "./lib.js";

await requireServer();
console.log("uno: a full game");

const hit = new Set();
const MAX_STEPS = 400;

// ── reading the table. Everything comes out of the DOM, because the client keeps
// its state in a module scope with nothing on window — so a test can only assert
// on what a player can actually see, which is what it should be asserting on.

const hand = (p) =>
  p.page.$$eval("#hand .uno-card", (els) =>
    els.map((e) => ({
      id: Number(e.dataset.id || 0),
      colour: e.dataset.colour || "",
      rank: e.dataset.rank || "",
      playable: e.classList.contains("playable"),
    })),
  );

const promptLabel = async (p) =>
  (await visible(p.$("#uno-prompt"))) ? (await p.$("#uno-prompt-label").textContent()).trim() : "";

const browser = await launch();
const phone = process.env.MOBILE === "1";
const players = [
  await seat(browser, "Ana", { phone }),
  await seat(browser, "Ben", { phone }),
  await seat(browser, "Caz", { phone }),
];
const [host] = players;

try {
  await step("three players reach a dealt UNO table", async () => {
    const code = await host.create({ game: "uno" });
    for (const p of players.slice(1)) await p.join(code, { game: "uno" });
    for (const p of players) {
      await waitFor(async () => (await p.lobbyNames()).length === 3, {
        what: `${p.name} to see everyone`,
      });
    }
    await host.deal();
    for (const p of players) await p.waitForScreen("table");
  });

  await check("everybody is dealt seven cards, and sees only their own", async () => {
    for (const p of players) {
      const mine = await hand(p);
      // Seven each, unless the card turned over was a Draw Two and this is the
      // player it landed on.
      assert(mine.length === 7 || mine.length === 9,
        `${p.name} holds ${mine.length} cards, want 7 (or 9 off an opening Draw Two)`);
      // Three players hold 21 cards between them; only a hand and the pile are
      // ever drawn on one screen.
      const all = await p.page.$$(".uno-card");
      assert(all.length <= mine.length + 2,
        `${p.name} can see ${all.length} cards holding ${mine.length} — a hand has leaked`);
    }
  });

  let winner = "";
  let steps = 0;
  let stalled = 0;
  let unchanged = 0;
  let lastSignature = "";

  await step("the game plays through to a winner", async () => {
    while (!winner && steps < MAX_STEPS) {
      steps++;

      // Read the winner before touching anything: the host's game-over card has
      // a "Deal again" button on it, and clicking that would restart the game
      // and wipe the log this is looking for.
      winner = await winnerFromLog(host);
      if (winner) break;

      // A prompt blocks its player and therefore the table, so it goes first.
      for (const p of players) {
        if (await answerPrompt(p)) break;
      }

      // Somebody caught out on one card is two free cards, and the button is only
      // up for a turn — so it is taken the moment it appears.
      for (const p of players) {
        if (await safeClick(p.$("#uno-catch"), 200)) hit.add("caught a silent UNO");
      }

      // A table that is technically answering somebody but never actually
      // changes is the other way this can hang, and the one a step budget
      // reports uselessly as "no winner after 400 steps".
      const now = await signature();
      if (now === lastSignature) {
        unchanged++;
        assert(unchanged < 40, `nothing has changed for 40 steps:\n      ${await describe()}`);
      } else {
        unchanged = 0;
        lastSignature = now;
      }

      const current = await whoseTurn();
      if (!current) {
        stalled++;
        // A table waiting on nobody is the failure worth reporting in detail:
        // "no winner after 600 steps" would say nothing about why.
        assert(stalled < 25, `the table is waiting on nobody: ${await describe()}`);
        await sleep(120);
        continue;
      }
      stalled = 0;
      await takeTurn(current);
    }
    assert(winner, `no winner after ${steps} steps`);
  });

  await check("the table ends on the winner", async () => {
    const banner = await host.turnBanner();
    assert(/won with \d+ points/.test(banner), `banner reads ${JSON.stringify(banner)}`);
  });

  await check("every player's log agrees on who won", async () => {
    for (const p of players) {
      const lines = await p.logLines();
      assert(lines.some((l) => l.includes("wins with")),
        `${p.name}'s log never says who won; last lines: ${JSON.stringify(lines.slice(-4))}`);
    }
  });

  await host.screenshot("uno-table");
} finally {
  console.log(`  covered: ${[...hit].sort().join(", ") || "nothing much"}`);
  await browser.close();
}

report("uno");

// ── driving

async function whoseTurn() {
  for (const p of players) {
    if (await p.isMyTurn()) return p;
    // Three states hold the table up with the deck disabled: owing a colour,
    // answering a Draw Four, and sitting on a card you have just drawn.
    if (await promptLabel(p)) return p;
    if (await visible(p.$("#uno-pass"))) return p;
  }
  return null;
}

async function takeTurn(p) {
  if (await answerPrompt(p)) return;

  const mine = await hand(p);
  const playable = mine.filter((c) => c.playable);

  if (playable.length) {
    const card = playable[Math.floor(Math.random() * playable.length)];
    // Down to one card after this, so say it — but only half the time, because
    // forgetting is the path the catch button is on.
    if (mine.length === 2 && Math.random() < 0.5) {
      if (await safeClick(p.$("#uno-call"), 300)) hit.add("said UNO with the play");
    }
    if (await safeClick(p.$(`#hand .uno-card[data-id="${card.id}"]`), 1500)) {
      hit.add(`played a ${card.rank}`);
      await answerPrompt(p); // a wild stops for its colour before anything is sent
    }
    return;
  }

  await safeClick(p.$("#deck"), 1500);
  hit.add("drew from the deck");

  // A playable draw leaves the turn open: play it or keep it, both are legal.
  if (await visible(p.$("#uno-pass"))) {
    hit.add("drew something playable");
    const drawn = (await hand(p)).filter((c) => c.playable);
    if (drawn.length && Math.random() < 0.7) {
      await safeClick(p.$(`#hand .uno-card[data-id="${drawn[0].id}"]`), 1000);
      await answerPrompt(p);
    } else {
      await safeClick(p.$("#uno-pass"), 1000);
      hit.add("kept the card it drew");
    }
  }
}

// answerPrompt deals with whichever blocking prompt is up. Returns whether it
// did anything, so callers can re-read the table afterwards.
async function answerPrompt(p) {
  const label = await promptLabel(p);
  if (!label) return await answerModal(p);

  if (/colour/i.test(label)) {
    hit.add(label.startsWith("Play it") ? "named a colour for a wild in hand" : "named a colour");
    const n = await p.page.locator("#uno-prompt-actions .uno-swatch").count();
    // Not an assertion: the row is rebuilt on every server message, so reading
    // the label and clicking a swatch are two moments, and the prompt may have
    // been answered or replaced in between. The loop comes round again.
    if (n < 4) return false;
    const pick = Math.floor(Math.random() * 4);
    return await safeClick(p.page.locator("#uno-prompt-actions .uno-swatch").nth(pick), 1000);
  }

  if (/Wild Draw Four/i.test(label)) {
    // Both answers are worth exercising, and a challenge is the only way the
    // bluff path ever runs.
    const challenge = Math.random() < 0.5;
    hit.add(challenge ? "challenged a Draw Four" : "took the four");
    await safeClick(
      p.$(challenge ? "#uno-prompt-actions .btn.ghost" : "#uno-prompt-actions .btn.primary"), 1000);
    return true;
  }

  return false;
}

// answerModal clears whatever is sitting over the table: a hand revealed by a
// challenge, or the game-over card.
async function answerModal(p) {
  if (!(await p.modalOpen())) return false;
  const title = (await p.$("#modal-title").textContent()) || "";
  // The game-over card is left alone deliberately: its buttons deal a new game
  // or reopen the lobby, and the checks after the loop read the finished table.
  if (title.includes("🏆")) return false;
  if (/hand$/.test(title)) hit.add("saw a challenged hand");
  await safeClick(p.$("#modal-ok"), 800);
  return true;
}

// describe is only ever printed on a stall: every player's banner and hint, so
// the report says which phase the table died in.
async function describe() {
  const out = [];
  for (const p of players) {
    out.push([
      p.name,
      `banner=${JSON.stringify(await p.turnBanner())}`,
      `hint=${JSON.stringify(await p.hint())}`,
      `prompt=${JSON.stringify(await promptLabel(p))}`,
      `deck=${(await p.isMyTurn()) ? "live" : "disabled"}`,
      `modal=${(await p.modalOpen()) ? JSON.stringify(await p.$("#modal-title").textContent()) : "none"}`,
      `hand=${(await hand(p)).length}`,
      `pass=${(await visible(p.$("#uno-pass"))) ? "yes" : "no"}`,
    ].join(" "));
  }
  return out.join("\n      ");
}

// signature is a cheap "has anything happened" fingerprint: whose turn it is,
// and how many cards each player is holding.
async function signature() {
  const parts = [];
  for (const p of players) {
    parts.push(await p.turnBanner(), String((await hand(p)).length));
  }
  return parts.join("|");
}

async function winnerFromLog(p) {
  const lines = await p.logLines();
  return lines.find((l) => l.includes("wins with")) || "";
}
