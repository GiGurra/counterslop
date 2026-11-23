import test from "node:test";
import assert from "node:assert/strict";
import { newGait, gaitStep, gaitFromWire, lerpGait, poseTime, gaitK } from "./gait.js";

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
