// Runs every check in order and reports one verdict. Each script is a separate
// process so that a crash in one does not take the rest down with it.

import { spawn } from "node:child_process";
import { requireServer } from "./lib.js";

await requireServer();

const scripts = [
  "smoke.js", "menu.js", "selection.js", "logtest.js",
  "publiclobby.js", "modals.js", "rules.js", "imploding.js", "play.js", "uno.js",
  "monopoly.js", "restart.js",
];
const failures = [];

for (const script of scripts) {
  console.log(`\n─── ${script} ${"─".repeat(Math.max(0, 40 - script.length))}`);
  const code = await run(script);
  if (code !== 0) failures.push(script);
}

// The full-game driver again on the expansion deck. Same script, other deck:
// the new cards only ever come up in a game that is dealt with them.
console.log(`\n─── play.js (imploding) ${"─".repeat(18)}`);
if ((await run("play.js", { GAME: "kittens-imploding" })) !== 0) {
  failures.push("play.js (imploding)");
}

console.log("\n════════════════════════════════════════════");
if (failures.length) {
  console.log(`FAILED: ${failures.join(", ")}`);
  process.exit(1);
}
console.log(`all ${scripts.length + 1} scripts passed`);

function run(script, env = {}) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [script], {
      stdio: "inherit",
      cwd: import.meta.dirname,
      env: { ...process.env, ...env },
    });
    child.on("exit", (code) => resolve(code ?? 1));
  });
}
