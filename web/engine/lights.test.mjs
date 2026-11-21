import { test } from "node:test";
import assert from "node:assert/strict";
import {
  buildGrid, skyVisibility, floodLight, buildLightLists, lightsAt, packLights, falloff, cellOf, cellIndex,
  buildZones, reachZones, activeLights, sphereInFrustum, shadowPick, importance, MAX_PER_CELL, CELL,
  cubeFace, shadowViews, atlasLayout, tileXY, dynamicScore, pickDynamic, allocTiles, fadeToward, contactBlobs,
} from "./lights.js";
import { perspective, mul, fpsView } from "./math.js";

const box = (x0, y0, z0, x1, y1, z1) => ({ min: [x0, y0, z0], max: [x1, y1, z1], kind: "concrete" });
const at = (g, p) => cellIndex(g, ...cellOf(g, p));

// Two rooms, 0..10 and 10.1..20 along x, split by a thin wall at x 10..10.1 (thinner
// than a cell, between two cell centres), with a door at z 4..5 when door is set.
// A roof at y 4 over both, with a skylight hole over 15..17 x 2..4 in the east room.
function rooms({ door = false, skylight = false } = {}) {
  const b = [box(-1, -1, -1, 21, 0, 11)]; // floor
  b.push(box(-1, 0, -1, 0, 4, 11), box(20, 0, -1, 21, 4, 11), box(0, 0, -1, 20, 4, 0), box(0, 0, 10, 20, 4, 11)); // outer walls
  if (door) b.push(box(10, 0, 0, 10.1, 4, 4), box(10, 0, 5, 10.1, 4, 10), box(10, 2.5, 4, 10.1, 4, 5));
  else b.push(box(10, 0, 0, 10.1, 4, 10));
  if (skylight) b.push(box(-1, 4, -1, 15, 4.3, 11), box(17, 4, -1, 21, 4.3, 11), box(15, 4, -1, 17, 4.3, 2), box(15, 4, 4, 17, 4.3, 11));
  else b.push(box(-1, 4, -1, 21, 4.3, 11));
  return b;
}

test("a thin wall blocks the faces across it, though no cell centre is inside it", () => {
  const g = buildGrid(rooms());
  const w = at(g, [9.9, 1, 5]), e = at(g, [10.3, 1, 5]);
  assert.equal(g.solid[w], 0);
  assert.equal(g.solid[e], 0);
  assert.equal(e - w, 1, "neighbours along x");
  assert.ok(g.blk[w] & 1, "the +x face of the west cell is blocked");
  assert.ok(!(g.blk[at(g, [5, 1, 5])] & 1), "open floor is not");
  assert.equal(g.solid[at(g, [5, -0.5, 5])], 1, "the floor slab is solid");
});

test("a light floods its room and stops at a wall; through a door it spills round", () => {
  const light = { kind: "point", pos: [8, 2, 5], color: [1, 1, 1], intensity: 1, range: 6 };
  for (const door of [false, true]) {
    const g = buildGrid(rooms({ door }));
    const cells = new Set(floodLight(g, light));
    assert.ok(cells.has(at(g, [8, 1, 5])), "its own room");
    assert.ok(cells.has(at(g, [3, 1, 5])), "5 m away in the room");
    assert.ok(!cells.has(at(g, [1, 1, 5])), "beyond its range");
    assert.equal(cells.has(at(g, [11, 1, 4.6])), door, "just through the doorway");
    if (!door) assert.ok(!cells.has(at(g, [11, 1, 8.5])), "the next room behind the wall");
  }
});

test("light does not go over a wall into the next yard (the path is a long detour)", () => {
  const g = buildGrid([box(-1, -1, -1, 21, 0, 11), box(10, 0, -1, 10.1, 4, 11)]);
  const cells = new Set(floodLight(g, { kind: "point", pos: [13, 1, 5], intensity: 1, range: 6 }));
  assert.ok(cells.has(at(g, [11, 0.3, 5])));
  assert.ok(cells.has(at(g, [10, 4.6, 5])), "above the wall top");
  assert.ok(!cells.has(at(g, [9.6, 0.3, 5])), "the floor behind it");
});

