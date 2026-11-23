import { test } from "node:test";
import assert from "node:assert/strict";
import { createPredictor, peersFrom } from "./predict.js";
import { H, H_CROUCH } from "../engine/physics.js";

test("peersFrom: the other living players on a team, in ID order, crouched ones 1.2 m tall; lo = a lower ID", () => {
  const players = [
    { id: 5, p: [1, 0, 1], c: true, hp: 100, team: "artisan" },
    { id: 2, p: [0, 0, 0], hp: 100, team: "vibe" },
    { id: 3, p: [9, 0, 9], hp: 100, team: "vibe" }, // me
    { id: 4, p: [2, 0, 2], hp: 0, dead: true, team: "vibe" },
    { id: 6, p: [3, 0, 3], hp: 100, team: "spec" },
  ];
  assert.deepEqual(peersFrom(players, 3), [{ pos: [0, 0, 0], h: H, lo: true }, { pos: [1, 0, 1], h: H_CROUCH, lo: false }]);
  assert.equal(peersFrom(players, 3, false), null); // mp_solid_players 0
  assert.equal(peersFrom([players[2]], 3), null);
  assert.equal(peersFrom(undefined, 3), null);
});

test("reset (respawn, teleport) forgets the others' places until the next snapshot", () => {
  const pr = createPredictor({ step() {} });
  pr.setPeers([{ pos: [0, 0, 0], h: H, lo: true }]);
  pr.reset();
  assert.equal(pr.peers, null); // a round restart moved everyone: the old snapshot would push us out of ghosts
});

test("the replay collides with the newest snapshot's peers", () => {
  const seen = [];
  const pr = createPredictor({ step(_me, _inp, _dt, _t, peers) { seen.push(peers); } });
  const peers = [{ pos: [1, 0, 0], h: H, lo: false }];
  pr.push({ seq: 1, f: 1, r: 0, yaw: 0, b: 0, speed: 1 });
  pr.push({ seq: 2, f: 1, r: 0, yaw: 0, b: 0, speed: 1 });
  pr.setPeers(peers);
  const me = { pos: [0, 0, 0], vel: [0, 0, 0], yaw: 0, pitch: 0 };
  pr.reconcile(me, { ack: 1, p: [0, 0, 0], v: [0, 0, 0], g: true, c: false }, 0.01);
  assert.deepEqual(seen, [peers]);
});

// A predictor over a no-op step: the replay leaves us where the server says,
// so a correction is exactly (our position before) - (server position).
function rig() {
  const pr = createPredictor({ step() {} });
  const fired = [];
  pr.onResync = (r) => fired.push(r);
  const me = { pos: [0, 0, 0], vel: [0, 0, 0], yaw: 0, pitch: 0 };
  let seq = 0;
  // Predict one command to `at`, then have the server ack it at `srv`.
  const round = (at, srv, opts = {}) => {
    const c = { seq: ++seq, f: 0, r: 0, yaw: 0, b: 0, speed: 1 };
    pr.push(c); me.pos = [...at]; c.pos = [...at];
    pr.reconcile(me, { ack: seq, p: [...srv], v: [0, 0, 0], g: true, c: false }, 0.01, opts);
  };
  return { pr, me, round, fired };
}

test("the smoothing offset eases out: a 0.3 m correction is under 5 cm within 150 ms of frames", () => {
  const { pr, round, fired } = rig();
  round([0.3, 0, 0], [0, 0, 0], { now: 0 });
  assert.ok(Math.abs(pr.error[0] - 0.3) < 1e-9); // drawn where we were: no visible jump
  for (let t = 1; t <= 9; t++) pr.decay(1 / 60, true, t * 16.7);
  assert.ok(Math.hypot(...pr.error) < 0.05, `offset ${pr.error[0]}`);
  assert.equal(fired.length, 0);
  assert.equal(pr.stats.resyncs, 0);
});

test("not playing a body (dead, spectating, observing): the offset is dropped, nothing re-syncs", () => {
  const { pr, round, fired } = rig();
  round([0.3, 0, 0], [0, 0, 0], { now: 0, watch: false });
  pr.decay(1 / 60, false, 10);
  assert.deepEqual(pr.error, [0, 0, 0]);
  round([0.9, 0, 0], [0, 0, 0], { now: 20, watch: false }); // big, but not watched
  assert.equal(fired.length, 0);
});

