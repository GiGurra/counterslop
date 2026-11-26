import test from "node:test";
import assert from "node:assert/strict";
import { playerParts, hitParts, PLAYER, torsoFrame } from "./player.js";
import { chain, translate, scale, rotY } from "../../engine/math.js";
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
    for (let i = 0; i < 137; i++) gaitStep(g, 0, 0, -v * 0.01, { onGround: !o.air, crouch: o.crouch, id: 4 });
    out.push(g);
  }
  return out;
}
// A part's drawn box (unit cube -> world) from a hitParts entry: frame·translate(c)·scale(s).
const boxOf = (h) => chain(h.m, translate(...h.c), scale(...h.s));
const near = (a, b) => a.every((v, i) => Math.abs(v - b[i]) < 1e-5);

test("the hit pose is the drawn pose: hit boxes move with the gait, exactly as drawn", () => {
  for (const st of STATES) {
    const still = hits(st);
    let moved = false;
    for (const g of gaits()) {
      const s2 = { ...st, gait: g, t: 12.34 };
      const drawn = playerParts({ ...s2, rgb: [1, 0, 0] });
      const boxes = hitParts(s2);
      // every hit box is a drawn part, to float32 precision
      for (const h of boxes) assert.ok(drawn.some((d) => near(d.m, boxOf(h))), `${h.region} box not drawn`);
      // and the ray test sees the moved pose
      if (JSON.stringify(hits(s2)) !== JSON.stringify(still)) moved = true;
    }
    assert.ok(moved, "a walk cycle changes some ray hits");
  }
});

test("at rest the animated model sits on the still pose (only an idle sway)", () => {
  const st = STATES[0], g = newGait();
  for (let i = 0; i < 100; i++) gaitStep(g, 0, 0, 0, { onGround: true, id: st.id });
  const drawn = playerParts({ ...st, rgb: [1, 0, 0], gait: g, t: 5 }), still = playerParts({ ...st, rgb: [1, 0, 0] });
  for (let i = 0; i < drawn.length; i++) for (const j of [12, 13, 14]) assert.ok(Math.abs(drawn[i].m[j] - still[i].m[j]) < 0.08);
});

test("torsoFrame: the upper group's frame with a gait, translate·rotY without", () => {
  const st = STATES[0], g = gaits()[0];
  assert.ok(near(torsoFrame(st), chain(translate(...st.p), rotY(st.yaw))));
  const f = torsoFrame({ ...st, gait: g, t: 1 });
  assert.ok(!near(f, torsoFrame(st)), "the torso moves with the gait");
});

// playerParts keeps each player's parts and colours between frames (they depend on
// its id, team and colour only): what it draws must be what a fresh build draws,
// every time, for any mix of players, teams, colours, poses and the time-cycling "rgb".
import { partsFor, poseFrames, partMatrix } from "../../gen/boxmodel.js";
test("playerParts' cached parts and colours draw the same as a fresh build", () => {
  const fresh = (p) => {
    const st = { p: p.p, yaw: p.yaw, pitch: p.pitch, crouch: !!p.crouch, team: p.team, id: p.id, gait: p.gait || null, t: p.t || 0, gv: p.gv | 0 };
    const frames = poseFrames(PLAYER, st);
    return partsFor(PLAYER, st).map((pt) => ({ m: [...partMatrix(pt, st, frames)], n: pt.name }));
  };
  const strip = (parts) => parts.map((x) => ({ m: [...x.m], n: x.name }));
  const players = [];
  for (let id = 1; id <= 12; id++) for (const team of ["vibe", "artisan", "spec"]) {
    players.push({ id, team, p: [id, 0, -id], yaw: id * 0.3, pitch: 0.1, rgb: [id / 12, 0.4, 1 - id / 12], crouch: id % 3 === 0, time: id * 0.7, gait: newGait(), t: id * 0.1 });
  }
  for (let round = 0; round < 3; round++) {
    for (const p of players) {
      const got = playerParts({ ...p, time: p.time + round });
      assert.deepEqual(strip(got), fresh(p), `${p.team} #${p.id} round ${round}`);
    }
  }
  // the colours: a player's colour change shows at once; seeded colours differ by id
  const a = playerParts({ ...players[0], rgb: [1, 0, 0] }), b = playerParts({ ...players[0], rgb: [0, 1, 0] });
  assert.notDeepEqual(a.map((x) => x.color), b.map((x) => x.color));
  assert.deepEqual(playerParts({ ...players[0], rgb: [1, 0, 0] }).map((x) => x.color), a.map((x) => x.color));
  // gamer RGB parts cycle with time, cached or not; the rest stay
  const at = (time) => playerParts({ ...players[0], team: "vibe", time }).map((x) => x.color.join());
  const t0 = at(0), t1 = at(1), t0again = at(0);
  assert.deepEqual(t0again, t0);
  const changed = t0.filter((c, i) => c !== t1[i]).length;
  assert.ok(changed > 0 && changed < t0.length, `${changed} of ${t0.length} colours cycle`);
});
