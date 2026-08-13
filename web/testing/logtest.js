// The play-by-play is meant to behave like a chat box, not like a dialog you
// open to check on things. That means: always on screen, appends without
// yanking somebody who has scrolled back, follows the newest line when they are
// pinned to the bottom, and collapses out of the way if they want the room.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  safeClick, clearNope, clearModals, requireServer,
} from "./lib.js";

await requireServer();
console.log("play-by-play");

// The client treats a reader within 48px of the bottom as pinned, so the scroll
// assertions need a log that overflows by comfortably more than that — otherwise
// scrolling to the very top would still count as pinned and prove nothing.
const OVERFLOW = 200;

const browser = await launch();
const players = [
  await seat(browser, "Alex"),
  await seat(browser, "Bea"),
  await seat(browser, "Cy"),
];
const [host, reader] = players;

try {
  await step("a game is dealt", async () => {
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

  await check("the log is on screen without opening anything", async () => {
    assert(await reader.$("#log").isVisible(), "the log is not visible on a fresh table");
    const box = await reader.$("#log").boundingBox();
    assert(box && box.height > 60, `the log strip is only ${box ? box.height : 0}px tall`);
  });

  await check("the log does not cover the hand", async () => {
    // This is the regression that only a screenshot ever caught: two elements
    // both visible and overlapping is still broken.
    const log = await reader.$("#log-panel").boundingBox();
    const hand = await reader.$("#hand").boundingBox();
    assert(log && hand, "could not measure the log and the hand");
    const overlaps =
      log.x < hand.x + hand.width && log.x + log.width > hand.x &&
      log.y < hand.y + hand.height && log.y + log.height > hand.y;
    assert(!overlaps, "the log panel overlaps the hand");
  });

  await check("a scrolled-back reader is not yanked to the bottom", async () => {
    await fillLog(reader);
    await reader.page.$eval("#log", (el) => { el.scrollTop = 0; });
    const before = await reader.page.$eval("#log", (el) => el.scrollTop);
    await newLine(reader);
    await sleep(300);
    const after = await reader.page.$eval("#log", (el) => el.scrollTop);
    assert(after === before, `the log jumped from ${before} to ${after} under a reader`);
  });

  await check("a reader pinned to the bottom follows the newest line", async () => {
    await reader.page.$eval("#log", (el) => { el.scrollTop = el.scrollHeight; });
    await newLine(reader);
    await sleep(300);
    const pinned = await reader.page.$eval(
      "#log",
      (el) => el.scrollHeight - el.scrollTop - el.clientHeight < 48,
    );
    assert(pinned, "the log did not follow the newest line for a pinned reader");
  });

  await check("the header collapses it and gets the space back", async () => {
    // Nothing below this line is about the game any more, so get the table out
    // from under whatever prompt happens to be open.
    await clearModals(players);
    const open = await reader.$("#log").boundingBox();
    await reader.$("#log-collapse").click();
    await sleep(250);
    assert(!(await reader.$("#log").isVisible()), "the log is still visible after collapsing");
    const panel = await reader.$("#log-panel").boundingBox();
    // Collapsed must actually shrink the panel, not just blank its contents —
    // a 300px empty bar is the bug this replaced.
    assert(
      panel.height < open.height || panel.width < 120,
      `collapsed panel is still ${Math.round(panel.width)}×${Math.round(panel.height)}`,
    );
  });

  await check("the collapsed state survives a reload", async () => {
    await reader.page.reload();
    await reader.waitForScreen("table");
    await sleep(400);
    await clearModals([reader]); // a reload replays whatever prompt was pending
    assert(
      !(await reader.$("#log").isVisible()),
      "the log came back open after a reload",
    );
    await reader.$("#log-collapse").click();
    await sleep(250);
    assert(await reader.$("#log").isVisible(), "the log did not reopen");
  });

  await check("your own actions read as yours", async () => {
    const lines = await host.logLines();
    assert(lines.length > 0, "the log is empty");
    assert(
      lines.some((l) => /^The cards are dealt/.test(l)),
      `no deal line: ${JSON.stringify(lines.slice(0, 3))}`,
    );
  });
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("play-by-play");

// ─────────────────────────────────────────────────────────── helpers

// makeNoise nudges the game along by one move. Drawing is the move always
// available to whoever has the turn, so it is the reliable way to get another
// line into the shared log.
async function makeNoise() {
  await clearNope(players);
  for (const p of players) {
    // A modal on any screen blocks the table; dismiss whatever is there.
    await safeClick(p.$("#place-confirm"), 300);
    await safeClick(p.$("#modal-cards .card").first(), 300);
    await safeClick(p.$("#modal-ok"), 300);
  }
  for (const p of players) {
    if (await p.isMyTurn()) {
      await safeClick(p.$("#deck"));
      return;
    }
  }
}

// A finished game stops producing log lines. Three players drawing at random get
// through a deck quickly, so a round can easily end before the log is long
// enough to scroll — deal another one and carry on.
async function restartIfOver() {
  const lines = await players[0].logLines();
  if (!lines.some((l) => /wins!/.test(l))) return false;
  await clearModals(players.slice(1)); // everyone else closes their result
  await safeClick(host.$("#modal-ok"), 1500); // the host's button says "Deal again"
  await host.waitForScreen("table");
  await sleep(300);
  return true;
}

// newLine keeps nudging until the reader actually sees another line. One call to
// makeNoise is not enough on its own: the only player on turn may be answering a
// modal, or a Nope window may still be closing.
async function newLine(p, tries = 25) {
  const before = (await p.logLines()).length;
  for (let i = 0; i < tries; i++) {
    await makeNoise();
    await sleep(180);
    if ((await p.logLines()).length > before) return;
  }
  throw new Error(`no new log line after ${tries} moves`);
}

// fillLog runs the game on until the log overflows its box by a comfortable
// margin. "Comfortable" matters: the client treats a reader within 48px of the
// bottom as pinned, so a log that only just overflows makes the scroll
// assertions vacuous — scrolling to the top would still count as pinned.
async function fillLog(p, tries = 60) {
  const overflowing = () =>
    p.page.$eval("#log", (el, n) => el.scrollHeight - el.clientHeight > n, OVERFLOW);

  // A short window needs far fewer lines to overflow, which keeps the round from
  // ending underneath the assertions. The scroll logic being tested does not
  // care how tall the box is.
  await p.page.setViewportSize({ width: 1100, height: 560 });

  for (let i = 0; i < tries; i++) {
    if (await overflowing()) return;
    await restartIfOver();
    await makeNoise();
    await sleep(150);
  }
  const lines = (await p.logLines()).length;
  assert(
    await overflowing(),
    `the log still scrolls by less than ${OVERFLOW}px after ${lines} lines`,
  );
}
