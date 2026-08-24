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

    const victim = await playAFavor(players);
    await step(`${phone.name}: somebody is asked to hand a card over`, async () => {
      assert(victim, "no Favor was played, so the picker never opened");
      const title = await victim.$("#modal-title").textContent();
      assert(/hand one over/i.test(title), `unexpected modal: ${JSON.stringify(title)}`);
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
        const targets = p.$("#target-buttons .btn");
        await waitFor(async () => (await targets.count()) > 0, {
          timeout: 2000, what: "a target list",
        }).catch(() => {});
        if (await targets.count()) await safeClick(targets.first());
      } else {
        await safeClick(p.$("#deck"), 1500);
      }
      break;
    }
    await sleep(120);
  }
  return null;
}