test("a ceiling light inside the slab's cell starts from the open cell below it", () => {
  const g = buildGrid(rooms());
  const cells = new Set(floodLight(g, { kind: "point", pos: [5, 4.1, 5], intensity: 1, range: 3 }));
  assert.ok(cells.has(at(g, [5, 3.5, 5])), "under the ceiling");
  assert.ok(!cells.has(at(g, [5, 5, 5])), "not the roof above");
});

test("a spot only reaches its cone", () => {
  const g = buildGrid(rooms());
  const s = { kind: "spot", pos: [5, 3.5, 5], dir: [0, -1, 0], cone: 20, intensity: 2, range: 6 };
  const cells = new Set(floodLight(g, s));
  assert.ok(cells.has(at(g, [5, 0.3, 5])), "the floor under it");
  assert.ok(cells.has(at(g, [5.8, 0.3, 5])), "the edge of its pool");
  assert.ok(!cells.has(at(g, [8, 0.3, 5])), "outside the cone");
});

test("sky: open ground sees the sky, a roofed room is dark, a skylight lights the floor under it", () => {
  const b = rooms({ skylight: true });
  const g = buildGrid(b);
  const v = skyVisibility(g, b);
  const outside = buildGrid([box(-5, -1, -5, 5, 0, 5)]);
  assert.equal(skyVisibility(outside, [box(-5, -1, -5, 5, 0, 5)])[at(outside, [0, 0.3, 0])], 255);
  assert.equal(v[at(g, [5, 0.3, 5])], 0, "the closed west room");
  assert.equal(v[at(g, [16, 0.3, 3])], 255, "under the skylight");
  const near = v[at(g, [14, 0.3, 3])], far = v[at(g, [12, 0.3, 8])];
  assert.ok(near > 50 && near < 255, `next to the skylight: ${near}`);
  assert.ok(far < near, `farther in: ${far} < ${near}`);
});

test("a thin wall cell takes no side: the floor next to it does not light from the far room", () => {
  // A wall at x 10.2..10.3 has the centre 10.25 inside it: a solid cell between two open ones.
  const b = [box(-1, -1, -1, 21, 0, 11), box(10.2, 0, -1, 10.3, 4, 11)];
  const g = buildGrid(b);
  const wall = at(g, [10.25, 1, 5]);
  assert.equal(g.solid[wall], 1);
  const lists = buildLightLists(g, [
    { kind: "point", pos: [8, 1, 5], intensity: 1, range: 6 },
    { kind: "point", pos: [13, 1, 5], intensity: 1, range: 6 },
  ]);
  assert.deepEqual(lightsAt(lists, at(g, [9.9, 1, 5])), [0]);
  assert.deepEqual(lightsAt(lists, at(g, [10.6, 1, 5])), [1]);
  assert.deepEqual(lightsAt(lists, wall), []);
  // A crate in the open: its cells take the light round it.
  const b2 = [box(-1, -1, -1, 21, 0, 11), box(4, 0, 4, 5, 1, 5)];
  const g2 = buildGrid(b2);
  const l2 = buildLightLists(g2, [{ kind: "point", pos: [3, 2, 3], intensity: 1, range: 6 }]);
  assert.deepEqual(lightsAt(l2, at(g2, [4.5, 0.5, 4.5])), [0]);
});

