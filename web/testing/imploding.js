// The Imploding Kittens expansion. play.js already drives a whole game on this
// deck; what is left is the things a random game cannot be relied on to reach,
// and the two rules a player would notice immediately if they were wrong: the
// sixth seat, and the kitten no Defuse stops.
//
// A two-player expansion game is the useful shape here. The deck holds
// players-1 kittens, one of which is always the Imploding Kitten — so at two
// players it is the *only* kitten in the deck, and drawing until somebody dies
// is guaranteed to go through it.

import {
  launch, seat, step, check, skip, report, assert, waitFor, sleep,
  safeClick, visible, clearNope, clearModals, listGames, requireServer, BASE,
} from "./lib.js";

await requireServer();
console.log("imploding kittens");

const GAME = "kittens-imploding";
const browser = await launch();

try {
  await menuAndRules();
  await sixSeats();
  await theKitten();
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("imploding kittens");

// ───────────────────────────────────────────────── the tile and the rules sheet

async function menuAndRules() {
  const p = await seat(browser, "Alex");
  try {
    await check("the expansion has a playable tile seating six", async () => {
      const listed = (await listGames()).find((g) => g.slug === GAME);
      assert(listed, `${GAME} is not in the catalogue`);
      assert(listed.playable, "the expansion tile is not playable");
      assert(listed.max === 6, `the tile says up to ${listed.max} players, want 6`);
      assert(listed.min === 2, `the tile says from ${listed.min} players, want 2`);
    });

    await check("the demand list follows the deck", async () => {
      // The expansion adds five more kinds somebody could be holding, so a Three
      // of a Kind has to be able to ask for them — and must not on the base deck.
      const base = await demandable("kittens");
      const with_ = await demandable(GAME);
      for (const slug of ["reverse", "bottom", "feral-cat", "alter", "targeted-attack"]) {
        assert(!base.includes(slug), `${slug} is demandable on the base deck`);
        assert(with_.includes(slug), `${slug} is in the expansion but cannot be demanded`);
      }
      for (const slug of ["exploding", "imploding"]) {
        assert(!with_.includes(slug), `${slug} is demandable, but is never in a hand`);
      }
    });

    await check("the expansion's rules are shown for it and hidden otherwise", async () => {
      // One expansion block per language, so this checks the English one.
      const sel = '.rules-lang[data-lang="en"] .rules-expansion';

      await p.home("kittens");
      assert(
        await p.page.$eval(sel, (el) => el.hidden),
        "the expansion rules are shown on the base game",
      );

      await p.home(GAME);
      assert(
        !(await p.page.$eval(sel, (el) => el.hidden)),
        "the expansion rules are hidden on the expansion",
      );
      // And they actually explain the new cards.
      const text = await p.page.$eval(sel, (el) => el.textContent);
      for (const name of [
        "Imploding Kitten", "Reverse", "Draw From the Bottom",
        "Feral Cat", "Alter the Future", "Targeted Attack",
      ]) {
        assert(text.includes(name), `the rules never mention ${name}`);
      }
    });
  } finally {
    await p.page.context().close();
  }
}

// ─────────────────────────────────────────────────────────────── the sixth seat

async function sixSeats() {
  const players = [];
  for (const name of ["Alex", "Bea", "Cy", "Dee", "Eve", "Fin"]) {
    players.push(await seat(browser, name));
  }
  const [host] = players;

  try {
    await step("six players fit an expansion table", async () => {
      const code = await host.create({ game: GAME });
      for (const p of players.slice(1)) await p.join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 6, {
          what: `${p.name} to see all six`,
        });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
    });

    await check("all six are dealt in", async () => {
      for (const p of players) {
        await waitFor(async () => (await p.hand()).length === 8, {
          what: `${p.name} to hold eight cards`,
        });
      }
      const seats = await host.page.$$eval(".seats .seat", (els) => els.length);
      assert(seats === 6, `the table shows ${seats} seats, want 6`);
    });

    await check("the base game stops at five", async () => {
      // The listing is the same code path the browser screen reads, and it is the
      // one place the cap is visible without filling a room.
      const base = (await listGames()).find((g) => g.slug === "kittens");
      assert(base.max === 5, `the base tile says up to ${base.max} players, want 5`);
    });

    // Feral Cats are a quarter of what the expansion adds, so with 48 cards
    // dealt somebody nearly always has one alongside a plain cat.
    const found = await findFeralAndCat(players);
    if (!found) skip("a Feral Cat stacks with a cat it does not match", "no Feral Cat dealt beside a cat");
    else await check("a Feral Cat stacks with a cat it does not match", async () => {
      const { player, feral, cat } = found;
      assert(await player.selectCard(cat.id), "could not select the plain cat");
      const hand = await player.hand();
      const wild = hand.find((c) => c.id === feral.id);
      assert(
        !wild.blocked,
        `a Feral Cat was blocked next to a ${cat.slug}, but it is a wildcard`,
      );
      assert(await player.selectCard(feral.id), "could not add the Feral Cat");

      // Both end up selected, which is the client rule under test. Deliberately
      // not asserting the Play button: that also needs it to be this player's
      // turn, and whoever happens to hold a Feral Cat usually is not on turn.
      const picked = (await player.hand()).filter((c) => c.selected).map((c) => c.slug);
      assert(picked.length === 2, `${picked.length} cards selected, want 2: ${picked}`);
      assert(picked.includes("feral-cat"), "the Feral Cat did not stay selected");

      for (const c of (await player.hand()).filter((x) => x.selected)) {
        await player.selectCard(c.id);
      }
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ────────────────────────────────────────────────────────── the Imploding Kitten

async function theKitten() {
  const players = [await seat(browser, "Alex"), await seat(browser, "Bea")];
  const [host] = players;

  try {
    await step("a two-player expansion game is dealt", async () => {
      const code = await host.create({ game: GAME });
      await players[1].join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
    });

    await check("the deck starts with the kitten unarmed", async () => {
      const armed = await host.page.$eval("#deck-risk", (el) => el.classList.contains("armed"));
      assert(!armed, "the Imploding Kitten is armed before anyone has drawn it");
    });

    let sawPrompt = false;
    await step("drawing turns it up, and it goes back face up", async () => {
      // Two players means exactly one kitten in the deck, and it is this one.
      for (let i = 0; i < 120; i++) {
        for (const p of players) {
          if (!(await visible(p.$("#modal")))) continue;
          const title = (await p.$("#modal-title").textContent()) || "";
          if (/imploding/i.test(title)) {
            sawPrompt = true;
            // No Defuse should have been spent putting it back.
            const before = (await p.hand()).filter((c) => c.slug === "defuse").length;
            await safeClick(p.$('[data-place="middle"]'), 800);
            await safeClick(p.$("#place-confirm"), 1500);
            await sleep(300);
            const after = (await p.hand()).filter((c) => c.slug === "defuse").length;
            assert(
              after === before,
              `putting the Imploding Kitten back cost a Defuse (${before} -> ${after})`,
            );
          }
        }
        if (sawPrompt) break;
        await clearNope(players);
        await clearModals(players);
        for (const p of players) {
          if (await p.isMyTurn()) {
            await safeClick(p.$("#deck"), 1500);
            break;
          }
        }
        await sleep(90);
      }
      assert(sawPrompt, "nobody ever drew the Imploding Kitten in 120 moves");
    });

    await check("the table is told the kitten is live", async () => {
      await waitFor(
        async () => host.page.$eval("#deck-risk", (el) => el.classList.contains("armed")),
        { what: "the armed indicator" },
      );
      // Both players, not just the one who buried it: everybody watched it go in.
      for (const p of players) {
        const armed = await p.page.$eval("#deck-risk", (el) => el.classList.contains("armed"));
        assert(armed, `${p.name} is not shown that the kitten is armed`);
        const title = await p.page.$eval("#deck-risk", (el) => el.title);
        assert(/no Defuse/i.test(title), `${p.name}'s readout does not warn about the Defuse`);
      }
      const lines = await host.logLines();
      assert(
        lines.some((l) => /imploding kitten/i.test(l)),
        `the log never mentions it: ${JSON.stringify(lines.slice(-4))}`,
      );
    });

    await check("buried, it is armed but not shown on the deck", async () => {
      // It went back in the middle, and the deck is far more than two cards
      // deep — so it provably is not the top card. Knowing it is armed is the
      // whole table's business; knowing where it sits is nobody's.
      const count = await deckCount(host);
      assert(count > 2, `the deck is only ${count} cards, so "middle" may be the top`);
      for (const p of players) {
        assert(
          !(await deckFace(p)),
          `${p.name} can see the buried kitten on top of the deck`,
        );
      }
    });

    // Filled in by the loop below: what each player's deck showed at the moment
    // the kitten surfaced. Collected there rather than checked there because the
    // window is one turn wide — by the time the loop ends the card is drawn.
    const surfaced = [];

    await check("it takes the next player to reach it, Defuse or not", async () => {
      for (let i = 0; i < 200; i++) {
        const lines = await host.logLines();
        if (lines.some((l) => /wins!/.test(l))) break;
        if (!surfaced.length) {
          for (const p of players) {
            const face = await deckFace(p);
            if (face) surfaced.push({ name: p.name, ...face });
          }
        }
        await clearNope(players);
        await clearModals(players);
        for (const p of players) {
          if (await p.isMyTurn()) {
            await safeClick(p.$("#deck"), 1500);
            break;
          }
        }
        await sleep(90);
      }

      const lines = await host.logLines();
      assert(lines.some((l) => /wins!/.test(l)), "the game never ended");
      // The armed kitten is the only kitten in a two-player expansion deck, so
      // whoever went out must have gone out to it.
      assert(
        lines.some((l) => /armed imploding kitten/i.test(l)),
        `nobody was taken by the Imploding Kitten: ${JSON.stringify(lines.slice(-6))}`,
      );
    });

    await check("once it surfaces, everybody sees the card itself", async () => {
      // It has to have been on top at some point — it was drawn, and a card is
      // drawn off the top. If nothing was captured the reveal is not happening.
      assert(surfaced.length > 0, "the kitten was drawn but never appeared on the deck");
      assert(
        surfaced.length === players.length,
        `only ${surfaced.map((s) => s.name).join(", ")} saw it — a face-up card is public`,
      );
      for (const s of surfaced) {
        assert(/\.gif$/.test(s.src), `${s.name} was shown ${s.src}, not the face-up animation`);
        assert(s.loaded, `${s.name}'s face-up picture never loaded (${s.src})`);
        assert(/imploding/i.test(s.alt), `${s.name}'s deck names the card as ${JSON.stringify(s.alt)}`);
      }
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ─────────────────────────────────────────────────────────────────── helpers

// The face-up card on the deck, or null when the deck is showing its back.
// Reports whether the picture actually arrived, because a wrong path leaves the
// element in place over the card back and looks like a styling choice.
function deckFace(p) {
  return p.page.$eval("#deck-face", (el) =>
    el.hidden || !el.getAttribute("src")
      ? null
      : { src: el.getAttribute("src"), alt: el.alt, loaded: el.naturalWidth > 0 },
  );
}

async function deckCount(p) {
  const text = (await p.$("#deck-count").textContent()) || "";
  return parseInt(text, 10);
}

async function demandable(slug) {
  const res = await fetch(`${BASE}/api/cards?game=${encodeURIComponent(slug)}`);
  assert(res.ok, `GET /api/cards?game=${slug} returned ${res.status}`);
  return ((await res.json()).demandable || []).map((k) => k.slug);
}

async function findFeralAndCat(players) {
  for (const player of players) {
    const hand = await player.hand();
    const feral = hand.find((c) => c.slug === "feral-cat");
    if (!feral) continue;
    // A plain cat, and deliberately not one it would match anyway.
    const cat = hand.find((c) => c.slug.startsWith("cat-"));
    if (cat) return { player, feral, cat };
  }
  return null;
}
