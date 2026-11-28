import test from "node:test";
import assert from "node:assert/strict";
import { newGait, gaitStep, gaitMove, gaitFromWire, lerpGait, poseTime, gaitK } from "./gait.js";
import { newBody } from "../engine/physics.js";
import { MoverOffset } from "../engine/sim.gen.js";

// The gait's rules are Go (internal/game/gait_shared.go, gait_test.go); these
// pin the adapter: wire decoding, stepping, interpolation.

test("snapshot g decodes to a Gait; absent is null (the still pose)", () => {
  assert.deepEqual(gaitFromWire([75, 25, 0, 50]), { phase: 1.5, amp: 0.5, air: 0, crouch: 1 });
  assert.deepEqual(gaitFromWire([75]), { phase: 1.5, amp: 0, air: 0, crouch: 0 }, "trailing zeros left out");
  assert.deepEqual(gaitFromWire([]), { phase: 0, amp: 0, air: 0, crouch: 0 });
  assert.equal(gaitFromWire(undefined), null);
});

test("stepping runs the shared rules: distance turns the phase, standing doesn't", () => {
  const g = newGait();
  for (let i = 0; i < 100; i++) gaitStep(g, 0, 0, -0.07, { onGround: true, id: 3 });
  assert.ok(g.phase > 0 && g.amp > 0.5);
  const p = g.phase;
  for (let i = 0; i < 20; i++) gaitStep(g, 0, 0, 0, { onGround: true, id: 3 });
  assert.equal(g.phase, p);
});

test("interpolation runs the phase forward through the wrap; pose time between ticks", () => {
  const a = { phase: 6, amp: 0, air: 0, crouch: 0 }, b = { phase: 0.5, amp: 1, air: 0, crouch: 0 };
  const m = lerpGait(a, b, 0.5);
  assert.ok(m.phase > 6 || m.phase < 0.5, `phase ${m.phase}`);
  assert.equal(m.amp, 0.5);
  assert.equal(poseTime(30, 33, 1 / 3), 0.31);
});

test("per-player factors are seeded by id", () => {
  assert.deepEqual(gaitK(3), gaitK(3));
  assert.notDeepEqual(gaitK(3), gaitK(4));
});

test("gaitMove: a rider's move in its ship's frame (the ship's travel off); off movers the world move", () => {
  const ship = { kind: "path", mat: "metal", min: [-4, -0.5, -4], max: [4, 0, 4], period: 20, phase: 0, path: [[0, 0, 0, 0], [10, 100, 0, 0]], hide: [0, 0] };
  const map = { movers: [ship], gravZones: [{ name: "cabin", min: [-4, -0.5, -4], max: [4, 3, 4], g: 1, jump: 1, mover: 1 }] };
  const t0 = 5, t1 = 5.01; // mid-way: ~15 m/s
  const b = newBody([0, 0, 0]);
  b.onGround = true;
  const at = (t) => MoverOffset(ship, t)[0]; // where the ship is
  assert.ok(at(t1) - at(t0) > 0.1, "the ship moves");
  b.pos = [at(t1), 0, 0.02];
  const m = gaitMove(map, b, [at(t0), 0, 0], t0, t1);
  assert.ok(Math.abs(m[0]) < 1e-9 && Math.abs(m[2] - 0.02) < 1e-12, `in the cabin: ${m}`);
  const off = newBody([50, 0, 50]);
  off.onGround = true;
  assert.deepEqual(gaitMove(map, off, [50, 0, 49.9], t0, t1).map((v) => +v.toFixed(9)), [0, 0, 0.1]);
  assert.deepEqual(gaitMove(null, off, [50, 0, 49.9], t0, t1).map((v) => +v.toFixed(9)), [0, 0, 0.1], "no map");
});
