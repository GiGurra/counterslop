import test from "node:test";
import assert from "node:assert/strict";
import { frameOf, turn, unturn, quatOf, matOf, slerp, createRoll, above } from "./gravity.js";
import { FromFrame, ToFrame, Above } from "./sim.gen.js";
import { basis, fpsView } from "./math.js";

const close = (a, b, eps = 1e-6) => a.every((v, i) => Math.abs(v - b[i]) < eps);

test("frameOf: each gravity's frame turns as Go sim FromFrame/ToFrame does; the world's is none", () => {
  assert.equal(frameOf(0), null);
  for (let g = 1; g < 6; g++) {
    for (const v of [[1, 0, 0], [0, 1, 0], [0, 0, 1], [0.3, -2, 5]]) {
      assert.ok(close(turn(frameOf(g), v), FromFrame(g, v)), `g ${g} ${v}`);
      assert.ok(close(unturn(frameOf(g), v), ToFrame(g, v)));
    }
    assert.deepEqual(above([1, 2, 3], g, 1.5), Above([1, 2, 3], g, 1.5)); // the same bits: the up is an axis
  }
});

test("quaternions: a frame round-trips through its quaternion; slerp ends at its ends", () => {
  for (let g = 0; g < 6; g++) {
    const m = frameOf(g) ?? new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]);
    assert.ok(close([...matOf(quatOf(frameOf(g)))], [...m], 1e-6), `g ${g}`);
    const a = quatOf(null), b = quatOf(frameOf(g));
    assert.ok(close(slerp(a, b, 1).map(Math.abs), b.map(Math.abs)));
    assert.ok(close(slerp(a, b, 0), a));
  }
});

test("the camera roll eases to the new gravity's frame and settles there; a spawn snaps", () => {
  const r = createRoll();
  assert.equal(r.update(0.016, 0), null);
  const mid = r.update(0.016, 3); // a flip onto the +x wall: part way
  assert.ok(mid && !r.settled);
  const up = turn(mid, [0, 1, 0]);
  assert.ok(up[1] > 0.5 && up[0] < 0 && up[0] > -0.5, `${up}`); // turning from +y toward -x
  let f = mid;
  for (let i = 0; i < 120; i++) f = r.update(0.016, 3);
  assert.ok(r.settled);
  assert.equal(f, frameOf(3));
  r.update(0.016, 1); // to the ceiling: 180°, still a smooth turn
  assert.ok(!r.settled);
  r.snap(0);
  assert.ok(r.settled);
  assert.equal(r.update(0.016, 0), null);
});

test("basis and fpsView in a frame: the view's axes turned with it", () => {
  for (let g = 1; g < 6; g++) {
    const F = frameOf(g), b0 = basis(0.7, -0.3), b = basis(0.7, -0.3, F);
    for (const k of ["fwd", "right", "up"]) assert.ok(close(b[k], turn(F, b0[k])), `${g} ${k}`);
    // the view matrix takes the turned forward to -z
    const v = fpsView([1, 2, 3], 0.7, -0.3, F), d = b.fwd;
    const z = [0, 1, 2].map((i) => v[i] * d[0] + v[4 + i] * d[1] + v[8 + i] * d[2]);
    assert.ok(close(z, [0, 0, -1], 1e-5), `${g} ${z}`);
  }
});

test("an edge wrap's twist keeps the view where it was: the turned yaw in the twisted frame looks the same way", () => {
  for (let g = 0; g < 6; g++) {
    for (const a of [Math.PI / 2, -Math.PI / 2, Math.PI]) {
      const r = createRoll();
      r.snap(g);
      const yaw = 0.4, b0 = basis(yaw, -0.2, frameOf(g));
      r.twist(a);
      assert.ok(!r.settled);
      const b1 = basis(yaw + a, -0.2, r.update(0, g)); // dt 0: the twisted frame itself
      for (const k of ["fwd", "up"]) assert.ok(close(b1[k], b0[k], 1e-6), `${g} ${a} ${k}: ${b1[k]} vs ${b0[k]}`);
      for (let i = 0; i < 200; i++) r.update(0.016, g); // and it eases back to the gravity's own frame
      assert.ok(r.settled);
    }
  }
  const r = createRoll();
  r.twist(0);
  assert.ok(r.settled);
});
