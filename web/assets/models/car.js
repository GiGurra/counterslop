// The car models (docs/racing.md phase 4): one box-built model per type of
// web/assets/cars.json, animated from the replicated car state only (no
// gameplay state of their own):
//   Intern Buggy    a tube-frame go-kart with an office chair bolted on, swivel
//                   casters for wheels (they turn towards where it's going), a
//                   rubber-duck mascot and a whip aerial
//   Senior Racecar  a wedge with a front and a rear wing, an airbox, slicks and
//                   twin exhausts that light up with the throttle
//   Legacy Tank     a box hull with a turret (it turns with the driver's aim), a
//                   rocket pod, and a track down each side that scrolls at its
//                   side's speed (turning: the sides run apart), road wheels turning
//   Series-A Hovercraft  an inflated skirt that wobbles, two caged fans at the
//                   back that spin with the throttle and rudders that steer
// Every type: the body rides the four suspension values (Comp: bob, roll and
// pitch over the wheels) and leans into turns; the wheels spin with the
// ground speed and the front ones steer; jet flames scale with the throttle
// and grow with the boost.
// Model space: the footprint's centre on the ground at the origin, the nose
// towards -z (as a player faces).
//   carParts({spec, pos, yaw, pitch, roll, steer, fwd, comp, contact, time, id, anim, aim})
//   newCarAnim() / stepCarAnim(a, s, dt)   the per-car animation phases (game/cars.js keeps them)
//   damageParts({spec, pos, yaw, vel, hp, maxHP, time, id})  smoke below half HP, then fire; a wreck burns
//   charred(parts)  a wreck's parts, burnt black
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";

// The look of each type: body, trim and accent colours (palette names).
export const CAR_LOOK = {
  buggy: { body: "toyYellow", trim: "caseBlack", accent: "neonLime" },
  racecar: { body: "carRed", trim: "steel", accent: "toyPink" },
  tank: { body: "containerGreen", trim: "gunmetal", accent: "hazard" },
  hovercraft: { body: "neonCyan", trim: "caseBlack", accent: "neonMagenta" },
};

// Snapshot "fx" bits (internal/game cars.go carFx): what the car's driver does
// this tick, for the looks only.
export const FX_THROTTLE = 1, FX_BOOST = 2, FX_SLIP = 4, FX_BRAKE = 8;

// comp: the four wheels' suspension packed a byte each (internal/game cars.go
// CarComp) or an array of four 0..1 values (our own predicted car). Returns 0..1 per wheel.
export function unpackComp(comp) {
  if (Array.isArray(comp)) return comp;
  const n = comp | 0;
  return [0, 1, 2, 3].map((i) => ((n >>> (8 * i)) & 255) / 255);
}

// The animation phases of one car, advanced every frame by stepCarAnim.
export function newCarAnim() {
  return { wheel: 0, trackL: 0, trackR: 0, swivel: 0, lean: 0, fan: 0, flame: 0, boost: 0, last: null };
}

// Advances a by dt (s) for a car in state s: {spec, fwd (m/s along the
// heading), yawVel (rad/s), steer, vel ([x, y, z]), yaw, fx}. Pure but for a.
export function stepCarAnim(a, s, dt) {
  dt = Math.max(0, Math.min(0.1, dt || 0));
  const sp = s.spec || {}, fwd = s.fwd || 0, yv = s.yawVel || 0, fx = s.fx | 0;
  const r = wheelRadius(sp);
  a.wheel = (a.wheel + (fwd / r) * dt) % (Math.PI * 2);
  const half = (sp.track || 2) / 2; // a track's speed: the centre's plus the turn's at its side (yaw grows turning left)
  a.trackL += (fwd - yv * half) * dt;
  a.trackR += (fwd + yv * half) * dt;
  // Casters trail the way the car moves over the ground (its slip angle; reversing: they flip round).
  const v = s.vel || [0, 0, 0], sx = Math.cos(s.yaw || 0), sz = -Math.sin(s.yaw || 0);
  const side = v[0] * sx + v[2] * sz, speed = Math.hypot(v[0], v[2]);
  const want = speed > 0.3 ? Math.atan2(side, fwd) : a.swivel;
  a.swivel += angleDiff(want, a.swivel) * Math.min(1, dt * 8);
  // The lean into a turn: the turn's sideways pull (yaw rate x speed), eased.
  const pull = Math.max(-1, Math.min(1, (yv * fwd) / 60));
  a.lean += (pull - a.lean) * Math.min(1, dt * 6);
  a.fan = (a.fan + dt * (8 + Math.min(30, Math.abs(fwd)) * 1.2 + (fx & FX_THROTTLE ? 14 : 0))) % (Math.PI * 2);
  const flameWant = fx & FX_BOOST ? 1 : fx & FX_THROTTLE ? 0.45 : 0.08;
  a.flame += (flameWant - a.flame) * Math.min(1, dt * 12);
  a.boost += ((fx & FX_BOOST ? 1 : 0) - a.boost) * Math.min(1, dt * 8);
  return a;
}

