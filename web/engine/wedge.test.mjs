// Wedges (content.Box.Ramp, internal/sim/wedge.go) on the client: the
// raycast (sparks, chase cam, sound occlusion), name-tag sight lines and
// casings meet the slope, not the wedge's bounding box.
import test from "node:test";
import assert from "node:assert/strict";
import { createPhysics } from "./physics.js";
import { segmentBlocked } from "../game/tags.js";
import { sweep, depenetrate } from "../game/casings.js";

// A 4 m ramp rising 2 m towards +x over a floor (a box without `ramp`, as the map JSON sends it).
const floor = { min: [-20, -1, -20], max: [20, 0, 20], kind: "concrete" };
const ramp = { min: [0, 0, -2], max: [4, 2, 2], kind: "concrete", ramp: 1 };

test("raycast hits the slope with its normal, and passes over it", () => {
  const phys = createPhysics();
  phys.setMap({ name: "t", boxes: [floor, ramp], movers: [] });
  const down = phys.raycast([1, 5, 0], [0, -1, 0], 50);
  assert.ok(Math.abs(down.t - 4.5) < 1e-9, `t ${down.t}`);
  assert.ok(Math.abs(down.n[0] + 1 / Math.sqrt(5)) < 1e-9 && Math.abs(down.n[1] - 2 / Math.sqrt(5)) < 1e-9, `n ${down.n}`);
  const over = phys.raycast([1, 0.55, -10], [0, 0, 1], 50);
  assert.equal(over, null);
  const floorHit = phys.raycast([-1, 5, 0], [0, -1, 0], 50);
  assert.deepEqual([floorHit.t, floorHit.n], [5, [0, 1, 0]]);
});

test("name tags: a sight line over the slope is clear, one into it is blocked", () => {
  assert.equal(segmentBlocked([ramp], [1, 0.55, -5], [1, 0.55, 5]), false);
  assert.equal(segmentBlocked([ramp], [1, 0.45, -5], [1, 0.45, 5]), true);
  assert.equal(segmentBlocked([{ ...ramp, ramp: undefined }], [1, 0.55, -5], [1, 0.55, 5]), true); // the box would block it
});

test("casings land on the slope and get pushed out of it", () => {
  const hit = sweep([ramp], [1, 1, 0], [0, -1, 0], 0.01);
  assert.ok(hit && Math.abs(hit.t - 0.5) < 1e-9 && hit.n[1] > 0.8, JSON.stringify(hit));
  assert.equal(sweep([ramp], [1, 0.6, -5], [0, 0, 10], 0.01), null); // over the slope
  const p = [1, 0.3, 0];
  const out = depenetrate([ramp], p, 0.01);
  assert.ok(out && p[1] > p[0] / 2 + 0.005, `p ${p}`); // above the slope (y = x / 2), by about its radius
});
