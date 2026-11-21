// Box-built models as data: pose a model's parts from a replicated state.
// The seeded part list and the ray test are internal/game/hitbox_shared.go,
// translated by cmd/gojs into game/shared.gen.js: the server's own code, so a
// part is drawn where it can be hit and a predicted hit is the server's hit.
// This file adapts it to the renderer's shapes and adds the draw matrices.
//   model: {seed?, vars?: [{name, int: [a, b]} | {name, range: [a, b]}], parts}
//   part:  {c, s, pivot?, crouch?: {c?, s?, pivot?}, team?, hit?, region,
//           when?: {var: min}, vary?: {var: {c?, s?}}, jitter?: [x, y, z]}
//   st:    {p: [x,y,z], yaw, pitch, crouch: bool, team?, id?}
// Per-player variation is seeded by the player id (Go Model.Seeded).
import { chain, translate, scale, rotX, rotY } from "../engine/math.js";
import * as G from "../game/shared.gen.js";

// The part's box for this crouch state.
export function partBox(part, crouched) {
  const k = crouched && part.crouch ? part.crouch : {};
  return { c: k.c || part.c, s: k.s || part.s, pivot: k.pivot || part.pivot };
}

// The model as the shared code reads it (Go Model), once per model object.
const compiled = new WeakMap();
function goModel(model) {
  let m = compiled.get(model);
  if (!m) compiled.set(model, m = G.Model_fromJSON(model));
  return m;
}
// A pose state as Go PoseState.
const poseOf = (st) => ({ pos: st.p, yaw: st.yaw || 0, pitch: st.pitch || 0, crouch: !!st.crouch, team: st.team || "", id: st.id | 0 });

// The seeded draws for one player: {vars: {name: value}, jitter: [per part: [x,y,z] | null]}.
const seededCache = new WeakMap();
export function seeded(model, id) {
  let byId = seededCache.get(model);
  if (!byId) seededCache.set(model, byId = new Map());
  const key = id | 0;
  let out = byId.get(key);
  if (!out) {
    const [vars, jitter] = G.Model_Seeded(goModel(model), key);
    byId.set(key, out = { vars: vars ?? {}, jitter });
  }
  return out;
}

// Parts present for this state (team outfits only on that team, seeded
// options), with seeded variation applied (Go Model.PartsFor): the model's own
// part objects (colour, emit, ...) with the posed c, s and crouch shapes.
export function partsFor(model, st) {
  return G.Model_PartsFor(goModel(model), poseOf(st)).map((p) => {
    const src = model.parts[G.ModelPart_Src(p)];
    const part = { ...src, c: p.c, s: p.s };
    if (src.crouch) {
      part.crouch = { ...src.crouch };
      if (src.crouch.c) part.crouch.c = p.crouch.c;
      if (src.crouch.s) part.crouch.s = p.crouch.s;
    }
    return part;
  });
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
// {region, name, t}, or null (Go RayModel, the same bits as the server's).
export function rayModel(model, st, o, d) {
  const [h, t, ok] = G.RayModel(goModel(model), poseOf(st), o, d);
  return ok ? { region: h.part, name: h.name, t } : null;
}
