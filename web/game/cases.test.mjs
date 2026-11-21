// sim/cases at the "js" level: every case through the client's predictor.
// internal/game/cases_test.go runs the same cases through the server.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { runCase } from "./cases.js";

// CASES_DIR / CASES_TRACE: as internal/game/cases_test.go (used by cmd/simdiff).
const dir = process.env.CASES_DIR ? pathToFileURL(process.env.CASES_DIR.replace(/\/?$/, "/")) : new URL("../../sim/cases/", import.meta.url);
const map = JSON.parse(readFileSync(new URL("../../internal/content/maps/testdata/de_slop.json", import.meta.url)));

for (const f of readdirSync(dir).filter((f) => f.endsWith(".json")).sort()) {
  const c = JSON.parse(readFileSync(new URL(f, dir)));
  if (c.levels && !c.levels.includes("js")) continue;
  test(`case ${f}: ${c.name}`, () => {
    const { fails, trace } = runCase(c, map);
    if (process.env.CASES_TRACE) writeFileSync(`${process.env.CASES_TRACE}/${f.replace(/\.json$/, "")}.js.json`, JSON.stringify(trace));
    assert.equal(fails.length, 0, fails.join("\n"));
  });
}
