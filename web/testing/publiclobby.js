// Rooms are public by default and listed in a browser anyone can join from.
// The rule the listing has to hold to is that it only shows rooms you can
// actually get into — so a private room, a full room, and a room that has
// already been dealt all have to disappear from it.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  listRooms, requireServer,
} from "./lib.js";

await requireServer();
console.log("public lobby");

const browser = await launch();
const host = await seat(browser, "Alex");
const bea = await seat(browser, "Bea");
const cy = await seat(browser, "Cy");
const nosy = await seat(browser, "Dee");

try {
  await check("an empty list says so rather than showing nothing", async () => {
    // Only meaningful on a server with no public rooms yet; if somebody left one
    // up, the status line still has to say something useful.
    await nosy.home();
    await nosy.$("#browse-btn").click();
    await nosy.waitForScreen("browser");
    // An empty <ul> has no size, so Playwright counts it as hidden — wait on the
    // status line, which always has text.
    await waitFor(async () => (await nosy.$("#browser-status").textContent()).trim().length > 0, {
      what: "the browser status line",
    });
    const status = (await nosy.$("#browser-status").textContent()).trim();
    assert(status.length > 0, "the empty lobby browser explains nothing");
  });

  await check("the browser screen gets the shared background", async () => {
    // A new screen that misses the #home, #lobby, #browser rule renders flush to
    // the top on flat crimson. Cheap to check, and it has happened.
    const bg = await nosy.page.$eval("#browser", (el) => getComputedStyle(el).backgroundImage);
    assert(bg && bg !== "none", "the lobby browser has no gradient background");
  });

  let privateCode;
  await step("a private room is created", async () => {
    privateCode = await host.create({ visibility: "private" });
  });

  await check("the lobby says the room is private", async () => {
    const label = (await host.$("#lobby-visibility").textContent()).trim();
    assert(/private/i.test(label), `the lobby calls it ${JSON.stringify(label)}`);
  });

  await check("a private room is not listed", async () => {
    const rooms = await listRooms();
    assert(
      !rooms.some((r) => r.code === privateCode),
      `${privateCode} was advertised despite being private`,
    );
  });

  await check("a private room is still joinable by code", async () => {
    await bea.join(privateCode);
    await waitFor(async () => (await host.lobbyNames()).length === 2, {
      what: "the private room to accept a join",
    });
  });

  let publicCode;
  await step("a public room is created", async () => {
    await host.$("#leave-btn").click();
    await host.waitForScreen("menu");
    publicCode = await host.create({ visibility: "public" });
  });

  await check("the public room is listed with host and seat count", async () => {
    await waitFor(async () => (await listRooms()).some((r) => r.code === publicCode), {
      what: "the room to appear in the listing",
    });
    const room = (await listRooms()).find((r) => r.code === publicCode);
    assert(room.host === "Alex", `host is ${JSON.stringify(room.host)}`);
    assert(room.players === 1, `player count is ${room.players}`);
    assert(room.capacity >= room.players, "capacity is below the player count");
  });

  await check("you can join from the list without ever seeing a code", async () => {
    await cy.home();
    await cy.$("#browse-btn").click();
    await cy.waitForScreen("browser");
    const row = cy.page.locator(`#browser-list li`, { hasText: publicCode });
    await waitFor(async () => (await row.count()) > 0, { what: "the room to be listed" });
    // The name still has to be filled in — the browser is a shortcut past the
    // code, not past identifying yourself.
    await row.locator("button").first().click();
    await cy.waitForScreen("lobby");
    assert(
      (await cy.$("#lobby-code").textContent()).trim() === publicCode,
      "joined the wrong room from the list",
    );
  });

  await check("a dealt game drops out of the list", async () => {
    await bea.home();
    await bea.join(publicCode);
    await waitFor(async () => (await host.lobbyNames()).length === 3, {
      what: "three players in the public room",
    });
    await host.deal();
    await host.waitForScreen("table");
    await waitFor(async () => !(await listRooms()).some((r) => r.code === publicCode), {
      timeout: 6000,
      what: "the started game to leave the listing",
    });
  });

  await check("the visibility choice is remembered", async () => {
    // Chosen on the home screen, so it has to survive a reload or it is a choice
    // you make every single time.
    await nosy.home();
    await nosy.$("#vis-private").click();
    await sleep(150);
    // A reload comes back to the menu, so the game has to be picked again before
    // the room screen — and its remembered state — is on show.
    await nosy.page.reload();
    await nosy.menu();
    await nosy.pickGame();
    const on = await nosy.page.$eval("#vis-private", (el) => el.classList.contains("on"));
    assert(on, "the private choice was forgotten across a reload");
    await nosy.$("#vis-public").click(); // leave the default as we found it
  });
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("public lobby");
