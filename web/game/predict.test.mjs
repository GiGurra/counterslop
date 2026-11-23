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