test("re-sync: a correction of 0.5 m or more snaps (no offset) and is counted", () => {
  const { pr, round, fired } = rig();
  round([0.6, 0, 0], [0, 0, 0], { now: 0 });
  assert.deepEqual(pr.error, [0, 0, 0]);
  assert.equal(pr.stats.resyncs, 1);
  assert.deepEqual(fired, [{ why: "correction", size: 0.6 }]);
});

test("re-sync: small corrections that keep coming and are never eased out (no decay) are dropped within 300 ms", () => {
  // The bug: main.js never called decay(), so every small collision correction
  // stacked onto the camera for good (operator: 1-2 m off after pushing a moving player).
  const { pr, me, round, fired } = rig();
  let t = 0, at = null, above = null;
  for (; t <= 400 && !fired.length; t += 16) {
    round([me.pos[0] + 0.02, 0, 0], [me.pos[0], 0, 0], { now: t });
    if (above === null && Math.hypot(...pr.error) > 0.05) above = t;
    if (fired.length) at = t;
  }
  assert.equal(fired.length, 1, `offset ${pr.error[0]} at ${t} ms`);
  assert.ok(at - above <= 320, `fired ${at - above} ms after the offset passed 5 cm`);
  assert.equal(fired[0].why, "stuck");
  assert.deepEqual(pr.error, [0, 0, 0]);
});

test("re-sync: small offsets piling up to 0.5 m snap at once", () => {
  const { pr, me, round, fired } = rig();
  for (let i = 0; i < 4; i++) round([me.pos[0] + 0.15, 0, 0], [me.pos[0], 0, 0], { now: i });
  assert.equal(fired.length, 1);
  assert.ok(["offset", "drawn"].includes(fired[0].why), fired[0].why);
  assert.deepEqual(pr.error, [0, 0, 0]);
});

test("re-sync: drawn 0.5 m off the server at the acked command (prediction exact, offset left over) snaps", () => {
  const { pr, round, fired } = rig();
  round([0.4, 0, 0], [0, 0, 0], { now: 0 }); // offset 0.4
  round([1, 0, 0], [0.85, 0, 0], { now: 5 }); // drawn at 1.4 (pushed with offset 0.4) vs server 0.85
  assert.equal(fired.length, 1);
  assert.equal(fired[0].why, "drawn");
  assert.deepEqual(pr.error, [0, 0, 0]);
});

test("a teleport (server move) snaps without counting as a re-sync", () => {
  const { pr, me, fired } = rig();
  pr.push({ seq: 1, f: 0, r: 0, yaw: 0, b: 0, speed: 1, pos: [0, 0, 0] });
  me.pos = [0, 0, 0];
  pr.moved(me, { ack: 1, p: [30, 0, 0], v: [0, 0, 0], g: true, c: false }, 0.01);
  assert.deepEqual(me.pos, [30, 0, 0]);
  assert.deepEqual(pr.error, [0, 0, 0]);
  assert.equal(fired.length, 0);
});

test("normal play: tiny corrections, eased out every frame, never re-sync", () => {
  const { pr, me, round, fired } = rig();
  let t = 0;
  for (let i = 0; i < 300; i++, t += 16) {
    if (i % 2 === 0) round([me.pos[0] + 0.004, 0, 0], [me.pos[0], 0, 0], { now: t });
    pr.decay(0.016, true, t + 8);
  }
  assert.equal(fired.length, 0);
  assert.ok(Math.hypot(...pr.error) < 0.05);
});

test("re-sync: one desync re-syncs once (commands drawn before the snap don't count it again when acked)", () => {
  const { pr, me, fired } = rig();
  const srv = (ack, p) => pr.reconcile(me, { ack, p, v: [0, 0, 0], g: true, c: false }, 0.01, { now: ack });
  // Three commands predicted 1 m off (the body was knocked aside), then the server acks them one by one.
  for (let s = 1; s <= 3; s++) { const c = { seq: s, f: 0, r: 0, yaw: 0, b: 0, speed: 1 }; pr.push(c); me.pos = [1, 0, 0]; c.pos = [1, 0, 0]; }
  srv(1, [0, 0, 0]);
  srv(2, [0, 0, 0]);
  srv(3, [0, 0, 0]);
  assert.equal(fired.length, 1, JSON.stringify(fired));
  // A command predicted after the snap that is off again is a new desync.
  const c = { seq: 4, f: 0, r: 0, yaw: 0, b: 0, speed: 1 }; pr.push(c); me.pos = [0.8, 0, 0]; c.pos = [0.8, 0, 0];
  srv(4, [0, 0, 0]);
  assert.equal(fired.length, 2);
});
