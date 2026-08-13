// Card selection has rules of its own: one non-cat card at a time, and cats only
// stack with matching cats. The point of these checks is that a refusal is
// *visible* — an incompatible card is dimmed before you touch it, and tapping it
// explains itself rather than doing nothing.
//
// Also covers the two lobby buttons, which have no server behaviour to test in
// Go: mute, and leaving the room properly.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  requireServer,
} from "./lib.js";

await requireServer();
console.log("selection and lobby controls");

const isCat = (slug) => slug.startsWith("cat-");

// A full table on purpose: with only three hands a matching cat pair is dealt
// barely half the time, which would leave the stacking rule untested on most
// runs. Five hands make it near-certain.
const browser = await launch();
const players = [];
for (const name of ["Alex", "Bea", "Cy", "Dee", "Eve"]) {
  players.push(await seat(browser, name));
}
const [host, bea] = players;
const seated = (p, n) =>
  waitFor(async () => (await p.lobbyNames()).length === n, {
    what: `${p.name} to see ${n} players`,
  });

try {
  let code;

  await step("a full table gathers in the lobby", async () => {
    code = await host.create();
    for (const p of players.slice(1)) await p.join(code);
    for (const p of players) await seated(p, players.length);
  });

  await check("mute in the lobby toggles and is remembered", async () => {
    const mute = bea.$("#sound-lobby .mute");
    assert(await mute.isVisible(), "there is no mute button in the lobby");
    assert((await mute.getAttribute("aria-pressed")) === "false", "sound starts muted");
    await mute.click();
    await waitFor(async () => (await mute.getAttribute("aria-pressed")) === "true", {
      what: "the mute button to switch on",
    });
    const stored = await bea.page.evaluate(() => localStorage.getItem("ek:muted"));
    assert(stored === "1", `mute was not stored, got ${JSON.stringify(stored)}`);
    await mute.click(); // leave the sound on for the rest of the run
  });

  await check("back to menu leaves the room and frees the seat", async () => {
    const cy = players[2];
    await cy.$("#leave-btn").click();
    await cy.waitForScreen("menu");

    // Invite links are …/#CODE, so leaving has to clear the hash or a refresh
    // would walk straight back into the room just left.
    const url = cy.page.url();
    assert(!/#\w/.test(url), `the room code is still in the URL: ${url}`);

    // The seat has to actually be released, or the room fills up with ghosts.
    await seated(host, players.length - 1);

    // And a reload must not walk straight back in.
    await cy.page.reload();
    await cy.waitForScreen("menu");
    await cy.join(code);
    await seated(host, players.length);
  });

  await step("the cards are dealt", async () => {
    await host.deal();
    for (const p of players) await p.waitForScreen("table");
  });

  await check("one non-cat card blocks everything else", async () => {
    const { player, card } = await findCard((c) => !isCat(c.slug) && c.slug !== "exploding");
    await player.selectCard(card.id);
    const hand = await player.hand();
    for (const c of hand) {
      if (c.id === card.id) {
        assert(c.selected, "the tapped card is not selected");
        continue;
      }
      assert(c.blocked, `${c.slug} is still selectable alongside ${card.slug}`);
    }
    await player.selectCard(card.id); // deselect
  });

  await check("cats stack only with matching cats", async () => {
    const { player, set } = await findCatSet(2);
    await player.selectCard(set[0].id);
    const hand = await player.hand();
    for (const c of hand) {
      if (c.id === set[0].id) continue;
      const shouldAllow = c.slug === set[0].slug;
      assert(
        c.blocked !== shouldAllow,
        `${c.slug} ${c.blocked ? "is blocked" : "is selectable"} next to ${set[0].slug}`,
      );
    }
    await player.selectCard(set[0].id);
  });

  await check("a blocked card explains itself instead of doing nothing", async () => {
    const { player, card } = await findCard((c) => !isCat(c.slug) && c.slug !== "exploding");
    await player.selectCard(card.id);

    const blocked = (await player.hand()).find((c) => c.blocked);
    assert(blocked, "nothing was blocked, so there is nothing to tap");
    await player.selectCard(blocked.id);

    const hint = await player.hint();
    assert(hint.trim().length > 0, "tapping a blocked card said nothing");
    const warned = await player.page.$eval("#hint", (el) => el.classList.contains("warn"));
    assert(warned, `the refusal was not flagged as a warning: ${JSON.stringify(hint)}`);

    const after = await player.hand();
    assert(
      !after.find((c) => c.id === blocked.id).selected,
      "a blocked card got selected anyway",
    );
    for (const c of after.filter((x) => x.selected)) await player.selectCard(c.id);
  });

  // Deliberately not tested here: the cap at three, and building a trio. Both
  // need three or four of one cat in a single dealt hand, which is rare enough
  // that the check would be skipped nearly every run — and a check that never
  // runs is worse than none, because it trains you to ignore the skip line. The
  // cap is enforced by the engine and covered in internal/games/kittens/game;
  // play.js exercises an actual trio whenever the deal hands it one.

  await check("the hand can be hidden from the person next to you", async () => {
    const p = host;
    await p.$("#hand-cover").click();
    await sleep(200);
    const hand = await p.hand();
    assert(
      hand.every((c) => c.faceDown),
      `${hand.filter((c) => !c.faceDown).length} cards stayed face up`,
    );
    await p.$("#hand-cover").click();
  });
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("selection and lobby controls");

// ─────────────────────────────────────────────────────────── helpers

// The deal is random, so the tests hunt for a hand that can demonstrate the rule
// rather than assuming one. Twenty-four cards are dealt, so this nearly always
// finds what it needs on the first look.
async function findCard(pred) {
  for (const player of players) {
    const card = (await player.hand()).find(pred);
    if (card) return { player, card };
  }
  throw new Error("no player was dealt a card matching the predicate");
}

async function findCatSet(n) {
  for (const player of players) {
    const groups = {};
    for (const c of await player.hand()) if (isCat(c.slug)) (groups[c.slug] ||= []).push(c);
    const set = Object.values(groups).find((g) => g.length >= n);
    if (set) return { player, set };
  }
  throw new Error(`nobody was dealt ${n} matching cats`);
}
