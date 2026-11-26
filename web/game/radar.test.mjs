import { test } from "node:test";
import assert from "node:assert/strict";
import { radarLayout, radarShade, toRadar } from "./radar.js";

const near = (a, b) => a.forEach((v, i) => assert.ok(Math.abs(v - b[i]) < 1e-9, `${a} vs ${b}`));

test("layout: the boxes' footprint, drawn low to high; nothing below the floor", () => {
  const l = radarLayout({ boxes: [
    { min: [-10, 0, -5], max: [-8, 3, 5] }, { min: [-20, -1, -20], max: [20, 0, 20] }, { min: [0, -9, 0], max: [1, -2, 1] },
  ] });
  assert.deepEqual([l.x0, l.z0, l.x1, l.z1], [-20, -20, 20, 20]);
  assert.deepEqual(l.order.map((b) => b.max[1]), [0, 3]);
  assert.equal(radarLayout({ boxes: [] }), null);
});

test("shade: the floor is dark, walls get lighter with height", () => {
  assert.ok(radarShade(0) < radarShade(1) && radarShade(1) < radarShade(3));
  assert.ok(radarShade(50) <= 0.85);
});

test("toRadar: north up without rotation; rotating puts where you look at the top", () => {
  const me = [5, 0, 5];
  near(toRadar(5, -5, me, 0, false, 2, 100, 100), [100, 80]); // 10 m north (-z) is 20 px up
  // Looking west (yaw π/2, forward = -x): a point 10 m west is straight up.
  near(toRadar(-5, 5, me, Math.PI / 2, true, 2, 100, 100), [100, 80]);
  // ... and one 10 m north is to the right.
  near(toRadar(5, -5, me, Math.PI / 2, true, 2, 100, 100), [120, 100]);
});

test("layout: a ceiling over the whole map is left out; boxes overhead are kept apart, drawn faint", () => {
  const floor = { min: [-20, -1, -20], max: [20, 0, 20] }, wall = { min: [-10, 0, -5], max: [-8, 3, 5] };
  const ceiling = { min: [-20, 16, -20], max: [20, 17, 20] }, hanging = { min: [2, 10, 2], max: [4, 16, 4] };
  const l = radarLayout({ boxes: [floor, wall, ceiling, hanging] });
  assert.deepEqual([l.x0, l.z0, l.x1, l.z1], [-20, -20, 20, 20]);
  assert.deepEqual(l.order, [floor, wall]);
  assert.deepEqual(l.over, [hanging]);
});
