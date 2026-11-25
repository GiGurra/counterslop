import { test } from "node:test";
import assert from "node:assert/strict";
import { spatialize, occlusion, falloff, NEAR, HALF, FAR, OPEN_CUT, WALL_CUT, WALL_GAIN } from "./spatial.js";
import { createPhysics } from "../engine/physics.js";

const near = (a, b, eps = 1e-6) => assert.ok(Math.abs(a - b) <= eps, `${a} vs ${b}`);
const ear = [0, 1.6, 0];
// yaw 0 looks along -z (engine/math.js basis): right is +x.
const L = (yaw = 0, pitch = 0) => ({ pos: ear, yaw, pitch });

test("direction follows the view: right, left, ahead, behind, above (WebAudio space: x right, y up, -z ahead)", () => {
  const right = spatialize(L(), [5, 1.6, 0]), left = spatialize(L(), [-5, 1.6, 0]);
  near(right.pan, 0.8); near(left.pan, -0.8);
  near(right.dir[0], 1); near(left.dir[0], -1);
  const ahead = spatialize(L(), [0, 1.6, -5]), behind = spatialize(L(), [0, 1.6, 5]);
  near(ahead.dir[2], -1); near(behind.dir[2], 1);
  near(ahead.pan, 0); near(behind.pan, 0); // stereo can't tell these apart; HRTF can
  near(spatialize(L(), [0, 6.6, 0]).dir[1], 1);
  // Turned around (yaw pi), what was on the right is on the left.
  near(spatialize(L(Math.PI), [5, 1.6, 0]).pan, -0.8);
  // Looking up 90 degrees, a sound straight above is ahead.
  near(spatialize(L(0, Math.PI / 2 - 1e-9), [0, 6.6, 0]).dir[2], -1, 1e-6);
  for (const s of [right, left, ahead, behind]) near(Math.hypot(...s.dir), 1);
});

test("distance falloff: full up close, half at NEAR + HALF, quieter with distance, silent at FAR", () => {
  assert.equal(falloff(0), 1);
  assert.equal(falloff(NEAR), 1);
  near(falloff(NEAR + HALF), 0.5);
  let prev = 1;
  for (let d = 0; d <= FAR + 5; d += 0.5) {
    const g = falloff(d);
    assert.ok(g <= prev + 1e-12 && g >= 0, `not decreasing at ${d}`);
    prev = g;
  }
  assert.equal(falloff(FAR), 0);
  assert.ok(falloff(30) > 0.15 && falloff(30) < 0.25, `a shot across a site (30 m) is still heard: ${falloff(30)}`);
});

test("walls between muffle: a low-pass and less gain; open air does not; the wall a sound is on does not", () => {
  const phys = createPhysics();
  const wall = { min: [-5, 0, -6], max: [5, 3, -5], kind: "brick" }; // a 3 m wall 5 m ahead
  const low = { min: [-5, 0, 5], max: [5, 1.2, 6], kind: "crate" }; // a waist-high wall behind
  phys.setMap({ boxes: [{ min: [-50, -1, -50], max: [50, 0, 50], kind: "sand" }, wall, low] });
  const raycast = (o, d, t) => phys.raycast(o, d, t);
  const open = spatialize(L(), [8, 1, 0], { raycast });
  assert.equal(open.cut, OPEN_CUT);
  near(open.gain, falloff(Math.hypot(8, 0.6)));
  const walled = spatialize(L(), [0, 1, -10], { raycast });
  assert.equal(occlusion(ear, [0, 1, -10], raycast), 1);
  near(walled.cut, WALL_CUT);
  near(walled.gain, falloff(Math.hypot(10, 0.6)) * WALL_GAIN);
  // Over a waist-high wall: the lower ray is blocked, the upper one not: half.
  assert.equal(occlusion(ear, [0, 0.5, 10], raycast), 0.5);
  const half = spatialize(L(), [0, 0.5, 10], { raycast });
  assert.ok(half.cut < OPEN_CUT && half.cut > WALL_CUT, `${half.cut}`);
  // A bullet impact on the wall's near face is on the wall, not behind it.
  assert.equal(occlusion(ear, [0, 1.5, -5], raycast), 0);
  // No raycast (no map yet): never muffled.
  assert.equal(spatialize(L(), [0, 1, -10]).cut, OPEN_CUT);
});

test("snd_hrtf only picks the panner: gain, pan and muffling are the same either way", () => {
  const a = spatialize(L(0.3, 0.1), [3, 2, -7], { hrtf: true }), b = spatialize(L(0.3, 0.1), [3, 2, -7], { hrtf: false });
  assert.equal(a.hrtf, true); assert.equal(b.hrtf, false);
  assert.deepEqual({ ...a, hrtf: 0 }, { ...b, hrtf: 0 });
});
