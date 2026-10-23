// Monopoly Myanmar — the first slice, driven by real clicks.
//
// What this is for: the board is the biggest unknown in the whole game. Forty
// squares round an 11×11 grid, positioned by arithmetic rather than by hand, with
// tokens on them and names in two languages. A square in the wrong place, a token
// that never moves, or a name that falls back to blank all look like styling and
// none of them would fail a Go test.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  safeClick, visible, requireServer, BASE,
} from "./lib.js";

await requireServer();
console.log("monopoly");

const GAME = "monopoly";
// U+1000–U+109F is the Burmese range: the board carries every name in both
// languages, so switching must actually change the letters on the squares.
const BURMESE = /[က-႟]/;

const browser = await launch();

try {
  await theBoard();
  await theTokenWalks();
  await onAPhone();
  await aGame();
} catch {
  // step() has already reported it; fall through to the verdict rather than
  // dying with a stack trace.
} finally {
  await browser.close();
}

report("monopoly");

// ───────────────────────────────────────────────────────────── the board

async function theBoard() {
  const players = [await seat(browser, "Aung"), await seat(browser, "Bo")];
  const [host] = players;

  // A stated window rather than whatever the driver defaults to. The board is
  // square and so sized by the shorter axis, which makes every measurement below
  // a function of the viewport height — and an assertion about legibility is
  // meaningless without saying at what size.
  for (const p of players) await p.page.setViewportSize({ width: 1440, height: 900 });

  try {
    await step("a two-player table is dealt", async () => {
      const code = await host.create({ game: GAME });
      await players[1].join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
      await host.page.waitForSelector("#board .tile", { timeout: 5000 });
    });

    // Before anything on this page touches the language. These contexts are
    // brand new, so localStorage is empty and what is on screen is the *default*
    // — which for Monopoly Myanmar is Burmese, because every square is named in
    // Burmese first and opening in English makes the localisation something you
    // have to go and find.
    await check("a fresh table opens in Burmese without being asked", async () => {
      const names = await squareNames(host);
      assert(BURMESE.test(names.join("")), `the board opened in English: ${names.slice(0, 4)}`);
      const chosen = await host.page.evaluate(() => localStorage.getItem("ek:lang"));
      // And it opened that way without *recording* a choice. If mounting wrote
      // the fallback back, it would read as a decision — every other game would
      // then open in Burmese too, and this default could never be changed for
      // anybody who had once opened a table.
      assert(chosen === null, `mounting recorded a language choice of ${JSON.stringify(chosen)}`);
    });

    // The rest of this function reads the board in English, so pick it — which
    // also proves an explicit choice beats the default.
    for (const p of players) {
      await safeClick(p.$("#lang-en"), 1500);
    }
    await sleep(250);

    await check("forty squares, each in its own place", async () => {
      const m = await measureBoard(host);
      assert(m.tiles === 40, `the board has ${m.tiles} squares`);
      // Every square in a distinct grid cell. The positions are computed, so the
      // mistake to catch is two squares in one cell — which reads as a missing
      // square rather than as a bug.
      assert(
        m.cells === 40,
        `${m.tiles} squares occupy only ${m.cells} grid cells — two are stacked`,
      );
      // A ring, not a block: nothing may sit in the middle nine-by-nine.
      assert(m.inTheMiddle === 0, `${m.inTheMiddle} squares are inside the ring`);
    });

    await check("the corners are where a Monopoly board has them", async () => {
      const m = await measureBoard(host);
      // GO bottom-right, jail bottom-left, free parking top-left, go-to-jail
      // top-right. Checked by geometry rather than by index, because getting the
      // index right and the geometry wrong is exactly the failure mode.
      const at = (i) => m.byIndex[i];
      assert(at(0).row === 11 && at(0).col === 11, `GO is at ${at(0).row},${at(0).col}`);
      assert(at(10).row === 11 && at(10).col === 1, `jail is at ${at(10).row},${at(10).col}`);
      assert(at(20).row === 1 && at(20).col === 1, `parking is at ${at(20).row},${at(20).col}`);
      assert(at(30).row === 1 && at(30).col === 11, `go-to-jail is at ${at(30).row},${at(30).col}`);
    });

    await check("the board is square and fits on screen", async () => {
      const m = await measureBoard(host);
      const ratio = m.board.w / m.board.h;
      assert(Math.abs(ratio - 1) < 0.02, `the board is ${ratio.toFixed(2)} wide-to-tall`);
      assert(m.board.withinViewport, `the board runs off a ${m.viewport.h}px window`);
      assert(m.board.w > 260, `the board is only ${Math.round(m.board.w)}px across`);
    });

    await check("every square is named, and the colour sets are drawn", async () => {
      const m = await measureBoard(host);
      const blank = m.byIndex.filter((t) => !t.name.trim());
      assert(blank.length === 0, `${blank.length} squares have no name, e.g. index ${blank[0] && blank[0].i}`);
      // Every square that can be bought carries a band — the eight colour sets,
      // and the stations and utilities, which charge rent and so are not scenery.
      // Asserted against the count of buyable squares rather than a number,
      // which would need editing every time the board changed.
      assert(
        m.bands === m.buyable,
        `${m.bands} bands for ${m.buyable} buyable squares`,
      );
      assert(m.unpaintedBands === 0, `${m.unpaintedBands} bands are transparent — an unknown group?`);
    });

    await check("the squares are big enough to read", async () => {
      // This is what the redesign was for. The first board was eleven equal
      // cells and the report on it was that the text was too small — so the
      // numbers are pinned rather than left to drift back.
      const m = await measureBoard(host);
      assert(
        m.corner.w > m.edge.w * 1.3,
        `a corner is ${Math.round(m.corner.w)}px against an edge square's ${Math.round(m.edge.w)}px — the board is back to equal cells`,
      );
      assert(m.nameSize >= 11, `names render at ${m.nameSize}px in a 1440x900 window`);
      // Nothing clipped to nothing: a name that does not fit its square is worse
      // than a name that wraps.
      assert(m.unreadableNames === 0, `${m.unreadableNames} squares show a name with no height`);
      // And the side columns run their text along the long axis, which is the
      // only way a name fits a narrow square at this size.
      assert(m.sidewaysNames >= 18, `only ${m.sidewaysNames} side squares set their text sideways`);
    });

    await check("both players' tokens start on GO", async () => {
      const m = await measureBoard(host);
      assert(m.tokensOnGo === 2, `${m.tokensOnGo} tokens on GO, want 2`);
      assert(m.tokensElsewhere === 0, `${m.tokensElsewhere} tokens are already off GO`);
    });

    await check("tapping a square says what it costs", async () => {
      await safeClick(host.$('#board .tile[data-tile="1"]'), 1500);
      await host.page.waitForSelector("#modal", { state: "visible", timeout: 3000 });
      const card = await host.page.evaluate(() => ({
        kind: document.getElementById("modal").dataset.kind,
        title: document.getElementById("modal-title").textContent,
        rows: document.querySelector(".tile-detail")?.textContent || "",
      }));
      assert(card.kind === "tile", `the modal opened as ${card.kind}`);
      assert(/Myeik|မြိတ်/.test(card.title), `the card is titled ${JSON.stringify(card.title)}`);
      // Price and rent, both in kyat. The numbers arrive as numbers and are
      // formatted here, so a missing format shows up as a bare integer.
      assert(/K60,000/.test(card.rows), `no price on the card: ${JSON.stringify(card.rows)}`);
      assert(/K2,000/.test(card.rows), `no rent on the card: ${JSON.stringify(card.rows)}`);
      await safeClick(host.$("#modal-alt"), 1000);
    });

    // The rent ladder is the question building exists to answer — "what is this
    // worth if I finish the set" — so it is on the card whether or not anything
    // is built yet. Which is also why this check can run on a fresh board and
    // will fire on every run, unlike building itself.
    await check("a square's card shows what building it would earn", async () => {
      await safeClick(host.$('#board .tile[data-tile="1"]'), 1500);
      await host.page.waitForSelector("#modal", { state: "visible", timeout: 3000 });
      const card = await host.page.evaluate(() => ({
        rows: document.querySelector(".tile-detail")?.textContent || "",
        buttons: [...document.querySelectorAll(".build-row button")].map((b) => b.id),
      }));

      // Myeik's six printed figures, unimproved through to the hotel.
      for (const want of ["K2,000", "K10,000", "K30,000", "K90,000", "K160,000", "K250,000"]) {
        assert(card.rows.includes(want), `the ladder is missing ${want}: ${JSON.stringify(card.rows)}`);
      }
      // And what a house on it costs, or there is no way to weigh the ladder
      // against the price.
      assert(/K50,000/.test(card.rows), `no house price on the card: ${JSON.stringify(card.rows)}`);
      // Nobody owns anything yet, so there must be nothing to press. A build
      // button here would be one the server refuses.
      assert(card.buttons.length === 0, `an unowned square offered ${card.buttons}`);
      await safeClick(host.$("#modal-alt"), 1000);
    });

    await check("the board reads in Burmese", async () => {
      const before = await squareNames(host);
      assert(!BURMESE.test(before.join("")), "the board is already in Burmese in English mode");

      await safeClick(host.$("#lang-my"), 1500);
      await sleep(300);

      const after = await squareNames(host);
      assert(BURMESE.test(after.join("")), "switching to Burmese did not change the squares");
      assert(after.length === before.length, `${after.length} squares after the switch, was ${before.length}`);
      // Every square, not just the ones that happened to have a translation.
      const untranslated = after.filter((n) => n.trim() && !BURMESE.test(n));
      assert(
        untranslated.length === 0,
        `${untranslated.length} squares stayed in English: ${untranslated.slice(0, 3)}`,
      );

      // And the choice is the shell's, so it outlives a reload.
      await host.page.reload();
      await host.waitForScreen("table");
      await host.page.waitForSelector("#board .tile", { timeout: 5000 });
      assert(BURMESE.test((await squareNames(host)).join("")), "the language was forgotten on reload");

      await safeClick(host.$("#lang-en"), 1500);
      await sleep(250);
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ────────────────────────────────────────────────────────── it actually moves

// Every other script in the suite runs with reduced motion on, so a token
// arrives instantly and nothing here would notice if the walk were broken,
// removed, or never written. This is the one place motion is switched on, and
// it is the only check that can tell "it moves" from "it teleports".
async function theTokenWalks() {
  const players = [
    await seat(browser, "Aung", { motion: "no-preference" }),
    await seat(browser, "Bo", { motion: "no-preference" }),
  ];
  const [host] = players;

  try {
    await step("a table is dealt with motion allowed", async () => {
      const code = await host.create({ game: GAME });
      await players[1].join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
      await host.page.waitForSelector("#board .tile", { timeout: 5000 });
    });

    await check("the tokens live in one layer, not inside the squares", async () => {
      // The refactor this whole feature stands on. A token parented to a square
      // is a token that gets destroyed every time that square re-renders, which
      // is why nothing could be animated before.
      const m = await host.page.evaluate(() => ({
        inLayer: document.querySelectorAll("#token-layer .token").length,
        inTiles: document.querySelectorAll(".tile .token").length,
        layers: document.querySelectorAll("#token-layer").length,
      }));
      assert(m.layers === 1, `${m.layers} token layers on the board`);
      assert(m.inLayer === 2, `${m.inLayer} tokens in the layer, want 2`);
      assert(m.inTiles === 0, `${m.inTiles} tokens are still parented to a square`);
    });

    // The claim is not merely "it animates" — `.token` carries a CSS transition,
    // so the transform slides smoothly between any two points whether or not a
    // walk ever runs. The first version of this check asserted exactly that and
    // passed with the walk deleted. What is actually being claimed is the
    // *route*: the piece goes round the ring one square at a time, because the
    // straight line between two squares on different edges is a diagonal across
    // the middle of the board, and watching the corner go by is what makes
    // passing GO legible.
    await check("a token walks round the board rather than cutting across it", async () => {
      let animated = false, moves = 0;
      let cornersTested = 0, cornersWalked = 0;
      const trace = [];

      // Budgeted generously rather than tightly: a turn can be spent in jail,
      // reading a card or answering an offer, and the check needs a *corner* to
      // say anything at all. Stops as soon as it has two.
      for (let turn = 0; turn < 22 && cornersTested < 2; turn++) {
        const roller = await whoeverCanRoll(players);
        if (!roller) continue; // a prompt may clear on the next pass
        const from = await tokenSquare(roller);

        // The squares the piece is nearest to as it goes.
        //
        // The window has to span the whole turn, not just the walk: the dice
        // tumble for 850ms and are then held for 700ms so the throw can be read,
        // so the token does not set off for about a second and a half. Sampling
        // only the first second would watch a stationary piece and conclude it
        // had teleported. See the pacing block in games/monopoly/index.js — if
        // those numbers change, this has to follow.
        const visited = new Set();
        const shapes = new Set();
        await safeClick(roller.$("#roll-btn"), 2000);
        for (let i = 0; i < 110; i++) {
          const probe = await tokenProbe(roller);
          if (probe.transform) shapes.add(probe.transform);
          if (probe.near >= 0) visited.add(probe.near);
          await sleep(20);
        }
        await sleep(500); // let the walk settle before reading where it landed

        const to = await tokenSquare(roller);
        trace.push(`${roller.name}:${from}>${to}`);
        if (to === from) continue;
        moves++;
        if (shapes.size > 2) animated = true;

        // A corner between start and finish is what makes the two routes
        // different. Walking passes *over* the corner square; the straight line
        // cuts inside it and never goes near. Anything that does not round a
        // corner tells us nothing either way, so it is not counted.
        const corner = cornerBetween(from, to);
        if (corner < 0) continue;
        cornersTested++;
        if (visited.has(corner)) cornersWalked++;
      }

      assert(moves > 0, "no token moved at all");
      assert(animated, "the token jumped between squares instead of travelling");
      // Said out loud rather than passing on an empty set: without a corner the
      // ring and the straight line are the same route and nothing was proved.
      assert(cornersTested > 0,
        `no move in ${moves} rounded a corner, so the route was never under test ` +
        `(${trace.join(" ")})`);
      // One, not all of them. Sampling can genuinely miss a square — the token
      // is only over each one for about 80ms — and demanding a clean sweep would
      // make this fail on timing rather than on behaviour. Cutting the corner
      // scores zero, which is the thing being caught.
      assert(cornersWalked > 0,
        `the token rounded ${cornersTested} corner(s) and was never once seen on ` +
        "any of them — it is cutting across rather than walking the ring");

      const m = await measureBoard(host);
      assert(m.tokensOutside === 0, `${m.tokensOutside} tokens finished outside the board`);
    });

    await check("the dice are thrown before they settle", async () => {
      // Whoever's turn it actually is. The walk above consumed a turn, and the
      // first version of this clicked the host's Roll button regardless — which
      // by then was disabled, so nothing was thrown and the check read the
      // *previous* result sitting on screen. It failed honestly rather than
      // passing, but only by luck: one unchanging value is also what a broken
      // tumble looks like.
      const roller = await whoeverCanRoll(players);
      assert(roller, "nobody could roll, so the dice were never thrown");

      // Sampled from a spectator, not the roller: the tumble is driven by the
      // event every client receives, so this also proves the throw is shown to
      // the whole table rather than only to whoever pressed the button.
      const watcher = players.find((p) => p !== roller) || roller;
      const faces = new Set();
      await safeClick(roller.$("#roll-btn"), 2000);
      // Spanning the 850ms tumble with room to spare. The faces slow as they
      // settle, so the last few are each on screen for most of this interval.
      for (let i = 0; i < 30; i++) {
        faces.add(await diceFaces(watcher));
        await sleep(40);
      }
      faces.delete("");
      assert(faces.size > 1, `the dice showed one face the whole time: ${[...faces]}`);
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ───────────────────────────────────────────────────────────── on a phone

// The board is limited by *width* on a phone, so it stops well short of the
// bottom of the screen — which is the half of the layout a 1440×900 window never
// exercises. The complaint was a band of dead space under the play-by-play, and
// it is the same CSS as the desktop overflow: a log sized by a number instead of
// by what is left.
async function onAPhone() {
  const players = [await seat(browser, "Aung"), await seat(browser, "Bo")];
  const [host] = players;
  // The reporter's handset, in CSS pixels.
  for (const p of players) await p.page.setViewportSize({ width: 411, height: 891 });

  try {
    await step("a table is dealt on a phone", async () => {
      const code = await host.create({ game: GAME });
      await players[1].join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
      await host.page.waitForSelector("#board .tile", { timeout: 5000 });
    });

    await check("the play-by-play fills the screen under the board", async () => {
      const m = await panelFill(host);
      assert(m, "there is no side panel on the table");
      // A hard number rather than a proportion: what was reported is a visible
      // band of nothing, and 24px is smaller than one line of the log.
      assert(m.gapBelow <= 24,
        `${Math.round(m.gapBelow)}px of the ${m.viewH}px screen is empty under the log`);
      assert(m.gapBelow >= -1,
        `the log runs ${Math.round(-m.gapBelow)}px past the bottom of the screen`);
      // And it grew into that space rather than the seats stretching to fill it.
      assert(m.logHeight > 104,
        `the log is still its ${Math.round(m.logHeight)}px default height`);
    });

    await check("the page itself does not scroll on a phone", async () => {
      const m = await pageOverflow(host);
      assert(!m.pageScrolls, `the page is ${m.docHeight}px tall in a ${m.viewH}px window`);
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ───────────────────────────────────────────────────────────── a game

async function aGame() {
  const players = [await seat(browser, "Aung"), await seat(browser, "Bo"), await seat(browser, "Cho")];
  const [host] = players;

  // Collected during play, asserted afterwards: each is a moment one turn wide.
  const seen = {
    rolled: false, moved: false, bought: false, rent: false, passedGo: false,
    // The card that was on screen the first time one was drawn, and whether the
    // two ways out of jail were exercised. Collected here because each is up for
    // one turn only.
    card: null, jailed: false, paidFine: false,
  };

  try {
    await step("a three-player table is dealt", async () => {
      const code = await host.create({ game: GAME });
      for (const p of players.slice(1)) await p.join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 3, { what: "three seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
      await host.page.waitForSelector("#board .tile", { timeout: 5000 });
      // These are fresh contexts, so the table opens in Burmese — its default,
      // asserted in theBoard() above. The play-by-play is read here with English
      // patterns, so pick English rather than making every one of them bilingual
      // and stop the coverage flags being a test of the translation.
      for (const p of players) await safeClick(p.$("#lang-en"), 1500);
      await sleep(250);
    });

    await check("everybody starts with the same money", async () => {
      for (const p of players) {
        const cash = await ownCash(p);
        assert(cash === 1_500_000, `${p.name} starts with ${cash}`);
      }
    });

    await step("the game plays through eighty turns", async () => {
      for (let i = 0; i < 240; i++) {
        const before = await positions(host);

        let acted = false;

        // A drawn card first, and across every player rather than the first one
        // with a modal open: the card is shown to the whole table, but only
        // whoever drew it gets the button. Pressing the first screen that has it
        // up presses a hidden button on a spectator's, and the table never moves.
        for (const p of players) {
          const st = await turnState(p);
          if (!st.card) continue;
          const card = await readCard(p);
          if (card && card.hasButton) {
            if (!seen.card) seen.card = card;
            await safeClick(p.$("#modal-ok"), 1200);
            acted = true;
            break;
          }
        }
        if (acted) {
          await sleep(70);
          continue;
        }

        // Building, the way a player does it: tap one of your own towns and
        // press what is on its card. Tried every tenth iteration rather than
        // every one, because it costs a modal open per square held and the
        // opportunity does not come round quickly.
        //
        // Reported rather than required. Completing a colour set needs the dice
        // to land the same player on both browns, and in a fixed number of turns
        // that is likely but not certain — so this drives it whenever it is
        // possible and says whether it happened, the way play.js reports its
        // coverage. What is *required* is the two checks below that fire every
        // run: the card offers no build button on a square you do not own, and
        // the ladder is on it either way.
        if (i > 20 && i % 10 === 0) {
          for (const p of players) {
            const built = await tryToBuild(p);
            if (!built) continue;
            seen.built = true;
            if (built.hotel) seen.hotel = true;
            break;
          }
        }

        for (const p of players) {
          const state = await turnState(p);
          // Being held is its own kind of turn: a double, the fine, or a card.
          if (state.jailed) {
            seen.jailed = true;
            if (state.canPayFine && i % 3 === 0) {
              seen.paidFine = await safeClick(p.$("#fine-btn"), 1200);
            } else {
              await safeClick(p.$("#roll-btn"), 1200);
            }
            acted = true;
            break;
          }
          if (state.offer) {
            // Answer it every time. Writing this as `seen.bought = seen.bought ||
            // click(...)` short-circuits once one square has been bought, so the
            // offer is never answered again and the table sits on it for the rest
            // of the loop — which looked like a passing test with thin coverage.
            if (!(await safeClick(p.$("#buy-btn"), 1200))) {
              await safeClick(p.$("#pass-btn"), 1200);
            }
            acted = true;
            break;
          }
          if (state.canRoll) {
            await safeClick(p.$("#roll-btn"), 1200);
            acted = true;
            break;
          }
        }
        if (!acted) break; // game over, or nobody can move

        await sleep(70);
        const lines = await host.logLines();
        const all = lines.join("\n");
        if (/rolled \d/.test(all)) seen.rolled = true;
        if (/↳ to /.test(all)) seen.moved = true;
        if (/bought /.test(all)) seen.bought = true;
        if (/paid .* for /.test(all)) seen.rent = true;
        if (/passed GO/.test(all)) seen.passedGo = true;
        if (/built a (house|hotel)/.test(all)) seen.built = true;

        // Somebody's token has to be moving, or the loop is spinning on a wedged
        // table and every check below would still pass.
        const after = await positions(host);
        if (i > 4 && JSON.stringify(before) === JSON.stringify(after) && !acted) break;
      }
    });

    await check("the dice were thrown and tokens moved", async () => {
      assert(seen.rolled, "nobody ever rolled");
      assert(seen.moved, "no token ever moved");
      const pos = await positions(host);
      assert(pos.some((p) => p !== 0), `everybody is still on GO: ${pos}`);
      // Positions stay on the board — an off-by-one in the modulus would show up
      // as a token vanishing rather than as an error.
      for (const p of pos) assert(p >= 0 && p < 40, `a token is on square ${p}`);
    });

    await check("squares were bought and rent was paid", async () => {
      assert(seen.bought, "nobody bought a square in eighty turns");
      const owned = await ownedCount(host);
      assert(owned > 0, "no square on the board has an owner");
      // Rent needs somebody to land on somebody else's square, which is likely
      // but not certain in a fixed number of turns — so it is reported rather
      // than required, the way play.js reports its coverage.
      console.log(`  covered: ${Object.entries(seen).filter(([, v]) => v).map(([k]) => k).join(", ")}`);
    });

    await check("a card was drawn, read, and did something", async () => {
      assert(seen.card, "nobody landed on Chance or Community Chest in eighty turns");
      const c = seen.card;
      assert(c.deck === "chance" || c.deck === "chest", `the card came from deck ${JSON.stringify(c.deck)}`);
      assert(c.emoji.trim(), "the card has no emoji");
      assert(c.title.trim(), "the card has no title");
      // The flavour line is the whole reason the pack was written by hand — a
      // card that shows only its rule has lost the joke.
      assert(c.flavour.trim(), `${c.title} has no flavour line`);
      // And it says what it does, generated from the effects rather than typed
      // into the pack, so a card cannot say one thing and do another.
      assert(c.effect.trim(), `${c.title} does not say what it does`);
      assert(c.hasButton, "the player who drew the card had no way to acknowledge it");
      console.log(`  card seen: ${c.emoji} ${c.title.trim()} — ${c.effect.trim()}`);
    });

    await check("the money adds up", async () => {
      // The bank pays for laps and takes for taxes and purchases, so the total is
      // not fixed — but nobody may hold less than nothing, and nobody who is
      // still in may have lost their seat.
      const cash = await allCash(host);
      for (const c of cash) assert(c >= 0, `somebody holds ${c}`);
      assert(cash.length === 3, `${cash.length} seats, want 3`);
    });

    await check("building is offered on a set you hold and refused on one you don't", async () => {
      // Every run: a town somebody else owns must carry no build button on its
      // card, whoever is looking. The list of buildable squares comes from the
      // server, so a button here would mean the client had decided for itself.
      const offered = await buildOfferedOnSomebodyElses(host);
      assert(offered.checked > 0, "nobody owns a town, so this checked nothing");
      assert(offered.withButton === 0,
        `${offered.withButton} of ${offered.checked} of other people's towns offered a build button`);

      // And when it did happen, it has to have landed on the board rather than
      // only in the log.
      if (seen.built) {
        const m = await measureBoard(host);
        assert(m.buildings > 0, "the log says a house went up and the board shows none");
        assert(m.buildingsOutside === 0,
          `${m.buildingsOutside} buildings are drawn outside their own square`);
      }
      console.log(`  building: ${seen.built ? "yes" : "no set was completed in eighty turns"}` +
        `${seen.hotel ? ", including a hotel" : ""}`);
    });

    await check("an owned square still shows its price", async () => {
      // Ownership is drawn over the square — it used to be a bar that sat on top
      // of the price. Checked on squares somebody actually owns, because none of
      // it is painted until then.
      const m = await pricesOnOwnedSquares(host);
      assert(m.owned > 0, "no square is owned, so this checked nothing");
      assert(m.readable === m.owned, `${m.owned - m.readable} owned squares show no price`);
      assert(m.clipped === 0, `${m.clipped} of ${m.owned} owned squares push the price outside the square`);
    });

    // The reported bug: a long game made the *page* grow instead of the log
    // scrolling, so the board crept up off the top of the window and the whole
    // thing became one long scroll. Checked after a played-out game because it
    // only appears once the log is taller than the column it sits in.
    await check("a long log scrolls itself, not the page", async () => {
      const m = await pageOverflow(host);
      // Without this the assertion below is vacuous: a log short enough to fit
      // could never have pushed the page taller whatever the CSS said.
      assert(m.logOverflows,
        `the log is ${m.logContent}px in a ${m.logBox}px panel after ${m.lines} lines, ` +
        "so nothing here was under pressure");
      assert(!m.pageScrolls,
        `the page is ${m.docHeight}px tall in a ${m.viewH}px window`);
    });

    await check("the board never overlapped the log or ran off screen", async () => {
      const m = await measureBoard(host);
      assert(m.board.withinViewport, `the board runs off a ${m.viewport.h}px window`);
      assert(!m.board.overLog, "the board is sitting on top of the play-by-play");
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ─────────────────────────────────────────────────────────────── helpers

function measureBoard(p) {
  return p.page.evaluate(() => {
    const grid = document.getElementById("board");
    const box = grid.getBoundingClientRect();
    // Plain numbers, not the DOMRect: a DOMRect does not survive being handed
    // back out of evaluate() and arrives as an empty object, which reads as NaN.
    const rectOf = (sel) => {
      const r = document.querySelector(sel).getBoundingClientRect();
      return { w: r.width, h: r.height };
    };
    const nameSizes = [];
    let unreadableNames = 0, sidewaysNames = 0;
    const log = document.getElementById("log-panel").getBoundingClientRect();
    const cells = new Set();
    const byIndex = [];
    let bands = 0, unpaintedBands = 0, inTheMiddle = 0, buyable = 0;
    let tokensOnGo = 0, tokensElsewhere = 0;
    let buildings = 0, buildingsOutside = 0;

    for (const el of grid.querySelectorAll(".tile")) {
      const cs = getComputedStyle(el);
      const row = Number(cs.gridRowStart), col = Number(cs.gridColumnStart);
      cells.add(`${row},${col}`);
      const i = Number(el.dataset.tile);
      byIndex[i] = { i, row, col, name: el.querySelector(".tile-name").textContent };
      if (row > 1 && row < 11 && col > 1 && col < 11) inTheMiddle++;

      if (["property", "station", "utility"].includes(el.dataset.kind)) buyable++;
      const band = el.querySelector(".tile-band");
      if (band) {
        bands++;
        const bg = getComputedStyle(band).backgroundColor;
        if (!bg || bg === "rgba(0, 0, 0, 0)" || bg === "transparent") unpaintedBands++;
      }
      const nm = el.querySelector(".tile-name");
      const nr = nm.getBoundingClientRect();
      if (nr.width < 1 || nr.height < 1) unreadableNames++;
      nameSizes.push(parseFloat(getComputedStyle(nm).fontSize));
      const side = el.dataset.side;
      if (side === "left" || side === "right") {
        const wm = getComputedStyle(el.querySelector(".tile-body")).writingMode;
        if (/vertical/.test(wm)) sidewaysNames++;
      }
      // Tokens are no longer children of a square — they live in one layer and
      // are positioned by transform — so they are counted after this loop, off
      // the square each one says it is drawn on.

      // Houses sit on the colour band, which is a narrow strip — the mistake to
      // catch is one drawn outside the square it belongs to.
      const tr2 = el.getBoundingClientRect();
      for (const b of el.querySelectorAll(".tile-houses i")) {
        buildings++;
        const br = b.getBoundingClientRect();
        if (br.width < 1 || br.height < 1 ||
            br.left < tr2.left - 0.5 || br.right > tr2.right + 0.5 ||
            br.top < tr2.top - 0.5 || br.bottom > tr2.bottom + 0.5) buildingsOutside++;
      }
    }

    // Where the pieces are, and whether any of them has been placed outside the
    // board it is supposed to be standing on — the failure a transform can have
    // that a child element never could.
    const boardBox = grid.getBoundingClientRect();
    let tokensOutside = 0;
    for (const t of grid.querySelectorAll("#token-layer .token")) {
      const sq = Number(t.dataset.square);
      if (sq === 0) tokensOnGo++;
      else tokensElsewhere++;
      const r = t.getBoundingClientRect();
      if (!r.width || !r.height ||
          r.left < boardBox.left - 2 || r.right > boardBox.right + 2 ||
          r.top < boardBox.top - 2 || r.bottom > boardBox.bottom + 2) tokensOutside++;
    }

    return {
      tiles: grid.querySelectorAll(".tile").length,
      tokensOutside,
      cells: cells.size,
      byIndex, bands, unpaintedBands, buyable, inTheMiddle, tokensOnGo, tokensElsewhere,
      buildings, buildingsOutside,
      unreadableNames, sidewaysNames,
      nameSize: Math.min(...nameSizes),
      corner: rectOf('.tile[data-tile="0"]'),
      edge: rectOf('.tile[data-tile="1"]'),
      board: {
        w: box.width, h: box.height,
        withinViewport: box.top >= -1 && box.bottom <= innerHeight + 1,
        overLog: box.left < log.right && box.right > log.left &&
                 box.top < log.bottom && box.bottom > log.top,
      },
      viewport: { w: innerWidth, h: innerHeight },
    };
  });
}

// Declarations rather than `const` arrows, and that is not style: these are
// called from the top-level flow above, and a const declared below it is still in
// its temporal dead zone when that runs. Function declarations hoist. See the
// note in README.md — this has caught three scripts now.
function squareNames(p) {
  return p.page.$$eval("#board .tile .tile-name", (els) => els.map((e) => e.textContent));
}

// Which squares the pieces are standing on, read off the tokens themselves now
// that they live in a layer rather than inside a square. Every script in the
// suite but theTokenWalks() runs with reduced motion, so this is the settled
// position rather than a frame of a walk.
function positions(p) {
  return p.page.$$eval("#token-layer .token", (els) =>
    els.map((e) => Number(e.dataset.square)).sort((a, b) => a - b));
}

function ownedCount(p) {
  return p.page.$$eval("#board .tile.owned", (els) => els.length);
}

// whoeverCanRoll answers whatever prompt is in the way — a purchase, a drawn
// card — until somebody is able to throw, and returns them. A turn does not end
// where you expect it to: landing on an unowned square opens an offer, and a
// Chance square puts a card up that has to be read before anybody rolls again.
async function whoeverCanRoll(players, tries = 12) {
  for (let i = 0; i < tries; i++) {
    for (const p of players) {
      const st = await turnState(p);
      if (st.canRoll) return p;
      // A drawn card is face up on *everybody's* screen, but only the player who
      // drew it gets the button. So a failed click here means "wrong screen",
      // not "nothing to do" — carry on to the next player rather than breaking.
      //
      // Breaking regardless is what made this helper hang: with a spectator
      // first in the list it pressed their hidden button, gave up, slept, and
      // tried the same screen again every pass, so the table sat on an unread
      // card until the whole budget was spent. It is the same trap the play loop
      // below has a paragraph about, and I walked straight into it.
      if (st.card) {
        if (await safeClick(p.$("#modal-ok"), 600)) break;
        continue;
      }
      if (st.offer) {
        if (await safeClick(p.$("#buy-btn"), 600)) break;
        if (await safeClick(p.$("#pass-btn"), 600)) break;
        continue;
      }
    }
    await sleep(150);
  }
  return null;
}

function diceFaces(p) {
  return p.page.evaluate(() =>
    [...document.querySelectorAll("#dice .die")].map((d) => d.dataset.face).join("-"));
}

// tokenProbe is one sample of the viewer's own piece mid-move: where it is, and
// whether it is somewhere it has no business being.
//
// The transform is read from the *computed* style, not the inline one: during a
// walk the Web Animations API owns the property and the inline style still holds
// where the token was before it set off.
//
// `near` is the discriminating half: which square the piece is closest to right
// now. Walking the ring puts it over every square on the way, the corner
// included; a straight line between two edges cuts the corner off and never goes
// near it. Nearest-centre rather than hit-testing, because a token mid-stride
// sits between two squares and should count as the one it is closest to.
function tokenProbe(p) {
  return p.page.evaluate(() => {
    const el = document.querySelector("#token-layer .token.me")
      || document.querySelector("#token-layer .token");
    if (!el) return { transform: "", near: -1 };
    const t = el.getBoundingClientRect();
    const tx = t.left + t.width / 2, ty = t.top + t.height / 2;

    let near = -1, best = Infinity;
    for (const tile of document.querySelectorAll("#board .tile")) {
      const r = tile.getBoundingClientRect();
      const dx = r.left + r.width / 2 - tx, dy = r.top + r.height / 2 - ty;
      const d = dx * dx + dy * dy;
      if (d < best) { best = d; near = Number(tile.dataset.tile); }
    }
    return { transform: getComputedStyle(el).transform, near };
  });
}

// cornerBetween is the corner square passed on the way from `from` to `to`
// going forwards, or -1 if the move stays on one side of the board. Corners are
// GO, jail, free parking and go-to-jail — every tenth square.
function cornerBetween(from, to) {
  const steps = ((to - from) % 40 + 40) % 40;
  for (let n = 1; n < steps; n++) {
    const sq = (from + n) % 40;
    if (sq % 10 === 0) return sq;
  }
  return -1;
}

// Which square the viewer's own piece is drawn on, once it has settled.
function tokenSquare(p) {
  return p.page.evaluate(() => {
    const el = document.querySelector("#token-layer .token.me")
      || document.querySelector("#token-layer .token");
    return el ? Number(el.dataset.square) : -1;
  });
}

// Does the log scroll, or does the page? Only one of them should, and which one
// is the difference between a table you can read and a very long web page.
function pageOverflow(p) {
  return p.page.evaluate(() => {
    const log = document.getElementById("log");
    const doc = document.documentElement;
    return {
      lines: log.querySelectorAll("li").length,
      logContent: log.scrollHeight,
      logBox: log.clientHeight,
      logOverflows: log.scrollHeight > log.clientHeight + 1,
      docHeight: doc.scrollHeight,
      viewH: window.innerHeight,
      // +1 for the fractional rounding a dvh-sized element can land on.
      pageScrolls: doc.scrollHeight > window.innerHeight + 1,
    };
  });
}

// How much of the window the column beside the board actually uses. The phone
// complaint was the other half of the same CSS: the board is limited by width
// there and stops short of the bottom, and a fixed-height log left the rest of
// the screen empty.
function panelFill(p) {
  return p.page.evaluate(() => {
    const panel = document.querySelector(".side-panel");
    const log = document.getElementById("log-panel");
    if (!panel || !log) return null;
    return {
      gapBelow: window.innerHeight - log.getBoundingClientRect().bottom,
      logHeight: log.getBoundingClientRect().height,
      viewH: window.innerHeight,
      panelHeight: panel.getBoundingClientRect().height,
    };
  });
}

// tryToBuild plays the building move the way a player does: tap one of your own
// towns, and press whatever its card offers. Returns what it built, or null.
//
// Deliberately DOM-driven and deliberately blind — it does not know which
// squares the server will accept, it opens the ones this player owns and looks.
// That is the whole point: a build button the server would refuse, or a missing
// one on a square it would accept, is exactly the disagreement this catches.
async function tryToBuild(p) {
  const mine = await p.page.$$eval("#board .tile.mine[data-group]",
    (els) => els.map((e) => Number(e.dataset.tile)));
  for (const i of mine) {
    if (!(await safeClick(p.$(`#board .tile[data-tile="${i}"]`), 800))) continue;
    const ready = await p.page
      .waitForSelector("#build-btn", { state: "visible", timeout: 600 })
      .catch(() => null);
    if (!ready) {
      await safeClick(p.$("#modal-alt"), 600);
      continue;
    }
    const hotel = /hotel|ဟိုတယ်/.test(await ready.textContent());
    await safeClick(p.$("#build-btn"), 900);
    // The card stays open and re-renders, so close it rather than leaving a
    // modal over the board for the rest of the loop.
    await safeClick(p.$("#modal-alt"), 800);
    return { tile: i, hotel };
  }
  return null;
}

// Somebody else's town must never offer a build button. Checked from the host's
// screen over every owned town that is not theirs — which is a real assertion on
// a played-out board and reports how many squares it looked at, so it cannot
// pass by having found none.
async function buildOfferedOnSomebodyElses(p) {
  const theirs = await p.page.$$eval("#board .tile.owned[data-group]:not(.mine)",
    (els) => els.map((e) => Number(e.dataset.tile)));
  let checked = 0, withButton = 0;
  for (const i of theirs.slice(0, 6)) {
    if (!(await safeClick(p.$(`#board .tile[data-tile="${i}"]`), 800))) continue;
    checked++;
    if (await p.page.$("#build-btn")) withButton++;
    await safeClick(p.$("#modal-alt"), 600);
  }
  return { checked, withButton };
}

// Ownership used to be a bar along one edge, and the bar sat on top of the price.
// It is a tint plus an edge on the *inner* side now, so the question is the same
// one: can you still read what a square costs once somebody owns it?
function pricesOnOwnedSquares(p) {
  return p.page.evaluate(() => {
    let owned = 0, readable = 0, clipped = 0;
    for (const el of document.querySelectorAll("#board .tile.owned")) {
      owned++;
      const price = el.querySelector(".tile-price");
      if (!price) continue;
      const pr = price.getBoundingClientRect();
      const tr = el.getBoundingClientRect();
      if (pr.width > 0 && pr.height > 0) readable++;
      // Inside its own square: a price pushed past the edge is as good as gone.
      if (pr.left < tr.left - 0.5 || pr.right > tr.right + 0.5 ||
          pr.top < tr.top - 0.5 || pr.bottom > tr.bottom + 0.5) clipped++;
    }
    return { owned, readable, clipped };
  });
}

// Read off the seat strip rather than from the server, so what is checked is what
// a player can actually see.
function allCash(p) {
  return p.page.$$eval(".seats .seat .seat-meta", (els) =>
    els.map((e) => Number((e.textContent.match(/K([\d,]+)/) || [0, "0"])[1].replace(/,/g, ""))));
}

async function ownCash(p) {
  const seats = await p.page.$$eval(".seats .seat", (els) =>
    els.map((el) => ({
      you: /\(you\)/.test(el.querySelector(".seat-name").textContent),
      cash: Number((el.querySelector(".seat-meta").textContent.match(/K([\d,]+)/) || [0, "0"])[1].replace(/,/g, "")),
    })));
  const mine = seats.find((s) => s.you);
  return mine ? mine.cash : -1;
}

function turnState(p) {
  return p.page.evaluate(() => {
    const modal = document.getElementById("modal");
    return {
      canRoll: !document.getElementById("roll-btn").disabled &&
               !document.getElementById("roll-btn").hidden,
      offer: !document.getElementById("buy-btn").hidden,
      // A drawn card is a prompt over the board, so nothing else on this
      // player's screen can be clicked until it is read.
      card: !modal.hidden && modal.dataset.kind === "monopoly-card",
      jailed: !document.getElementById("fine-btn").hidden ||
              /jail/i.test(document.getElementById("board-status").textContent || ""),
      canPayFine: !document.getElementById("fine-btn").hidden,
    };
  });
}

// readCard captures the card on screen. Collected during play rather than
// asserted here, because a card is up for one turn and the checks want all of it.
function readCard(p) {
  return p.page.evaluate(() => {
    const face = document.querySelector(".draw-card");
    if (!face) return null;
    const ok = document.getElementById("modal-ok");
    return {
      deck: face.dataset.deck || "",
      emoji: (face.querySelector(".draw-emoji") || {}).textContent || "",
      title: (face.querySelector(".draw-title") || {}).textContent || "",
      flavour: (face.querySelector(".draw-flavour") || {}).textContent || "",
      effect: (face.querySelector(".draw-effect") || {}).textContent || "",
      hasButton: Boolean(ok) && !ok.hidden,
    };
  });
}
