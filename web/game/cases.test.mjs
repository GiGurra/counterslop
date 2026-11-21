// sim/cases at the "js" level: every case through the client's predictor.
// internal/game/cases_test.go runs the same cases through the server.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { runCase } from "./cases.js";

const dir = new URL("../../sim/cases/", import.meta.url);
const map = JSON.parse(readFileSync(new URL("../../internal/content/maps/testdata/de_slop.json", import.meta.url)));

for (const f of readdirSync(dir).filter((f) => f.endsWith(".json")).sort()) {
  const c = JSON.parse(readFileSync(new URL(f, dir)));
  if (c.levels && !c.levels.includes("js")) continue;
  test(`case ${f}: ${c.name}`, () => {
    const fails = runCase(c, map);
    assert.equal(fails.length, 0, fails.join("\n"));
  });
}
