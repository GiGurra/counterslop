// Box-built models as data: pose a model's parts from a replicated state.
// internal/game/hitbox.go implements the same pose for the server's ray tests
// (parity-tested), so a part is drawn exactly where it can be hit.
//   part: {c, s, pivot?, crouch?: {c?, s?, pivot?}, team?, hit?, region}
//   st:   {p: [x,y,z], yaw, pitch, crouch: bool, team?}
import { chain, translate, scale, rotX, rotY } from "../engine/math.js";

// The part's box for this crouch state.
export function partBox(part, crouched) {
  const k = crouched && part.crouch ? part.crouch : {};
  return { c: k.c || part.c, s: k.s || part.s, pivot: k.pivot || part.pivot };
}

// Parts present for this state (team outfits only on that team).
export const partsFor = (model, st) => model.parts.filter((pt) => !pt.team || pt.team === st.team);

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