test("light lists keep the strongest MAX_PER_CELL per cell, in index order", () => {
  const g = buildGrid([box(-10, -1, -10, 10, 0, 10)]);
  const lights = [];
  for (let i = 0; i < 24; i++) lights.push({ kind: "point", pos: [i * 0.1, 1, 0], intensity: i < 4 ? 0.1 : 1, range: 8 });
  const lists = buildLightLists(g, lights);
  const here = lightsAt(lists, at(g, [0, 1, 0]));
  assert.equal(here.length, MAX_PER_CELL);
  assert.ok(here.every((i) => i >= 4), "the weak ones dropped");
  assert.deepEqual(here, [...here].sort((a, b) => a - b));
  assert.equal(lists.stats.maxCount, MAX_PER_CELL);
  assert.ok(lists.stats.capped > 0);
});

test("falloff: 1 at the source, inverse square, zero at the range", () => {
  assert.equal(falloff(0, 5), 1);
  assert.equal(falloff(5, 5), 0);
  assert.equal(falloff(6, 5), 0);
  assert.ok(Math.abs(falloff(1, 100) - 0.5) < 1e-3);
  for (let d = 0; d < 5; d += 0.25) assert.ok(falloff(d + 0.25, 5) < falloff(d, 5));
});

test("packLights: 16 floats per light, points marked with cos(outer) -2", () => {
  const d = packLights([
    { kind: "point", pos: [1, 2, 3], color: [1, 0.5, 0], intensity: 2, range: 7, flicker: 12 },
    { kind: "spot", pos: [0, 3, 0], dir: [0, -2, 0], color: [1, 1, 1], intensity: 1, range: 5, cone: 60, inner: 30 },
  ]);
  assert.equal(d.length, 32);
  assert.deepEqual([...d.subarray(0, 8)], [1, 2, 3, 7, 2, 1, 0, 12]);
  assert.equal(d[11], -2);
  assert.deepEqual([...d.subarray(24, 27)], [0, -1, 0]);
  assert.ok(Math.abs(d[27] - 0.5) < 1e-6);
  assert.ok(Math.abs(d[28] - Math.cos(Math.PI / 6)) < 1e-6);
  assert.equal(d[29], -1, "no shadow slot");
});

test("zones: lights behind more openings than the walk allows drop out", () => {
  // A corridor 0..60 along x, 2 m wide, walls each side, a light every 10 m.
  const b = [box(-1, -1, -1, 61, 0, 3), box(-1, 0, -1, 61, 3, 0), box(-1, 0, 2, 61, 3, 3), box(-1, 3, -1, 61, 3.3, 3)];
  const g = buildGrid(b);
  const lights = [0, 1, 2, 3, 4, 5].map((i) => ({ kind: "point", pos: [i * 10 + 2, 2, 1], intensity: 1, range: 3 }));
  const lists = buildLightLists(g, lights);
  const z = buildZones(g, lists.perLight);
  assert.deepEqual(activeLights(g, z, lights, [2, 1.6, 1], null, 3), [0, 1]);
  assert.deepEqual(activeLights(g, z, lights, [2, 1.6, 1], null, 20), [0, 1, 2, 3, 4, 5]);
  const s = reachZones(g, z, [2, 1.6, 1], 3);
  assert.equal(reachZones(g, z, [2.2, 1.6, 1], 3), s, "same zone: cached");
  assert.equal(reachZones(g, z, [500, 1, 1], 3), -1, "outside the grid: everything");
  assert.deepEqual(activeLights(g, z, lights, [500, 1, 1], null, 3), [0, 1, 2, 3, 4, 5]);
});

test("frustum: a light behind the camera is culled unless the camera is in its range", () => {
  const vp = mul(perspective(1.3, 16 / 9, 0.05, 400), fpsView([0, 1.6, 0], 0, 0));
  // Find which way the camera looks: the sphere in front is visible, the opposite one not.
  const a = sphereInFrustum(vp, [0, 1.6, -10], 1), b = sphereInFrustum(vp, [0, 1.6, 10], 1);
  assert.notEqual(a, b);
  const front = a ? -10 : 10;
  assert.ok(sphereInFrustum(vp, [0, 1.6, front * 3], 0.5));
  assert.ok(!sphereInFrustum(vp, [0, 1.6, -front], 0.5));
  assert.ok(sphereInFrustum(vp, [0, 1.6, -front], 12), "big enough to reach into view");
  assert.ok(!sphereInFrustum(vp, [front * 5, 1.6, front], 1), "far off to the side");
});

