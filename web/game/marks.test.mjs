import test from "node:test";
import assert from "node:assert/strict";
import { createRing, createHoles, createLifetime, seedOf, boxAt, MAX_HOLES } from "./marks.js";

test("ring: fills to its cap, then recycles the oldest first", () => {
  const r = createRing(3, 1);
  for (let i = 0; i < 3; i++) r.data[r.add()] = i;
  assert.equal(r.count, 3);
  assert.deepEqual(r.slots().map((s) => r.data[s]), [0, 1, 2]);
  r.data[r.add()] = 3; // overwrites 0
  assert.equal(r.count, 3);
  assert.deepEqual(r.slots().map((s) => r.data[s]), [1, 2, 3]);
  const v = r.version;
  r.clear();
  assert.equal(r.count, 0);
  assert.ok(r.version > v);
});

test("holes: capped at MAX_HOLES, seeded by position", () => {
  const h = createHoles();
  for (let i = 0; i < MAX_HOLES + 10; i++) h.add([i * 0.1, 1, 2], [0, 0, 1]);
  assert.equal(h.count, MAX_HOLES);
  assert.equal(seedOf([1, 2, 3]), seedOf([1, 2, 3]));
  assert.notEqual(seedOf([1, 2, 3]), seedOf([1, 2, 3.01]));
  const s = seedOf([5, 6, 7]);
  assert.ok(s >= 0 && s < 1);
});

test("boxAt: the box under a face point", () => {
  const a = { min: [0, 0, 0], max: [1, 1, 1], kind: "crate" }, b = { min: [2, 0, 0], max: [3, 1, 1], kind: "metal" };
  assert.equal(boxAt([a, b], [1, 0.5, 0.5], [1, 0, 0]), a);
  assert.equal(boxAt([a, b], [2, 0.5, 0.5], [-1, 0, 0]), b);
  assert.equal(boxAt([a, b], [1.5, 0.5, 0.5], [0, 1, 0]), null);
});

test("lifetime: clears on a new round and a new map, not within a round or in warmup", () => {
  const l = createLifetime();
  assert.equal(l.update({ ph: "warmup", n: 0 }, "de_slop"), false); // first sight
  assert.equal(l.update({ ph: "warmup", n: 0 }, "de_slop"), false);
  assert.equal(l.update(null, "de_slop"), false); // deathmatch-like: no rounds
  assert.equal(l.update({ ph: "freeze", n: 1 }, "de_slop"), true); // warmup over, round 1
  assert.equal(l.update({ ph: "live", n: 1 }, "de_slop"), false);
  assert.equal(l.update({ ph: "over", n: 1 }, "de_slop"), false);
  assert.equal(l.update({ ph: "freeze", n: 2 }, "de_slop"), true);
  assert.equal(l.update({ ph: "live", n: 2 }, "de_slop"), false);
  assert.equal(l.update({ ph: "live", n: 2 }, "de_runway"), true); // map change
  assert.equal(l.update({ ph: "freeze", n: 1 }, "de_runway"), true); // match restart
  assert.equal(l.update({ ph: "live", n: 1 }, "de_runway"), false);
  assert.equal(l.update({ ph: "freeze", n: 1 }, "de_runway"), true); // restart again, same round number
});

// GitHub #17: each hole carries its surface's look; kind and seed share one
// float of the 8-float record (kind + seed, seed < 1).
test("a hole's kind and seed share the seed float: the integer is the kind, the fraction the seed", async () => {
  const { createHoles, holeKind, HOLE_KINDS, HOLE_STRIDE, seedOf } = await import("./marks.js");
  assert.deepEqual(HOLE_KINDS, ["stone", "metal", "wood", "glass", "soft"]);
  assert.equal(holeKind("stone"), 0); assert.equal(holeKind("metal"), 1); assert.equal(holeKind("wood"), 2);
  assert.equal(holeKind("glass"), 3); assert.equal(holeKind("dirt"), 4); assert.equal(holeKind("cardboard"), 4); assert.equal(holeKind("soft"), 4);
  assert.equal(holeKind(undefined), 0);
  const h = createHoles(8);
  for (let k = 0; k < HOLE_KINDS.length; k++) {
    const p = [k * 1.37, 0.5, -k * 2.11];
    h.add(p, [0, 1, 0], 0.1, k);
    const f = h.ring.data[k * HOLE_STRIDE + 3];
    assert.equal(Math.floor(f), k);
    assert.ok(Math.abs((f - k) - seedOf(p)) < 1e-6, `seed survives float32 next to kind ${k}`);
  }
});
