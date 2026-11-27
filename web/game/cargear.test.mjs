import test from "node:test";
import assert from "node:assert/strict";
import { createCarGear, gearParts, beamParts, boomParts, wreckParts, flashParts } from "./cargear.js";
import { CAR_ITEMS } from "./cars.js";

test("every car item has a look; a beam and a blast fade out", () => {
  for (const it of CAR_ITEMS) {
    if (it.kind === "beam" || it.kind === "ram") continue; // no gear: the beam is an event, the plate passive
    const parts = gearParts({ id: 1, k: it.id, p: [0, 0, 0], v: [1, 0, 0], a: true }, [0, 0, 0], 0.5);
    assert.ok(parts.length > 0 && parts.every((x) => x.m.length === 16 && x.color), it.id);
  }
  assert.equal(beamParts([0, 1, 0], [10, 1, 0], 0.01).length, 1);
  assert.equal(beamParts([0, 1, 0], [10, 1, 0], 1).length, 0);
  assert.ok(boomParts([0, 0, 0], 3, 0.05).length > 5);
  assert.equal(boomParts([0, 0, 0], 3, 0.9).length, 0);
});

test("the snapshot's gear drawn, run ahead by its velocity; events become effects that age out; sounds by event", () => {
  const heard = [];
  const rec = (n) => (...a) => heard.push([n, ...a.slice(1)].join(":"));
  const g = createCarGear({ sfx: { carBoom: rec("boom"), carWreck: rec("wreck"), carBeam: rec("beam"), carRocket: rec("rocket"), carOrb: rec("orb"), carDrop: rec("drop"), carRam: rec("ram"), carSkid: rec("skid") }, spatial: (p) => p });
  g.ingest([{ id: 3, k: "hotfix_missiles", p: [0, 1, 0], v: [-60, 0, 0], a: true }], 1000);
  assert.equal(g.list.length, 1);
  const s0 = g.solids(0, 1000)[0].m, s1 = g.solids(0, 1100)[0].m;
  assert.ok(Math.abs(s1[12] - s0[12] + 6) < 1e-9, "0.1 s later: 6 m further on (-x)");
  g.event({ t: "car", ev: "beam", p: [0, 1, 0], to: [20, 1, 0] });
  g.event({ t: "car", ev: "boom", p: [5, 0, 0], r: 3 });
  g.event({ t: "car", ev: "wreck", p: [5, 0, 0] });
  assert.equal(g.fxCount, 3);
  assert.deepEqual(heard, ["beam", "boom", "wreck"]);
  for (const k of ["hotfix_missiles", "rollback_rockets", "synergy_orbs", "breaking_changes", "spaghetti_code", "tech_debt"]) g.event({ t: "car", ev: "fire", k, p: [0, 0, 0] });
  g.event({ t: "car", ev: "ram", p: [0, 0, 0] });
  g.event({ t: "car", ev: "slip", p: [0, 0, 0] });
  assert.deepEqual(heard.slice(3), ["rocket:false", "rocket:true", "orb", "drop:spikes", "drop:oil", "drop:mine", "ram", "skid"]);
  assert.equal(g.fxCount, 6, "two rocket flashes, a ram spark");
  assert.ok(g.solids(0, 1000).every((p) => p.m.every(Number.isFinite)));
  g.update(0.2);
  assert.equal(g.fxCount, 2, "the beam and the flashes are gone");
  g.update(2);
  assert.equal(g.fxCount, 0);
  g.ingest(undefined, 2000);
  assert.equal(g.solids(0, 2000).length, 0);
});

test("a wreck's explosion: fireball, shockwave, flying bits, smoke; all gone by its end", () => {
  const early = wreckParts([0, 0, 0], 0.05), mid = wreckParts([0, 0, 0], 0.4), late = wreckParts([0, 0, 0], 1.5);
  assert.ok(early.some((p) => p.emit === 1), "white-hot");
  assert.ok(mid.length > 10 && late.length > 0);
  for (const p of [...early, ...mid, ...late, ...flashParts([1, 2, 3], 0.8, 0.05)]) assert.ok(p.m.every(Number.isFinite));
  assert.equal(flashParts([0, 0, 0], 1, 0.2).length, 0);
});
