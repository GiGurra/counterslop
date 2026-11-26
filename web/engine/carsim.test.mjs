// The car sim as the client runs it (internal/sim car.go, generated into
// sim.gen.js; physics.js stepCar): every type of web/assets/cars.json on a
// small test yard with a ramp, a bank, a wall, water and another car. Go
// pins the bits (internal/sim TestCarParityWithJS); this pins the behaviour
// on the client's side.
import test from "node:test";
import assert from "node:assert/strict";
import { createPhysics } from "./physics.js";
import { NewCar, CarSpeed, CarForward, CarDiscs, CarSeatWorld } from "./sim.gen.js";
import carsData from "../assets/cars.json" with { type: "json" };

const SPECS = Object.fromEntries(carsData.cars.map((c) => [c.id, c]));
const EAST = -Math.PI / 2; // facing +x
// A 200 x 100 m floor (top y 0) with a wall at z 20, a ramp (x 0..14, rising
// 2.2 m towards +x, z -10..10, then a drop), a bank (x 60..80 rising 4 m) and
// a water basin (x -60..-40, bed y -2, surface -0.4) entered on slopes.
const yard = () => ({
  boxes: [
    { min: [-100, -3, -50], max: [-60, 0, 50], kind: "concrete" },
    { min: [-40, -3, -50], max: [100, 0, 50], kind: "concrete" },
    { min: [-60, -3, -50], max: [-40, -2, 50], kind: "concrete" },
    { min: [-60, -2, -50], max: [-54, 0, 50], kind: "concrete", ramp: 2 },
    { min: [-46, -2, -50], max: [-40, 0, 50], kind: "concrete", ramp: 1 },
    { min: [-100, 0, 20], max: [100, 4, 21], kind: "brick" },
    { min: [0, 0, -10], max: [14, 2.2, 10], kind: "metal", ramp: 1 },
    { min: [60, 0, -50], max: [80, 4, 20], kind: "concrete", ramp: 1 },
  ],
  water: [{ min: [-60, -2, -50], max: [-40, -0.4, 50], kind: "water" }],
});
function drive(type, pos, yaw, inp, n, others = null) {
  const phys = createPhysics();
  phys.setMap(yard());
  const sp = SPECS[type], c = NewCar(sp, [...pos], yaw);
  const trace = [];
  for (let i = 0; i < n; i++) {
    phys.stepCar(sp, c, typeof inp === "function" ? inp(i) : inp, 0.01, others);
    trace.push({ y: c.pos[1], g: c.onGround, pitch: c.pitch, wet: c.wet });
  }
  return { c, sp, trace, phys };
}

test("every type gets to its top speed on the flat and slows down coasting", () => {
  for (const type of Object.keys(SPECS)) {
    const { c, sp } = drive(type, [-30, 0, -30], EAST, { throttle: 1, steer: 0, handbrake: false }, 400);
    assert.ok(CarForward(c) > sp.topSpeed - 0.1 && CarForward(c) <= sp.topSpeed + 1e-9, `${type}: ${CarForward(c)}`);
    const phys = createPhysics(); phys.setMap(yard());
    for (let i = 0; i < 100; i++) phys.stepCar(sp, c, { throttle: 0, steer: 0, handbrake: false }, 0.01);
    assert.ok(CarSpeed(c) < sp.topSpeed - 0.1, `${type} not slowing`);
  }
});

