import { test } from "node:test";
import assert from "node:assert/strict";
import { padArcs, padArcParts } from "./padarcs.js";

const pad = (o) => ({ name: "p", min: [-1, -0.1, -1], max: [1, 0.3, 1], vel: [0, 0, 0], mover: 0, from: 0, to: 0, air: false, target: [0, 0, 0], apex: 0, ...o });

test("a jump pad's arc runs from the pad to its target, its top apex over the higher end", () => {
  const [pts] = padArcs({ launchers: [pad({ target: [30, 6, 0], apex: 4 })] });
  const end = pts[pts.length - 1];
  assert.ok(Math.hypot(end[0] - 30, end[1] - 6, end[2]) < 1e-6, `ends at ${end}`);
  const top = Math.max(...pts.map((p) => p[1]));
  assert.ok(top > 9.5 && top <= 10 + 1e-9, `top ${top}`);
});

test("a fixed pad's arc follows its velocity; a mover's pads and space aren't drawn", () => {
  const arcs = padArcs({
    launchers: [pad({ vel: [12, 6, 0] }), pad({ vel: [0, 30, 0], mover: 1 })],
    gravZones: [{ name: "space", min: [50, -10, -10], max: [60, 10, 10], g: 0, jump: 0, mover: 0 }],
  });
  assert.equal(arcs.length, 1);
  assert.ok(arcs[0][arcs[0].length - 1][0] > 17);
  assert.equal(padArcs({ launchers: [pad({ min: [49, -0.1, -1], max: [51, 0.3, 1], vel: [1, 1, 0] })], gravZones: [{ name: "space", min: [40, -10, -10], max: [60, 10, 10], g: 0, jump: 0, mover: 0 }] }).length, 0);
});

test("the dashes: a third of the segments, moving with time", () => {
  const arcs = padArcs({ launchers: [pad({ target: [30, 6, 0], apex: 4 })] });
  const a = padArcParts(arcs, 0), b = padArcParts(arcs, 0.13);
  assert.equal(a.length, 12);
  assert.notDeepEqual(a[0].m, b[0].m);
});
