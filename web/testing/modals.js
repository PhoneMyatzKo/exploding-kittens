// Modals have to survive the biggest thing that can go in them. The Favor
// picker shows your whole hand, and a hand grows: Favors and steals push it well
// past the eight you were dealt. Before this was fixed the box simply ran off the
// bottom of the screen — nothing scrolled, and the cards down there could not be
// picked at all.
//
// The invariant, for any modal: the box fits on screen, and everything in it can
// be reached.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  safeClick, visible, clearNope, clearModals, requireServer,
} from "./lib.js";

await requireServer();
console.log("modals");

// Desktop is the worse case, not the easier one: the card width clamps at its
// maximum on a wide screen, so a hand stacks two-per-row into a very tall box.
// A short phone is included because a tall one passes on card count alone and
// would not notice the scrolling breaking.
const SCREENS = [
  { name: "laptop", size: { width: 1366, height: 768 } },
  { name: "small window", size: { width: 1000, height: 600 } },
  { name: "tall phone", size: { width: 390, height: 844 }, phone: true },
  { name: "short phone", size: { width: 360, height: 600 }, phone: true },
];

// Filled in the first time each prompt appears, and asserted on afterwards. Both
// are up for one moment in the middle of play — the ring between pressing Play
// and picking somebody, the Nope window until the table stands down — so they
// have to be measured where they happen rather than set up on demand.
//
// Declared here rather than beside the helpers that fill them: the loop below is
// top-level code, and a const declared under it is still in its temporal dead
// zone when it runs. See the note in README.md.
const ringSeen = [];
const nopeSeen = [];

const browser = await launch();

