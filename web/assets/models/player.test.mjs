import test from "node:test";
import assert from "node:assert/strict";
import { playerParts, hitParts, PLAYER, silly } from "./player.js";
import { rayModel } from "../../gen/boxmodel.js";
import { newGait, gaitStep } from "../../gen/gait.js";

const STATES = [
  { p: [3, 0, -2], yaw: 0.7, pitch: 0.2, crouch: false, team: "vibe", id: 4 },
  { p: [-1, 4, 5], yaw: -2, pitch: -0.4, crouch: true, team: "artisan", id: 9 },
];
// A fan of rays at each body from a few sides and heights.
function rays(st) {
  const out = [];
  for (let a = 0; a < 8; a++) for (let h = 0.05; h < 2.2; h += 0.15) for (let off = -0.4; off <= 0.4; off += 0.2) {
    const ang = a * Math.PI / 4, d = [-Math.sin(ang), 0, -Math.cos(ang)];
    const side = [d[2], 0, -d[0]];
    out.push([[st.p[0] - d[0] * 5 + side[0] * off, st.p[1] + h, st.p[2] - d[2] * 5 + side[2] * off], d]);
  }
  return out;
}
const hits = (st) => rays(st).map(([o, d]) => rayModel(PLAYER, st, o, d));
function gaits() {
  const out = [];
  for (const [v, o] of [[7, {}], [3.5, {}], [2.3, { crouch: true }], [7, { air: true }]]) {
    const g = newGait();
    for (let i = 0; i < 137; i++) gaitStep(g, 0, 0, -v * 0.01, 0.01, o);
    out.push(g);
  }
  return out;
}

test("animation is draw-only: hitboxes and ray hits are identical whatever the gait", () => {
  for (const st of STATES) {
    const before = hits(st), boxes = JSON.stringify(hitParts(st));
    for (const g of gaits()) {
      const drawn = playerParts({ ...st, rgb: [1, 0, 0], gait: g, alpha: 0.4 });
      const still = playerParts({ ...st, rgb: [1, 0, 0] });
      // the animation really moves the drawn model...
      assert.ok(drawn.some((d, i) => d.m.some((v, j) => Math.abs(v - still[i].m[j]) > 1e-3)));
      // ...and nothing that can be hit
      assert.deepEqual(hits(st), before);
      assert.equal(JSON.stringify(hitParts(st)), boxes);
    }
  }
});

test("at rest the animated model sits on the hit pose (only an idle sway)", () => {
  const st = STATES[0], g = newGait();
  for (let i = 0; i < 100; i++) gaitStep(g, 0, 0, 0, 0.01);
  const drawn = playerParts({ ...st, rgb: [1, 0, 0], gait: g }), still = playerParts({ ...st, rgb: [1, 0, 0] });
  for (let i = 0; i < drawn.length; i++) for (const j of [12, 13, 14]) assert.ok(Math.abs(drawn[i].m[j] - still[i].m[j]) < 0.08);
});

test("silliness is seeded per player and differs between players", () => {
  assert.deepEqual(silly(3), silly(3));
  assert.notDeepEqual(silly(3), silly(4));
});
