import { test } from "node:test";
import assert from "node:assert/strict";
import { pickupParts, shieldParts, PICKUP_COLORS, PICKUP_NAMES } from "./pickups.js";

const KINDS = ["bonus", "hotfix", "reorg", "caffeine", "freeze", "flag"];

test("every kind has a colour and a name", () => {
  for (const k of KINDS) { assert.ok(PICKUP_COLORS[k], k); assert.ok(PICKUP_NAMES[k], k); }
});

test("a pickup up is drawn floating over its spot; a taken one is not", () => {
  const parts = pickupParts([{ k: "bonus", p: [10, 2, -5], up: true }, { k: "freeze", p: [0, 0, 0] }], 1.25);
  assert.equal(parts.length, 2 + 10 + 12);
  const crystal = parts[0].m;
  assert.ok(Math.abs(crystal[12] - 10) < 1e-6 && Math.abs(crystal[14] + 5) < 1e-6);
  assert.ok(crystal[13] > 2 + 1 && crystal[13] < 2 + 1.6, `floats at ${crystal[13]}`);
  assert.deepEqual(parts[0].color, PICKUP_COLORS.bonus);
  for (const p of parts.slice(2, 12)) assert.ok(Math.abs(Math.hypot(p.m[12] - 10, p.m[13] - crystal[13], p.m[14] + 5) - 0.95) < 0.1, "the halo round the crystal");
  for (const p of parts.slice(12)) assert.ok(Math.abs(p.m[13] - 2.06) < 1e-3, "the ring lies on the road");
  assert.deepEqual(pickupParts(null, 0), []);
});

test("the crystal spins (its axes are unit-scaled by its size)", () => {
  const a = pickupParts([{ k: "flag", p: [0, 0, 0], up: true }], 0)[0].m, b = pickupParts([{ k: "flag", p: [0, 0, 0], up: true }], 0.5)[0].m;
  assert.notDeepEqual([...a.slice(0, 3)], [...b.slice(0, 3)]);
  for (const m of [a, b]) for (const o of [0, 4, 8]) assert.ok(Math.abs(Math.hypot(m[o], m[o + 1], m[o + 2]) - 0.8) < 1e-5);
});

test("a Code Freeze draws two rings round the car", () => {
  const parts = shieldParts([5, 1, 5], 2, 0.3);
  assert.equal(parts.length, 32);
  for (const p of parts) {
    const r = Math.hypot(p.m[12] - 5, p.m[14] - 5);
    assert.ok(r > 1.2 && r < 2.8, `ring radius ${r}`);
    assert.ok(p.m[13] > 1.3 && p.m[13] < 2.5);
  }
});
