// The shortest path that proves the whole stack is alive: three people reach a
// dealt table from a cold page load. If this fails, nothing else is worth
// running.

import {
  launch, seat, step, check, report, assert, waitFor, requireServer,
} from "./lib.js";

await requireServer();
console.log("smoke");

const browser = await launch();
const host = await seat(browser, "Alex");
const bea = await seat(browser, "Bea");
const cy = await seat(browser, "Cy");
const all = [host, bea, cy];

try {
  let code;

  await step("host creates a room and lands in the lobby", async () => {
    code = await host.create();
    assert(await host.onScreen("lobby"), "host is not on the lobby screen");
  });

  await step("two more join by code", async () => {
    await bea.join(code);
    await cy.join(code);
  });

  await step("everyone sees all three names", async () => {
    for (const p of all) {
      await waitFor(async () => (await p.lobbyNames()).length === 3, {
        what: `${p.name} to see three players`,
      });
      const names = (await p.lobbyNames()).join(" ");
      for (const who of ["Alex", "Bea", "Cy"]) {
        assert(names.includes(who), `${p.name} cannot see ${who} in ${JSON.stringify(names)}`);
      }
    }
  });

  await check("only the host is offered the deal button", async () => {
    assert(await host.$("#start-btn").isVisible(), "host has no deal button");
    assert(!(await bea.$("#start-btn").isVisible()), "a non-host was offered the deal button");
  });

  await step("dealing puts everyone at the table", async () => {
    await host.deal();
    for (const p of all) await p.waitForScreen("table");
  });

  await step("every player is dealt eight cards", async () => {
    for (const p of all) {
      await waitFor(async () => (await p.hand()).length === 8, {
        what: `${p.name} to hold eight cards`,
      });
    }
  });

  await check("exactly one player has the turn", async () => {
    const turns = [];
    for (const p of all) if (await p.isMyTurn()) turns.push(p.name);
    assert(turns.length === 1, `expected one player on turn, got [${turns}]`);
  });

  await check("nobody can see anyone else's cards", async () => {
    // The redaction boundary has a Go test; this is the client half of it —
    // no other seat should render card faces, only a count.
    const faces = await host.page.$$eval(
      ".seats .card[data-slug]",
      (els) => els.length,
    );
    assert(faces === 0, `${faces} opponent card faces are rendered on the table`);
  });

  await check("the card faces actually load", async () => {
    // A card whose art 404s quietly falls back to an emoji glyph, so a broken
    // asset path breaks nothing visible to the other checks — it just makes the
    // game look like a prototype. Ask the browser whether the pixels arrived.
    await waitFor(
      async () =>
        (await host.page.$$eval("#hand .card img.art", (els) => els.length)) > 0,
      { what: "card art to be attempted at all" },
    );
    const arts = await host.page.$$eval("#hand .card img.art", (els) =>
      els.map((e) => ({ src: e.getAttribute("src"), w: e.naturalWidth })),
    );
    const broken = arts.filter((a) => a.w === 0);
    assert(
      broken.length === 0,
      `${broken.length} of ${arts.length} card faces failed to load, e.g. ${broken[0] && broken[0].src}`,
    );
  });

  await check("the log is visible without opening anything", async () => {
    assert(await host.$("#log").isVisible(), "the play-by-play is not on screen");
    const lines = await host.logLines();
    assert(lines.some((l) => /dealt/i.test(l)), `no deal line in the log: ${JSON.stringify(lines)}`);
  });
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("smoke");