test("shadow budget: shadowed lights only, by importance, capped", () => {
  const lights = [
    { pos: [0, 2, 0], intensity: 1, range: 5, shadow: true },
    { pos: [20, 2, 0], intensity: 1, range: 5, shadow: true },
    { pos: [3, 2, 0], intensity: 3, range: 5, shadow: true },
    { pos: [1, 2, 0], intensity: 5, range: 5 },
  ];
  const eye = [0, 1.6, 0];
  assert.ok(importance(lights[0], eye) > importance(lights[1], eye));
  assert.deepEqual(shadowPick(lights, [0, 1, 2, 3], eye, 2), [2, 0]);
  assert.deepEqual(shadowPick(lights, [1, 2, 3], eye, 4), [2, 1]);
  assert.deepEqual(shadowPick(lights, [0, 1, 2, 3], eye, 0), []);
});

test("grid size on a 150 x 120 m map stays reasonable", () => {
  const g = buildGrid([box(-75, -1, -61, 75, 11, 61)]);
  assert.ok(g.n < 4e6, `${g.n} cells`);
  assert.equal(g.cell, CELL);
});

// Project p with a column-major vp; returns NDC xyz or null behind.
function ndc(vp, p) {
  const o = [0, 1, 2, 3].map((r) => vp[r] * p[0] + vp[4 + r] * p[1] + vp[8 + r] * p[2] + vp[12 + r]);
  return o[3] <= 0 ? null : [o[0] / o[3], o[1] / o[3], o[2] / o[3]];
}

test("point shadows: the cube face picked for a direction sees it, depth grows with distance", () => {
  const l = { kind: "point", pos: [1, 2, 3], range: 10, intensity: 1 };
  const views = shadowViews(l, 512);
  assert.equal(views.length, 6);
  let seed = 1;
  const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647) * 2 - 1;
  for (let i = 0; i < 500; i++) {
    const v = [rnd(), rnd(), rnd()];
    const d = 0.5 + Math.abs(rnd()) * 9;
    const len = Math.hypot(...v);
    const p = v.map((x, k) => l.pos[k] + (x / len) * d);
    const c = ndc(views[cubeFace(v)].vp, p);
    assert.ok(c && c.every((x) => x >= -1 && x <= 1), `dir ${v} d ${d}: ${c}`);
    // Inside the 90 degree core, with room for PCF taps up to the 95 degree edge.
    assert.ok(Math.abs(c[0]) <= 0.92 && Math.abs(c[1]) <= 0.92);
    const c2 = ndc(views[cubeFace(v)].vp, v.map((x, k) => l.pos[k] + (x / len) * (d * 0.9)));
    assert.ok(c2[2] < c[2], "nearer is shallower");
  }
});

test("spot shadows: one view covering the cone", () => {
  const l = { kind: "spot", pos: [0, 4, 0], dir: [1, -1, 0], cone: 30, range: 10, intensity: 1 };
  const [v, more] = shadowViews(l, 512);
  assert.equal(more, undefined);
  const c = ndc(v.vp, [3, 1, 0]);
  assert.ok(Math.abs(c[0]) < 1e-5 && Math.abs(c[1]) < 1e-5, "the axis maps to the centre");
  // A point on the cone's edge is inside the view, one well outside it is not.
  const a = (29 * Math.PI) / 180, axis = [Math.SQRT1_2, -Math.SQRT1_2, 0], side = [Math.SQRT1_2, Math.SQRT1_2, 0];
  const edge = [0, 1, 2].map((k) => l.pos[k] + 5 * (Math.cos(a) * axis[k] + Math.sin(a) * side[k]));
  assert.ok(ndc(v.vp, edge).every((x) => Math.abs(x) <= 1));
  assert.ok(ndc(v.vp, [0, 1, 0]).some((x) => Math.abs(x) > 1), "straight down is outside a 30 degree cone round 45 degrees");
  assert.ok(v.texel > 0 && v.texel < 0.01);
});

