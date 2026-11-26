import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createCars, carHud, carFields, carSpec, simCar, CAR_TYPES } from "./cars.js";
import { carParts, unpackComp, CAR_LOOK } from "../assets/models/car.js";
import { createPredictor, adoptCar } from "./predict.js";
import { stepLocal, leaveCar } from "./localsim.js";
import { createPhysics, newBody } from "../engine/physics.js";
import { CarBoardReach } from "./shared.gen.js";
import { CarSeatWorld, NewCar } from "../engine/sim.gen.js";
import { BTN } from "../engine/cmd.js";

const kart = JSON.parse(readFileSync(new URL("../../internal/content/maps/testdata/lab_kart.json", import.meta.url)));
const EAST = -Math.PI / 2;
// A car as the snapshot carries it (internal/game cars.go Car).
const snapCar = (id, k, p, extra = {}) => ({ id, k, p, y: EAST, pt: 0, rl: 0, v: [0, 0, 0], f: 0, st: 0, wc: 15, wz: 0x80808080, hp: carSpec(k)?.hp ?? 1, mhp: carSpec(k)?.hp ?? 1, ...extra });
// Our car as the self-snapshot carries it (cars.go carExact).
const cb = (k, p, extra = {}) => ({ k, p, v: [0, 0, 0], y: EAST, yv: 0, pt: 0, rl: 0, st: 0, g: true, wc: 15, wz: [0.5, 0.5, 0.5, 0.5], hp: carSpec(k).hp, ...extra });

test("the four types, each with a look; the map's cars on lab_kart", () => {
  assert.deepEqual(CAR_TYPES.map((c) => c.id), ["buggy", "racecar", "tank", "hovercraft"]);
  for (const c of CAR_TYPES) assert.ok(CAR_LOOK[c.id], c.id);
  assert.equal(carSpec("bus"), null);
  assert.deepEqual(kart.cars.map((c) => c.type), ["buggy", "racecar", "tank", "hovercraft"]);
});

test("placeholder models: wheels, tracks or a skirt per type, from the replicated state", () => {
  assert.deepEqual(unpackComp(0xff00ff80).map((v) => Math.round(v * 255)), [128, 255, 0, 255]);
  assert.deepEqual(unpackComp([0.1, 0.2, 0.3, 0.4]), [0.1, 0.2, 0.3, 0.4]);
  const n = (id, s = {}) => carParts({ spec: carSpec(id), pos: [0, 0, 0], ...s }).length;
  assert.ok(n("buggy") > n("hovercraft") && n("tank") > n("hovercraft"));
  const steer = carParts({ spec: carSpec("buggy"), pos: [0, 0, 0], steer: 1, fwd: 10, time: 1, contact: 3 });
  const straight = carParts({ spec: carSpec("buggy"), pos: [0, 0, 0], steer: 0, fwd: 10, time: 1, contact: 3 });
  assert.notDeepEqual(steer[4].m, straight[4].m, "the front wheels turn with the steering");
  assert.deepEqual(steer.at(-1).m, straight.at(-1).m, "a rear wheel doesn't");
  for (const p of [...steer, ...carParts({ spec: carSpec("tank"), pos: [1, 2, 3], fwd: 5, time: 2 }), ...carParts({ spec: carSpec("hovercraft"), pos: [1, 2, 3], fwd: 5, time: 2 })]) {
    assert.ok(p.m.every(Number.isFinite) && p.color.length === 3);
  }
});

test("createCars: drawn ahead by their velocity, the other cars' discs, ours drawn where we predict it", () => {
  const cars = createCars();
  cars.ingest([snapCar(1, "buggy", [0, 0, 0], { v: [10, 0, 0] }), snapCar(2, "tank", [10, 0, 0]), snapCar(3, "bus", [0, 0, 0])], 1000);
  assert.deepEqual(cars.drawnPos(cars.list[0], 1100), [1, 0, 0]);
  assert.deepEqual(cars.drawnPos(cars.list[0], 5000), [1.5, 0, 0], "at most 0.15 s ahead");
  assert.equal(cars.discs(1).length, 3, "the tank's three discs (not ours, not an unknown type)");
  assert.equal(cars.discs(2).length, 3);
  const own = { id: 1, fwd: 3, car: { ...NewCar(carSpec("buggy"), [50, 0, 0], 0) } };
  const solids = cars.solids(1, 1000, own);
  assert.ok(solids.length > 10);
  assert.ok(solids.some((s) => Math.abs(s.m[12] - 50) < 3), "ours at its predicted place");
  cars.ingest(null, 0);
  assert.equal(cars.discs(), null);
});