const angleDiff = (a, b) => Math.atan2(Math.sin(a - b), Math.cos(a - b));
export const wheelRadius = (spec) => Math.max(0.15, (spec.ride || 0.3) * 0.9 + (spec.travel || 0.2) * 0.4);

// spec: the car's cars.json entry; the rest is its state (snapshot or predicted);
// anim: its phases (newCarAnim; at rest when left out); aim: the driver's view yaw (the tank's turret).
export function carParts({ spec, pos, yaw = 0, pitch = 0, roll = 0, steer = 0, fwd = 0, comp = 0, contact = 15, time = 0, id = 1, anim = null, aim = null }) {
  const a = anim || newCarAnim();
  const look = CAR_LOOK[spec.id] || CAR_LOOK.buggy;
  const col = (n) => P[n] || P.gunmetal;
  const [w, h, l] = spec.size, ride = spec.ride, travel = spec.travel;
  const kind = spec.drive === "hover" ? "hover" : spec.drive === "tracks" ? "tracks" : spec.id === "buggy" ? "buggy" : "wheels";
  const comps = unpackComp(comp).map((c, i) => ((contact >> i) & 1 ? c : 0));
  // The body over its suspension: up and down with the mean, rolled by the
  // sides' difference, pitched by front against back, plus the lean into turns.
  const sus = kind === "hover" ? 0.5 : (comps[0] + comps[1] + comps[2] + comps[3]) / 4;
  const bodyBob = kind === "hover" ? 0.05 * Math.sin(time * 4.5 + id) : (sus - 0.5) * travel * 0.6;
  const bodyRoll = (kind === "hover" ? 0.03 * Math.sin(time * 3.1 + id * 2) : ((comps[0] + comps[2]) - (comps[1] + comps[3])) * 0.04) + a.lean * (kind === "tracks" ? 0.03 : 0.08);
  const bodyPitch = kind === "hover" ? 0.02 * Math.sin(time * 2.3 + id) : ((comps[0] + comps[1]) - (comps[2] + comps[3])) * 0.03;
  const base = chain(translate(pos[0], pos[1], pos[2]), rotY(yaw), rotX(pitch), rotZ(-roll));
  const body = chain(base, translate(0, bodyBob, 0), rotZ(-bodyRoll), rotX(bodyPitch));
  const out = [];
  const add = (frame, x, y, z, sx, sy, sz, c, emit = 0, extra = null) => out.push({ m: chain(frame, translate(x, y, z), ...(extra ? [extra] : []), scale(sx, sy, sz)), color: typeof c === "string" ? col(c) : c, emit });
  const B = (...args) => add(body, ...args), G = (...args) => add(base, ...args);
  const r = wheelRadius(spec);
  const xs = [-spec.track / 2, spec.track / 2, -spec.track / 2, spec.track / 2], zs = [-spec.wheelbase / 2, -spec.wheelbase / 2, spec.wheelbase / 2, spec.wheelbase / 2];
  const wheelY = (i) => r + (comps[i] - 0.5) * travel; // compressed: the wheel rides up
  const turn = -steer * 0.45;
  // A wheel in frame f at (x, y, z), width wd, radius rr, rolled by the spin:
  // an octagon (two squares 45 degrees apart), a coloured hub and a spoke that shows the turn.
  const wheel = (f, x, y, z, wd, rr, hub = "steel") => {
    const k = 2 * rr * 0.83;
    add(f, x, y, z, wd, k, k, "tyre", 0, rotX(-a.wheel));
    add(f, x, y, z, wd, k, k, "tyre", 0, rotX(-a.wheel + Math.PI / 4));
    const side = Math.sign(x || 1) * (wd / 2 + 0.005);
    add(f, x + side, y, z, 0.02, rr * 0.9, rr * 0.9, hub, 0, rotX(-a.wheel + Math.PI / 4));
    add(f, x + side * 1.4, y, z, 0.02, rr * 1.5, rr * 0.18, hub === "steel" ? "gunmetal" : "steel", 0, rotX(-a.wheel));
  };
  // A jet flame out of the back at (x, y, z): its length with the throttle, longer and hotter boosting.
  const flame = (frame, x, y, z, size) => {
    const k = a.flame, flick = 0.85 + 0.15 * Math.sin(time * 41 + x * 7 + id);
    if (k < 0.05) return;
    const len = size * (0.6 + 2.4 * k) * flick;
    add(frame, x, y, z + len / 2, size * 0.7, size * 0.7, len, a.boost > 0.5 ? "neonCyan" : "sunGlow", 1);
    add(frame, x, y, z + len * 0.25, size * 0.4, size * 0.4, len * 0.5, "sunCore", 1);
  };

  if (kind === "buggy") {
    // The frame: two side rails, cross bars, a bumper; an office chair on a post; casters at each corner.
    const fy = ride + 0.05;
    for (const s of [-1, 1]) {
      B(s * w * 0.32, fy, 0, 0.08, 0.08, l * 0.9, look.trim);
      B(s * w * 0.32, fy + 0.35, -l * 0.3, 0.06, 0.7, 0.06, look.trim, 0, rotX(-0.5)); // the front hoop's legs
    }
    for (const z of [-l * 0.42, 0, l * 0.4]) B(0, fy, z, w * 0.7, 0.08, 0.08, look.trim);
    B(0, fy + 0.62, -l * 0.42, w * 0.66, 0.07, 0.07, look.trim); // the hoop's top
    B(0, fy + 0.02, -l * 0.47, w * 0.9, 0.16, 0.12, look.body); // the bumper
    B(0, fy + 0.04, 0, w * 0.5, 0.05, l * 0.7, look.body); // the floor pan
    // The office chair: a gas-lift post, the seat, the backrest, armrests (the driver sits in it).
    B(0, fy + 0.12, spec.seat[2], 0.08, 0.2, 0.08, "steel");
    B(0, fy + 0.24, spec.seat[2], 0.62, 0.1, 0.6, look.trim);
    B(0, fy + 0.62, spec.seat[2] + 0.3, 0.58, 0.7, 0.1, look.trim, 0, rotX(0.15));
    for (const s of [-1, 1]) B(s * 0.33, fy + 0.42, spec.seat[2] - 0.02, 0.06, 0.06, 0.4, "steel");
    // The rubber duck on the nose, the whip aerial with its flag at the back, the engine box.
    B(0, fy + 0.2, -l * 0.42, 0.18, 0.16, 0.2, "toyYellow");
    B(0, fy + 0.32, -l * 0.47, 0.1, 0.1, 0.1, "toyYellow");
    B(0, fy + 0.31, -l * 0.53, 0.06, 0.03, 0.06, "hazard");
    const wob = 0.12 * Math.sin(time * 6 + id) * Math.min(1, Math.abs(fwd) / 8);
    B(w * 0.3, fy + 0.8, l * 0.45, 0.03, 1.5, 0.03, "steel", 0, rotZ(wob));
    B(w * 0.3 + Math.sin(wob) * -1.4 + 0.12, fy + 1.45, l * 0.45, 0.24, 0.16, 0.02, look.accent, 0.5, rotZ(wob));
    B(0, fy + 0.2, l * 0.4, w * 0.5, 0.3, 0.35, "gunmetal");
    flame(body, 0, fy + 0.22, l * 0.58, 0.16);
    // Casters: a fork that swivels about its post (towards where the car is going; the
    // front ones also with the steering), a twin wheel that rolls with the ground speed.
    for (let i = 0; i < 4; i++) {
      const sw = (i < 2 ? turn * 0.5 : 0) + a.swivel, y = wheelY(i);
      G(xs[i], y + r + 0.08, zs[i], 0.06, 0.22, 0.06, "steel"); // the post
      const fork = chain(base, translate(xs[i], y, zs[i]), rotY(sw));
      add(fork, 0, r * 1.02, r * 0.35, 0.28, 0.07, r * 0.9, "steel"); // the fork's top
      for (const s of [-1, 1]) add(fork, s * 0.13, r * 0.5, r * 0.35, 0.03, r * 1.1, 0.06, "steel"); // its legs
      for (const s of [-1, 1]) wheel(fork, s * 0.065, 0, r * 0.35, 0.1, r, look.accent); // the twin wheels
    }
    return out;
  }

  if (kind === "wheels") { // the racecar
    const by = ride + 0.12;
    B(0, by + 0.12, 0.1, w * 0.55, 0.3, l * 0.8, look.body); // the tub
    B(0, by + 0.06, -l * 0.38, w * 0.3, 0.16, l * 0.3, look.body, 0, rotX(0.06)); // the nose
    for (const s of [-1, 1]) B(s * w * 0.33, by + 0.12, l * 0.08, w * 0.2, 0.22, l * 0.45, look.body); // the sidepods
    B(0, by - 0.04, -l * 0.5, w * 0.95, 0.04, 0.28, look.trim); // the front wing
    for (const s of [-1, 1]) B(s * w * 0.47, by + 0.02, -l * 0.5, 0.03, 0.16, 0.3, look.accent, 0.3);
    B(0, by + 0.55, l * 0.47, w * 0.8, 0.05, 0.32, look.trim); // the rear wing
    for (const s of [-1, 1]) B(s * w * 0.38, by + 0.38, l * 0.47, 0.04, 0.36, 0.34, look.accent, 0.3);
    B(0, by + 0.42, l * 0.22, 0.3, 0.32, 0.5, look.body); // the airbox behind the driver
    B(0, by + 0.27, -0.1, 0.9, 0.05, 0.05, "steel"); // the halo
    B(0, by + 0.3, -l * 0.05, w * 0.2, 0.04, 0.3, look.accent, 0.4); // the number stripe
    B(0, by + 0.06, l * 0.5 + 0.02, w * 0.6, 0.08, 0.04, "reviewRed", 0.7); // the rain light
    for (const s of [-1, 1]) { B(s * 0.18, by + 0.16, l * 0.47, 0.12, 0.12, 0.2, "gunmetal"); flame(body, s * 0.18, by + 0.16, l * 0.57, 0.14); }
    for (let i = 0; i < 4; i++) {
      const y = wheelY(i), steerI = i < 2 ? turn : 0, wide = i < 2 ? 0.34 : 0.42;
      const hub = chain(base, translate(xs[i] * 1.08, y, zs[i]), rotY(steerI));
      wheel(hub, Math.sign(xs[i]) * 0.001, 0, 0, wide, r, look.accent);
      add(base, xs[i] * 0.6, (y + by) / 2, zs[i], Math.abs(xs[i]) * 0.9, 0.04, 0.06, "steel"); // the wishbone
    }
    return out;
  }

  if (kind === "tracks") {
    const hy = r * 2 + 0.05, hullH = Math.max(0.5, h - hy - 0.45);
    B(0, hy + hullH / 2, 0, w * 0.75, hullH, l * 0.92, look.body); // the hull
    B(0, hy + hullH * 0.75, -l * 0.47, w * 0.7, hullH * 0.5, 0.2, look.body, 0, rotX(0.5)); // the glacis
    for (const s of [-1, 1]) B(s * w * 0.42, hy + 0.04, 0, w * 0.14, 0.08, l * 0.95, look.trim); // the track guards
    for (let k = 0; k < 4; k++) B(-w * 0.2 + k * w * 0.13, hy + hullH + 0.01, l * 0.33, w * 0.1, 0.02, 0.2, look.accent, 0.2); // hazard stripes
    // The turret, turned to where the driver looks (relative to the hull): a dome, the rocket pod, a hatch.
    const ty = aim == null ? 0 : angleDiff(aim, yaw);
    const tur = chain(body, translate(0, hy + hullH, l * 0.05), rotY(ty));
    add(tur, 0, 0.18, 0, w * 0.5, 0.36, l * 0.35, look.body);
    add(tur, 0, 0.38, 0.1, w * 0.3, 0.06, 0.4, look.trim);
    for (const s of [-1, 1]) {
      add(tur, s * w * 0.3, 0.22, -0.3, 0.32, 0.32, 0.9, look.trim); // the pods
      for (const u of [-1, 1]) add(tur, s * w * 0.3 + u * 0.08, 0.22, -0.76, 0.1, 0.1, 0.03, "hazardDark", 0.2);
    }
    add(tur, 0, 0.1, -l * 0.18, 0.12, 0.12, l * 0.35, "gunmetal"); // a stubby barrel
    B(0, hy + hullH * 0.5, l * 0.47, w * 0.4, 0.2, 0.06, "reviewRed", 0.6);
    flame(body, 0, hy + hullH * 0.35, l * 0.5, 0.22);
    // A track down each side: the belt, its links scrolling with that side's speed, road wheels turning.
    for (const s of [-1, 1]) {
      const x = s * (w / 2 - 0.25), run = s < 0 ? a.trackL : a.trackR, pitchL = 0.32, n = Math.ceil(l * 0.95 / pitchL);
      G(x, r, 0, 0.5, 2 * r * 0.7, l * 0.86, "tyre");
      const off = ((run % pitchL) + pitchL) % pitchL;
      for (let k = 0; k < n; k++) {
        const z = -l * 0.475 + ((k * pitchL + off) % (n * pitchL)) * (l * 0.95) / (n * pitchL);
        G(x, 0.03, z, 0.54, 0.06, 0.07, "gunmetal"); // the bottom run lies on the ground: in the car's frame it runs back...
        G(x, 2 * r - 0.03, -z, 0.54, 0.06, 0.07, "gunmetal"); // ...and the top run forward
      }
      for (let k = 0; k < 5; k++) G(x + s * 0.27, r, -l * 0.38 + k * l * 0.19, 0.04, r * 1.2, r * 0.25, "steel", 0, rotX(-run / r)); // the road wheels' spokes
      G(x, r, -l * 0.46, 0.5, 2 * r * 0.75, 2 * r * 0.75, "gunmetal", 0, rotX(-run / r)); // the sprockets
      G(x, r, l * 0.46, 0.5, 2 * r * 0.75, 2 * r * 0.75, "gunmetal", 0, rotX(-run / r));
    }
    return out;
  }

  // The hovercraft: the skirt (puffing in and out), the deck, the cabin, two caged fans and rudders.
  const puff = (k) => 1 + 0.04 * Math.sin(time * 9 + k * 1.7 + id);
  for (let k = 0; k < 6; k++) { // the skirt in segments round the hull, each wobbling on its own
    const z = -l * 0.42 + k * l * 0.168;
    G(0, ride * 0.5, z, w * puff(k), ride * puff(k + 3), l * 0.18, "caseBlack");
  }
  const dy = ride + 0.12;
  B(0, dy, 0, w * 0.96, 0.24, l * 0.98, look.body); // the deck
  B(0, dy + 0.13, -l * 0.47, w * 0.8, 0.04, 0.1, look.accent, 0.5); // the bow stripe
  B(0, dy + 0.4, -l * 0.08, w * 0.6, 0.5, l * 0.38, look.trim); // the cabin...
  B(0, dy + 0.48, -l * 0.27, w * 0.56, 0.28, 0.05, "neonCyan", 0.35); // ...its windscreen
  B(0, dy + 0.68, -l * 0.05, w * 0.3, 0.06, 0.4, look.accent, 0.4); // a light bar: "Series A"
  for (const s of [-1, 1]) {
    const fx = s * w * 0.24, fz = l * 0.38, fy = dy + 0.6;
    B(fx, fy, fz, 0.95, 0.95, 0.1, "steel"); // the cage ring
    B(fx, fy, fz + 0.02, 0.85, 0.12, 0.06, "gunmetal", 0, rotZ(a.fan * s)); // the blades
    B(fx, fy, fz + 0.02, 0.12, 0.85, 0.06, "gunmetal", 0, rotZ(a.fan * s));
    B(fx, fy, fz + 0.02, 0.18, 0.18, 0.1, look.accent, 0.4);
    B(fx, fy, fz + 0.42, 0.04, 0.7, 0.4, look.body, 0, rotY(turn * 1.2)); // the rudder, steering
    flame(body, fx, fy, fz + 0.65, 0.22 * a.boost); // boosting: a jet out of each fan
  }
  return out;
}

