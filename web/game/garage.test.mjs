import test from "node:test";
import assert from "node:assert/strict";
import { garageAllowed, garageView, moveFocus, focusTarget, SLOTS, fitOf, presentAngle, orbitEye, SLOT_FACING } from "./garage.js";
import { kitAnchors, kitParts } from "../assets/models/carkit.js";
import { BAYS, GARAGE_ROOM, garageProps } from "../assets/garage.js";
import { MATERIALS } from "../assets/materials/index.js";
import { identity } from "../engine/math.js";
import carsData from "../assets/cars.json" with { type: "json" };

const CARS = carsData.cars;

test("the garage opens on a race map before GO and over the results, for a racer; never in a live race", () => {
  const ok = { mode: "race", team: "vibe", dead: false };
  for (const phase of ["warmup", "freeze", "over"]) assert.equal(garageAllowed({ ...ok, phase }), "", phase);
  for (const phase of ["live", "matchover", "halftime"]) assert.match(garageAllowed({ ...ok, phase }), /closed|race/, phase);
  assert.match(garageAllowed({ ...ok, phase: "freeze", mode: "defuse" }), /race maps/);
  assert.match(garageAllowed({ ...ok, phase: "freeze", team: "spectator" }), /team/);
  assert.match(garageAllowed({ ...ok, phase: "freeze", dead: true }), /respawn/);
});

test("a stock garage: the buggy on the table, its built-ins fitted, every bay priced by what we can afford", () => {
  const v = garageView({ gar: null, carType: "buggy", money: 3000, shopOpen: true });
  assert.equal(v.own, "buggy");
  assert.equal(v.shown, "buggy");
  assert.ok(!v.previewing);
  assert.deepEqual(v.bays.map((b) => [b.id, b.state, b.shown]), [["buggy", "owned", true], ["racecar", "buy", false], ["tank", "poor", false], ["hovercraft", "poor", false]]);
  assert.deepEqual(v.slots.map((s) => s.id), SLOTS.map((s) => s.id));
  assert.deepEqual(v.slots.map((s) => s.id), ["front", "rear", "charges", "tank", "plating", "engine", "tyres"]);
  const front = v.slots.find((s) => s.id === "front");
  assert.equal(front.fitted, "Linter Beam");
  assert.deepEqual(front.options.map((o) => [o.buy, o.state]), [["front:linter_beam", "owned"], ["front:hotfix_missiles", "buy"], ["front:rollback_rockets", "buy"], ["front:synergy_orbs", "buy"], ["front:force_push", "buy"]]);
  const eng = v.slots.find((s) => s.id === "engine");
  assert.deepEqual([eng.tier, eng.fitted, eng.options.map((o) => [o.name, o.price, o.state])], [0, "stock", [["Engine I", 700, "buy"]]]);
  assert.deepEqual(v.card, fitOf(CARS[0], {}, "linter_beam", "breaking_changes"));
  assert.deepEqual([v.card.topKmh, v.card.hp, v.card.tank, v.card.front.charges, v.card.rear.charges], [86, 100, 2.5, 40, 3]);
});

test("upgrades show on the card with the server's numbers (car_shared.go Fit*): plating, engine, tank, charge packs", () => {
  const v = garageView({ gar: { car: "tank", f: "force_push", pl: 2, en: 3, tk: 2, ch: 1, ty: 1 }, money: 0, shopOpen: true });
  assert.equal(v.own, "tank");
  assert.equal(v.card.hp, 375); // 250 x (1 + 0.25 x 2)
  assert.equal(v.card.topKmh, Math.round(27 * 1.06 * 3.6));
  assert.equal(v.card.tank, 5.5); // 2.5 + 1.5 x 2
  assert.equal(v.card.front.name, "Force-Push Ram Plate");
  assert.equal(v.card.rear.charges, 5); // Tech Debt 3 x 1.5, rounded
  const eng = v.slots.find((s) => s.id === "engine");
  assert.deepEqual([eng.tier, eng.fitted, eng.options], [3, "tier III", []]); // maxed: nothing to buy
  const pl = v.slots.find((s) => s.id === "plating");
  assert.deepEqual(pl.options.map((o) => [o.name, o.state]), [["Plating III", "poor"]]);
  assert.equal(v.slots.find((s) => s.id === "front").options.find((o) => o.buy === "front:rollback_rockets").state, "free"); // the tank's own built-in back: free
});

test("the hovercraft has no tyres to sell; a previewed car comes with its own built-ins and our tiers", () => {
  const v = garageView({ gar: { car: "hovercraft", ty: 2 }, money: 9000, shopOpen: true });
  const ty = v.slots.find((s) => s.id === "tyres");
  assert.ok(ty.none);
  assert.deepEqual(ty.options, []);
  assert.equal(v.card.tiers.tyres, 0);
  const p = garageView({ gar: { car: "buggy", f: "force_push", pl: 1 }, money: 9000, shopOpen: true, preview: "racecar" });
  assert.ok(p.previewing);
  assert.equal(p.shown, "racecar");
  assert.equal(p.card.front.name, "Hotfix Missiles");
  assert.equal(p.card.hp, 100); // 80 x 1.25
  assert.deepEqual(p.bays.find((b) => b.id === "racecar"), { ...p.bays.find((b) => b.id === "racecar"), shown: true, state: "buy", buy: "car:racecar" });
  assert.equal(garageView({ gar: { car: "buggy" }, preview: "buggy" }).previewing, false); // previewing our own: none
});