test("the HUD: driving shows the speed in km/h and the car's HP; near a free car on foot, E", () => {
  const me = newBody([0, 0, 0]);
  assert.equal(carHud(null, []), null);
  assert.equal(carHud(me, [snapCar(1, "buggy", [10, 0, 0])], CarBoardReach), null);
  assert.deepEqual(carHud(me, [snapCar(1, "buggy", [2, 0, 0])], CarBoardReach), { role: "near", name: "Intern Buggy" });
  assert.equal(carHud(me, [snapCar(1, "buggy", [2, 0, 0], { d: 7 })], CarBoardReach), null, "someone drives it");
  Object.assign(me, { car: 2, carType: "tank", carState: simCar(cb("tank", [0, 0, 0], { v: [10, 0, 0], hp: 99 })) });
  assert.deepEqual(carHud(me, [], CarBoardReach), { role: "driver", name: "Legacy Tank", kmh: 36, hp: 99, maxHP: 250, front: null, rear: null, boost: 0, slip: false });
  // The equipment (the server's: me.carGear from the self-snapshot), the boost tank, slipping.
  me.carGear = { fi: "rollback_rockets", ri: "tech_debt", fn: 3, rn: 0, mhp: 375 };
  Object.assign(me.carState, { boost: 2.04, slip: 1 });
  assert.deepEqual(carHud(me, [], CarBoardReach), { role: "driver", name: "Legacy Tank", kmh: 36, hp: 99, maxHP: 375,
    front: { id: "rollback_rockets", name: "Rollback Rockets", n: 3, kind: "rocket" }, rear: { id: "tech_debt", name: "Tech Debt", n: 0, kind: "mine" }, boost: 2, slip: true });
  assert.equal(carHud(me, [], CarBoardReach, [{ min: [-1, -1, -1], max: [1, 4, 1] }]).pit, true, "in the pit lane: repairing");
  assert.equal(carHud(me, [], CarBoardReach, [{ min: [5, -1, -1], max: [9, 4, 1] }]).pit, undefined);
  me.dead = true;
  assert.equal(carHud(me, [], CarBoardReach), null);
  assert.deepEqual(carFields({ car: 0 }), {});
  assert.deepEqual(Object.keys(carFields(me)), ["car", "carPos", "carVel", "carYaw", "carGround", "carWet", "carHP", "carBoost"]);
});

test("a driver's replay drives the car with the pending commands (as the server), and an E press in them gets out on its roof", () => {
  const phys = createPhysics();
  phys.setMap(kart);
  const pr = createPredictor(phys);
  const me = newBody([-60, 0, 24]);
  for (let seq = 1; seq <= 50; seq++) pr.push({ seq, f: 1, r: 0, b: 0, yaw: EAST, pitch: 0, speed: 1 });
  pr.reconcile(me, { ack: 0, p: [-60, 0.35, 24], v: [0, 0, 0], g: true, c: true, car: 1, cb: cb("buggy", [-60, 0, 24]) }, 0.01, { watch: false });
  assert.equal(me.car, 1);
  assert.ok(me.carState.pos[0] > -59, "the car moved ahead");
  assert.deepEqual(me.pos, CarSeatWorld(carSpec("buggy"), me.carState));
  assert.equal(me.crouched, true);
  pr.push({ seq: 51, f: 0, r: 0, b: BTN.USE, yaw: EAST, pitch: 0, speed: 1 });
  const c = me.carState;
  pr.reconcile(me, { ack: 50, p: [...me.pos], v: [...c.vel], g: true, c: true, car: 1, cb: cb("buggy", [...c.pos], { v: [...c.vel], y: c.yaw, yv: c.yawVel, pt: c.pitch, rl: c.roll, st: c.steer, wz: [...c.comp] }) }, 0.01, { watch: false });
  assert.equal(me.car, 0, "out");
  assert.equal(me.crouched, false);
  assert.ok(me.pos[1] > 1, "on its roof, falling");
  adoptCar(me, { car: 3 });
  assert.equal(me.car, 0, "never a car without its state");
});

test("stepLocal: the driver drives (no walking events, the handbrake on jump); E gets out", () => {
  const phys = createPhysics();
  phys.setMap(kart);
  const me = newBody([-60, 0.35, 24]);
  Object.assign(me, { car: 1, carType: "buggy", carState: simCar(cb("buggy", [-60, 0, 24])) });
  const arsenal = { moveSpeed: () => 1, tick: () => null };
  for (let seq = 1; seq <= 100; seq++) stepLocal({ phys, arsenal, me, cmd: { seq, f: 1, r: 0, b: 0, yaw: EAST, pitch: 0 }, prevB: 0, dt: 0.01 });
  const { ev } = stepLocal({ phys, arsenal, me, cmd: { seq: 101, f: 1, r: 1, b: BTN.JUMP, yaw: EAST, pitch: 0 }, prevB: 0, dt: 0.01 });
  assert.equal(ev.jumped, false, "jump is the handbrake");
  assert.ok(me.carState.vel[0] > 10 && me.car === 1);
  stepLocal({ phys, arsenal, me, cmd: { seq: 102, f: 0, r: 0, b: BTN.USE, yaw: EAST, pitch: 0 }, prevB: 0, dt: 0.01 });
  assert.equal(me.car, 0);
  assert.equal(me.carState, null);
  Object.assign(me, { car: 1, carType: "buggy", carState: simCar(cb("buggy", [-60, 0, 24])) });
  leaveCar(phys, me);
  assert.equal(me.car, 0);
});
