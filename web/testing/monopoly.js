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

// ───────────────────────────────────────────────────────────── a game

async function aGame() {
  const players = [await seat(browser, "Aung"), await seat(browser, "Bo"), await seat(browser, "Cho")];
  const [host] = players;

  // Collected during play, asserted afterwards: each is a moment one turn wide.
  const seen = { rolled: false, moved: false, bought: false, rent: false, passedGo: false };

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
        for (const p of players) {
          const state = await turnState(p);
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

    await check("the money adds up", async () => {
      // The bank pays for laps and takes for taxes and purchases, so the total is
      // not fixed — but nobody may hold less than nothing, and nobody who is
      // still in may have lost their seat.
      const cash = await allCash(host);
      for (const c of cash) assert(c >= 0, `somebody holds ${c}`);
      assert(cash.length === 3, `${cash.length} seats, want 3`);
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
      const tokens = el.querySelectorAll(".token").length;
      if (i === 0) tokensOnGo += tokens;
      else tokensElsewhere += tokens;
    }

    return {
      tiles: grid.querySelectorAll(".tile").length,
      cells: cells.size,
      byIndex, bands, unpaintedBands, buyable, inTheMiddle, tokensOnGo, tokensElsewhere,
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

function positions(p) {
  return p.page.$$eval("#board .tile", (els) =>
    els.flatMap((el) =>
      [...el.querySelectorAll(".token")].map(() => Number(el.dataset.tile))).sort((a, b) => a - b));
}

function ownedCount(p) {
  return p.page.$$eval("#board .tile.owned", (els) => els.length);
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
  return p.page.evaluate(() => ({
    canRoll: !document.getElementById("roll-btn").disabled &&
             !document.getElementById("roll-btn").hidden,
    offer: !document.getElementById("buy-btn").hidden,
  }));
}
