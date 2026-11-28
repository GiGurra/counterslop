import { test } from "node:test";
import assert from "node:assert/strict";
import { FONT_H, FONT_W, GLYPHS, glyph, glyphRects } from "../font.js";
import { layout, letterBoxes, signBoxes, signSolids, signText } from "./signs.js";

test("every glyph is 7 rows of 5 dots and its rectangles cover exactly its lit dots", () => {
  for (const ch of GLYPHS) {
    const rows = glyph(ch);
    assert.equal(rows.length, FONT_H, ch);
    for (const r of rows) assert.equal(r.length, FONT_W, ch);
    const lit = new Set(), cover = [];
    rows.forEach((r, y) => [...r].forEach((d, x) => d === "#" && lit.add(`${x},${y}`)));
    for (const { x, y, w, h } of glyphRects(ch)) for (let yy = y; yy < y + h; yy++) for (let xx = x; xx < x + w; xx++) cover.push(`${xx},${yy}`);
    assert.equal(cover.length, new Set(cover).size, `${ch}: rectangles overlap`);
    assert.deepEqual(new Set(cover), lit, `${ch}: rectangles cover the lit dots`);
  }
  assert.deepEqual(glyph("~"), glyph("?"));
  assert.deepEqual(glyph("a"), glyph("A"));
});

test("a sign's letters: centred on its position, out of its surface towards its reader", () => {
  const s = { text: "HH", pos: [10, 2, 5], face: "+x", size: 0.7, mat: "led" };
  const boxes = letterBoxes(s);
  assert.ok(boxes.length > 0 && boxes.every((b) => b.kind === "led"));
  const lo = (a) => Math.min(...boxes.map((b) => b.min[a])), hi = (a) => Math.max(...boxes.map((b) => b.max[a]));
  assert.ok(lo(0) > 10 && hi(0) < 10.1, "standing a few cm out of the wall at x 10, towards +x");
  assert.ok(Math.abs((lo(2) + hi(2)) / 2 - 5) < 1e-9 && Math.abs((lo(1) + hi(1)) / 2 - 2) < 1e-9, "centred");
  // Read from +x the reader's right is -z: "H" (first) sits at larger z than "I".
  const first = letterBoxes({ ...s, text: "H" }, "H ");
  assert.ok(Math.min(...first.map((b) => b.min[2])) > 5 - 0.01, "the first letter on the reader's left (+z)");
  assert.equal(layout("AB\nC", 0.7).cells.length, 3);
});

test("still signs go in the map's mesh; moving and live ones are drawn each frame", () => {
  const map = {
    signs: [{ text: "A", pos: [0, 0, 0], face: "+z", size: 0.7, mat: "ink" }, { text: "{backlog:vibe}", pos: [0, 0, 0], face: "+z", size: 0.7, mat: "led", live: true },
      { text: "B", pos: [0, 0, 0], face: "+z", size: 0.7, mat: "ink", mover: 1 }],
    movers: [{ kind: "path", min: [0, 0, 0], max: [1, 1, 1], period: 10, phase: 0, path: [[0, 0, 0, 0], [5, 0, 10, 0]], hide: [0, 0], move: [0, 0, 0] }],
  };
  assert.equal(signBoxes(map.signs).length, glyphRects("A").length);
  assert.equal(signText("{backlog:vibe} left, {x}", { "backlog:vibe": 42 }), "42 left, ");
  const at = (t, values) => signSolids(map, { t, now: t, values });
  const n = at(0, { "backlog:vibe": "150" }).length;
  assert.ok(n > 3 * 2, "three tiles and their digits, B's dots");
  // B rides its mover: 10 m up by t = 5.
  const yOf = (sol) => sol.map((x) => x.m[13]);
  const b0 = Math.max(...yOf(at(0, {}))), b5 = Math.max(...yOf(at(5, {})));
  assert.ok(Math.abs(b5 - b0 - 10) < 0.5, `rode the mover: ${b0} -> ${b5}`);
  // A changed digit flips: one more solid (the flap) for a moment.
  at(9, { "backlog:vibe": "149" }); // settle: the flaps of the change to 149 are down by 10
  const before = at(10, { "backlog:vibe": "149" }).length, flip = at(10.05, { "backlog:vibe": "148" }).length, after = at(11, { "backlog:vibe": "148" }).length;
  assert.equal(flip, after + 1, "the flap over the changed digit");
  assert.ok(before > 0);
});

test("a floor sign (+y): letters lying on the floor, read facing +x (its first letter towards -z, its top towards +x)", () => {
  const s = { text: "HH\nH", pos: [3, 0.005, -2], face: "+y", size: 0.7, mat: "led" };
  const boxes = letterBoxes(s);
  const lo = (a) => Math.min(...boxes.map((b) => b.min[a])), hi = (a) => Math.max(...boxes.map((b) => b.max[a]));
  assert.ok(lo(1) > 0.005 && hi(1) < 0.1, "a few cm over the floor");
  assert.ok(Math.abs((lo(2) + hi(2)) / 2 + 2) < 1e-9 && Math.abs((lo(0) + hi(0)) / 2 - 3) < 1e-9, "centred");
  const first = letterBoxes({ ...s, text: "H" }, "H ");
  assert.ok(Math.max(...first.map((b) => b.max[2])) < -2 + 0.01, "the first letter on the reader's left (-z)");
  const top = letterBoxes({ ...s, text: "H" }, "H\n ");
  assert.ok(Math.min(...top.map((b) => b.min[0])) > 3 - 0.01, "the first line ahead of the reader (+x)");
});

test("a sign on a mover is drawn only while its mover's box is there (the cabin floor's markings as the pods open it)", () => {
  const floor = { kind: "path", mat: "carpet", min: [-1, -0.5, -1], max: [1, 0, 1], period: 10, phase: 0, path: [[0, 0, 0, 0], [5, 10, 0, 0]], hide: [6, 8] };
  const map = { movers: [floor], signs: [{ text: "HI", pos: [0, 0.005, 0], face: "+y", size: 0.3, mat: "led", mover: 1 }] };
  const at5 = signSolids(map, { t: 5 });
  assert.ok(at5.length > 0, "there with the floor");
  assert.equal(signSolids(map, { t: 7 }).length, 0, "gone with it");
  assert.ok(signSolids(map, { t: 9 }).length > 0, "back with it");
});
