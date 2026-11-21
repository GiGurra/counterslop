import { test } from "node:test";
import assert from "node:assert/strict";
import { muzzlePose, flashSpread, MUZZLE } from "./muzzle.js";

const dist = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1], a[2] - b[2]);

// Operator: the flash lit the shooter's own gun too much. It starts at the
// muzzle (ahead of the gun, not at the eye) and shines mostly forward.
test("the flash sits at the muzzle, ahead of the gun", () => {
  const eye = [0, 1.62, 0];
  const p = muzzlePose(eye, 0, 0, null); // yaw 0 looks down -z
  assert.ok(p.pos[2] < -MUZZLE.ahead + 0.01, `ahead of the eye: ${p.pos}`);
  assert.ok(p.pos[0] > 0 && p.pos[1] < eye[1], "right of and below the eye, where the gun is");
  assert.deepEqual(p.dir.map((v) => Math.round(v * 1e6) / 1e6), [-0, 0, -1]);
});

test("a wall closer than the muzzle pulls the flash back to our side", () => {
  const p = muzzlePose([0, 1.62, 0], 0, 0, 0.4);
  assert.ok(dist(p.pos, [0, 1.62, 0]) < 0.4, `stays in front of the wall: ${p.pos}`);
});

test("the spread is forward: full ahead, a quarter behind, smooth between", () => {
  assert.equal(flashSpread(1), 1);
  assert.ok(Math.abs(flashSpread(-1) - MUZZLE.back) < 1e-9);
  let prev = flashSpread(-1);
  for (let c = -0.9; c <= 1; c += 0.1) {
    const v = flashSpread(c);
    assert.ok(v >= prev - 1e-12, "never dips going forward");
    prev = v;
  }
});
