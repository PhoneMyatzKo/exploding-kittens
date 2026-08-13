// The front page is a game picker. With one game playable it would be tempting
// to skip straight past it, so what these checks hold to is that the menu earns
// its place: it shows what is coming as well as what works, it refuses to open a
// room for a game that does not exist, and it never gets between somebody and an
// invite link.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  listGames, requireServer, BASE,
} from "./lib.js";

await requireServer();
console.log("game menu");

const browser = await launch();
const alex = await seat(browser, "Alex");
const bea = await seat(browser, "Bea");

try {
  await step("a cold load lands on the menu, not in a game", async () => {
    await alex.page.goto(`${BASE}/`);
    await alex.menu();
    assert(!(await alex.onScreen("home")), "the room screen was up before a game was chosen");
    assert(!(await alex.onScreen("lobby")), "the lobby was up before a game was chosen");
  });

  await check("the menu screen gets the shared background", async () => {
    // A new screen that misses the #menu, #home, #lobby, #browser rule renders
    // flush to the top on flat crimson. This has happened before.
    const bg = await alex.page.$eval("#menu", (el) => getComputedStyle(el).backgroundImage);
    assert(bg && bg !== "none", "the menu has no gradient background");
  });

  await check("every catalogued game gets a tile", async () => {
    const served = await listGames();
    const tiles = await alex.games();
    assert(
      tiles.length === served.length,
      `${tiles.length} tiles for ${served.length} catalogued games`,
    );
    for (const g of served) {
      const tile = tiles.find((t) => t.slug === g.slug);
      assert(tile, `no tile for ${g.slug}`);
      assert(
        tile.locked === !g.playable,
        `${g.slug} tile is ${tile.locked ? "locked" : "open"} but the server says playable=${g.playable}`,
      );
      assert(tile.text.includes(g.name), `the ${g.slug} tile does not name the game`);
    }
  });

  await check("unbuilt games are shown, not hidden", async () => {
    // An empty-looking hub reads as broken. "Coming soon" is information, so the
    // locked tiles have to actually be on screen.
    const locked = (await alex.games()).filter((t) => t.locked);
    assert(locked.length > 0, "no coming-soon tiles at all");
    for (const t of locked) {
      assert(/soon/i.test(t.text), `the ${t.slug} tile does not say it is coming`);
    }
  });

  await check("a locked tile says so instead of doing nothing", async () => {
    const locked = (await alex.games()).find((t) => t.locked);
    await alex.$(`.game-tile[data-slug="${locked.slug}"]`).click();
    await sleep(300);
    assert(await alex.onScreen("menu"), "a locked game let us off the menu");
    const toast = await alex.$("#toasts").textContent();
    assert(toast.trim().length > 0, "tapping a locked game said nothing");
  });

  await check("the server refuses to open a room for an unbuilt game", async () => {
    // The tile being disabled is a courtesy; the door is what has to be locked.
    const res = await fetch(`${BASE}/api/rooms`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ game: "uno" }),
    });
    assert(res.status === 400, `POST for an unbuilt game returned ${res.status}, want 400`);
  });

  await step("picking the playable game reaches the room screen", async () => {
    await alex.pickGame("kittens");
    assert(await alex.onScreen("home"), "picking a game did not open the room screen");
  });

  await check("the room screen is dressed as the game you picked", async () => {
    const logo = await alex.$("#home-logo").textContent();
    assert(/exploding kittens/i.test(logo), `the room screen still says ${JSON.stringify(logo)}`);
  });

  let code;
  await check("a created room carries the game through to the lobby", async () => {
    code = await alex.create();
    await waitFor(async () => (await listRoomsRaw()).some((r) => r.code === code), {
      what: "the room to be listed",
    });
    const row = (await listRoomsRaw()).find((r) => r.code === code);
    assert(row.game === "kittens", `the listing says the room is ${JSON.stringify(row.game)}`);
  });

  await check("an invite link skips the menu entirely", async () => {
    // The person who sent the link already chose the game. Making their guests
    // pick it again would be a detour, and would let them pick wrong.
    // The name has to be stored for the link to go straight in, and storage can
    // only be written once the page is on the server's origin.
    await bea.page.goto(`${BASE}/`);
    await bea.page.evaluate(() => localStorage.setItem("ek:name", "Bea"));
    await bea.page.goto(`${BASE}/#${code}`);
    // Adding a hash to the URL you are already on is a same-document navigation,
    // so the boot code never re-runs. A real guest opens the link cold; reload to
    // reproduce that rather than testing a no-op.
    await bea.page.reload();
    await bea.waitForScreen("lobby");
    assert(!(await bea.onScreen("menu")), "the menu got in the way of an invite link");
    assert(
      (await bea.$("#lobby-code").textContent()).trim() === code,
      "the invite link landed in the wrong room",
    );
  });

  await check("leaving a room goes back to the menu", async () => {
    await bea.$("#leave-btn").click();
    await bea.waitForScreen("menu");
    const url = bea.page.url();
    assert(!/#\w/.test(url), `the room code is still in the URL: ${url}`);
  });
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("game menu");

async function listRoomsRaw() {
  const res = await fetch(`${BASE}/api/rooms`);
  return (await res.json()).rooms || [];
}
