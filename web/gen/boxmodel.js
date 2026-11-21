// Box-built models as data: pose a model's parts from a replicated state.
// internal/game/hitbox.go implements the same pose for the server's ray tests
// (parity-tested), so a part is drawn exactly where it can be hit.
//   model: {seed?, vars?: [{name, int: [a, b]} | {name, range: [a, b]}], parts}
//   part:  {c, s, pivot?, crouch?: {c?, s?, pivot?}, team?, hit?, region,
//           when?: {var: min}, vary?: {var: {c?, s?}}, jitter?: [x, y, z]}
//   st:    {p: [x,y,z], yaw, pitch, crouch: bool, team?, id?}
// Per-player variation is seeded by the player id: the vars are drawn in order
// from rng(seed + id), then one value per jitter axis for each part with a
// jitter, in part order (whether or not the part is shown, so the stream
// never shifts). vary adds var × vector to c and s (stand and crouch shapes);
// jitter adds ±jitter to c.
import { chain, translate, scale, rotX, rotY } from "../engine/math.js";
import { rng } from "./rng.js";

// The part's box for this crouch state.
export function partBox(part, crouched) {
  const k = crouched && part.crouch ? part.crouch : {};
  return { c: k.c || part.c, s: k.s || part.s, pivot: k.pivot || part.pivot };
}

// A part's team filter: absent = everyone, a string = that team, a list =
// any of those ("" is teamless, e.g. bots).
export const teamMatches = (t, team) => t === undefined || (Array.isArray(t) ? t.includes(team || "") : t === team);

// The seeded draws for one player: {vars: {name: value}, jitter: [per part: [x,y,z] | null]}.
const seededCache = new WeakMap();
export function seeded(model, id) {
  let byId = seededCache.get(model);
  if (!byId) seededCache.set(model, byId = new Map());
  const key = id | 0;
  let out = byId.get(key);
  if (out) return out;
  const r = rng(((model.seed || 0) + key) >>> 0);
  const vars = {};
  for (const v of model.vars || []) vars[v.name] = v.int ? r.int(v.int[0], v.int[1]) : r.range(v.range[0], v.range[1]);
  const jitter = model.parts.map((pt) => (pt.jitter ? pt.jitter.map((j) => (r.next() * 2 - 1) * j) : null));
  byId.set(key, out = { vars, jitter });
  return out;
}

// Parts present for this state (team outfits only on that team, seeded
// options), with seeded variation applied.
export function partsFor(model, st) {
  const { vars, jitter } = seeded(model, st.id);
  const out = [];
  model.parts.forEach((pt, i) => {
    if (!teamMatches(pt.team, st.team)) return;
    if (pt.when && Object.entries(pt.when).some(([k, min]) => !(vars[k] >= min))) return;
    const j = jitter[i];
    if (!pt.vary && !j) { out.push(pt); return; }
    const adj = (v, field) => {
      const r = [...v];
      for (const [k, d] of Object.entries(pt.vary || {})) if (d[field]) for (let a = 0; a < 3; a++) r[a] += vars[k] * d[field][a];
      if (field === "c" && j) for (let a = 0; a < 3; a++) r[a] += j[a];
      return r;
    };
    const part = { ...pt, c: adj(pt.c, "c"), s: adj(pt.s, "s") };
    if (pt.crouch) {
      part.crouch = { ...pt.crouch };
      if (pt.crouch.c) part.crouch.c = adj(pt.crouch.c, "c");
      if (pt.crouch.s) part.crouch.s = adj(pt.crouch.s, "s");
    }
    out.push(part);
  });
  return out;
}

// The part's frame (everything but its own centre and size): translate(p)
// rotY(yaw), then translate(pivot) rotX(pitch) for pivoting parts.
export function partFrame(part, st) {
  const b = partBox(part, st.crouch);
  let m = chain(translate(...st.p), rotY(st.yaw));
  if (b.pivot) m = chain(m, translate(...b.pivot), rotX(st.pitch));
  return { m, c: b.c, s: b.s };
}

// Unit cube -> world: partFrame, then translate(c) scale(s).
export function partMatrix(part, st) {
  const f = partFrame(part, st);
  return chain(f.m, translate(...f.c), scale(...f.s));
}

// World-space corners of a part (for parity tests and debugging).
export function partCorners(part, st) {
  const m = partMatrix(part, st), out = [];
  for (const x of [-0.5, 0.5]) for (const y of [-0.5, 0.5]) for (const z of [-0.5, 0.5]) {
    out.push([0, 1, 2].map((i) => m[i] * x + m[4 + i] * y + m[8 + i] * z + m[12 + i]));
  }
  return out;
}

// Ray o + t·d against a posed model's hittable parts: the nearest
// {region, name, t}, or null. The twin of internal/game RayModel (same parts,
// same pose, same slab test), so the client predicts exactly the server's hit.
export function rayModel(model, st, o, d) {
  let best = null;
  for (const part of partsFor(model, st)) {
    if (part.hit === false) continue;
    const { m, c, s } = partFrame(part, st);
    // m is rigid (rotation + translation): world -> part frame is R^T (v - t).
    const w = [o[0] - m[12], o[1] - m[13], o[2] - m[14]];
    const lo = [0, 1, 2].map((k) => m[k * 4] * w[0] + m[k * 4 + 1] * w[1] + m[k * 4 + 2] * w[2]);
    const ld = [0, 1, 2].map((k) => m[k * 4] * d[0] + m[k * 4 + 1] * d[1] + m[k * 4 + 2] * d[2]);
    const t = slab(lo, ld, c.map((v, i) => v - s[i] / 2), c.map((v, i) => v + s[i] / 2));
    if (t >= 0 && (!best || t < best.t)) best = { region: part.region, name: part.name, t };
  }
  return best;
}

function slab(o, d, mn, mx) {
  let t0 = 0, t1 = Infinity;
  for (let a = 0; a < 3; a++) {
    if (Math.abs(d[a]) < 1e-12) { if (o[a] < mn[a] || o[a] > mx[a]) return -1; continue; }
    let ta = (mn[a] - o[a]) / d[a], tb = (mx[a] - o[a]) / d[a];
    if (ta > tb) [ta, tb] = [tb, ta];
    t0 = Math.max(t0, ta); t1 = Math.min(t1, tb);
    if (t0 > t1) return -1;
  }
  return t0;
}
