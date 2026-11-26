// Cars on the client (internal/sim car.go, internal/game cars.go; the
// server's snapshot "cars"): each drawn where its last snapshot put it, run
// ahead by its velocity for the time since (at most 0.15 s); the one we drive
// where we predict it. The types are web/assets/cars.json (the same objects
// the generated sim steps).
//   createCars()  .ingest(list, now) .list .drawnPos(c, now) .discs(now, skipId) .solids(time, now, own)
import carsData from "../assets/cars.json" with { type: "json" };
import { carParts } from "../assets/models/car.js";
import { CarDiscs } from "../engine/sim.gen.js";

export const CAR_TYPES = carsData.cars;
export const carSpec = (id) => CAR_TYPES.find((c) => c.id === id) || null;
// The car equipment (cars.json "equipment", internal/game carcombat.go) and the race shop ("shop", carshop.go).
export const CAR_ITEMS = carsData.equipment || [];
export const CAR_SHOP = carsData.shop || { upgrades: {} };
export const carItem = (id) => CAR_ITEMS.find((it) => it.id === id) || null;

// Our own car from a self-snapshot's "cb" (cars.go carExact) as sim.Car.
export const simCar = (cb) => ({ pos: [...cb.p], vel: [...cb.v], yaw: cb.y, yawVel: cb.yv, pitch: cb.pt, roll: cb.rl, steer: cb.st, onGround: !!cb.g, contact: cb.wc | 0, comp: [...cb.wz], hp: cb.hp | 0, wet: !!cb.wet,
  boost: cb.bo || 0, slip: cb.sl || 0, engine: cb.en | 0, tyres: cb.ty | 0 });

export function createCars() {
  let live = [], at = 0;
  return {
    get list() { return live; },
    ingest(list, now) { live = list || []; at = now; },
    // Where car c is drawn at now (ms).
    drawnPos(c, now) {
      const dt = Math.min(0.15, Math.max(0, (now - at) / 1000));
      return [c.p[0] + c.v[0] * dt, c.p[1] + c.v[1] * dt, c.p[2] + c.v[2] * dt];
    },
    // The other cars' discs (all but skipId) as the newest snapshot has them:
    // what our predicted car bumps into (= server carDiscs; null: none).
    discs(skipId = 0) {
      let out = null;
      for (const c of live) { const sp = carSpec(c.k); if (sp && c.id !== skipId) out = CarDiscs(sp, { pos: c.p, yaw: c.y }, out); }
      return out;
    },
    // own: {id, car: sim.Car} the car we drive, drawn there instead (predicted).
    solids(time, now, own = null) {
      return live.flatMap((c) => {
        const spec = carSpec(c.k);
        if (!spec) return [];
        if (own && own.id === c.id) { const o = own.car; return carParts({ spec, pos: o.pos, yaw: o.yaw, pitch: o.pitch, roll: o.roll, steer: o.steer, fwd: own.fwd, comp: o.comp, contact: o.contact, time, id: c.id }); }
        return carParts({ spec, pos: this.drawnPos(c, now), yaw: c.y, pitch: c.pt, roll: c.rl, steer: c.st, fwd: c.f, comp: c.wz, contact: c.wc, time, id: c.id });
      });
    },
  };
}

// What the HUD shows about cars (ui/hud.js CarHud): driving: the type, the
// speed (km/h) and the car's HP; on foot near a free car: the E hint. Pure.
// me: our body (car, carState, carType, dead); cars: the snapshot's list;
// pitLanes: the map's (a car in one is repaired: pit).
export function carHud(me, cars, reach, pitLanes = []) {
  if (!me || me.dead) return null;
  if (me.car && me.carState) {
    const sp = carSpec(me.carType), c = me.carState, g = me.carGear || {};
    const item = (id, n) => (id ? { id, name: carItem(id)?.name || id, n: n | 0, kind: carItem(id)?.kind || "" } : null);
    return { role: "driver", name: sp?.name || me.carType, kmh: Math.round(Math.hypot(c.vel[0], c.vel[2]) * 3.6), hp: c.hp, maxHP: g.mhp || sp?.hp || c.hp,
      front: item(g.fi, g.fn), rear: item(g.ri, g.rn), boost: Math.round(c.boost * 10) / 10, slip: c.slip > 0,
      ...((pitLanes || []).some((z) => c.pos.every((v, i) => v >= z.min[i] && v <= z.max[i])) ? { pit: true } : {}) };
  }
  for (const c of cars || []) {
    if (!c.d && Math.hypot(c.p[0] - me.pos[0], c.p[2] - me.pos[2]) < reach && Math.abs(c.p[1] - me.pos[1]) < 2.5) return { role: "near", name: carSpec(c.k)?.name || c.k };
  }
  return null;
}

// The car we drive in sim/cases terms (= server cars.go carCaseState; {} on foot).
export function carFields(me) {
  const r4 = (v) => Math.round(v * 1e4) / 1e4;
  const c = me.car && me.carState;
  if (!c) return {};
  return { car: me.car, carPos: c.pos.map(r4), carVel: c.vel.map(r4), carYaw: r4(c.yaw), carGround: c.onGround, carWet: c.wet, carHP: c.hp, carBoost: r4(c.boost || 0) };
}