test("keys: left/right walk the bays, down to the parts and into one's options, Enter buys, up/backspace back", () => {
  const v = garageView({ gar: null, carType: "buggy", money: 3000, shopOpen: true });
  let f = { row: "bays", i: 0, slot: "" };
  assert.deepEqual(focusTarget(v, f), null); // our own car on the table: nothing to do
  f = moveFocus(v, f, "ArrowRight");
  assert.deepEqual(focusTarget(v, f), { kind: "preview", id: "racecar" }); // not on the table yet: it rolls on first
  assert.deepEqual(moveFocus(v, f, "ArrowLeft").i, 0);
  assert.deepEqual(moveFocus(v, { row: "bays", i: 0 }, "ArrowLeft").i, 3); // wraps
  const p = garageView({ gar: null, carType: "buggy", money: 3000, shopOpen: true, preview: "racecar" });
  assert.deepEqual(focusTarget(p, { row: "bays", i: 1 }), { kind: "buy", item: "car:racecar" }); // on the table: Enter buys it
  f = moveFocus(v, { row: "bays", i: 0 }, "ArrowDown");
  assert.deepEqual(f, { row: "slots", i: 0, slot: "" });
  f = moveFocus(v, moveFocus(v, f, "ArrowRight"), "Enter");
  assert.deepEqual(f, { row: "options", i: 0, slot: "rear" });
  assert.equal(focusTarget(v, f), null); // the fitted one: owned, nothing to buy
  f = moveFocus(v, f, "ArrowRight");
  assert.deepEqual(focusTarget(v, f), { kind: "buy", item: "rear:spaghetti_code" });
  assert.deepEqual(moveFocus(v, f, "Backspace"), { row: "slots", i: 1, slot: "" });
  assert.deepEqual(moveFocus(v, { row: "slots", i: 2 }, "ArrowUp"), { row: "bays", i: 0, slot: "" });
});

test("the kit: an anchor for every slot on every type, parts tagged by slot, more parts a tier, the pointed slot lit", () => {
  for (const sp of CARS) {
    const A = kitAnchors(sp);
    for (const s of SLOTS) assert.ok(A[s.id]?.every(Number.isFinite), `${sp.id} ${s.id}`);
    for (const s of SLOTS) assert.ok(SLOT_FACING[s.id], s.id);
    const stock = kitParts(sp, fitOf(sp, {}, sp.front, sp.rear), identity());
    const full = kitParts(sp, fitOf(sp, { ch: 3, tk: 3, pl: 3, en: 3, ty: 3 }, sp.front, sp.rear), identity());
    assert.ok(full.length > stock.length, sp.id);
    assert.ok(full.every((p) => SLOTS.some((s) => s.id === p.slot)), sp.id);
    for (const s of ["plating", "engine", "charges"]) assert.ok(!stock.some((p) => p.slot === s) && full.some((p) => p.slot === s), `${sp.id} ${s}`);
    const lit = kitParts(sp, fitOf(sp, { pl: 1 }, sp.front, sp.rear), identity(), { glow: "plating" });
    assert.ok(lit.filter((p) => p.slot === "plating").every((p) => p.emit >= 0.35));
    assert.ok(lit.filter((p) => p.slot === "tank").every((p) => p.emit < 0.9));
  }
  // Every front and rear kind has a look of its own.
  const sp = CARS[0], kinds = new Map();
  for (const it of carsData.equipment) kinds.set(it.id, kitParts(sp, fitOf(sp, {}, it.slot === "front" ? it.id : sp.front, it.slot === "rear" ? it.id : sp.rear), identity()).filter((p) => p.slot === it.slot).length);
  for (const [id, n] of kinds) assert.ok(n > 1, id);
});

test("the room: known materials, a bay per car type, the turntable turning; a picked part turns to face the camera", () => {
  const names = new Set(MATERIALS.map((m) => m.name));
  for (const b of GARAGE_ROOM.boxes) assert.ok(names.has(b.kind), b.kind);
  assert.ok(names.has(GARAGE_ROOM.signs[0].mat));
  assert.equal(BAYS.length, CARS.length);
  assert.notDeepEqual(garageProps(0)[0].m, garageProps(1)[0].m);
  // presentAngle: the slot's facing, turned by the table's angle, points at the camera.
  for (const s of Object.keys(SLOT_FACING)) {
    for (const cam of [0, 1.2, -2]) {
      const a = presentAngle(s, cam), [fx, fz] = SLOT_FACING[s];
      assert.ok(Math.abs(Math.cos(a + Math.atan2(fx, fz) - cam) - 1) < 1e-9, `${s} ${cam}`);
    }
  }
  assert.equal(presentAngle("", 0), null);
  const o = orbitEye([0, 1, 0], 0, 0.3, 10);
  assert.ok(o.eye[2] > 9 && o.eye[1] > 1 && o.pitch < 0);
});
