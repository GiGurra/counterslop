import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createBoats } from "./boats.js";
import { boatParts, BOAT } from "../assets/models/boat.js";
import { createPredictor, adoptRide } from "./predict.js";
import { stepLocal, leave } from "./localsim.js";
import { createPhysics, newBody } from "../engine/physics.js";
import { SeatWorld } from "./shared.gen.js";
import { BTN } from "../engine/cmd.js";

const harbor = JSON.parse(readFileSync(new URL("../../internal/content/maps/testdata/lab_harbor.json", import.meta.url)));
const snapBoat = { id: 1, p: [-12, -0.6, 8], y: 0, v: [0, 0, -10] };

test("boats: drawn where the snapshot put them, run ahead by their velocity for at most 0.15 s; our own boat drawn where we predict it", () => {
  const b = createBoats();
  b.ingest([snapBoat], 1000);
  assert.deepEqual(b.drawnPos(snapBoat, 1100), [-12, -0.6, 7]);
  assert.deepEqual(b.drawnPos(snapBoat, 5000), [-12, -0.6, 8 - 1.5]);
  assert.equal(b.list.length, 1);
  const parts = b.solids(0, 1000);
  assert.equal(parts.length, boatParts({ pos: [0, 0, 0] }).length);
  const own = b.solids(0, 1000, { id: 1, p: [50, 0, 50], y: 0, v: [0, 0, 0] });
  assert.ok(Math.abs(own[0].m[12] - 50) < 2, "drawn at the predicted pose");
  b.ingest(undefined, 2000);
  assert.deepEqual(b.solids(0, 2000), []);
});

test("the boat model: boxes about its size, the bow lifting with speed", () => {
  const still = boatParts({ pos: [0, 0, 0], time: 0 }), fast = boatParts({ pos: [0, 0, 0], time: 0, speed: 11 });
  assert.ok(still.length > 10 && still.every((p) => p.m.length === 16 && p.color.length === 3));
  assert.notDeepEqual(still[0].m, fast[0].m);
  assert.ok(BOAT.len > 3 && BOAT.beam < 2);
});

test("adoptRide takes the seat and the boat's exact state from the self-snapshot; on foot clears them", () => {
  const me = {};
  adoptRide(me, { ride: 1, seat: 0, rb: { p: [1, 2, 3], v: [0, 0, -1], y: 0.5, yv: 0.1 } });
  assert.deepEqual(me, { ride: 1, seat: 0, boat: { pos: [1, 2, 3], vel: [0, 0, -1], yaw: 0.5, yawVel: 0.1 } });
  adoptRide(me, {});
  assert.equal(me.ride, 0);
  assert.equal(me.boat, null);
  adoptRide(me, { ride: 2, seat: 1 }); // never sent without the boat: treated as on foot
  assert.equal(me.ride, 0);
});

test("a driver's replay steers the boat with the pending commands (as the server), and an E press in them gets out", () => {
  const phys = createPhysics();
  phys.setMap(harbor);
  const pr = createPredictor(phys);
  const me = newBody([-12, -0.5, 9.15]);
  for (let seq = 1; seq <= 50; seq++) pr.push({ seq, f: 1, r: 0, b: 0, yaw: 0, pitch: 0, speed: 1 });
  pr.reconcile(me, { ack: 0, p: [-12, -0.5, 9.15], v: [0, 0, 0], g: true, c: true, ride: 1, seat: 0, rb: { p: [-12, -0.6, 8], v: [0, 0, 0], y: 0, yv: 0 } }, 0.01, { watch: false });
  assert.equal(me.ride, 1);
  assert.ok(me.boat.pos[2] < 7.5, "the boat moved ahead");
  assert.deepEqual(me.pos, SeatWorld(me.boat, 0));
  pr.push({ seq: 51, f: 0, r: 0, b: BTN.USE, yaw: 0, pitch: 0, speed: 1 });
  pr.reconcile(me, { ack: 50, p: [...me.pos], v: [...me.boat.vel], g: true, c: true, ride: 1, seat: 0, rb: { p: [...me.boat.pos], v: [...me.boat.vel], y: me.boat.yaw, yv: me.boat.yawVel } }, 0.01, { watch: false });
  assert.equal(me.ride, 0, "out");
  assert.equal(me.onGround, false);
});

test("stepLocal: a rider steps the boat (no walking events); a jump press hops out on foot, up at jump speed", () => {
  const phys = createPhysics();
  phys.setMap(harbor);
  const me = newBody([-12, -0.5, 9.15]);
  Object.assign(me, { ride: 1, seat: 0, boat: { pos: [-12, -0.6, 8], vel: [0, 0, 0], yaw: 0, yawVel: 0 } });
  const arsenal = { moveSpeed: () => 1, tick: () => null };
  const { ev } = stepLocal({ phys, arsenal, me, cmd: { seq: 1, f: 1, r: 0, b: 0, yaw: 0, pitch: 0 }, prevB: 0, dt: 0.01 });
  assert.equal(ev.jumped, false);
  assert.ok(me.boat.vel[2] < 0);
  stepLocal({ phys, arsenal, me, cmd: { seq: 2, f: 0, r: 0, b: BTN.JUMP, yaw: 0, pitch: 0 }, prevB: 0, dt: 0.01 });
  assert.equal(me.ride, 0);
  assert.ok(me.vel[1] > 6, "hopped");
  const p2 = newBody([0, 0, 0]);
  Object.assign(p2, { ride: 1, seat: 1, boat: { pos: [0, -0.6, 0], vel: [1, 0, 0], yaw: 0, yawVel: 0 } });
  leave(phys, p2, BTN.USE, 0);
  assert.deepEqual([p2.ride, p2.vel[0], p2.vel[1]], [0, 1, 0]);
});
