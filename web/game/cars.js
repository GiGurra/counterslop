// Cars on the client (internal/sim car.go, internal/game cars.go; the
// server's snapshot "cars"): each drawn where its last snapshot put it, run
// ahead by its velocity for the time since (at most 0.15 s); the one we drive
// where we predict it. The types are web/assets/cars.json (the same objects
// the generated sim steps).
//   createCars()  .ingest(list, now) .list .drawnPos(c, now) .discs(now, skipId) .solids(time, now, own, aimOf)
// Each car's animation (wheel spin, track scroll, caster swivel, lean, fans,
// flames: models/car.js stepCarAnim) is advanced once a frame in solids().
import carsData from "../assets/cars.json" with { type: "json" };
import { carParts, newCarAnim, stepCarAnim, damageParts, charred, FX_THROTTLE, FX_BOOST, FX_SLIP, FX_BRAKE } from "../assets/models/car.js";
import { BTN } from "../engine/cmd.js";
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

// A car's parts with its damage on them: smoke, fire; a wreck charred and burning.
function withDamage(parts, d) {
  const smoke = damageParts(d);
  return d.hp <= 0 ? [...charred(parts), ...smoke] : smoke.length ? [...parts, ...smoke] : parts;
}

// The looks bits of our own car under our command (= server cars.go carFx).
export function carFxOf(car, f, b) {
  let fx = 0;
  if (f > 0) fx |= FX_THROTTLE;
  if (b & BTN.BOOST && car.boost > 0) fx |= FX_BOOST | FX_THROTTLE;
  if (car.slip > 0) fx |= FX_SLIP;
  if (f < 0 || b & BTN.JUMP) fx |= FX_BRAKE;
  return fx;
}

// deps (sounds; all optional): engine(sp, kind) -> {update(sp), stop()} | null,
// spatial(pos) -> {pan, gain}, sfx {carBoostOn(sp), carPit(sp)}.
export const ENGINE_RANGE = 45; // m: engines further away aren't heard (nor started)
export function createCars({ engine = null, spatial = null, sfx = {} } = {}) {
  let live = [], at = 0, animAt = null, pitAt = -Infinity;
  const engines = new Map(); // car id -> its engine's sound handle
  const boosting = new Map(); // car id -> boosting last frame (the boost's whump on the edge)
  const anims = new Map(); // car id -> its animation phases
  const animOf = (id) => { let a = anims.get(id); if (!a) anims.set(id, (a = newCarAnim())); return a; };
  return {
    get list() { return live; },
    anim: (id) => anims.get(id) || null,
    get engines() { return engines.size; },
    // Every frame: start, rev and stop the engines (listener: where we hear from;
    // null: nowhere); own: as in solids (our car's speed and looks bits);
    // pit: our car is being repaired in a pit lane (the impact wrench, now and then).
    sound(listener, now, own = null, pit = false) {
      const seen = new Set();
      for (const c of live) {
        const spec = carSpec(c.k);
        if (!spec || !listener) continue;
        const mine = own && own.id === c.id, p = mine ? own.car.pos : c.p;
        const d = Math.hypot(p[0] - listener[0], p[1] - listener[1], p[2] - listener[2]);
        const fx = mine ? own.fx | 0 : c.fx | 0, fwd = mine ? own.fwd : c.f;
        const boost = (fx & FX_BOOST) !== 0;
        if (boost && !boosting.get(c.id) && d < ENGINE_RANGE) sfx.carBoostOn?.(spatial ? spatial(p) : undefined);
        boosting.set(c.id, boost);
        if (!engine || c.hp <= 0 || d > ENGINE_RANGE || (!c.d && !mine)) continue; // a wreck or an empty car: quiet
        seen.add(c.id);
        const sp = spatial ? spatial(p) : { pan: 0, gain: 1 }, rev = Math.min(1, Math.abs(fwd) / (spec.topSpeed || 20));
        const out = { pan: mine ? 0 : sp.pan, gain: (mine ? 0.5 : sp.gain) * (0.3 + 0.3 * rev + (boost ? 0.15 : 0)) * Math.min(1, (ENGINE_RANGE - d) / 8), rev, boost: boost ? 1 : 0 };
        const h = engines.get(c.id);
        if (h) h.update(out);
        else { const n = engine(out, spec.id); if (n) engines.set(c.id, n); }
      }
      for (const [id, h] of engines) if (!seen.has(id)) { h.stop(); engines.delete(id); }
      for (const id of boosting.keys()) if (!live.some((c) => c.id === id)) boosting.delete(id);
      if (pit && now - pitAt > 700) { pitAt = now; sfx.carPit?.(); }
    },
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
    // own: {id, car: sim.Car, fwd, fx} the car we drive, drawn there instead (predicted);
    // aimOf(driverId): that driver's view yaw (a tank's turret follows it) or null.
    solids(time, now, own = null, aimOf = () => null) {
      const dt = animAt == null ? 0 : (now - animAt) / 1000;
      animAt = now;
      for (const id of anims.keys()) if (!live.some((c) => c.id === id)) anims.delete(id);
      return live.flatMap((c) => {
        const spec = carSpec(c.k);
        if (!spec) return [];
        const a = animOf(c.id), aim = c.d ? aimOf(c.d) : null;
        if (own && own.id === c.id) {
          const o = own.car;
          stepCarAnim(a, { spec, fwd: own.fwd, yawVel: o.yawVel, steer: o.steer, vel: o.vel, yaw: o.yaw, fx: own.fx | 0 }, dt);
          return withDamage(carParts({ spec, pos: o.pos, yaw: o.yaw, pitch: o.pitch, roll: o.roll, steer: o.steer, fwd: own.fwd, comp: o.comp, contact: o.contact, time, id: c.id, anim: a, aim }),
            { spec, pos: o.pos, yaw: o.yaw, vel: o.vel, hp: o.hp, maxHP: c.mhp, time, id: c.id });
        }
        stepCarAnim(a, { spec, fwd: c.f, yawVel: c.yv || 0, steer: c.st, vel: c.v, yaw: c.y, fx: c.fx | 0 }, dt);
        const pos = this.drawnPos(c, now);
        return withDamage(carParts({ spec, pos, yaw: c.y, pitch: c.pt, roll: c.rl, steer: c.st, fwd: c.f, comp: c.wz, contact: c.wc, time, id: c.id, anim: a, aim }),
          { spec, pos, yaw: c.y, vel: c.v, hp: c.hp, maxHP: c.mhp, time, id: c.id });
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