try {
  for (const screen of SCREENS) {
    console.log(`  — ${screen.name}`);
    await runFor(screen);
  }
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("modals");

async function runFor(phone) {
  // Per viewport: the whole point is that each window size gets its own look at
  // these, so a measurement from the laptop must not stand in for the phone.
  ringSeen.length = 0;
  nopeSeen.length = 0;

  const players = [];
  for (const name of ["Alex", "Bea", "Cy", "Dee", "Eve"]) {
    const p = await seat(browser, name, { phone: Boolean(phone.phone) });
    await p.page.setViewportSize(phone.size);
    players.push(p);
  }
  const [host] = players;

  try {
    await step(`${phone.name}: a table is dealt`, async () => {
      const code = await host.create();
      for (const p of players.slice(1)) await p.join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === players.length, {
          what: `${p.name} to see everyone`,
        });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
    });

    await check(`${phone.name}: a big hand never covers the table`, async () => {
      // The reported bug: with a hand this size the deck and the discard slid
      // under the hand bar and were cut off. Desktop is the case that shows it —
      // the card width clamps at its maximum there while the height does not.
      const t = await measureTable(host);
      assert(t.handCards >= 13, `only ${t.handCards} cards after stuffing the hand`);
      assert(!t.deckUnderHand, `the deck (${t.deck.h}px) runs under the hand`);
      assert(!t.discardUnderHand, "the discard runs under the hand");
      assert(!t.gaugeUnderHand, "the risk gauge runs under the hand");
      // Shrinking is the fix; disappearing would pass the checks above.
      assert(t.deck.h >= 90 && t.deck.w >= 60, `the deck collapsed to ${t.deck.w}×${t.deck.h}`);
      // 3:4, still — a squashed card reads as a broken image.
      const ratio = t.deck.w / t.deck.h;
      assert(Math.abs(ratio - 0.75) < 0.06, `the deck is ${ratio.toFixed(2)} wide-to-tall, want 0.75`);
      assert(t.handbarShare <= 62, `the hand takes ${t.handbarShare}% of a ${t.viewport}px window`);
    });

    const victim = await playAFavor(players);
    await step(`${phone.name}: somebody is asked to hand a card over`, async () => {
      assert(victim, "no Favor was played, so the picker never opened");
      const title = await victim.$("#modal-title").textContent();
      assert(/hand one over/i.test(title), `unexpected modal: ${JSON.stringify(title)}`);
    });

    await check(`${phone.name}: the Nope window is a centred prompt`, async () => {
      assert(nopeSeen.length, "no Nope window ever opened, so its layout is unchecked");
      for (const m of nopeSeen) {
        assert(m.withinViewport, `the prompt runs off a ${m.viewport}px window`);
        assert(m.centred, "the prompt is not centred — is it still pinned to an edge?");
        assert(m.clearOfEdges, "the prompt is flush against the top or bottom of the screen");
        assert(m.text.trim().length > 0, "the prompt does not say what was played");
        assert(m.timerTrack > 0, "the countdown has no track");
        assert(
          m.timerWidth > 0 && m.timerWidth <= m.timerTrack,
          `the countdown reads ${m.timerWidth} of ${m.timerTrack}px`,
        );
      }
      // Somebody has to have been offered the buttons, or only the bystander's
      // view of the window was ever looked at.
      const answerable = nopeSeen.filter((m) => m.hasButtons);
      assert(answerable.length, "nobody was offered NOPE! or Let it happen");
      for (const m of answerable) {
        assert(m.buttonsInside, "the buttons are outside the prompt box");
      }
    });

    await check(`${phone.name}: the target dial fits and spreads out`, async () => {
      const r = ringSeen[0];
      assert(r, "the target ring never opened, so picking a player was not exercised");
      assert(
        r.picks.length === r.seats - 1,
        `${r.picks.length} candidates for ${r.seats} seats — you cannot target yourself`,
      );
      assert(r.boxWithinViewport, `the dial runs off a ${r.viewport.h}px window`);
      for (const pick of r.picks) {
        assert(pick.inside, `${pick.name} sits outside the box at ${pick.cx},${pick.cy}`);
        assert(pick.hasAvatar, `${pick.name} has no face to tap`);
        assert(pick.w > 40 && pick.h > 40, `${pick.name} is only ${pick.w}×${pick.h}`);
      }
      // Distinct positions: the angles are computed, and a bad count would put
      // two candidates in the same place rather than fail outright.
      const spots = new Set(r.picks.map((k) => `${k.cx},${k.cy}`));
      assert(spots.size === r.picks.length, `two candidates share a position: ${[...spots]}`);
    });

    await check(`${phone.name}: the picker fits on screen`, async () => {
      const m = await measureModal(victim);
      assert(
        m.withinViewport,
        `the box is ${m.boxHeight}px in a ${m.viewport}px viewport, spanning ${m.top}…${m.bottom}`,
      );
    });

    await check(`${phone.name}: every card in the hand can be reached`, async () => {
      const m = await measureModal(victim);
      assert(m.cards > 0, "the picker is showing no cards at all");
      assert(
        m.unreachable === 0,
        `${m.unreachable} of ${m.cards} cards cannot be scrolled to`,
      );
    });

    await check(`${phone.name}: a hand twice the size still works`, async () => {
      // A deal cannot hand out twenty cards, so they are cloned in. The markup is
      // exactly what the client renders, which is what the layout is under test
      // for — only the count is contrived.
      await victim.page.evaluate(() => {
        const list = document.getElementById("modal-cards");
        const first = list.querySelector(".card");
        while (list.querySelectorAll(".card").length < 20) {
          list.append(first.cloneNode(true));
        }
      });
      // Nudge the window so the resize observer re-measures: the clones went in
      // after the modal opened, which a real hand never does.
      const size = victim.page.viewportSize();
      await victim.page.setViewportSize({ width: size.width, height: size.height - 1 });
      await sleep(250);

      const m = await measureModal(victim);
      assert(m.cards >= 20, `only ${m.cards} cards after stuffing the hand`);
      assert(m.withinViewport, `the box grew to ${m.boxHeight}px in a ${m.viewport}px viewport`);
      assert(m.listScrolls, "twenty cards did not make the list scrollable");
      assert(m.unreachable === 0, `${m.unreachable} of ${m.cards} cards cannot be scrolled to`);
      // Without a visible scrollbar, the faded edge is the only thing saying
      // there is more below. That absence is what the bug report described.
      assert(m.saysMore, "an overflowing list gave no sign it could be scrolled");
    });

    await check(`${phone.name}: picking a card actually gives it`, async () => {
      // The scroll box must not have broken the thing it contains.
      await victim.page.reload();
      await victim.waitForScreen("table");
      await waitFor(async () => visible(victim.$("#modal")), { what: "the picker to come back" });
      const before = (await victim.hand()).length;
      await safeClick(victim.$("#modal-cards .card").first(), 3000);
      await waitFor(async () => (await victim.hand()).length === before - 1, {
        what: "the card to leave the hand",
      });
      assert(!(await visible(victim.$("#modal"))), "the picker stayed open after picking");
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ─────────────────────────────────────────────────────────── helpers

// measureModal asks the browser the only questions that matter: does the box fit,
// and can you get to everything in it.
function measureModal(p) {
  return p.page.evaluate(() => {
    const box = document.querySelector(".modal-box");
    const list = document.getElementById("modal-cards");
    const vh = window.innerHeight;
    const r = box.getBoundingClientRect();
    const items = [...list.querySelectorAll(".card, .demand-pick")];
    // Read before the sweep below scrolls the list, which would clear the flag.
    const saysMore = list.classList.contains("has-more");
    const listScrolls = list.scrollHeight > list.clientHeight;

    // Reachable means what it means to a player: scroll to it, and it is fully on
    // screen. Measured by actually doing that rather than by arithmetic on
    // offsets — offsetTop is relative to the nearest positioned ancestor, not to
    // the scroll box, which makes the sums quietly meaningless.
    const unreachable = items.filter((el) => {
      el.scrollIntoView({ block: "nearest", inline: "nearest" });
      const b = el.getBoundingClientRect();
      return !(b.top >= -1 && b.bottom <= vh + 1);
    }).length;
    list.scrollTop = 0;

    return {
      viewport: vh,
      top: Math.round(r.top),
      bottom: Math.round(r.bottom),
      boxHeight: Math.round(r.height),
      withinViewport: r.top >= -1 && r.bottom <= vh + 1,
      cards: items.length,
      listScrolls,
      saysMore,
      unreachable,
    };
  });
}

// playAFavor drives the game until somebody plays Favor, and returns the player
// who has to hand a card over.
async function playAFavor(players) {
  for (let i = 0; i < 80; i++) {
    // Before standing down, not after: passing is what closes the window.
    if (!nopeSeen.length) {
      for (const p of players) {
        const m = await measureNope(p);
        if (m) nopeSeen.push(m);
      }
      if (nopeSeen.length && !nopeSeen.some((m) => m.hasButtons)) {
        // Everybody was a bystander this time; keep looking for a seat that can
        // actually answer, or the buttons go unchecked.
        nopeSeen.length = 0;
      }
    }
    await clearNope(players);

    // A modal on any screen blocks the table — but if it is the one we are
    // waiting for, stop and hand it back.
    for (const p of players) {
      if (!(await visible(p.$("#modal")))) continue;
      const title = (await p.$("#modal-title").textContent()) || "";
      if (/hand one over/i.test(title)) return p;
    }
    await clearModals(players);

    for (const p of players) {
      if (!(await p.isMyTurn())) continue;
      const favor = (await p.hand()).find((c) => c.slug === "favor");
      if (favor && (await p.selectCard(favor.id))) {
        await safeClick(p.$("#play-btn"), 1500);
        const targets = p.$("#target-buttons .target-pick");
        await waitFor(async () => (await targets.count()) > 0, {
          timeout: 2000, what: "a target list",
        }).catch(() => {});
        if (await targets.count()) {
          if (!ringSeen.length) ringSeen.push(await measureRing(p));
          await safeClick(targets.first());
        }
      } else {
        await safeClick(p.$("#deck"), 1500);
      }
      break;
    }
    await sleep(120);
  }
  return null;
}

// measureNope returns the Nope prompt as it stands on this player's screen, or
// null if it is not up. It used to be a bar fixed to the bottom of the viewport,
// which on a phone landed on top of the hand and the action row — so what is
// checked is that it is a centred prompt clear of both.
function measureNope(p) {
  return p.page.evaluate(() => {
    const modal = document.getElementById("modal");
    if (modal.hidden || modal.dataset.kind !== "nope") return null;
    const box = document.querySelector(".modal-box").getBoundingClientRect();
    const nope = document.getElementById("nope-btn");
    const pass = document.getElementById("pass-btn");
    const fill = document.getElementById("nope-fill");
    return {
      viewport: innerHeight,
      withinViewport: box.top >= -1 && box.bottom <= innerHeight + 1,
      // Centred, not parked at an edge: the two margins within a hair of equal.
      // Deliberately *not* "does it overlap the hand" — a prompt with a scrim
      // over it is meant to. What the old bar got wrong was sharing the bottom of
      // the screen with a live hand and a live action row.
      centred: Math.abs(box.top - (innerHeight - box.bottom)) < 24,
      clearOfEdges: box.top > 4 && box.bottom < innerHeight - 4,
      text: (document.getElementById("nope-text") || {}).textContent || "",
      hasButtons: Boolean((nope && !nope.hidden) || (pass && !pass.hidden)),
      // Both must be reachable when offered — the old bar could push them under
      // the safe-area inset on a phone.
      buttonsInside: [nope, pass].filter((b) => b && !b.hidden).every((b) => {
        const r = b.getBoundingClientRect();
        return r.top >= box.top - 1 && r.bottom <= box.bottom + 1 && r.height > 20;
      }),
      // Draining, so a stalled countdown shows up as a full or empty bar.
      timerWidth: fill ? Math.round(fill.getBoundingClientRect().width) : -1,
      timerTrack: fill ? Math.round(fill.parentElement.getBoundingClientRect().width) : -1,
    };
  });
}

// measureRing describes the target dial: where each candidate sits, and whether
// the whole thing is on screen. Positions are the interesting part — they are set
// from JS, so a bad angle calculation would stack two candidates on top of each
// other and look like a rendering glitch rather than a bug.
function measureRing(p) {
  return p.page.evaluate(() => {
    const box = document.querySelector(".modal-box").getBoundingClientRect();
    const picks = [...document.querySelectorAll("#target-buttons .target-pick")].map((el) => {
      const r = el.getBoundingClientRect();
      return {
        seat: el.dataset.seat,
        cx: Math.round(r.left + r.width / 2),
        cy: Math.round(r.top + r.height / 2),
        w: Math.round(r.width), h: Math.round(r.height),
        hasAvatar: Boolean(el.querySelector(".avatar-chip")),
        name: (el.querySelector(".target-name") || {}).textContent || "",
        inside: r.top >= box.top - 1 && r.bottom <= box.bottom + 1 &&
                r.left >= box.left - 1 && r.right <= box.right + 1,
      };
    });
    return {
      picks,
      seats: [...document.querySelectorAll(".seats .seat")].length,
      viewport: { w: innerWidth, h: innerHeight },
      boxWithinViewport: box.top >= -1 && box.bottom <= innerHeight + 1,
    };
  });
}

// measureTable checks the complaint directly: table furniture disappearing under
// the hand. The hand is stuffed first, because that is the state it happens in
// and a dealt hand of eight does not reach it on every window size.
function measureTable(p, cards = 13) {
  return p.page.evaluate((n) => {
    const hand = document.getElementById("hand");
    const first = hand.querySelector(".card");
    if (first) {
      while (hand.querySelectorAll(".card").length < n) hand.append(first.cloneNode(true));
    }
    const box = (el) => (el ? el.getBoundingClientRect() : null);
    const deck = box(document.getElementById("deck"));
    const discard = box(document.getElementById("discard"));
    const bar = box(document.querySelector(".handbar"));
    const gauge = box(document.querySelector(".risk-meter"));
    return {
      handCards: hand.querySelectorAll(".card").length,
      // A card that has slid under the hand, which is what was reported.
      deckUnderHand: deck.bottom > bar.top + 1,
      discardUnderHand: discard.bottom > bar.top + 1,
      gaugeUnderHand: gauge ? gauge.bottom > bar.top + 1 : false,
      // It must shrink rather than vanish: a 0-height deck would pass the test
      // above and be just as broken.
      deck: { w: Math.round(deck.width), h: Math.round(deck.height) },
      handbarShare: Math.round((bar.height / innerHeight) * 100),
      viewport: innerHeight,
    };
  }, cards);
}
