import test from "node:test";
import assert from "node:assert/strict";
import { newGait, gaitStep, gaitPose, newPose, GAIT, TELEPORT_SPEED } from "./gait.js";

const DT = 0.01;
const K = { swing: 1, waddle: 1, bob: 1, wobble: 1, lean: 1, offset: 0 };
// Run n ticks at speed v (m/s) along -Z.
function run(g, v, n, o = {}) {
  for (let i = 0; i < n; i++) gaitStep(g, 0, 0, -v * DT, DT, o);
  return g;
}
const mag = (p) => Math.max(Math.abs(p.footL.z), Math.abs(p.footL.y), Math.abs(p.footR.z), Math.abs(p.legsRoll), Math.abs(p.body.y), Math.abs(p.body.pitch));

test("the phase advances with distance travelled, not time", () => {
  const a = run(newGait(), 7, 200), b = newGait();
  run(b, 7, 200);
  const before = b.phase;
  run(b, 0, 50); // standing: time passes, no distance
  const still = b.phase;
  assert.ok(a.phase > 0);
  assert.ok(Math.abs(still - before) < 0.05, `phase moved ${still - before} while standing`);
  // at a steady run, twice the distance is twice the phase
  const g = run(newGait(), 7, 300), p0 = g.phase;
  run(g, 7, 100); const d1 = g.phase - p0;
  run(g, 7, 200); const d2 = g.phase - p0 - d1;
  assert.ok(Math.abs(d2 / d1 - 2) < 0.01, `ratio ${d2 / d1}`);
});

test("a planted foot doesn't skate: it slides back as fast as the body moves", () => {
  const g = run(newGait(), 7, 400), pose = newPose();
  // find mid-stance of the left foot (sin(phase) passing through 0 going down: cos = -1)
  let best = null;
  for (let i = 0; i < 200; i++) {
    const z0 = gaitPose(g, 1, K, pose).footL.z;
    gaitStep(g, 0, 0, -7 * DT, DT);
    const z1 = gaitPose(g, 1, K, pose).footL.z;
    const c = Math.cos(g.phase);
    if (!best || c < best.c) best = { c, v: (z1 - z0) / DT };
  }
  // the foot moves +Z (backwards) relative to the body at ~ run speed: world speed ~ 0
  assert.ok(Math.abs(best.v - 7) < 0.5, `foot relative speed ${best.v}`);
});

test("amplitude is 0 at rest and eases back after running", () => {
  const pose = newPose();
  const idle = run(newGait(), 0, 100);
  assert.equal(idle.amp, 0);
  assert.ok(mag(gaitPose(idle, 1, K, pose)) < 0.06, "only an idle sway at rest");
  const g = run(newGait(), 7, 200);
  assert.ok(g.amp > 0.9);
  run(g, 0, 300);
  assert.ok(g.amp < 0.01);
  assert.ok(Math.abs(gaitPose(g, 1, K, pose).footL.z) < 0.01);
});

test("walking swings less than running; bounded at and past max speed", () => {
  const pose = newPose();
  const peak = (v) => {
    const g = run(newGait(), v, 300);
    let m = 0;
    for (let i = 0; i < 300; i++) { gaitStep(g, 0, 0, -v * DT, DT); m = Math.max(m, Math.abs(gaitPose(g, 1, K, pose).footL.z)); }
    return m;
  };
  const walk = peak(3.5), runP = peak(7), fast = peak(40);
  assert.ok(walk < runP * 0.7, `walk ${walk} run ${runP}`);
  assert.ok(runP <= GAIT.swing * 1.0001);
  assert.ok(fast <= GAIT.swing * 1.0001, `fast ${fast}`);
});

test("speed changes never pop: a foot never moves faster than a running stride", () => {
  const g = newGait(), pose = newPose();
  let prev = { ...gaitPose(g, 1, K, pose).footL };
  for (let i = 0; i < 600; i++) {
    const v = i < 200 ? 0 : i < 400 ? 7 : 3.5;
    gaitStep(g, 0, 0, -v * DT, DT, { crouch: i > 500 });
    const f = gaitPose(g, 1, K, pose).footL;
    assert.ok(Math.abs(f.z - prev.z) < 0.075 && Math.abs(f.y - prev.y) < 0.075, `tick ${i}`);
    prev = { ...f };
  }
});

test("a teleport or respawn resets the cycle", () => {
  const g = run(newGait(), 7, 200);
  assert.ok(g.phase > 0 && g.amp > 0.5);
  gaitStep(g, TELEPORT_SPEED * DT * 2, 0, 0, DT);
  assert.equal(g.phase, 0); assert.equal(g.amp, 0); assert.equal(g.air, 0);
  // and the pose is the rest pose, with no interpolation from the old one
  assert.ok(mag(gaitPose(g, 0.5, K, newPose())) < 0.06);
  // a 1 m hop in one tick is still a step... a vertical warp is a teleport
  gaitStep(g, 0, 50, 0, DT);
  assert.equal(g.phase, 0);
});

test("airborne: feet flail, the stride stops; crouch: short shuffle", () => {
  const g = run(newGait(), 7, 200, { air: true }), pose = newPose();
  assert.ok(g.air > 0.99);
  const p = g.phase;
  run(g, 7, 50, { air: true });
  assert.ok(Math.abs(g.phase - p) < 1e-9, "no stepping in the air");
  gaitPose(g, 1, K, pose);
  assert.ok(Math.abs(pose.footL.z) > 0 || Math.abs(pose.footL.y) > 0);
  // inferred from vertical speed when the caller doesn't know
  const h = newGait();
  for (let i = 0; i < 30; i++) gaitStep(h, 0, 6 * DT, 0, DT);
  assert.ok(h.air > 0.9);
  const peak = (v, o) => {
    const c = run(newGait(), v, 300, o);
    let m = 0;
    for (let i = 0; i < 300; i++) { gaitStep(c, 0, 0, -v * DT, DT, o); m = Math.max(m, Math.abs(gaitPose(c, 1, K, pose).footL.z)); }
    return m;
  };
  const shuffle = peak(2.3, { crouch: true });
  assert.ok(shuffle > 0.05, `a crouch-walk still steps (${shuffle})`);
  assert.ok(shuffle < peak(7, {}) * 0.6, "but with short steps");
});

test("nonsense input is a reset, never NaN", () => {
  const g = run(newGait(), 7, 100);
  gaitStep(g, NaN, 0, 0, DT);
  gaitStep(g, 0, 0, 0, 0);
  const p = gaitPose(g, 1, K, newPose());
  for (const v of [p.footL.z, p.footL.y, p.body.y, p.head.roll]) assert.ok(Number.isFinite(v));
});
