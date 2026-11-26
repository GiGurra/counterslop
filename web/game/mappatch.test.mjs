// mappatch.js applies the map editor's deltas as the server does (Go
// patchDoc): the golden run of ops and undos from TestMapPatchGolden.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { applyPatch, applyPatches } from "./mappatch.js";

const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/mappatch_golden.json", import.meta.url)));

test("the golden patches give the server's map after every step", () => {
  const doc = structuredClone(golden.start);
  assert.ok(golden.steps.length > 15);
  golden.steps.forEach((s, n) => {
    applyPatches(doc, s.patch);
    assert.deepEqual(doc, s.after, `after step ${n}`);
  });
});

test("a bad patch throws (the client then asks for the whole map)", () => {
  assert.throws(() => applyPatch({ boxes: [] }, { p: ["boxes"], a: "set", i: 0, v: 1 }), /out of range/);
  assert.throws(() => applyPatch({ boxes: [] }, { p: ["boxes"], a: "del", i: 3 }), /out of range/);
  assert.throws(() => applyPatch({}, { p: [], a: "set" }), /path/);
  assert.throws(() => applyPatch({}, { p: ["x"], a: "frob" }), /action/);
  const d = applyPatch({}, { p: ["teamSpawns", "vibe"], a: "ins", i: 0, v: [1, 2, 3] });
  assert.deepEqual(d, { teamSpawns: { vibe: [[1, 2, 3]] } });
});
