import { test } from "node:test";
import assert from "node:assert/strict";
import { createGhost, poseAt, parseRun, GHOST_HZ, GHOST_V, FOOT, CAR, GROUND } from "./ghost.js";

// memStore: a localStorage stand-in; broken: one whose every access throws.
const memStore = () => { const m = new Map(); return { m, get: (k) => m.get(k) ?? null, set: (k, v) => { m.set(k, v); return true; } }; };

// race drives one race at 60 fps: x = 5 m/s * clock, finishing at t.
function race(g, map, t, { speed = 5, from = 0 } = {}) {
  for (let c = from; c < t; c += 1 / 60) g.update(map, { live: true, racing: true, clock: c }, { pos: [c * speed, 0, 0], yaw: -Math.PI / 2, kind: FOOT, ground: true });
  g.update(map, { live: true, racing: true, clock: t, finished: 1, finishTime: t }, { pos: [t * speed, 0, 0], yaw: -Math.PI / 2, kind: FOOT });
}

test("a finished run becomes the map's best, kept in storage, and plays back on the race clock", () => {
  const store = memStore(), g = createGhost({ store });
  g.update("pk_a", null, null);
  assert.equal(g.best, null);
  race(g, "pk_a", 10);
  assert.ok(g.best, "the first finish is the best");
  assert.equal(g.best.t, 10);
  assert.ok(Math.abs(g.best.s.length - 10 * GHOST_HZ) <= 2, `${g.best.s.length} samples at ${GHOST_HZ} Hz`);
  const p = g.at(4.05);
  assert.ok(Math.abs(p.pos[0] - 20.25) < 0.1, `at 4.05 s: x ${p.pos[0]}`);
  assert.equal(p.kind, FOOT);
  assert.ok(p.ground);
  assert.equal(g.at(10.5), null, "after its finish: gone");
  // Another client (a reload) reads it back.
  const g2 = createGhost({ store });
  g2.update("pk_a", null, null);
  assert.equal(g2.best.t, 10);
  assert.deepEqual(g2.at(4.05), p);
  // Another map has none.
  g2.update("pk_b", null, null);
  assert.equal(g2.best, null);
});

test("only a faster run replaces the best; each race records afresh", () => {
  const store = memStore(), g = createGhost({ store });
  race(g, "pk_a", 10);
  g.update("pk_a", { live: false }, null); // between races
  race(g, "pk_a", 12, { speed: 4 }); // slower: kept the 10 s run
  assert.equal(g.best.t, 10);
  g.update("pk_a", { live: false }, null);
  race(g, "pk_a", 8, { speed: 6 }); // faster
  assert.equal(g.best.t, 8);
  assert.equal(JSON.parse(store.m.get("counterslop.ghost.pk_a")).t, 8);
  assert.ok(Math.abs(g.at(2).pos[0] - 12) < 0.1);
});

test("a race restarted without a phase between (the clock goes back) starts a new recording", () => {
  const g = createGhost({ store: memStore() });
  for (let c = 0; c < 5; c += 1 / 60) g.update("pk_a", { live: true, racing: true, clock: c }, { pos: [100 + c, 0, 0], yaw: 0, kind: FOOT });
  race(g, "pk_a", 6);
  assert.ok(Math.abs(g.at(1).pos[0] - 5) < 0.1, "the old race's samples are gone");
});

test("not racing (spectating, a bot-only race) records nothing; dead (no pose) leaves a gap", () => {
  const g = createGhost({ store: memStore() });
  for (let c = 0; c < 3; c += 1 / 60) g.update("pk_a", { live: true, racing: false, clock: c }, { pos: [c, 0, 0], yaw: 0, kind: FOOT });
  assert.equal(g.recording, 0);
  for (let c = 0; c < 3; c += 1 / 60) g.update("pk_a", { live: true, racing: true, clock: c }, c > 1 && c < 2 ? null : { pos: [c, 0, 0], yaw: 0, kind: FOOT });
  assert.ok(Math.abs(g.recording - 2 * GHOST_HZ) <= 2, `${g.recording} samples`);
});