// How a car's damage shows: "ok", "smoke" (under half its HP), "fire" (under
// a quarter), "wreck" (none left).
export function damageStage(hp, maxHP) {
  const k = maxHP > 0 ? hp / maxHP : 1;
  return hp <= 0 ? "wreck" : k < 0.25 ? "fire" : k < 0.5 ? "smoke" : "ok";
}

// Smoke and fire out of the engine (the back of the car, on top), the puffs
// trailing behind it by its velocity as they age; deterministic in time.
export function damageParts({ spec, pos, yaw = 0, vel = [0, 0, 0], hp, maxHP, time = 0, id = 1 }) {
  const stage = damageStage(hp, maxHP);
  if (stage === "ok") return [];
  const [, h, l] = spec.size, sy = Math.sin(yaw), cy = Math.cos(yaw);
  const ex = pos[0] + sy * l * 0.35, ez = pos[2] + cy * l * 0.35, ey = pos[1] + h * 0.85; // the engine: aft, on top
  const out = [], n = stage === "smoke" ? 7 : 10, life = stage === "wreck" ? 2.6 : 1.6;
  const dark = stage !== "smoke";
  for (let i = 0; i < n; i++) {
    const age = (((time / life + i / n + id * 0.137) % 1) + 1) % 1, t = age * life;
    const s = 0.25 + age * (dark ? 1.3 : 0.9), drift = Math.sin(i * 2.1 + id) * 0.4 * age;
    out.push({ m: chain(translate(ex - vel[0] * t * 0.6 + drift, ey + t * (dark ? 1.6 : 1.1), ez - vel[2] * t * 0.6 - drift), rotY(i + time * 0.5), scale(s, s * 0.8, s)),
      color: dark ? (age < 0.2 ? P.hazardDark : P.smokeShade.map((v) => v * 0.35)) : P.smoke.map((v) => v * (0.75 + 0.25 * age)), emit: 0 });
  }
  if (stage !== "smoke") { // flames licking up off the engine deck, flickering
    const k = stage === "wreck" ? 1.4 : 1;
    for (let i = 0; i < 5; i++) {
      const f = 0.5 + 0.5 * Math.sin(time * (13 + i * 3) + i * 1.7 + id), fh = (0.35 + 0.45 * f) * k;
      const ox = ((i % 3) - 1) * 0.35 * k, oz = (i < 3 ? -0.2 : 0.25) * k;
      out.push({ m: chain(translate(ex + cy * ox + sy * oz, ey + fh / 2 - 0.1, ez - sy * ox + cy * oz), rotY(time * 2 + i), scale(0.28 * k, fh, 0.28 * k)), color: i % 2 ? P.sunGlow : P.sunCore, emit: 1 });
    }
  }
  return out;
}

// A wreck's parts, burnt: everything dark, the glowing bits out.
export function charred(parts) {
  return parts.map((p) => ({ ...p, color: p.color.map((v) => v * 0.07 + 0.05), emit: 0 }));
}
