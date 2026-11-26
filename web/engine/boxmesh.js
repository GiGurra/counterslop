// Box meshes: the faces of axis-aligned boxes as vertex arrays (position,
// normal, face uv + size, material kind) and triangle indices, for gl.js.
// A wedge (b.ramp > 0, content.Box.Ramp: the box's lower half under a slope
// rising towards +x, -x, +z or -z) is the same box with its top corners on
// the low side pulled down: the top face becomes the slope, the sides
// triangles (a quad with two corners together), the low end face goes.
import { SlopeNormal } from "./sim.gen.js";

// lowestFloor: the lowest top of any box (0 when there are none below ground).
export function lowestFloor(boxes) {
  let y = 0;
  for (const b of boxes) y = Math.min(y, b.max[1]);
  return y;
}

const FACES = [
  { n: [1, 0, 0], ax: 0, hi: 1, u: 2, v: 1 }, { n: [-1, 0, 0], ax: 0, hi: 0, u: 2, v: 1 },
  { n: [0, 1, 0], ax: 1, hi: 1, u: 0, v: 2 }, { n: [0, -1, 0], ax: 1, hi: 0, u: 0, v: 2 },
  { n: [0, 0, 1], ax: 2, hi: 1, u: 0, v: 1 }, { n: [0, 0, -1], ax: 2, hi: 0, u: 0, v: 1 },
];

// Undersides at or below the lowest floor (the lowest box top: y 0 on most
// maps, a basement's floor where there is one) are never seen, so they aren't
// built (floor: lowestFloor of the map; -Infinity builds every face).
export function boxMesh(boxes, kindOf, floor) {
  const pos = [], nrm = [], face = [], kind = [], idx = [];
  for (const b of boxes) {
    const k = kindOf(b), w = wedge(b);
    for (const f of FACES) {
      if (f.n[1] < 0 && b.min[1] <= floor) continue; // nobody sees under the floor
      if (w && f.ax === w.ax && f.hi === (w.sg < 0 ? 1 : 0)) continue; // a wedge's low end: no face
      const base = pos.length / 3;
      const slope = w && f.n[1] > 0;
      // The slope's uv runs along it (its length, not its footprint).
      const stretch = (a) => (slope && a === w.ax ? w.len / w.l : 1);
      const su = (b.max[f.u] - b.min[f.u]) * stretch(f.u), sv = (b.max[f.v] - b.min[f.v]) * stretch(f.v);
      for (const [i, j] of [[0, 0], [1, 0], [1, 1], [0, 1]]) {
        const p = [0, 0, 0];
        p[f.ax] = f.hi ? b.max[f.ax] : b.min[f.ax];
        p[f.u] = i ? b.max[f.u] : b.min[f.u];
        p[f.v] = j ? b.max[f.v] : b.min[f.v];
        if (w && p[1] === b.max[1]) p[1] = b.min[1] + w.h * (w.sg > 0 ? p[w.ax] - b.min[w.ax] : b.max[w.ax] - p[w.ax]) / w.l;
        pos.push(...p);
        nrm.push(...(slope ? w.n : f.n));
        face.push((p[f.u] - b.min[f.u]) * stretch(f.u), (p[f.v] - b.min[f.v]) * stretch(f.v), su, sv);
        kind.push(k);
      }
      idx.push(base, base + 1, base + 2, base, base + 2, base + 3);
    }
  }
  return {
    pos: new Float32Array(pos), nrm: new Float32Array(nrm), face: new Float32Array(face),
    kind: new Float32Array(kind), idx: new Uint32Array(idx),
  };
}

// A wedge's slope: the axis it rises along, which way, rise, run, slope
// length and outward unit normal (internal/sim's SlopeNormal; null for a
// plain box).
export function wedge(b) {
  if (!(b.ramp > 0)) return null;
  const ax = b.ramp <= 2 ? 0 : 2, sg = b.ramp % 2 ? 1 : -1;
  const h = b.max[1] - b.min[1], l = b.max[ax] - b.min[ax];
  return { ax, sg, h, l, len: Math.hypot(h, l), n: SlopeNormal(b) };
}

// splitCut: a map's boxes below its cut (content.Map.Cut) and its lid, the
// boxes wholly above it, which a view from over the map leaves out. No cut:
// everything is below.
export function splitCut(boxes, cut) {
  if (cut == null) return { below: boxes, lid: [] };
  return { below: boxes.filter((b) => b.min[1] < cut), lid: boxes.filter((b) => b.min[1] >= cut) };
}
