import { test } from "node:test";
import assert from "node:assert/strict";
import { corpsePose, corpseParts, bodyRests, landedBetween, FALL_S, BOUNCE_S, VANISH_S, LIFT } from "./corpse.js";
import { playerParts } from "../assets/models/player.js";

const FLAT = Math.PI / 2;

test("the fall: upright at death, monotonic ease-in to flat", () => {
  assert.equal(corpsePose(0, false).angle, 0);
  assert.equal(corpsePose(-1, false).angle, 0); // a death tick just ahead of the view
  let prev = -1;
  for (let t = 0; t < FALL_S; t += 0.01) {
    const a = corpsePose(t, false).angle;
    assert.ok(a > prev, `not monotonic at ${t}`);
    prev = a;
  }
  // ease-in: the first half covers well under half the angle
  assert.ok(corpsePose(FALL_S / 2, false).angle < FLAT / 3);
  assert.equal(corpsePose(FALL_S, false).angle, FLAT);
});

test("the bounce lifts a little, then it settles flat for good", () => {
  let maxLift = 0;
  for (let t = FALL_S; t <= FALL_S + BOUNCE_S; t += 0.005) {
    const a = corpsePose(t, false).angle;
    assert.ok(a <= FLAT && a > FLAT - 0.3, `angle ${a} at ${t}`);
    maxLift = Math.max(maxLift, FLAT - a);
  }
  assert.ok(maxLift > 0.1, "no bounce");
  for (const t of [FALL_S + BOUNCE_S, 5, 120]) assert.deepEqual(corpsePose(t, false), { angle: FLAT, scale: 1 });
});

test("warmup: shrinks away and is gone", () => {
  assert.deepEqual(corpsePose(0, true), { angle: 0, scale: 1 });
  assert.ok(corpsePose(VANISH_S / 2, true).scale < 1);
  assert.equal(corpsePose(VANISH_S, true), null);
  assert.equal(corpsePose(30, true), null);
});

test("landing fires once", () => {
  assert.ok(landedBetween(FALL_S - 0.01, FALL_S + 0.01));
  assert.ok(!landedBetween(FALL_S + 0.01, FALL_S + 0.02));
  assert.ok(!landedBetween(0, FALL_S - 0.01));
});

const corners = (parts) => parts.flatMap(({ m }) => [-0.5, 0.5].flatMap((x) => [-0.5, 0.5].flatMap((y) => [-0.5, 0.5].map((z) => [0, 1, 2].map((i) => m[i] * x + m[4 + i] * y + m[8 + i] * z + m[12 + i])))));

test("lying flat: low on the floor, just above it, toward the fall direction", () => {
  const feet = [3, 1, -2];
  const body = playerParts({ id: 4, team: "vibe", p: feet, yaw: 0.7, pitch: 0, rgb: [1, 1, 1], crouch: 0 });
  for (const fy of [0, 1, Math.PI, -2.5]) {
    const c = corners(corpseParts(body, feet, fy, { angle: FLAT, scale: 1 }, bodyRests).filter(bodyRests));
    const ys = c.map((v) => v[1]);
    assert.ok(Math.abs(Math.min(...ys) - (feet[1] + LIFT)) < 1e-4, "the lowest corner sits LIFT above the floor");
    assert.ok(Math.max(...ys) - feet[1] < 0.9, `lying, not standing: top at ${Math.max(...ys) - feet[1]}`);
    // the head end lies along the fall direction (-sin fy, -cos fy)
    const f = [-Math.sin(fy), -Math.cos(fy)];
    const along = c.map((v) => (v[0] - feet[0]) * f[0] + (v[2] - feet[2]) * f[1]);
    assert.ok(Math.max(...along) > 1.4, `fell toward ${fy}: reaches ${Math.max(...along)}`);
  }
  // upright (death tick): the body as it stood, LIFT higher
  const up = corners(corpseParts(body, feet, 1, { angle: 0, scale: 1 }, bodyRests)), orig = corners(body);
  up.forEach((v, i) => assert.ok(Math.abs(v[1] - orig[i][1] - LIFT) < 1e-4 && Math.abs(v[0] - orig[i][0]) < 1e-4));
});
