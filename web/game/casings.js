// Ejected casings: a short ballistic arc with spin, a bounce or two off the map's
// boxes (a swept point against each box, no physics engine), then they lie
// still for the rest of the round. Pure and seeded: no GL, no game state.
import { RaySolid, FaceNormal, SlopeNormal } from "../engine/sim.gen.js";
import { createRing } from "./marks.js";
import { mulberry32 } from "../gen/rng.js";

// What each weapon class ejects (len/rad in metres, a little over life size so
// they read on screen). base: the share of the length that is brass (a shell's head).
export const KINDS = [
  { name: "pistol", len: 0.04, rad: 0.009, color: [0.86, 0.66, 0.3], base: 1, hard: true },
  { name: "rifle", len: 0.065, rad: 0.01, color: [0.9, 0.7, 0.32], base: 1, hard: true },
  { name: "sniper", len: 0.09, rad: 0.012, color: [0.82, 0.6, 0.26], base: 1, hard: true },
  { name: "shell-red", len: 0.075, rad: 0.016, color: [0.75, 0.1, 0.08], base: 0.25, hard: false },
  { name: "shell-green", len: 0.075, rad: 0.016, color: [0.12, 0.5, 0.18], base: 0.25, hard: false },
];
const KIND_INDEX = Object.fromEntries(KINDS.map((k, i) => [k.name, i]));

// The casing a weapon ejects (by its weapons.json class), or null: melee,
// grenades and the bomb eject nothing. seed picks a shotgun shell's colour.
export function casingKind(w, seed = 0) {
  switch (w?.class) {
    case "pistol": case "smg": return KIND_INDEX.pistol;
    case "rifle": case "heavy": return KIND_INDEX.rifle;
    case "sniper": return KIND_INDEX.sniper;
    case "shotgun": return seed < 0.5 ? KIND_INDEX["shell-red"] : KIND_INDEX["shell-green"];
    default: return -1;
  }
}

// When the casing comes out after the shot (s): a bolt action only when the bolt cycles.
export const ejectDelay = (w) => (w?.action === "bolt" ? Math.max(0.2, (w.cycle ?? w.interval) * 0.55) : 0);

// Where it leaves the gun and how fast: from the eye, to the right and a
// little down and forward (the ejection port), out to the right and up.
// eye, basis {fwd, right, up}, the shooter's velocity, seed.
export function ejection(eye, b, vel = [0, 0, 0], seed = 0) {
  const r = mulberry32((seed * 4294967296) >>> 0);
  const side = 1.6 + r() * 0.8, upv = 1.2 + r() * 0.8, back = -0.2 - r() * 0.4;
  const p = [0, 0, 0], v = [0, 0, 0];
  for (let k = 0; k < 3; k++) {
    p[k] = eye[k] + b.right[k] * 0.14 + b.fwd[k] * 0.32 - b.up[k] * 0.13;
    v[k] = b.right[k] * side + b.up[k] * upv + b.fwd[k] * back + vel[k];
  }
  return { p, v };
}

export const CASING_STRIDE = 8; // pos.xyz, kind, yaw, tilt, roll, unused
export const MAX_CASINGS = 512;
const G = 9.81, BOUNCE = 0.35, FRICTION = 0.55, MAX_AGE = 4, MAX_STEP = 1 / 120;