test("poseAt: blends position and yaw (the short way round), not across a change of vehicle or a teleport", () => {
  const run = { v: GHOST_V, map: "m", t: 3, s: [[0, 0, 0, 0, 3.1, FOOT], [1, 10, 0, 0, -3.1, FOOT | GROUND], [2, 10, 1, 0, 0, CAR, "buggy"], [3, 500, 1, 0, 0, CAR, "buggy"]] };
  const a = poseAt(run, 0.5);
  assert.ok(Math.abs(a.pos[0] - 5) < 1e-9);
  assert.ok(Math.abs(Math.abs(a.yaw) - Math.PI) < 0.05, `yaw ${a.yaw}: through pi, not through 0`);
  const b = poseAt(run, 1.5);
  assert.deepEqual(b.pos, [10, 0, 0], "foot -> car: held, not blended");
  assert.ok(b.ground);
  const c = poseAt(run, 2.5);
  assert.deepEqual(c.pos, [10, 1, 0], "490 m in a second: a respawn, not blended");
  assert.equal(c.kind, CAR);
  assert.equal(c.car, "buggy");
  assert.equal(poseAt(run, -0.1), null);
  assert.equal(poseAt(null, 1), null);
});

test("parseRun refuses what isn't ours: another map, another version, junk", () => {
  const ok = { v: GHOST_V, map: "m", t: 2, s: [[0, 0, 0, 0, 0, 0], [1, 1, 0, 0, 0, 0]] };
  assert.ok(parseRun(JSON.stringify(ok), "m"));
  assert.equal(parseRun(JSON.stringify(ok), "other"), null);
  assert.equal(parseRun(JSON.stringify({ ...ok, v: 99 }), "m"), null);
  assert.equal(parseRun(JSON.stringify({ ...ok, s: [[0, "x", 0, 0, 0, 0], [1, 1, 0, 0, 0, 0]] }), "m"), null);
  assert.equal(parseRun("{not json", "m"), null);
  assert.equal(parseRun("", "m"), null);
});

test("storage that throws (a private window, blocked site data) or is missing: the ghost still works in memory", () => {
  const broken = { get: () => { throw new Error("blocked"); }, set: () => { throw new Error("blocked"); } };
  const safe = { get: (k) => { try { return broken.get(k); } catch { return null; } }, set: (k, v) => { try { return broken.set(k, v); } catch { return false; } } };
  for (const store of [safe, null]) {
    const g = createGhost({ store });
    race(g, "pk_a", 5);
    assert.equal(g.best.t, 5);
    assert.ok(g.at(1));
  }
});

test("clear forgets the map's best run, also in storage", () => {
  const store = memStore(), g = createGhost({ store });
  race(g, "pk_a", 5);
  g.clear();
  assert.equal(g.best, null);
  const g2 = createGhost({ store });
  g2.update("pk_a", null, null);
  assert.equal(g2.best, null);
});

test("ghostParts: the model it was in (on foot, a car, a boat), every part see-through and tinted", async () => {
  const { ghostParts, GHOST_ALPHA, BOAT } = await import("./ghost.js");
  const { readFileSync } = await import("node:fs");
  const spec = JSON.parse(readFileSync(new URL("../assets/cars.json", import.meta.url))).cars.find((c) => c.id === "buggy");
  for (const g of [{ pos: [1, 0, 2], yaw: 0, kind: FOOT, ground: true }, { pos: [1, 0, 2], yaw: 0, kind: CAR, car: "buggy" }, { pos: [1, 0, 2], yaw: 0, kind: BOAT }]) {
    const parts = ghostParts(g, { time: 1, carSpec: () => spec });
    assert.ok(parts.length > 3, `kind ${g.kind}: ${parts.length} parts`);
    for (const p of parts) {
      assert.equal(p.alpha, GHOST_ALPHA);
      assert.equal(p.m.length, 16);
      assert.ok(p.color.every(Number.isFinite));
    }
  }
});

test("adopt: a valid run becomes the map's best (stored); junk is refused", () => {
  const store = memStore(), g = createGhost({ store });
  g.update("pk_a", null, null);
  assert.equal(g.adopt({ v: GHOST_V, map: "pk_a", t: 2, s: [[0, 0, 0, 0, 0, 4], [2, 10, 0, 0, 0, 4]] }), true);
  assert.equal(g.best.t, 2);
  assert.ok(store.m.get("counterslop.ghost.pk_a"));
  assert.equal(g.adopt({ v: GHOST_V, map: "other", t: 2, s: [] }), false);
  assert.equal(g.best.t, 2);
});
