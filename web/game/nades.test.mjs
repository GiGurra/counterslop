import { test } from "node:test";
import assert from "node:assert/strict";
import { fireParts } from "./nades.js";

// Fire pools (#11): every flame point gets a scorch tile and three lit tongues,
// the same on every client (seeded by pool id and point), flickering with time
// and shrinking to bare scorch as the pool dies (k).
test("fire pool parts: per flame, seeded, flickering, dying down", () => {
  const pts = [[0, 0, 0], [0.7, 0, 0], [0, 0, 0.7]];
  const a = fireParts(pts, 3, 1.5), b = fireParts(pts, 3, 1.5);
  assert.equal(a.length, pts.length * 4);
  assert.deepEqual(a.map((p) => [...p.m]), b.map((p) => [...p.m]), "the same on every client");
  assert.ok(a.filter((p) => p.emit === 1).length === pts.length * 3, "lit tongues");
  const later = fireParts(pts, 3, 1.62);
  assert.notDeepEqual(a.map((p) => [...p.m]), later.map((p) => [...p.m]), "it flickers");
  assert.equal(fireParts(pts, 3, 1.5, 0).length, pts.length, "out: only the scorch is left");
  assert.deepEqual(fireParts(null, 3, 1), []);
});
