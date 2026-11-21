import test from "node:test";
import assert from "node:assert/strict";
import { createCasings, casingKind, ejectDelay, ejection, sweep, KINDS, MAX_CASINGS } from "./casings.js";
import { basis } from "../engine/math.js";

const floor = { min: [-200, -1, -200], max: [200, 0, 200], kind: "concrete" };
const wall = { min: [0.5, 0, -5], max: [0.7, 3, 5], kind: "brick" };
const run = (c, boxes, s, hits) => { for (let i = 0; i < s * 60; i++) c.step(1 / 60, boxes, (b) => hits?.push(b)); };

test("kinds: per weapon class; nothing from melee, grenades, the bomb", () => {
  const k = (cls, seed) => KINDS[casingKind({ class: cls }, seed)]?.name;
  assert.equal(k("pistol"), "pistol");
  assert.equal(k("smg"), "pistol");
  assert.equal(k("rifle"), "rifle");
  assert.equal(k("sniper"), "sniper");
  assert.equal(k("shotgun", 0.2), "shell-red");
  assert.equal(k("shotgun", 0.8), "shell-green");
  for (const c of ["melee", "grenade", "bomb"]) assert.equal(casingKind({ class: c }), -1);
  assert.equal(ejectDelay({ action: "auto" }), 0);
  assert.ok(ejectDelay({ action: "bolt", cycle: 1.1 }) > 0.4); // when the bolt cycles
});

test("an ejected casing arcs out to the right, bounces on the floor and lies still", () => {
  const c = createCasings(), hits = [];
  const b = basis(0, 0), { p, v } = ejection([0, 1.6, 0], b, [0, 0, 0], 0.3);
  assert.ok(p[0] > 0 && v[0] > 0 && v[1] > 0, "out of the right side, upwards");
  const slot = c.spawn(p, v, 1, 0.3);
  let top = 0;
  for (let i = 0; i < 20; i++) { c.step(1 / 60, [floor], (h) => hits.push(h)); top = Math.max(top, c.at(slot).p[1]); }
  assert.ok(top > p[1], "rises first");
  run(c, [floor], 3, hits);
  const s = c.at(slot);
  assert.equal(s.moving, false);
  assert.ok(Math.abs(s.p[1] - KINDS[1].rad) < 1e-6, `rests on the floor: y ${s.p[1]}`);
  assert.equal(s.tilt, 0, "lying flat");
  assert.ok(hits.length >= 1 && hits.length <= 3, `${hits.length} bounces`);
  assert.equal(hits[0].first, true);
  assert.equal(hits[0].box, floor);
  assert.ok(s.p[0] > 0.5, "landed off to the right");
  // Settled: stepping again moves nothing.
  const before = c.at(slot).p;
  run(c, [floor], 1);
  assert.deepEqual(c.at(slot).p, before);
});

test("never through a wall, even thrown hard at it", () => {
  const c = createCasings();
  for (let i = 0; i < 50; i++) c.spawn([0, 1.5, (i - 25) * 0.1], [30 + i, 2, 0], 1, i / 50);
  run(c, [floor, wall], 4);
  for (let i = 0; i < 50; i++) {
    const s = c.at(i);
    assert.ok(s.p[0] < wall.min[0], `casing ${i} at x ${s.p[0]}`);
    assert.ok(s.p[1] >= 0, `casing ${i} under the floor`);
  }
});

test("a fast fall lands on a thin floor instead of tunnelling", () => {
  const c = createCasings(), thin = { min: [-5, -0.02, -5], max: [5, 0, 5] };
  const slot = c.spawn([0, 10, 0], [0, -60, 0], 0);
  run(c, [thin], 2);
  assert.ok(c.at(slot).p[1] > -0.001);
});

test("capped: past MAX_CASINGS the oldest is recycled; clear empties", () => {
  const c = createCasings();
  for (let i = 0; i < MAX_CASINGS + 5; i++) c.spawn([i * 0.01, 1, 0], [0, 0, 0], 0);
  assert.equal(c.count, MAX_CASINGS);
  assert.ok(c.flying <= MAX_CASINGS);
  c.clear();
  assert.equal(c.count, 0);
  assert.equal(c.flying, 0);
  assert.equal(c.spawn([0, 1, 0], [0, 0, 0], -1), -1, "kind -1 spawns nothing");
  assert.equal(c.count, 0);
});

test("sweep: entry face and time", () => {
  const h = sweep([wall], [0, 1, 0], [1, 0, 0], 0);
  assert.ok(Math.abs(h.t - 0.5) < 1e-9);
  assert.deepEqual(h.n, [-1, 0, 0]);
  assert.equal(sweep([wall], [0, 1, 0], [0.3, 0, 0], 0), null);
  assert.equal(sweep([wall], [0.6, 1, 0], [1, 0, 0], 0), null, "starting inside: ignored");
});
