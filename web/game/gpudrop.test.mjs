import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { inReach, gpuHud, gpuSolids, endOf, exitOutlines, createGpuDrop } from "./gpudrop.js";
import { GPU_BOX } from "../assets/models/gpu.js";

const vectors = JSON.parse(readFileSync(new URL("../../internal/content/testdata/grab_reach.json", import.meta.url)));

test("grab reach matches content.InGrabReach (shared vectors)", () => {
  for (const c of vectors.cases) assert.equal(inReach(c.feet, c.gpu), c.reach, `feet ${c.feet} -> gpu ${c.gpu}`);
});

test("the model's box is content.GPUSize", () => {
  const src = readFileSync(new URL("../../internal/content/modes.go", import.meta.url), "utf8");
  const m = /GPUSize = \[3\]float64\{([\d.]+), ([\d.]+), ([\d.]+)\}/.exec(src);
  assert.ok(m, "GPUSize in modes.go");
  assert.deepEqual([+m[1], +m[2], +m[3]], [GPU_BOX.w, GPU_BOX.h, GPU_BOX.d]);
});

const map = { exits: [{ name: "entrance", min: [-24, 0, -4], max: [-20, 3, 4] }, { name: "dock", min: [0, 0, 20], max: [8, 3, 24] }],
  gpus: [{ pos: [20.35, 0.9, -5], yaw: 1.57 }, { pos: [20.35, 0.9, -3], yaw: 1.57 }] };
const round = (gpus, extra = {}) => ({ mode: "gpudrop", ph: "live", n: 1, secured: 0, toWin: 2, gpus, ...extra });
const shelf = (z) => ({ st: "shelf", p: [20.35, 0.9, z] });
const me = (team, pos = [0, 0, 0], extra = {}) => ({ id: 1, team, pos, dead: false, ...extra });

test("HUD: the counter, and nothing in defusal or warmup", () => {
  const h = gpuHud(round([shelf(-5), { st: "carried", by: 2, p: [0, 0, 0] }, { st: "secured", by: 2, p: [0, 0, 0] }], { secured: 1 }), me("vibe"), map);
  assert.deepEqual(h.gpu, { secured: 1, toWin: 2, stock: 1, loose: 1 });
  assert.equal(gpuHud({ ph: "live", bomb: {} }, me("vibe"), map).gpu, null);
  assert.equal(gpuHud(round([], { ph: "warmup" }), me("vibe"), map).gpu, null);
});

test("HUD hints: grab in reach (scalpers only), carry to an exit, restock a dropped one (staff)", () => {
  const st = round([shelf(-5), shelf(-3)]);
  assert.equal(gpuHud(st, me("vibe", [19.45, 0, -3]), map).hint, "Hold E to grab the RTX 9090");
  assert.equal(gpuHud(st, me("vibe", [16, 0, -3]), map).hint, "");
  assert.equal(gpuHud(st, me("artisan", [19.45, 0, -3]), map).hint, "", "the staff don't grab from their own shelves");
  const carry = round([{ st: "carried", by: 1, p: [0, 0, 0] }, shelf(-3)]);
  const h = gpuHud(carry, me("vibe"), map);
  assert.equal(h.carrying, true);
  assert.match(h.hint, /get it to an exit \(the entrance or the dock\)/);
  const dropped = round([{ st: "dropped", p: [4, 0, 0] }, shelf(-3)]);
  assert.equal(gpuHud(dropped, me("artisan", [4.5, 0, 0]), map).hint, "Hold E to restock it");
  assert.equal(gpuHud(dropped, me("vibe", [4.5, 0, 0]), map).hint, "Hold E to grab the RTX 9090");
  assert.equal(gpuHud(dropped, me("artisan", [4.5, 0, 0], { dead: true }), map).hint, "");
  assert.equal(gpuHud(round([shelf(-3)], { ph: "freeze" }), me("vibe", [19.45, 0, -3]), map).hint, "", "no grabbing in freeze time");
});

