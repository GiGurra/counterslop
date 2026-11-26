// sim/cases at the "js" level: every case through the client's predictor.
// internal/game/cases_test.go runs the same cases through the server.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { runCase } from "./cases.js";
import { mapFor } from "./casemap.mjs";

// CASES_DIR / CASES_TRACE: as internal/game/cases_test.go (used by cmd/simdiff).
const dir = process.env.CASES_DIR ? pathToFileURL(process.env.CASES_DIR.replace(/\/?$/, "/")) : new URL("../../sim/cases/", import.meta.url);

for (const f of readdirSync(dir).filter((f) => f.endsWith(".json")).sort()) {
  let c;
  try { c = JSON.parse(readFileSync(new URL(f, dir))); } catch (e) { // this case only: the rest still run (cmd/simdiff)
    test(`case ${f}`, () => { throw e; });
    continue;
  }
  if (c.levels && !c.levels.includes("js")) continue;
  test(`case ${f}: ${c.name}`, () => {
    const { fails, trace } = runCase(c, mapFor(c.map));
    if (process.env.CASES_TRACE) writeFileSync(`${process.env.CASES_TRACE}/${f.replace(/\.json$/, "")}.js.json`, JSON.stringify(trace));
    assert.equal(fails.length, 0, fails.join("\n"));
  });
}