test("braking, reversing, steering, the handbrake slide and the tank's pivot", () => {
  const brake = drive("racecar", [-30, 0, -30], EAST, (i) => ({ throttle: i < 150 ? 1 : -1, steer: 0, handbrake: false }), 400).c;
  assert.ok(CarForward(brake) < -1, "braked, then reversing");
  const turn = drive("buggy", [-30, 0, -30], EAST, (i) => ({ throttle: 1, steer: i > 100 ? 1 : 0, handbrake: i > 140 }), 180).c;
  assert.ok(turn.yaw < EAST - 0.5, `turned right: ${turn.yaw}`);
  const reverseTurn = drive("buggy", [-30, 0, -30], EAST, { throttle: -1, steer: 1, handbrake: false }, 200).c;
  assert.ok(reverseTurn.yaw > EAST, "reversing swaps the steering");
  const still = drive("buggy", [-30, 0, -30], EAST, { throttle: 0, steer: 1, handbrake: false }, 100).c;
  assert.equal(still.yaw, EAST, "wheels don't turn standing");
  const pivot = drive("tank", [-30, 0, -30], EAST, { throttle: 0, steer: -1, handbrake: false }, 100).c;
  assert.ok(pivot.yaw > EAST + 1 && CarSpeed(pivot) < 1e-9, "the tank pivots on the spot");
  const hover = drive("hovercraft", [-30, 0, -30], EAST, (i) => ({ throttle: 1, steer: i > 150 ? 1 : 0, handbrake: i > 200 }), 260).c;
  const lat = Math.abs(hover.vel[0] * Math.cos(hover.yaw) - hover.vel[2] * Math.sin(hover.yaw));
  assert.ok(lat > 3, `the hovercraft drifts: ${lat}`);
});

test("off the ramp it flies and lands; the bank tilts it; over nothing it falls", () => {
  const { trace } = drive("buggy", [-40, 0, 0], EAST, { throttle: 1, steer: 0, handbrake: false }, 400);
  assert.ok(Math.max(...trace.map((t) => t.y)) > 2.2 && trace.some((t) => !t.g) && trace.at(-1).g, "a jump");
  assert.ok(Math.max(...trace.map((t) => t.pitch)) > 0.1, "nose up on the ramp");
  const bank = drive("racecar", [70, 2, 0], 0, { throttle: 1, steer: 0, handbrake: false }, 30).c;
  assert.ok(bank.roll < -0.1 && bank.onGround, `banked: roll ${bank.roll}`);
  const fall = drive("buggy", [150, 0, 0], EAST, { throttle: 0, steer: 0, handbrake: false }, 100).c;
  assert.ok(!fall.onGround && fall.pos[1] < -5, "falls off the edge");
});

test("water: the hovercraft floats over it, a buggy bogs down in it", () => {
  const h = drive("hovercraft", [-20, 0, 0], Math.PI / 2, { throttle: 1, steer: 0, handbrake: false }, 300);
  assert.ok(h.c.pos[0] < -60 && Math.min(...h.trace.map((t) => t.y)) >= -0.4 - 1e-9, `hover at ${h.c.pos}`);
  const b = drive("buggy", [-20, 0, 0], Math.PI / 2, { throttle: 1, steer: 0, handbrake: false }, 300);
  assert.ok(b.trace.some((t) => t.wet) && CarSpeed(b.c) < 5, "the buggy wades");
});

test("walls and other cars stop it; the driver's seat moves with it", () => {
  const w = drive("tank", [-20, 0, 0], 0 + Math.PI, { throttle: 1, steer: 0, handbrake: false }, 300).c; // facing +z, into the wall at z 20
  assert.ok(w.pos[2] < 20 - SPECS.tank.size[0] / 2 + 1e-6 && CarSpeed(w) < 0.5, `wall: ${w.pos}`);
  const parked = NewCar(SPECS.buggy, [-10, 0, -30], 0);
  const hit = drive("buggy", [-30, 0, -30], EAST, { throttle: 1, steer: 0, handbrake: false }, 200, CarDiscs(SPECS.buggy, parked, null)).c;
  assert.ok(hit.pos[0] < -10 - SPECS.buggy.size[0] + 1e-6, `through the other car: ${hit.pos}`);
  const seat = CarSeatWorld(SPECS.buggy, hit);
  assert.ok(Math.hypot(seat[0] - hit.pos[0], seat[2] - hit.pos[2]) < 1 && seat[1] > hit.pos[1], "the seat");
});
