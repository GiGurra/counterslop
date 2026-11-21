import { test } from "node:test";
import assert from "node:assert/strict";
import { envelope, createLightPool, createLightFlood, lightMask, MASK, ATTACK, CURVES } from "./dynlights.js";
import { buildGrid, cellOf } from "./lights.js";

const box = (x0, y0, z0, x1, y1, z1) => ({ min: [x0, y0, z0], max: [x1, y1, z1], kind: "concrete" });

test("envelope: bright from the first instant, peak at the attack, smooth decay to exactly zero", () => {
  const life = 0.06;
  assert.equal(envelope(0, life), 0.5);
  assert.ok(Math.abs(envelope(ATTACK, life) - 1) < 1e-9, "full at the end of the attack");
  for (let t = 0; t < ATTACK; t += 0.001) assert.ok(envelope(t + 0.001, life) >= envelope(t, life), "rises during the attack");
  let prev = 1;
  for (let t = ATTACK; t <= life; t += 0.0005) {
    const v = envelope(t, life);
    assert.ok(v <= prev + 1e-12, `decays monotonically (t ${t})`);
    assert.ok(prev - v < 0.06, `no jump at t ${t}: ${prev} -> ${v}`);
    prev = v;
  }
  assert.equal(envelope(life, life), 0);
  assert.equal(envelope(1, life), 0);
  assert.equal(envelope(-0.01, life), 0);
  assert.ok(envelope(life - 0.001, life) < 1e-3, "no hard cut: almost nothing left just before the end");
  // Exponential-ish: half-way through the decay well under half.
  assert.ok(envelope(ATTACK + (life - ATTACK) / 2, life) < 0.25);
});

test("pool: free slots first, then the weakest lit flash is evicted (ties: the oldest)", () => {
  const p = createLightPool(3);
  const f = (intensity, life = 0.1) => ({ pos: [0, 0, 0], color: [1, 1, 1], intensity, range: 6, life });
  assert.equal(p.add(f(10), 0), 0);
  assert.equal(p.add(f(5), 0), 1);
  assert.equal(p.add(f(8), 0), 2);
  assert.equal(p.add(f(9), 0.001), 1, "full: replaces the weakest (5)");
  assert.equal(p.slots[1].intensity, 9);
  assert.equal(p.add(f(1), 0.002), -1, "weaker than everything lit: dropped");
  // Equal strength at the same age: the oldest goes.
  const q = createLightPool(2);
  q.add(f(4), 0); q.add(f(4), 0);
  assert.equal(q.add(f(4), 0), 0);
  // Expired slots are free again, and live() drops them.
  assert.equal(p.add(f(1), 0.5), 0, "slot 0 expired: taken");
  assert.deepEqual(p.live(0.5).map((e) => e.slot), [0]);
  assert.equal(p.slots[1], null);
  assert.ok(Math.abs(p.live(0.5)[0].strength - 0.5) < 1e-9, "strength = intensity x envelope");
});

test("pool: with an eye, a far flash matters less than a near one", () => {
  const p = createLightPool(1);
  const eye = [0, 0, 0];
  p.add({ pos: [1, 0, 0], intensity: 10, range: 6, life: 0.1 }, 0, eye);
  assert.equal(p.add({ pos: [40, 0, 0], intensity: 10, range: 6, life: 0.1 }, 0.001, eye), -1);
  assert.equal(p.add({ pos: [0.5, 0, 0], intensity: 10, range: 6, life: 0.1 }, 0.001, eye), 0);
});

// Two rooms split by a thin wall at x 10..10.1; a door at z 4..5 when door is set.
function rooms(door) {
  const b = [box(-1, -1, -1, 21, 0, 11), box(-1, 4, -1, 21, 4.3, 11)];
  b.push(box(-1, 0, -1, 0, 4, 11), box(20, 0, -1, 21, 4, 11), box(0, 0, -1, 20, 4, 0), box(0, 0, 10, 20, 4, 11));
  if (door) b.push(box(10, 0, 0, 10.1, 4, 4), box(10, 0, 5, 10.1, 4, 10), box(10, 2.5, 4, 10.1, 4, 5));
  else b.push(box(10, 0, 0, 10.1, 4, 10));
  return b;
}
const lit = (g, m, p) => {
  const c = cellOf(g, p).map((k, a) => k - m.off[a]);
  return m.data[c[0] + MASK * (c[2] + MASK * c[1])] === 255;
};

test("flood mask: a flash lights its room, not the next one through the wall; through a door it spills", () => {
  for (const door of [false, true]) {
    const g = buildGrid(rooms(door));
    const fl = createLightFlood(g);
    const pos = [8, 1.5, 4.5], m = lightMask(g, pos, fl.cells(pos, 8));
    assert.ok(lit(g, m, [6, 1, 5]), "own room");
    assert.ok(lit(g, m, [9.9, 1, 7]), "up to the wall");
    assert.ok(!lit(g, m, [19, 1, 1]), "beyond the range");
    assert.equal(lit(g, m, [10.6, 1, 4.5]), door, `just behind the wall (door ${door})`);
  }
});

test("flood: cached per start cell; a shot from the same cell floods nothing", () => {
  const g = buildGrid(rooms(false));
  const fl = createLightFlood(g);
  const a = fl.cells([5.1, 1.6, 5.1], 7);
  const b = fl.cells([5.2, 1.7, 5.2], 7);
  assert.equal(a, b);
  assert.deepEqual(fl.stats().floods, 1);
  assert.equal(fl.stats().hits, 1);
  fl.cells([3, 1.6, 5], 7);
  assert.equal(fl.stats().floods, 2);
});

test("pool: a light's curve sets its strength (a name or a function); steady lights stay until their life ends", () => {
  const p = createLightPool(2);
  p.add({ pos: [0, 0, 0], intensity: 4, range: 6, life: 10, curve: "steady" }, 0);
  p.add({ pos: [0, 0, 0], intensity: 2, range: 6, life: 1, curve: (t, life) => 1 - t / life }, 0);
  const at = (t) => p.live(t).map((e) => +e.strength.toFixed(3));
  assert.deepEqual(at(0.5), [4, 1]);
  assert.deepEqual(at(5), [4]);
  assert.deepEqual(at(10), []);
  assert.equal(CURVES.steady(3, 2), 0);
});