test("HUD work bar: yours, a team-mate's (by id), never the other side's", () => {
  const st = round([{ ...shelf(-5), who: 1, pr: 0.4 }, shelf(-3)]);
  assert.deepEqual(gpuHud(st, me("vibe", [19.45, 0, -5]), map).work, { kind: "grab", pr: 0.4 });
  const teamOf = (id) => ({ 1: "artisan", 2: "artisan", 3: "vibe" })[id];
  const restock = round([{ st: "dropped", p: [4, 0, 0], who: 2, pr: 0.5 }]);
  assert.deepEqual(gpuHud(restock, me("artisan"), map, teamOf).work, { kind: "restock", pr: 0.5, other: 2 });
  const enemy = round([{ ...shelf(-5), who: 3, pr: 0.5 }]);
  assert.equal(gpuHud(enemy, me("artisan"), map, teamOf).work, null);
});

test("round end lines and text by winner and reason", () => {
  assert.deepEqual(endOf("vibe", "secured"), { line: "scalpers_win", text: "Sold out: the GPUs left the store" });
  assert.equal(endOf("artisan", "time").line, "staff_wins");
  assert.equal(endOf("artisan", "weird").text, "");
});

test("solids: shelf and dropped boxes, carriers' backs (not yours in first person), exits outlined while you carry", () => {
  const players = [{ id: 1, p: [0, 0, 0], yaw: 0 }, { id: 2, p: [5, 0, 5], yaw: 1 }];
  const none = gpuSolids(round([shelf(-5)]), players, 0, 0, map, 1).length;
  assert.ok(none > 0, "a GPU on the shelf is drawn");
  assert.equal(gpuSolids(round([{ st: "secured", p: [0, 0, 0] }]), players, 0, 0, map, 1).length, 0);
  assert.equal(gpuSolids({ ph: "live", bomb: {} }, players, 0, 0, map, 1).length, 0, "defusal: nothing");
  const back = gpuSolids(round([{ st: "carried", by: 2, p: [5, 0, 5] }]), players, 0, 0, map, 1).length;
  assert.ok(back > 0, "a carrier's back");
  const mine = round([{ st: "carried", by: 1, p: [0, 0, 0] }]);
  assert.equal(gpuSolids(mine, players, 0, 1, map, 1).length, exitOutlines(map, 0).length, "first person: no back, the exits outlined");
  assert.equal(exitOutlines(map, 0).length, 8);
});

test("lines: grab, restock, secure (+ stinger) from events; Out of stock! from the state, once", () => {
  const said = [], fed = [], snd = [];
  let stingers = 0;
  const gd = createGpuDrop({
    sfx: { gpuGrab: () => snd.push("grab"), gpuRestock: () => snd.push("restock"), gpuSecure: () => snd.push("secure") },
    say: (id) => said.push(id), spatial: (p) => p, feed: (t) => fed.push(t), nameOf: (id) => `#${id}`, me: () => ({ id: 1 }),
    music: { secured: () => stingers++ },
  });
  gd.event({ ev: "grab", by: 2, p: [0, 0, 0] });
  gd.event({ ev: "secure", by: 1, p: [0, 0, 0] });
  gd.event({ ev: "restock", by: 3, p: [0, 0, 0] });
  gd.event({ ev: "drop", p: [0, 0, 0] });
  assert.deepEqual(said, ["gpu_grabbed", "gpu_secured", "restocked"]);
  assert.deepEqual(snd, ["grab", "secure", "restock"]);
  assert.equal(stingers, 1);
  assert.deepEqual(fed, ["#2 grabbed an RTX 9090.", "You secured an RTX 9090.", "#3 restocked an RTX 9090.", "An RTX 9090 is on the floor!"]);
  said.length = 0;
  const two = round([shelf(-5), { st: "carried", by: 2 }]);
  const none = round([{ st: "carried", by: 3 }, { st: "carried", by: 2 }]);
  gd.ingest(two); gd.ingest(none); gd.ingest(none);
  assert.deepEqual(said, ["out_of_stock"], "once, when the last one leaves the shelf");
  gd.ingest(round([{ st: "dropped" }], { n: 2 })); // a new round that starts empty (none on a shelf): no line
  assert.deepEqual(said, ["out_of_stock"]);
});
