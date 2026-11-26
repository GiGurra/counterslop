import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createBoats, ENGINE_RANGE } from "./boats.js";
import { boatParts, BOAT } from "../assets/models/boat.js";
import { createPredictor, adoptRide } from "./predict.js";
import { stepLocal, leave, moveCmd } from "./localsim.js";
import { createPhysics, newBody } from "../engine/physics.js";
import { SeatWorld, SeatLocal, SeatDeck } from "./shared.gen.js";
import { DeckWorld, DeckTop } from "../engine/sim.gen.js";
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
  assert.equal(parts.length, boatParts({ pos: [0, 0, 0], speed: 10 }).length); // the hull and its wake
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

test("adoptRide takes the seat, the boat's exact state and where we stand on its deck from the self-snapshot; on foot clears them", () => {
  const me = {};
  adoptRide(me, { ride: 1, seat: 0, rb: { p: [1, 2, 3], v: [0, 0, -1], y: 0.5, yv: 0.1 } });
  assert.deepEqual(me, { ride: 1, seat: 0, boat: { pos: [1, 2, 3], vel: [0, 0, -1], yaw: 0.5, yawVel: 0.1, driven: false }, deck: null });
  adoptRide(me, { ride: 1, seat: SeatDeck, rb: { p: [1, 2, 3], v: [0, 0, -1], y: 0.5, yv: 0.1, rd: true }, dk: { p: [0.2, 0.1, -1], v: [0, 0, 0] } });
  assert.deepEqual([me.seat, me.boat.driven, me.deck], [SeatDeck, true, { pos: [0.2, 0.1, -1], vel: [0, 0, 0] }]);
  adoptRide(me, { ride: 1, seat: SeatDeck, rb: { p: [1, 2, 3], v: [0, 0, 0], y: 0, yv: 0 } }); // (never: the deck comes with the seat)
  assert.equal(me.ride, 0);
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
  assert.deepEqual([me.ride, me.seat], [1, SeatDeck], "up: standing on the deck");
  assert.deepEqual(me.deck.pos, [0, DeckTop, SeatLocal(0)[2]]);
  assert.equal(me.onGround, true);
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
  assert.deepEqual([me.ride, me.seat], [1, SeatDeck], "hopped up onto the deck");
  assert.ok(me.deck.vel[1] > 6 && me.deck.pos[1] > DeckTop, "hopped");
  const p2 = newBody([0, 0, 0]);
  Object.assign(p2, { ride: 1, seat: 1, boots: true, boat: { pos: [0, -0.6, 0], vel: [1, 0, 0], yaw: 0, yawVel: 0 } });
  leave(phys, p2, BTN.HOOK, 0); // the hook pulls you out
  assert.deepEqual([p2.ride, p2.vel[0], p2.vel[1], p2.deck], [0, 1, 0, null]);
});

test("moveCmd: a body walking off the quay lands on a moored boat's floor (the snapshot's boats), stands on its deck, walks on it in the boat's frame and off its side", () => {
  const phys = createPhysics();
  phys.setMap(harbor);
  const me = newBody([-12, 0, 10.5]);
  const boats = [{ id: 1, p: [-12, -0.6, 8], y: 0, v: [0, 0, 0] }];
  let prevB = 0, n = 0;
  const cmd = (f, r, b = 0) => { const c = { seq: ++n, f, r, b, yaw: 0, pitch: 0 }; const ev = moveCmd(phys, me, c, { f, r, walk: true, jump: !!(b & BTN.JUMP) && !(prevB & BTN.JUMP), crouch: false, speed: 1, pitch: 0 }, prevB, 0.01, 0, null, boats); prevB = b; return ev; };
  for (let i = 0; i < 40; i++) cmd(1, 0);
  for (let i = 0; i < 20; i++) cmd(0, 0);
  assert.deepEqual([me.ride, me.seat, me.onGround], [1, SeatDeck, true]);
  assert.equal(me.deck.pos[1], DeckTop);
  assert.deepEqual(me.pos, DeckWorld(me.boat, me.deck.pos));
  const z = me.deck.pos[2];
  for (let i = 0; i < 10; i++) cmd(1, 0); // forward: towards the bow (-z of the boat)
  assert.ok(me.deck.pos[2] < z - 0.2);
  for (let i = 0; i < 40; i++) cmd(0, 1); // to starboard, off the side
  assert.deepEqual([me.ride, me.deck], [0, null]);
});

test("moveCmd on the deck: a boat nobody drives drifts on under us (as the server's tickBoats), we stay where we stand on it; someone driving it: we leave it to the snapshots", () => {
  const phys = createPhysics();
  phys.setMap(harbor);
  for (const driven of [false, true]) {
    const me = newBody([0, 0, 0]);
    Object.assign(me, { ride: 1, seat: SeatDeck, onGround: true, boat: { pos: [0, -0.6, -15], vel: [0, 0, -5], yaw: 0, yawVel: 0, driven }, deck: { pos: [0, DeckTop, 0], vel: [0, 0, 0] } });
    for (let n = 1; n <= 10; n++) moveCmd(phys, me, { seq: n, f: 0, r: 0, b: 0, yaw: 0, pitch: 0 }, { f: 0, r: 0, speed: 1 }, 0, 0.01);
    assert.deepEqual(me.deck.pos, [0, DeckTop, 0]);
    if (driven) assert.equal(me.boat.pos[2], -15);
    else assert.ok(me.boat.pos[2] < -15.4 && me.boat.vel[2] > -5, "drifted, slowing");
    assert.deepEqual(me.pos, DeckWorld(me.boat, me.deck.pos));
  }
});

test("a moving boat's outboard plays near the listener (revs from its speed), stops when it stops or we are far", () => {
  const started = [];
  const b = createBoats({ start: (sp) => { const h = { sp, stopped: false, update(s) { h.sp = s; }, stop() { h.stopped = true; } }; started.push(h); return h; }, spatial: () => ({ pan: 0.5, gain: 1 }) });
  b.ingest([{ id: 1, p: [0, 0, 0], y: 0, v: [0, 0, -11] }, { id: 2, p: [5, 0, 0], y: 0, v: [0, 0, 0] }], 0);
  b.sound([0, 0, 5], 0);
  assert.equal(b.engines, 1); // the still one is quiet
  assert.equal(started[0].sp.rev, 1);
  assert.equal(started[0].sp.pan, 0.5);
  b.ingest([{ id: 1, p: [0, 0, 0], y: 0, v: [0, 0, -5.5] }], 10);
  b.sound([0, 0, 5], 10);
  assert.equal(started[0].sp.rev, 0.5);
  b.sound([0, 0, ENGINE_RANGE + 10], 10);
  assert.equal(b.engines, 0);
  assert.ok(started[0].stopped);
  b.sound(null, 10);
  assert.equal(started.length, 1);
});

test("the wake: foam behind a moving boat only", () => {
  assert.ok(boatParts({ pos: [0, 0, 0], speed: 8 }).length > boatParts({ pos: [0, 0, 0], speed: 0 }).length + 10);
});
