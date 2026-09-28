// Chat, in real browsers.
//
// The Go tests in internal/room prove the server half: who gets a line, what the
// rate limiter refuses, that a conversation survives a deal. None of them can see
// the half that decides whether the feature exists for a player — whether the
// launcher is reachable from the lobby, whether an unread badge appears, whether
// the panel covers the hand on a phone, and whether what somebody typed arrives
// as text rather than as markup.
//
// Two browsers throughout, because a chat box you cannot test against a second
// person is not being tested at all.

import {
  launch, seat, step, check, report, assert, waitFor, sleep,
  safeClick, visible, requireServer, BASE,
} from "./lib.js";

await requireServer();
console.log("chat");

const BURMESE = /[က-႟]/;

const browser = await launch();

try {
  await inTheLobby();
  await atTheTable();
  await onAPhone();
} catch {
  // step() has already reported it; fall through to the verdict.
} finally {
  await browser.close();
}

report("chat");

// ───────────────────────────────────────────────────────── the lobby

// The reason chat is handled above the room's "has the game started" guard: half
// of what anybody says in a party game is said before the deal.
async function inTheLobby() {
  const players = [await seat(browser, "Aung"), await seat(browser, "Bo")];
  const [host, guest] = players;

  try {
    await check("chat is not offered outside a room", async () => {
      await host.page.goto(`${BASE}/`);
      await host.menu();
      assert(!(await visible(host.$("#chat-launch"))),
        "the chat button is on the menu, where there is nobody to talk to");
    });

    let code = "";
    await step("two people reach the lobby", async () => {
      code = await host.create({ game: "kittens" });
      await guest.join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
      }
    });

    await check("the chat button is there before the deal", async () => {
      for (const p of players) {
        assert(await visible(p.$("#chat-launch")), `${p.name} has no chat button in the lobby`);
      }
    });

    await check("a message sent in the lobby reaches the other person", async () => {
      await say(host, "are we waiting for anyone?");
      await waitFor(async () => (await lines(guest)).length > 0,
        { what: "the message to arrive", timeout: 5000 });

      const got = await lines(guest);
      assert(got.length === 1, `the guest sees ${got.length} lines: ${JSON.stringify(got)}`);
      assert(got[0].text === "are we waiting for anyone?", `got ${JSON.stringify(got[0].text)}`);
      // The name comes down with the line, so it renders without a seat lookup.
      assert(got[0].name === "Aung", `attributed to ${JSON.stringify(got[0].name)}`);
    });

    // A conversation happening behind a shut panel has to announce itself, or
    // the feature is only useful to whoever already has it open.
    await check("an unread badge counts what arrived while it was shut", async () => {
      assert(!(await chatOpen(guest)), "the guest's panel opened by itself");
      const badge = await badgeText(guest);
      assert(badge === "1", `the badge reads ${JSON.stringify(badge)}, want "1"`);

      // And your own message is not news to you.
      assert((await badgeText(host)) === "", `the sender has a badge reading ${await badgeText(host)}`);
    });

    await check("opening the panel clears the badge and shows the conversation", async () => {
      await ensureChatOpen(guest);
      assert((await badgeText(guest)) === "", `the badge survived opening: ${await badgeText(guest)}`);
      const got = await lines(guest);
      assert(got.length === 1, `the panel shows ${got.length} lines`);
    });

    await check("markup arrives as text, not as HTML", async () => {
      // The one place a stranger's typing reaches the DOM. textContent is what
      // makes it inert, and this is the check that keeps it that way.
      await say(host, "<img src=x onerror=alert(1)><b>bold?</b>");
      await waitFor(async () => (await lines(guest)).length === 2, { what: "the second line" });

      const shown = (await lines(guest))[1];
      assert(shown.text === "<img src=x onerror=alert(1)><b>bold?</b>",
        `the text was altered: ${JSON.stringify(shown.text)}`);
      // If it had been parsed there would be an element inside the line.
      const injected = await guest.page.evaluate(() =>
        document.querySelectorAll("#chat-log b, #chat-log img").length);
      // One <b> per line is the author's name, which the client builds itself.
      assert(injected === 2, `${injected} elements inside the log, want 2 name tags`);
    });

    await check("Burmese survives the round trip", async () => {
      await say(host, "မင်္ဂလာပါ");
      await waitFor(async () => (await lines(guest)).length === 3, { what: "the Burmese line" });
      const shown = (await lines(guest))[2].text;
      assert(BURMESE.test(shown), `Burmese came back as ${JSON.stringify(shown)}`);
      assert(shown === "မင်္ဂလာပါ", `mangled to ${JSON.stringify(shown)}`);
    });

    await check("an empty message cannot be sent", async () => {
      const before = (await lines(guest)).length;
      await say(host, "   ");
      await sleep(400);
      assert((await lines(guest)).length === before, "whitespace became a message");
    });

    // Chat is the room's, so it must never appear in the game's play-by-play.
    await check("nothing said reaches the play-by-play", async () => {
      const log = await host.page.$$eval("#log li", (els) => els.map((e) => e.textContent));
      const leaked = log.filter((l) => /waiting for anyone|မင်္ဂလာပါ|bold\?/.test(l));
      assert(leaked.length === 0, `chat leaked into the log: ${JSON.stringify(leaked)}`);
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ───────────────────────────────────────────────────────── the table

async function atTheTable() {
  const players = [
    await seat(browser, "Aung"), await seat(browser, "Bo"), await seat(browser, "Cho"),
  ];
  const [host] = players;

  try {
    await step("three people are dealt in", async () => {
      const code = await host.create({ game: "kittens" });
      for (const p of players.slice(1)) await p.join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 3, { what: "three seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
    });

    await check("chat still works once the cards are out", async () => {
      await say(host, "good luck");
      await waitFor(async () => (await lines(players[2])).length > 0, { what: "the message" });
      assert((await lines(players[2]))[0].text === "good luck", "the message did not arrive");
    });

    // On a laptop there is room to avoid the hand entirely, so nothing about
    // chat may cover the cards or the deck. This is the regression this project
    // cares about most — a bar sitting on the hand is the bug that started the
    // whole browser suite.
    await check("the panel covers neither the hand nor the deck", async () => {
      await ensureChatOpen(host);

      const m = await overlap(host, "#chat-panel", ["#hand", "#deck"]);
      assert(m.panel.w > 0 && m.panel.h > 0, "the panel has no size");
      for (const [sel, hit] of Object.entries(m.hits)) {
        assert(!hit, `the chat panel is sitting on top of ${sel}`);
      }
      assert(m.panel.onScreen, "the chat panel runs off the window");
    });

    // The panel is dismissible; the launcher is not. Whatever else it does, the
    // one permanently floating thing on the screen must never be between a
    // finger and a card.
    await check("the launcher never sits on a card or an action", async () => {
      const m = await overlap(host, "#chat-launch", [".card", ".actions", "#deck"]);
      for (const [sel, hit] of Object.entries(m.hits)) {
        assert(!hit, `the chat button is on top of ${sel}`);
      }
    });

    // The whole conversation is replayed by the server on the way back in,
    // because chat — unlike the play-by-play — is only ever sent as it happens.
    await check("a reload brings the conversation back", async () => {
      await host.page.reload();
      await host.waitForScreen("table");
      await ensureChatOpen(host);

      await waitFor(async () => (await lines(host)).length > 0,
        { what: "the replayed conversation", timeout: 6000 });
      const got = await lines(host);
      assert(got.some((l) => l.text === "good luck"),
        `came back to ${JSON.stringify(got.map((l) => l.text))}`);
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ───────────────────────────────────────────────────────── on a phone

// The panel is a floating card on a laptop and a sheet on a phone, and the
// phone is where it can go wrong: a fixed panel plus a raised keyboard is how
// composers end up off the bottom of the screen.
async function onAPhone() {
  const players = [await seat(browser, "Aung"), await seat(browser, "Bo")];
  const [host, guest] = players;

  try {
    for (const p of players) await p.page.setViewportSize({ width: 390, height: 844 });

    await step("a game is dealt on a phone", async () => {
      const code = await host.create({ game: "kittens" });
      await guest.join(code);
      for (const p of players) {
        await waitFor(async () => (await p.lobbyNames()).length === 2, { what: "both seats" });
      }
      await host.deal();
      for (const p of players) await p.waitForScreen("table");
    });

    await check("the panel and its composer fit on a phone screen", async () => {
      await ensureChatOpen(host);

      const m = await overlap(host, "#chat-panel", ["#hand"]);
      assert(m.panel.onScreen,
        `the panel runs off a ${m.viewport.h}px screen (bottom at ${Math.round(m.panel.bottom)})`);

      // The box you type into is the part that must never be the bit that falls
      // off, since a panel you can read but not reply in is not a chat box.
      const box = await rect(host, "#chat-input");
      assert(box.w > 0 && box.h > 0, "the message box has no size");
      assert(box.bottom <= m.viewport.h + 1,
        `the message box is ${Math.round(box.bottom - m.viewport.h)}px below the screen`);
      assert(box.left >= -1 && box.right <= m.viewport.w + 1, "the message box runs off sideways");

      // On a phone the open panel *does* cover the table, and that is the
      // deliberate trade: the screen is not wide enough to have both, you are
      // typing rather than playing, and it is one tap to dismiss. What is not
      // negotiable is the always-present launcher.
      const launcher = await overlap(host, "#chat-launch", [".card", ".actions"]);
      for (const [sel, hit] of Object.entries(launcher.hits)) {
        assert(!hit, `on a phone the chat button is on top of ${sel}`);
      }
    });

    await check("sending from a phone works", async () => {
      await say(host, "on the bus");
      await waitFor(async () => (await lines(guest)).length > 0, { what: "the message" });
      assert((await lines(guest))[0].text === "on the bus", "the phone's message did not arrive");
    });
  } finally {
    for (const p of players) await p.page.context().close();
  }
}

// ───────────────────────────────────────────────────────────── helpers

// Declarations, not const arrows: these are called from the top-level flow above
// and a const declared below it is still in its temporal dead zone. See
// README.md — this has caught several scripts.

// ensureChatOpen opens the panel only if it is shut. The launcher is a *toggle*,
// so clicking it blind is how a check ends up waiting for a panel it just
// closed itself — which is exactly what happened the first time this ran.
async function ensureChatOpen(p) {
  if (await chatOpen(p)) return;
  await safeClick(p.$("#chat-launch"), 1500);
  await p.page.waitForSelector("#chat-panel", { state: "visible", timeout: 3000 });
}

// say types and submits — what a player does.
async function say(p, text) {
  await ensureChatOpen(p);
  await p.$("#chat-input").fill(text);
  await p.$("#chat-input").press("Enter");
}

function chatOpen(p) {
  return visible(p.$("#chat-panel"));
}

// lines reads the panel the way a person does — name and text per row.
function lines(p) {
  return p.page.$$eval("#chat-log .chat-line", (els) =>
    els.map((e) => ({
      name: (e.querySelector("b") || {}).textContent || "",
      text: (e.querySelector("span") || {}).textContent || "",
      mine: e.classList.contains("mine"),
    })));
}

async function badgeText(p) {
  return p.page.evaluate(() => {
    const b = document.getElementById("chat-badge");
    if (!b || b.hidden) return "";
    return b.textContent || "";
  });
}

// rect returns plain numbers: a DOMRect does not survive page.evaluate and
// arrives as an empty object that reads as NaN.
function rect(p, sel) {
  return p.page.evaluate((s) => {
    const el = document.querySelector(s);
    if (!el) return { w: 0, h: 0, left: 0, right: 0, top: 0, bottom: 0 };
    const r = el.getBoundingClientRect();
    return { w: r.width, h: r.height, left: r.left, right: r.right, top: r.top, bottom: r.bottom };
  }, sel);
}

// overlap measures one element against everything it must not cover.
//
// Every match of each selector, not just the first: ".card" is eight cards, and
// checking only the leftmost would pass while the button sat on the last one.
function overlap(p, panelSel, otherSels) {
  return p.page.evaluate(([panel, others]) => {
    const box = document.querySelector(panel).getBoundingClientRect();
    const hits = {};
    for (const sel of others) {
      hits[sel] = [...document.querySelectorAll(sel)].some((el) => {
        const r = el.getBoundingClientRect();
        if (!r.width || !r.height) return false;
        return box.left < r.right && box.right > r.left &&
               box.top < r.bottom && box.bottom > r.top;
      });
    }
    return {
      panel: {
        w: box.width, h: box.height, bottom: box.bottom,
        onScreen: box.top >= -1 && box.bottom <= window.innerHeight + 1 &&
                  box.left >= -1 && box.right <= window.innerWidth + 1,
      },
      hits,
      viewport: { w: window.innerWidth, h: window.innerHeight },
    };
  }, [panelSel, otherSels]);
}
