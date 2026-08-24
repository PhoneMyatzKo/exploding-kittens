// The how-to-play sheet. Two things here are easy to break silently: the card
// pictures (a wrong path degrades to an emoji, which looks deliberate) and the
// Burmese translation (an empty or half-copied block still renders as a panel
// with headings). Both are checked by asking what actually reached the screen.

import {
  launch, seat, step, check, report, assert, waitFor, sleep, requireServer,
} from "./lib.js";

await requireServer();
console.log("how to play");

// The Myanmar block. U+1000–U+109F is the Burmese range; if a section claims to
// be Burmese and contains none of it, the translation never landed.
const BURMESE = /[က-႟]/;

const browser = await launch();

try {
  for (const game of ["kittens", "kittens-imploding"]) {
    console.log(`  — ${game}`);
    await runFor(game);
  }
} catch {
  // step() has already reported the failure; fall through to the summary rather
  // than dying with a stack trace and no verdict.
} finally {
  await browser.close();
}

report("how to play");

async function runFor(game) {
  const p = await seat(browser, "Alex");
  const expansion = game === "kittens-imploding";

  try {
    await step(`${game}: the sheet opens from the lobby`, async () => {
      await p.create({ game });
      await p.$("#rules-lobby").click();
      await p.page.waitForSelector("#rules-panel", { state: "visible" });
    });

    await check(`${game}: every card in this deck is listed once`, async () => {
      // The list has to follow the deck: the base game must not advertise cards
      // it cannot deal, and the expansion must not hide the ones it adds.
      const slugs = await visibleSlugs(p);
      const base = ["exploding", "defuse", "skip", "attack", "shuffle", "future", "favor", "nope"];
      for (const s of base) {
        assert(slugs.includes(s), `${s} is missing from the sheet`);
      }
      const extra = ["imploding", "reverse", "bottom", "feral-cat", "alter", "targeted-attack"];
      for (const s of extra) {
        assert(
          slugs.includes(s) === expansion,
          `${s} is ${slugs.includes(s) ? "listed" : "missing"} but expansion=${expansion}`,
        );
      }
    });

    await check(`${game}: the cards are shown as pictures, not emoji`, async () => {
      // A broken art path silently falls back to the emoji, so ask the browser
      // whether the pixels arrived rather than whether an <img> exists.
      await waitFor(async () => (await artState(p)).length > 0, { what: "card art in the sheet" });
      const arts = await artState(p);
      const broken = arts.filter((a) => !a.loaded);
      assert(
        broken.length === 0,
        `${broken.length} of ${arts.length} rules cards have no picture, e.g. ${broken[0] && broken[0].slug} (${broken[0] && broken[0].src})`,
      );
      // Big enough to actually recognise — the whole point of using art here.
      assert(arts[0].width >= 50, `the pictures render ${arts[0].width}px wide, too small to read`);
    });

    await check(`${game}: it can be read in Burmese`, async () => {
      await p.$("#lang-my").click();
      await sleep(250);

      const my = await langSection(p, "my");
      assert(my.visible, "the Burmese section did not appear");
      assert(BURMESE.test(my.text), "the Burmese section contains no Burmese text");
      assert(my.text.length > 800, `the Burmese section is only ${my.text.length} characters — half a translation?`);

      const en = await langSection(p, "en");
      assert(!en.visible, "both languages are showing at once");

      // The heading and the close button are part of the sheet too.
      const title = await p.$(".rules-title").textContent();
      assert(BURMESE.test(title), `the title is still ${JSON.stringify(title)}`);

      if (expansion) {
        const block = await p.page.$eval(
          '.rules-lang[data-lang="my"] .rules-expansion',
          (el) => ({ hidden: el.hidden, text: el.textContent }),
        );
        assert(!block.hidden, "the expansion rules are hidden in Burmese");
        assert(BURMESE.test(block.text), "the Burmese expansion rules are not in Burmese");
      }
    });

    await check(`${game}: the card pictures survive the language switch`, async () => {
      // Both languages ship their own <dl>, so the art has to be painted into
      // both — easy to get wrong by painting only what was visible at load.
      const arts = await artState(p);
      assert(arts.length > 0, "the Burmese sheet shows no card pictures");
      assert(arts.every((a) => a.loaded), "some pictures are missing in the Burmese sheet");
    });

    await check(`${game}: the language choice is remembered`, async () => {
      await p.page.reload();
      await p.waitForScreen("lobby");
      await p.$("#rules-lobby").click();
      await p.page.waitForSelector("#rules-panel", { state: "visible" });

      const my = await langSection(p, "my");
      assert(my.visible, "the sheet came back in English after choosing Burmese");

      // Put it back, so the next script starts from the default.
      await p.$("#lang-en").click();
      await sleep(200);
      assert((await langSection(p, "en")).visible, "could not switch back to English");
    });
  } finally {
    await p.page.context().close();
  }
}

// ─────────────────────────────────────────────────────────────────── helpers

// Only the language that is on screen, since the other one is a full duplicate.
function visibleSlugs(p) {
  return p.page.$$eval(".rules-lang:not([hidden]) dt[data-slug]", (els) =>
    els
      .filter((el) => el.offsetParent !== null)
      .map((el) => el.dataset.slug),
  );
}

function artState(p) {
  return p.page.$$eval(".rules-lang:not([hidden]) dt[data-slug] img.rules-art", (els) =>
    els
      .filter((el) => el.offsetParent !== null)
      .map((el) => ({
        slug: el.closest("dt").dataset.slug,
        src: el.getAttribute("src"),
        loaded: el.naturalWidth > 0,
        width: Math.round(el.getBoundingClientRect().width),
      })),
  );
}

function langSection(p, lang) {
  return p.page.$eval(
    `.rules-lang[data-lang="${lang}"]`,
    (el) => ({ visible: !el.hidden, text: el.textContent.trim() }),
  );
}
