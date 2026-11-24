import test from "node:test";
import assert from "node:assert/strict";
import { hitSeed, slopBurst, stepBurst, burstSize, relAngle, flinch, vignette, vignetteCss, lowHpEdge, VIGNETTE_S, FLINCH_S, BURST_LIFE } from "./hitfx.js";
import { PALETTE } from "../assets/palette.js";

const TEAM = [0.2, 0.4, 1];

test("the same hit makes the same burst for everyone; another hit a different one", () => {
  const at = [1.234, 1.5, -3.2];
  const a = slopBurst(hitSeed(7, at), at, [0, 0, -1], TEAM);
  const b = slopBurst(hitSeed(7, at), at, [0, 0, -1], TEAM);
  assert.deepEqual(a, b);
  assert.notDeepEqual(slopBurst(hitSeed(8, at), at, [0, 0, -1], TEAM), a);
  assert.notEqual(hitSeed(7, at), hitSeed(7, [1.244, 1.5, -3.2]));
});

test("burst: green flecks plus a few chunks in the team colour, sprayed away from the shooter, short", () => {
  const at = [0, 1, 0];
  const parts = slopBurst(123, at, [0, 0, -5], TEAM);
  const chunks = parts.filter((p) => p.color === TEAM), flecks = parts.filter((p) => p.color !== TEAM);
  assert.ok(chunks.length >= 2 && chunks.length <= 5, `chunks ${chunks.length}`);
  assert.ok(flecks.length > chunks.length);
  for (const f of flecks) assert.ok(f.color[1] > f.color[0] && f.color[1] > f.color[2], "flecks are green (slop), never red");
  const meanVz = parts.reduce((s, p) => s + p.v[2], 0) / parts.length;
  assert.ok(meanVz < -0.5, `the spray goes on along the shot (-z): ${meanVz}`);
  for (const p of parts) assert.ok(p.life > 0 && p.life <= BURST_LIFE);
  assert.ok(slopBurst(123, at, [0, 0, -1], TEAM, { head: true }).length > parts.length, "headshots spray more");
  assert.deepEqual(flecks[0].color.map((c) => Math.min(1, c)), flecks[0].color);
  assert.ok(PALETTE.slop);
});

test("stepBurst moves, drops with gravity and removes expired particles", () => {
  const parts = slopBurst(5, [0, 1, 0], [1, 0, 0], TEAM);
  const p0 = parts[0], y0 = p0.v[1];
  stepBurst(parts, 0.05);
  assert.ok(p0.v[1] < y0);
  assert.ok(burstSize(p0) > 0 && burstSize(p0) <= p0.size);
  for (let i = 0; i < 40; i++) stepBurst(parts, 0.05);
  assert.equal(parts.length, 0);
});

test("relAngle: ahead 0, right +90°, behind 180° (yaw 0 faces -Z)", () => {
  const near = (a, b) => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b))) < 1e-9;
  assert.ok(near(relAngle([0, 0, 0], 0, [0, 0, -5]), 0));
  assert.ok(near(relAngle([0, 0, 0], 0, [5, 0, 0]), Math.PI / 2));
  assert.ok(near(relAngle([0, 0, 0], 0, [0, 0, 5]), Math.PI));
  assert.ok(near(relAngle([0, 0, 0], Math.PI / 2, [-5, 0, 0]), 0), "turned left, the attacker on the left is ahead");
});

test("flinch: a degree or two away from the attacker, back to zero within FLINCH_S", () => {
  const hit = { at: 1, dmg: 30, a0: Math.PI / 2 }; // from the right
  const peak = flinch([hit], 1.06);
  assert.ok(peak.yaw < 0, "pushed away from the right");
  assert.ok(peak.pitch > 0, "and up");
  const deg = Math.hypot(peak.yaw, peak.pitch) * 180 / Math.PI;
  assert.ok(deg > 0.5 && deg <= 2.5, `${deg}°`);
  const big = flinch([{ ...hit, dmg: 500 }], 1.06);
  assert.ok(Math.hypot(big.yaw, big.pitch) * 180 / Math.PI <= 2.5, "capped");
  assert.deepEqual(flinch([hit], 1 + FLINCH_S), { yaw: 0, pitch: 0 });
  assert.deepEqual(flinch([hit], 0.5), { yaw: 0, pitch: 0 });
  const late = flinch([hit], 1 + FLINCH_S - 0.01);
  assert.ok(Math.abs(late.yaw) < Math.abs(peak.yaw) * 0.1, "springs back");
});

test("vignette: on the attacker's side, stronger for more damage, gone after VIGNETTE_S", () => {
  const right = vignette([{ at: 0, dmg: 20 }], 0, () => Math.PI / 2);
  assert.equal(right.length, 1);
  assert.ok(right[0].x > 100 && Math.abs(right[0].y - 50) < 1e-6, "right edge");
  const behind = vignette([{ at: 0, dmg: 20 }], 0, () => Math.PI)[0];
  assert.ok(behind.y > 100, "behind: the bottom edge");
  const big = vignette([{ at: 0, dmg: 60 }], 0, () => 0)[0];
  assert.ok(big.a > right[0].a);
  assert.ok(vignette([{ at: 0, dmg: 20 }], VIGNETTE_S / 2, () => 0)[0].a < right[0].a, "fading");
  assert.equal(vignette([{ at: 0, dmg: 20 }], VIGNETTE_S, () => 0).length, 0);
  assert.equal(vignetteCss([]), "");
  assert.match(vignetteCss([], 0.5), /rgba\(52, 50, 56, 0.475\)/);
  assert.match(vignetteCss(right), /radial-gradient\(.*rgba\(255, 24, 24, 0\.\d+\)/);
});

test("lowHpEdge: nothing above 25 HP or when dead, stronger nearer death", () => {
  assert.equal(lowHpEdge(100), 0);
  assert.equal(lowHpEdge(26), 0);
  assert.equal(lowHpEdge(0), 0);
  assert.ok(lowHpEdge(25) > 0);
  assert.ok(lowHpEdge(5) > lowHpEdge(20));
  assert.ok(lowHpEdge(1) <= 1);
  assert.ok(lowHpEdge(19) >= 0.5, "clearly visible");
  assert.ok(lowHpEdge(19, 0) > lowHpEdge(19, 0.5), "swells on the heartbeat");
  assert.ok(Math.abs(lowHpEdge(19, 5) - lowHpEdge(19)) < 1e-9, "settles between beats");
});