test("static atlas: every shadowed light gets tiles, the tile size shrinks to fit", () => {
  const spot = { kind: "spot", shadow: true }, point = { kind: "point", shadow: true }, off = { kind: "spot" };
  const a = atlasLayout([spot, off, point, spot]);
  assert.deepEqual([...a.base], [0, -1, 1, 7]);
  assert.equal(a.tiles, 8);
  assert.equal(a.tile, 512);
  assert.deepEqual([a.cols, a.rows, a.width, a.height], [8, 1, 4096, 512]);
  const many = atlasLayout(Array(93).fill(spot));
  assert.equal(many.tile, 256, "93 tiles do not fit 64 at 512");
  assert.deepEqual([many.cols, many.rows], [16, 6]);
  const tooMany = atlasLayout(Array(1030).fill(spot));
  assert.equal(tooMany.tile, 128);
  assert.equal(tooMany.base[1023], 1023);
  assert.equal(tooMany.base[1024], -1, "beyond 32 x 32 tiles: no shadow");
  assert.deepEqual(tileXY(17, 16), [1, 1]);
  assert.deepEqual(atlasLayout([off]).tiles, 0);
});

test("live shadows: scored by players in range, near the camera and on screen; sticky; tiles and fades", () => {
  const l = { kind: "spot", pos: [0, 4, 0], intensity: 10, range: 8, shadow: true };
  const eye = [0, 1.6, 6], all = () => true, none = () => false;
  assert.equal(dynamicScore(l, [], eye, all), 0);
  assert.equal(dynamicScore(l, [[20, 0, 0]], eye, all), 0, "out of range");
  const near = dynamicScore(l, [[1, 0, 0]], eye, all);
  assert.ok(near > 0);
  assert.ok(dynamicScore(l, [[1, 0, 0]], eye, none) < near, "off screen counts less");
  assert.ok(dynamicScore(l, [[1, 0, 0]], [0, 1.6, 40], all) < near, "far from the camera counts less");
  assert.ok(dynamicScore(l, [[1, 0, 0], [-1, 0, 0]], eye, all) > near, "two players more than one");
  assert.deepEqual(pickDynamic([0, 2, 5, 1], 2, new Set()), [2, 1]);
  assert.deepEqual(pickDynamic([0, 2, 2.2, 1], 1, new Set([1])), [1], "held");
  assert.deepEqual(pickDynamic([0, 0], 3, new Set()), [], "nobody near: no live shadows");
  const used = new Uint8Array(8);
  assert.equal(allocTiles(used, 6), 0);
  assert.equal(allocTiles(used, 6), -1);
  assert.equal(allocTiles(used, 1), 6);
  used.fill(0, 0, 6);
  assert.equal(allocTiles(used, 2), 0);
  assert.equal(fadeToward(0, 1, 0.1, 0.3).toFixed(3), "0.333");
  assert.equal(fadeToward(0.9, 1, 0.1, 0.3), 1);
  assert.equal(fadeToward(0.1, 0, 0.1, 0.3), 0);
});

test("contact shadows: the players nearest the eye, feet and radius", () => {
  const b = contactBlobs([[10, 0, 0], [1, 0, 0], [5, 2, 0]], [0, 1.6, 0], 2);
  assert.equal(b.n, 2);
  assert.deepEqual([...b.data].map((v) => +v.toFixed(3)), [1, 0, 0, 0.8, 5, 2, 0, 0.8]);
  assert.equal(contactBlobs([], [0, 0, 0], 8).n, 0);
});
