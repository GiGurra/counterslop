import test from "node:test";
import assert from "node:assert/strict";
import { createCarGear, gearParts, beamParts, boomParts } from "./cargear.js";
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
  const g = createCarGear({ sfx: { heBlast: () => heard.push("boom"), carBeam: () => heard.push("beam") }, spatial: (p) => p });
  g.ingest([{ id: 3, k: "hotfix_missiles", p: [0, 1, 0], v: [-60, 0, 0], a: true }], 1000);
  assert.equal(g.list.length, 1);
  const s0 = g.solids(0, 1000)[0].m, s1 = g.solids(0, 1100)[0].m;
  assert.ok(Math.abs(s1[12] - s0[12] + 6) < 1e-9, "0.1 s later: 6 m further on (-x)");
  g.event({ t: "car", ev: "beam", p: [0, 1, 0], to: [20, 1, 0] });
  g.event({ t: "car", ev: "boom", p: [5, 0, 0], r: 3 });
  g.event({ t: "car", ev: "wreck", p: [5, 0, 0] });
  assert.equal(g.fxCount, 3);
  assert.deepEqual(heard, ["beam", "boom", "boom"]);
  g.update(0.2);
  assert.equal(g.fxCount, 2, "the beam is gone");
  g.update(2);
  assert.equal(g.fxCount, 0);
  g.ingest(undefined, 2000);
  assert.equal(g.solids(0, 2000).length, 0);
});
