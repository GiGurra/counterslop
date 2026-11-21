// Prints the client predictor's per-tick trace of every js-level case in a
// directory as JSON {case: trace}. internal/game/cases_test.go compares it with
// the server's trace tick by tick (TestCasesAgree). Usage: node casetrace.mjs <dir>
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { runCase } from "./cases.js";
import { mapFor } from "./casemap.mjs";

const dir = process.argv[2];
const out = {};
for (const f of readdirSync(dir).filter((f) => f.endsWith(".json")).sort()) {
  const c = JSON.parse(readFileSync(join(dir, f)));
  if (c.levels && !c.levels.includes("js")) continue;
  out[f.replace(/\.json$/, "")] = runCase(c, mapFor(c.map)).trace;
}
process.stdout.write(JSON.stringify(out));
