import test from "node:test";
import assert from "node:assert/strict";
import { segment, ropeParts } from "./rope.js";

const apply = (m, v) => [0, 1, 2].map((i) => m[i] * v[0] + m[4 + i] * v[1] + m[8 + i] * v[2] + m[12 + i]);
const close = (a, b) => a.every((v, i) => Math.abs(v - b[i]) < 1e-5);

test("segment: the unit cube's ends land on a and b, thin across", () => {
  for (const [a, b] of [[[0, 0, 0], [0, 5, 0]], [[1, 2, 3], [7, -1, 4]], [[0, 0, 0], [0, -3, 0.001]]]) {
    const m = segment(a, b, 0.1);
    assert.ok(close(apply(m, [0, -0.5, 0]), a) && close(apply(m, [0, 0.5, 0]), b), `${a} ${b}`);
    const side = apply(m, [0.5, 0, 0]), mid = apply(m, [0, 0, 0]);
    assert.ok(Math.abs(Math.hypot(...side.map((v, i) => v - mid[i])) - 0.05) < 1e-5);
  }
  assert.equal(segment([1, 1, 1], [1, 1, 1]), null);
});

test("ropeParts: from the chest along the player's up to the anchor; nothing unhooked", () => {
  assert.deepEqual(ropeParts([0, 0, 0], 0, null, [1, 0, 0]), []);
  const [beam, clamp] = ropeParts([12, 5, 0], 3, [0, 5, 0], [1, 0, 0]); // on the +x wall: its chest is toward -x
  assert.ok(close(apply(beam.m, [0, -0.5, 0]), [12 - 1.25, 5, 0]));
  assert.ok(close(apply(clamp.m, [0, 0, 0]), [0, 5, 0]));
});