export function createCasings(cap = MAX_CASINGS) {
  const ring = createRing(cap, CASING_STRIDE);
  const d = ring.data;
  // The ones still in the air (slot -> motion); a recycled slot drops its motion.
  const moving = new Map();

  function spawn(p, v, kind, seed = 0) {
    if (kind < 0) return -1;
    const slot = ring.add(), o = slot * CASING_STRIDE, r = mulberry32(((seed * 4294967296) >>> 0) ^ 0x9e3779b9);
    d[o] = p[0]; d[o + 1] = p[1]; d[o + 2] = p[2]; d[o + 3] = kind;
    d[o + 4] = r() * 6.283; d[o + 5] = r() * 6.283; d[o + 6] = 0; d[o + 7] = 0;
    moving.set(slot, { v: [...v], w: [(r() - 0.5) * 20, 18 + r() * 14, (r() - 0.5) * 10], age: 0, bounces: 0, r });
    return slot;
  }

  // One collision step for a casing at slot: returns the bounce, or null.
  function move(slot, m, dt, boxes, onBounce) {
    const o = slot * CASING_STRIDE, k = KINDS[d[o + 3]], rad = k.rad;
    m.v[1] -= G * dt;
    const p = [d[o], d[o + 1], d[o + 2]], s = [m.v[0] * dt, m.v[1] * dt, m.v[2] * dt];
    const pushed = depenetrate(boxes, p, rad); // resting in a corner: out of the other box first
    if (pushed) {
      for (let a = 0; a < 3; a++) d[o + a] = p[a];
      const vn = m.v[0] * pushed.n[0] + m.v[1] * pushed.n[1] + m.v[2] * pushed.n[2];
      if (vn < 0) for (let a = 0; a < 3; a++) m.v[a] -= vn * pushed.n[a];
      if (pushed.n[1] > 0.7 && Math.hypot(...m.v) < 0.6) { settle(slot, pushed.box.max[1] + rad); return; }
    }
    const hit = sweep(boxes, p, s, rad);
    d[o + 4] += m.w[0] * dt; d[o + 5] += m.w[1] * dt; d[o + 6] += m.w[2] * dt;
    if (!hit) { d[o] += s[0]; d[o + 1] += s[1]; d[o + 2] += s[2]; return; }
    const n = hit.n;
    for (let a = 0; a < 3; a++) d[o + a] = p[a] + s[a] * hit.t + n[a] * 1e-4;
    const vn = m.v[0] * n[0] + m.v[1] * n[1] + m.v[2] * n[2];
    const speed = Math.hypot(...m.v);
    for (let a = 0; a < 3; a++) {
      const nv = vn * n[a], tv = m.v[a] - nv;
      m.v[a] = tv * FRICTION - nv * BOUNCE;
    }
    m.bounces++;
    m.w = m.w.map((x) => x * 0.5 + (m.r() - 0.5) * 8);
    onBounce?.({ p: [d[o], d[o + 1], d[o + 2]], speed, box: hit.box, kind: d[o + 3], first: m.bounces === 1 });
    // Lands on a floor slowly enough (or has bounced enough): lie down there.
    if (n[1] > 0.7 && (-vn < 1.2 || m.bounces >= 3)) settle(slot, hit.box.max[1] + rad);
  }

  function settle(slot, y) {
    const o = slot * CASING_STRIDE;
    if (y != null) d[o + 1] = y;
    d[o + 5] = 0; // lying flat: the long axis horizontal
    moving.delete(slot);
  }

  // Advance every flying casing by dt seconds against boxes ([{min, max, kind}]).
  // onBounce({p, speed, box, kind, first}) hears each impact (sounds).
  function step(dt, boxes, onBounce) {
    if (!moving.size) return;
    const n = Math.max(1, Math.ceil(dt / MAX_STEP)), h = dt / n;
    for (const [slot, m] of moving) {
      for (let i = 0; i < n && moving.has(slot); i++) move(slot, m, h, boxes, onBounce);
      m.age += dt;
      const o = slot * CASING_STRIDE;
      if (moving.has(slot) && (m.age > MAX_AGE || d[o + 1] < -50)) settle(slot, null);
    }
    ring.touch();
  }

  return {
    ring, spawn, step,
    get count() { return ring.count; },
    get flying() { return moving.size; },
    // Recycled past the cap: the oldest goes, motion and all.
    clear() { ring.clear(); moving.clear(); },
    // The slot's state for tests: {p, kind, moving}.
    at(slot) { const o = slot * CASING_STRIDE; return { p: [d[o], d[o + 1], d[o + 2]], kind: d[o + 3], tilt: d[o + 5], moving: moving.has(slot) }; },
  };
}

// Push p (a sphere of radius rad) out of any box it is inside, along the
// shallowest way out; returns the last {n, box} it was pushed out of, or null.
export function depenetrate(boxes, p, rad) {
  let out = null;
  for (const b of boxes) {
    if (b.ramp > 0) { // a wedge: inside it, out along the slope's normal (or whichever face is nearer that way)
      const n = SlopeNormal(b), [t0, t1, face] = RaySolid(p, n, b);
      if (face !== -1 || t0 !== 0) continue;
      for (let a = 0; a < 3; a++) p[a] += n[a] * (t1 + rad + 1e-5);
      out = { n, box: b };
      continue;
    }
    let depth = Infinity, ax = -1, sg = 0;
    for (let a = 0; a < 3; a++) {
      const lo = b.min[a] - rad, hi = b.max[a] + rad;
      if (p[a] <= lo || p[a] >= hi) { ax = -2; break; }
      if (p[a] - lo < depth) { depth = p[a] - lo; ax = a; sg = -1; }
      if (hi - p[a] < depth) { depth = hi - p[a]; ax = a; sg = 1; }
    }
    if (ax < 0) continue;
    p[ax] += sg * (depth + 1e-5);
    const n = [0, 0, 0]; n[ax] = sg;
    out = { n, box: b };
  }
  return out;
}

// The first box the segment p -> p+s (a sphere of radius rad) enters: {t in [0,1], n, box}.
// A start already inside a box is ignored (an ejection into a wall then falls out of it).
export function sweep(boxes, p, s, rad) {
  let best = null;
  for (const b of boxes) {
    if (b.ramp > 0) { // a wedge: the casing's centre against the solid (it is a few mm)
      const [t0, , face] = RaySolid(p, s, b);
      if (face >= 0 && t0 <= 1 && (!best || t0 < best.t)) best = { t: t0, n: FaceNormal(b, face, s), box: b };
      continue;
    }
    let t0 = 0, t1 = 1, ax = -1, sg = 0, inside = true;
    for (let a = 0; a < 3; a++) {
      const lo = b.min[a] - rad, hi = b.max[a] + rad;
      if (p[a] < lo || p[a] > hi) inside = false;
      if (Math.abs(s[a]) < 1e-12) { if (p[a] < lo || p[a] > hi) { t0 = 2; break; } continue; }
      let ta = (lo - p[a]) / s[a], tb = (hi - p[a]) / s[a], g = -1;
      if (ta > tb) { [ta, tb] = [tb, ta]; g = 1; }
      if (ta > t0) { t0 = ta; ax = a; sg = g; }
      if (tb < t1) t1 = tb;
      if (t0 > t1) break;
    }
    if (inside || ax < 0 || t0 > t1 || t0 > 1) continue;
    if (!best || t0 < best.t) { const n = [0, 0, 0]; n[ax] = sg; best = { t: t0, n, box: b }; }
  }
  return best;
}
