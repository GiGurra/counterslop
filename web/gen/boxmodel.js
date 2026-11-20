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
