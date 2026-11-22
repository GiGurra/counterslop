// Box meshes: the faces of axis-aligned boxes as vertex arrays (position,
// normal, face uv + size, material kind) and triangle indices, for gl.js.

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
    const k = kindOf(b);
    for (const f of FACES) {
      if (f.n[1] < 0 && b.min[1] <= floor) continue; // nobody sees under the floor
      const base = pos.length / 3;
      const su = b.max[f.u] - b.min[f.u], sv = b.max[f.v] - b.min[f.v];
      for (const [i, j] of [[0, 0], [1, 0], [1, 1], [0, 1]]) {
        const p = [0, 0, 0];
        p[f.ax] = f.hi ? b.max[f.ax] : b.min[f.ax];
        p[f.u] = i ? b.max[f.u] : b.min[f.u];
        p[f.v] = j ? b.max[f.v] : b.min[f.v];
        pos.push(...p);
        nrm.push(...f.n);
        face.push(p[f.u] - b.min[f.u], p[f.v] - b.min[f.v], su, sv);
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
