import { test } from "node:test";
import assert from "node:assert/strict";
import { chunkMesh, chunkRanges } from "./meshchunks.js";
import { boxMesh } from "./boxmesh.js";

// The triangles (as sorted index triples) the ranges draw.
const tris = (ch, ranges) => {
  const s = new Set();
  for (let k = 0; k < ranges.length; k += 2) for (let i = ranges[k]; i < ranges[k] + ranges[k + 1]; i += 3) s.add([ch.idx[i], ch.idx[i + 1], ch.idx[i + 2]].join(","));
  return s;
};

test("chunkMesh: near a point, only the nearby boxes' triangles; big ones everywhere they reach", () => {
  const boxes = [
    { min: [-50, -1, -50], max: [50, 0, 50], kind: "a" }, // floor: in every column
    { min: [1, 0, 1], max: [2, 1, 2], kind: "a" },        // near the light
    { min: [30, 0, 30], max: [31, 1, 31], kind: "a" },    // far
  ];
  const m = boxMesh(boxes, () => 0, -Infinity);
  const ch = chunkMesh(m.pos, m.idx, 4);
  assert.equal(ch.nx * ch.nz, ch.count.length);
  const near = tris(ch, chunkRanges(ch, [1.5, 1, 1.5], 6));
  const all = tris(ch, [0, ch.idx.length]);
  assert.equal(all.size, m.idx.length / 3, "every triangle is in some column");
  const box = (b) => { const o = boxMesh([boxes[b]], () => 0, -Infinity), base = b * 24; return [...Array(o.idx.length / 3)].map((_, t) => [0, 1, 2].map((k) => o.idx[t * 3 + k] + base).join(",")); };
  const reaches = (t) => { const v = t.split(",").map((i) => [m.pos[i * 3], m.pos[i * 3 + 2]]); return Math.min(...v.map((q) => q[0])) <= 7.5 && Math.max(...v.map((q) => q[0])) >= -4.5 && Math.min(...v.map((q) => q[1])) <= 7.5 && Math.max(...v.map((q) => q[1])) >= -4.5; };
  const floor = box(0).filter(reaches);
  assert.ok(floor.length >= 2);
  for (const t of floor) assert.ok(near.has(t), "the floor's top is drawn");
  assert.ok(!box(0).every((t) => near.has(t)), "not its far sides");
  for (const t of box(1)) assert.ok(near.has(t), "the near box is drawn");
  for (const t of box(2)) assert.ok(!near.has(t), "the far box is not");
  // A sphere above everything's height reaches nothing.
  assert.equal(chunkRanges(ch, [1.5, 20, 1.5], 6).length, 0);
});

test("chunkRanges: adjacent columns in a row merge into one draw", () => {
  const boxes = [];
  for (let x = 0; x < 40; x += 2) for (let z = 0; z < 40; z += 2) boxes.push({ min: [x, 0, z], max: [x + 1, 1, z + 1] });
  const m = boxMesh(boxes, () => 0, -Infinity);
  const ch = chunkMesh(m.pos, m.idx, 4);
  const r = chunkRanges(ch, [20, 0.5, 20], 7.5);
  assert.ok(r.length / 2 <= 5, `one range per row touched: ${r.length / 2}`);
  // Compared with the whole map, a fraction of the indices.
  let n = 0;
  for (let k = 1; k < r.length; k += 2) n += r[k];
  assert.ok(n < m.idx.length * 0.3, `${n} of ${m.idx.length}`);
});
