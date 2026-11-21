// Local lights over a map: pure, deterministic, no GL (the renderer uploads what
// this builds). Game-agnostic: a map is boxes + lights (content.Light's JSON).
//
// The map is cut into a grid of cells (CELL metres). A cell is solid when its
// centre is inside a box, and the face between two neighbouring cells is
// blocked when a box crosses the segment between their centres, so a thin wall
// between two centres separates them even though neither centre is inside it.
//
// - Light influence: each light floods out from its cell through open faces to
//   its range (a breadth-first fill), so light never leaks through a wall, only
//   round it through doors and windows. Each cell keeps a short list of the
//   lights that reach it (the most important MAX_PER_CELL); the fragment shader
//   looks up its cell and loops over just those.
// - Sky visibility: a cell with no box anywhere above its centre sees the sky
//   (1); from those, visibility floods inwards and decays per step, so roofed
//   rooms fall dark except near windows, doors and under skylights.
// - Per-frame culling: cells are grouped into coarse zones (ZONE cells a side);
//   a light is in play when one of its zones is within a few zone steps
//   ("openings") of the camera's zone and its sphere is in the view frustum. The
//   shadow budget goes to the most important lights in play (shadowPick).

import { perspective, lookAt, mul } from "./math.js";

export const CELL = 0.5; // metres
export const MAX_PER_CELL = 16;
export const ZONE = 8; // cells per coarse zone side (4 m)
export const SKY_DECAY = 0.89; // sky visibility kept per cell step (~0.8 per metre)
const EPS = 1e-6;
export const DETOUR = 1.0; // metres of detour floodLight allows

// Face bits in grid.blk: the face towards +x, +y, +z from a cell is blocked.
const BX = 1, BY = 2, BZ = 4;

// Cell index range [i0, i1] whose centres o + (i+0.5)c lie in [lo, hi] (inclusive) or (lo, hi) (strict).
function span(lo, hi, o, c, n, strict) {
  const e = strict ? EPS : -EPS;
  const i0 = Math.max(0, Math.ceil((lo - o) / c - 0.5 + e));
  const i1 = Math.min(n - 1, Math.floor((hi - o) / c - 0.5 - e));
  return [i0, i1];
}

// Grid over the boxes' bounds (plus a margin above for the sky). Cells are indexed
// x + nx*(z + nz*y): one xz layer per y, the layout of a 2D array texture.
export function buildGrid(boxes, cell = CELL) {
  const mn = [Infinity, Infinity, Infinity], mx = [-Infinity, -Infinity, -Infinity];
  for (const b of boxes) for (let a = 0; a < 3; a++) { mn[a] = Math.min(mn[a], b.min[a]); mx[a] = Math.max(mx[a], b.max[a]); }
  if (!boxes.length) { mn.fill(0); mx.fill(1); }
  mx[1] += 2;
  const origin = mn.map((v) => Math.floor(v / cell) * cell - cell);
  const [nx, ny, nz] = [0, 1, 2].map((a) => Math.max(1, Math.ceil((mx[a] - origin[a]) / cell) + 1));
  const n = nx * ny * nz;
  const solid = new Uint8Array(n), blk = new Uint8Array(n);
  const g = { cell, origin, nx, ny, nz, n, solid, blk };
  for (const b of boxes) {
    // Solid: centre strictly inside.
    const [x0, x1] = span(b.min[0], b.max[0], origin[0], cell, nx, true);
    const [y0, y1] = span(b.min[1], b.max[1], origin[1], cell, ny, true);
    const [z0, z1] = span(b.min[2], b.max[2], origin[2], cell, nz, true);
    for (let y = y0; y <= y1; y++) for (let z = z0; z <= z1; z++) for (let x = x0; x <= x1; x++) solid[x + nx * (z + nz * y)] = 1;
    // Blocked faces: the segment from a centre one cell along an axis meets the box
    // (perpendicular centres inside it, inclusive; the segment overlaps its extent).
    for (let a = 0; a < 3; a++) {
      const r = [0, 1, 2].map((k) => k === a
        ? span(b.min[k] - cell, b.max[k], origin[k], cell, [nx, ny, nz][k], true)
        : span(b.min[k], b.max[k], origin[k], cell, [nx, ny, nz][k], false));
      const bit = [BX, BY, BZ][a];
      for (let y = r[1][0]; y <= r[1][1]; y++) for (let z = r[2][0]; z <= r[2][1]; z++) for (let x = r[0][0]; x <= r[0][1]; x++) blk[x + nx * (z + nz * y)] |= bit;
    }
  }
  return g;
}

export const cellIndex = (g, x, y, z) => x + g.nx * (z + g.nz * y);
export function cellOf(g, p) {
  return [0, 1, 2].map((a) => Math.floor((p[a] - g.origin[a]) / g.cell));
}
export function centre(g, x, y, z) {
  const c = g.cell;
  return [g.origin[0] + (x + 0.5) * c, g.origin[1] + (y + 0.5) * c, g.origin[2] + (z + 0.5) * c];
}
const inside = (g, x, y, z) => x >= 0 && y >= 0 && z >= 0 && x < g.nx && y < g.ny && z < g.nz;

// The open neighbours of cell i, reached through unblocked faces: their indices go
// into out (length >= 6); returns how many.
function openSteps(g, i, out) {
  const { nx, ny, nz, blk, solid } = g;
  const layer = nx * nz;
  const x = i % nx, z = ((i / nx) | 0) % nz, y = (i / layer) | 0;
  let k = 0, j;
  if (x + 1 < nx && !(blk[i] & BX) && !solid[(j = i + 1)]) out[k++] = j;
  if (x > 0 && !(blk[i - 1] & BX) && !solid[(j = i - 1)]) out[k++] = j;
  if (y + 1 < ny && !(blk[i] & BY) && !solid[(j = i + layer)]) out[k++] = j;
  if (y > 0 && !(blk[i - layer] & BY) && !solid[(j = i - layer)]) out[k++] = j;
  if (z + 1 < nz && !(blk[i] & BZ) && !solid[(j = i + nx)]) out[k++] = j;
  if (z > 0 && !(blk[i - nx] & BZ) && !solid[(j = i - nx)]) out[k++] = j;
  return k;
}

// The 6 neighbours of cell i (any face, inside the grid) into out; returns how many.
function neighbours(g, i, out) {
  const { nx, ny, nz } = g;
  const layer = nx * nz;
  const x = i % nx, z = ((i / nx) | 0) % nz, y = (i / layer) | 0;
  let k = 0;
  if (x + 1 < nx) out[k++] = i + 1;
  if (x > 0) out[k++] = i - 1;
  if (y + 1 < ny) out[k++] = i + layer;
  if (y > 0) out[k++] = i - layer;
  if (z + 1 < nz) out[k++] = i + nx;
  if (z > 0) out[k++] = i - nx;
  return k;
}

// Sky visibility per cell, 0..255. Open cells with nothing above see the sky;
// the rest get SKY_DECAY^steps from the nearest such cell through open faces;
// solid cells the darkest of their open neighbours.
export function skyVisibility(g, boxes) {
  const { nx, ny, nz, cell, origin, solid } = g;
  // Highest box bottom over each column centre: a cell below it is under cover.
  const roof = new Float32Array(nx * nz).fill(-Infinity);
  for (const b of boxes) {
    const [x0, x1] = span(b.min[0], b.max[0], origin[0], cell, nx, false);
    const [z0, z1] = span(b.min[2], b.max[2], origin[2], cell, nz, false);
    for (let z = z0; z <= z1; z++) for (let x = x0; x <= x1; x++) {
      const k = x + nx * z;
      if (b.min[1] > roof[k]) roof[k] = b.min[1];
    }
  }
  const vis = new Uint8Array(g.n);
  const dist = new Int16Array(g.n).fill(-1);
  const q = new Int32Array(g.n);
  let head = 0, tail = 0;
  for (let y = 0; y < ny; y++) {
    const cy = origin[1] + (y + 0.5) * cell;
    for (let z = 0; z < nz; z++) for (let x = 0; x < nx; x++) {
      const i = x + nx * (z + nz * y);
      if (!solid[i] && !(cy < roof[x + nx * z])) { dist[i] = 0; q[tail++] = i; }
    }
  }
  const maxSteps = Math.ceil(Math.log(1 / 255) / Math.log(SKY_DECAY));
  const nb = new Int32Array(6);
  while (head < tail) {
    const i = q[head++], d = dist[i];
    if (d >= maxSteps) continue;
    const k = openSteps(g, i, nb);
    for (let a = 0; a < k; a++) { const j = nb[a]; if (dist[j] < 0) { dist[j] = d + 1; q[tail++] = j; } }
  }
  const lut = new Uint8Array(maxSteps + 1);
  for (let d = 0; d <= maxSteps; d++) lut[d] = Math.round(255 * SKY_DECAY ** d);
  for (let i = 0; i < g.n; i++) if (dist[i] >= 0) vis[i] = lut[dist[i]];
  for (let i = 0; i < g.n; i++) {
    if (!solid[i]) continue;
    let m = 256;
    const k = neighbours(g, i, nb);
    for (let a = 0; a < k; a++) { const j = nb[a]; if (!solid[j] && vis[j] < m) m = vis[j]; }
    vis[i] = m === 256 ? 0 : m;
  }
  return vis;
}

// ---- lights ----

// Strength of a light at distance d (before the surface's N.L and the spot cone):
// inverse square softened at the source, windowed to zero at the range. The shader
// (gen/glsl/lights.js) uses the same formula.
export function falloff(d, range) {
  if (d >= range) return 0;
  const w = 1 - (d / range) ** 2;
  return (w * w) / (1 + d * d);
}

const norm3 = (v) => { const l = Math.hypot(v[0], v[1], v[2]) || 1; return [v[0] / l, v[1] / l, v[2] / l]; };
const deg = (a) => (a * Math.PI) / 180;
export function coneOf(l) {
  const outer = deg(l.cone || 45), inner = deg(l.inner || (l.cone || 45) * 0.7);
  return { dir: norm3(l.dir || [0, -1, 0]), cosOuter: Math.cos(outer), cosInner: Math.cos(Math.min(inner, outer)), outer };
}

// The cells one light reaches: flood from its cell through open faces while the
// cell centre is within range (plus half a cell diagonal), for a spot inside its
// cone (widened by the cell's angular size), and the path there (6-neighbour steps)
// is at most DETOUR longer than the straight line's own Manhattan length: about line
// of sight, so light goes through a door but not far round its corner, nor over a
// wall into the next yard.
// Returns cell indices.
export function floodLight(g, l, scratch = new Int32Array(g.n).fill(-1), stamp = 0) {
  const { nx, nz, cell, solid } = g;
  const half = cell * 0.87;
  const spot = l.kind === "spot" ? coneOf(l) : null;
  const [lx, ly, lz] = l.pos, [ox, oy, oz] = g.origin, maxD = l.range + half;
  const cosO = spot ? Math.cos(spot.outer) : 0, sinO = spot ? Math.sin(spot.outer) : 0;
  const layer = nx * nz;
  // Within range, on a short enough path, and (spot) inside the cone widened by the
  // cell's angular size: angle <= outer + atan(half/d), compared as cosines.
  const reach = (i, steps) => {
    const x = i % nx, z = ((i / nx) | 0) % nz, y = (i / layer) | 0;
    const vx = ox + (x + 0.5) * cell - lx, vy = oy + (y + 0.5) * cell - ly, vz = oz + (z + 0.5) * cell - lz;
    const d2 = vx * vx + vy * vy + vz * vz;
    if (d2 > maxD * maxD) return false;
    if (steps * cell > Math.abs(vx) + Math.abs(vy) + Math.abs(vz) + DETOUR + cell) return false;
    const d = Math.sqrt(d2);
    if (!spot || d <= half * 2) return true;
    const cos = (vx * spot.dir[0] + vy * spot.dir[1] + vz * spot.dir[2]) / d;
    const h = Math.hypot(d, half), cosW = cosO * (d / h) - sinO * (half / h); // cos(outer + atan(half/d))
    return spot.outer + Math.atan(half / d) >= Math.PI || cos >= cosW;
  };
  let [sx, sy, sz] = cellOf(g, l.pos);
  if (!inside(g, sx, sy, sz)) return [];
  // A light on a ceiling or wall surface can sit in a solid cell: start from the
  // open neighbour whose centre is nearest the light instead.
  if (solid[cellIndex(g, sx, sy, sz)]) {
    let best = null, bd = Infinity;
    for (const [dx, dy, dz] of [[1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0], [0, 0, 1], [0, 0, -1]]) {
      const x = sx + dx, y = sy + dy, z = sz + dz;
      if (!inside(g, x, y, z) || solid[cellIndex(g, x, y, z)]) continue;
      const c = centre(g, x, y, z), d = Math.hypot(c[0] - l.pos[0], c[1] - l.pos[1], c[2] - l.pos[2]);
      if (d < bd) { bd = d; best = [x, y, z]; }
    }
    if (!best) return [];
    [sx, sy, sz] = best;
  }
  const out = [], steps = [];
  const s0 = cellIndex(g, sx, sy, sz);
  scratch[s0] = stamp;
  out.push(s0); steps.push(0);
  const nb = new Int32Array(6);
  for (let h = 0; h < out.length; h++) {
    const k = steps[h] + 1, cnt = openSteps(g, out[h], nb);
    for (let a = 0; a < cnt; a++) {
      const j = nb[a];
      if (scratch[j] === stamp) continue;
      scratch[j] = stamp;
      if (reach(j, k)) { out.push(j); steps.push(k); }
    }
  }
  return out;
}

// Per-cell light lists. cells[i] = (offset << 8) | count into list (light indices);
// count <= MAX_PER_CELL, the strongest at the cell centre kept. Solid cells get
// the lights every open neighbour has (a wall's cell takes no side). Also returns
// each light's flooded cells (for zones) and stats.
export function buildLightLists(g, lights) {
  const { n, nx, nz, solid } = g;
  const scratch = new Int32Array(n).fill(-1);
  const nbrHits = new Uint8Array(n), nbrStamp = new Int32Array(n).fill(-1);
  const nb = new Int32Array(6);
  const openNbrs = (i) => {
    const c = neighbours(g, i, nb);
    let k = 0;
    for (let a = 0; a < c; a++) if (!solid[nb[a]]) k++;
    return k;
  };
  const perLight = [];
  let pairs = 0, cap = 1 << 16;
  let pc = new Int32Array(cap), pl = new Uint16Array(cap), pw = new Float32Array(cap);
  const { cell, origin } = g;
  lights.forEach((l, li) => {
    const cells = floodLight(g, l, scratch, li);
    // Solid cells next to the flood: count how many of their open neighbours it reached.
    const walls = [];
    for (const i of cells) {
      const c = neighbours(g, i, nb);
      for (let a = 0; a < c; a++) {
        const j = nb[a];
        if (!solid[j]) continue;
        if (nbrStamp[j] !== li) { nbrStamp[j] = li; nbrHits[j] = 0; walls.push(j); }
        nbrHits[j]++;
      }
    }
    for (const j of walls) if (nbrHits[j] === openNbrs(j)) cells.push(j);
    perLight.push(cells);
    if (pairs + cells.length > cap) {
      while (pairs + cells.length > cap) cap *= 2;
      const grow = (a, T) => { const b = new T(cap); b.set(a.subarray(0, pairs)); return b; };
      pc = grow(pc, Int32Array); pl = grow(pl, Uint16Array); pw = grow(pw, Float32Array);
    }
    const k = l.intensity ?? 1, r = l.range + cell, [lx, ly, lz] = l.pos;
    for (const i of cells) {
      const x = i % nx, z = ((i / nx) | 0) % nz, y = (i / (nx * nz)) | 0;
      const dx = origin[0] + (x + 0.5) * cell - lx, dy = origin[1] + (y + 0.5) * cell - ly, dz = origin[2] + (z + 0.5) * cell - lz;
      pc[pairs] = i; pl[pairs] = li; pw[pairs] = k * falloff(Math.sqrt(dx * dx + dy * dy + dz * dz), r);
      pairs++;
    }
  });
  // Counting sort by cell, then keep the strongest MAX_PER_CELL per cell.
  const count = new Uint32Array(n);
  for (let k = 0; k < pairs; k++) count[pc[k]]++;
  const start = new Uint32Array(n + 1);
  for (let i = 0; i < n; i++) start[i + 1] = start[i] + count[i];
  const order = new Uint32Array(pairs), fill = start.slice(0, n);
  for (let k = 0; k < pairs; k++) order[fill[pc[k]]++] = k;
  const cells = new Uint32Array(n);
  const list = new Uint16Array(pairs);
  let len = 0, capped = 0, maxCount = 0;
  for (let i = 0; i < n; i++) {
    const c = count[i];
    if (!c) continue;
    const off = len;
    if (c > MAX_PER_CELL) {
      // Pairs were appended light by light, so each cell's run is in index order already.
      const ks = Array.from(order.subarray(start[i], start[i + 1]));
      ks.sort((a, b) => pw[b] - pw[a] || pl[a] - pl[b]);
      const keep = ks.slice(0, MAX_PER_CELL).map((k) => pl[k]).sort((a, b) => a - b);
      for (const li of keep) list[len++] = li;
      capped++;
    } else {
      for (let k = start[i]; k < start[i + 1]; k++) list[len++] = pl[order[k]];
    }
    cells[i] = (off << 8) | (len - off);
    maxCount = Math.max(maxCount, len - off);
  }
  return { cells, list: list.slice(0, len), perLight, stats: { pairs, entries: len, capped, maxCount } };
}

// Lights in cell i (decoded from the lists): for tests and debugging.
export function lightsAt(lists, i) {
  const e = lists.cells[i], off = e >>> 8, cnt = e & 255;
  return Array.from(lists.list.subarray(off, off + cnt));
}

// Light data rows for the shader: 4 RGBA32F texels per light.
//   0: pos.xyz, range        1: colour * intensity, flicker seed (0 = steady)
//   2: spot dir, cos(outer) (-2 for a point light)
//   3: cos(inner), static shadow base tile, live shadow base tile (-1 none), live weight 0..1
export const TEXELS_PER_LIGHT = 4;
export function packLights(lights) {
  const d = new Float32Array(Math.max(1, lights.length) * 16);
  lights.forEach((l, i) => {
    const o = i * 16, c = l.color || [1, 1, 1], k = l.intensity ?? 1;
    d.set([l.pos[0], l.pos[1], l.pos[2], l.range], o);
    d.set([c[0] * k, c[1] * k, c[2] * k, (l.flicker || 0) % 9973], o + 4);
    if (l.kind === "spot") {
      const s = coneOf(l);
      d.set([...s.dir, s.cosOuter], o + 8);
      d.set([s.cosInner, -1, -1, 0], o + 12);
    } else {
      d.set([0, -1, 0, -2], o + 8);
      d.set([1, -1, -1, 0], o + 12);
    }
  });
  return d;
}

// ---- zones and per-frame culling ----

// Coarse zones of ZONE^3 cells and which neighbours connect through an open face.
// Each light lists the zones its flood touches.
export function buildZones(g, perLight, zone = ZONE) {
  const zx = Math.ceil(g.nx / zone), zy = Math.ceil(g.ny / zone), zz = Math.ceil(g.nz / zone);
  const links = new Uint8Array(zx * zy * zz); // bits: +x, +y, +z open
  const zi = (x, y, z) => ((x / zone) | 0) + zx * (((z / zone) | 0) + zz * ((y / zone) | 0));
  const { nx, ny, nz, solid, blk } = g;
  // Only faces on zone boundaries can link zones: cell b-1 to cell b along each axis.
  const open = (i, j, bit) => !solid[i] && !solid[j] && !(blk[i] & bit);
  for (let y = 0; y < ny; y++) for (let z = 0; z < nz; z++) for (let x = zone; x < nx; x += zone) {
    const j = x + nx * (z + nz * y);
    if (open(j - 1, j, BX)) links[zi(x - 1, y, z)] |= BX;
  }
  for (let y = zone; y < ny; y += zone) for (let z = 0; z < nz; z++) for (let x = 0; x < nx; x++) {
    const j = x + nx * (z + nz * y), i = j - nx * nz;
    if (open(i, j, BY)) links[zi(x, y - 1, z)] |= BY;
  }
  for (let y = 0; y < ny; y++) for (let z = zone; z < nz; z += zone) for (let x = 0; x < nx; x++) {
    const j = x + nx * (z + nz * y), i = j - nx;
    if (open(i, j, BZ)) links[zi(x, y, z - 1)] |= BZ;
  }
  const lightZones = perLight.map((cells) => {
    const s = new Set();
    for (const i of cells) { const x = i % nx, z = ((i / nx) | 0) % nz, y = (i / (nx * nz)) | 0; s.add(zi(x, y, z)); }
    return Int32Array.from([...s].sort((a, b) => a - b));
  });
  return { zone, zx, zy, zz, links, lightZones, mark: new Int32Array(zx * zy * zz).fill(-1), stampNo: 0, from: -2 };
}

// Zones within `steps` open zone boundaries of the camera (a breadth-first walk);
// cached until the camera changes zone. Returns the stamp marking reached zones in z.mark,
// or -1 when the camera is outside the grid (everything counts).
export function reachZones(g, z, eye, steps) {
  const [x, y, zc] = cellOf(g, eye);
  if (!inside(g, x, y, zc)) { z.from = -1; return -1; }
  const zone = z.zone;
  const start = ((x / zone) | 0) + z.zx * (((zc / zone) | 0) + z.zz * ((y / zone) | 0));
  if (z.from === start && z.steps === steps) return z.stampNo;
  z.from = start; z.steps = steps;
  const s = ++z.stampNo;
  const q = [start], d = [0];
  z.mark[start] = s;
  const per = z.zx * z.zz;
  for (let h = 0; h < q.length; h++) {
    const i = q[h], k = d[h];
    if (k >= steps) continue;
    const ix = i % z.zx, iz = ((i / z.zx) | 0) % z.zz, iy = (i / per) | 0;
    const push = (j) => { if (z.mark[j] !== s) { z.mark[j] = s; q.push(j); d.push(k + 1); } };
    if (z.links[i] & BX) push(i + 1);
    if (ix > 0 && z.links[i - 1] & BX) push(i - 1);
    if (z.links[i] & BY) push(i + per);
    if (iy > 0 && z.links[i - per] & BY) push(i - per);
    if (z.links[i] & BZ) push(i + z.zx);
    if (iz > 0 && z.links[i - z.zx] & BZ) push(i - z.zx);
  }
  return s;
}

// Whether a sphere is at least partly inside the frustum of the view-projection vp
// (column-major, as math.js builds it).
export function sphereInFrustum(vp, c, r) {
  const row = (k) => [vp[k], vp[4 + k], vp[8 + k], vp[12 + k]];
  const r0 = row(0), r1 = row(1), r2 = row(2), r3 = row(3);
  const planes = [
    r3.map((v, i) => v + r0[i]), r3.map((v, i) => v - r0[i]),
    r3.map((v, i) => v + r1[i]), r3.map((v, i) => v - r1[i]),
    r3.map((v, i) => v + r2[i]), r3.map((v, i) => v - r2[i]),
  ];
  for (const p of planes) {
    const l = Math.hypot(p[0], p[1], p[2]);
    if ((p[0] * c[0] + p[1] * c[1] + p[2] * c[2] + p[3]) / l < -r) return false;
  }
  return true;
}

// The lights in play this frame: their flood reaches a zone near the camera and
// their sphere is in view (or the camera is inside it). Returns indices.
export function activeLights(g, z, lights, eye, vp, steps) {
  const s = reachZones(g, z, eye, steps);
  const out = [];
  lights.forEach((l, i) => {
    if (s >= 0) {
      const zs = z.lightZones[i];
      let hit = false;
      for (let k = 0; k < zs.length && !hit; k++) hit = z.mark[zs[k]] === s;
      if (!hit) return;
    }
    const d = Math.hypot(l.pos[0] - eye[0], l.pos[1] - eye[1], l.pos[2] - eye[2]);
    if (d > l.range && vp && !sphereInFrustum(vp, l.pos, l.range)) return;
    out.push(i);
  });
  return out;
}

// How much a light matters on screen: intensity x its apparent size (range over
// distance, capped near the camera).
export function importance(l, eye) {
  const d = Math.hypot(l.pos[0] - eye[0], l.pos[1] - eye[1], l.pos[2] - eye[2]);
  return (l.intensity ?? 1) * Math.min(4, l.range / Math.max(d, 0.5));
}

// The shadow budget: the n most important shadow-casting lights among `active`,
// in a stable order (ties by index).
export function shadowPick(lights, active, eye, n) {
  if (n <= 0) return [];
  return active.filter((i) => lights[i].shadow)
    .map((i) => [i, importance(lights[i], eye)])
    .sort((a, b) => b[1] - a[1] || a[0] - b[0])
    .slice(0, n).map((e) => e[0]);
}

// ---- shadow maps for the shadowed lights ----

export const SHADOW_NEAR = 0.1;
// A point light renders a cube of 6 faces, a little wider than 90 degrees so the
// PCF taps near a face's edge stay inside its tile.
export const CUBE_FOV = (95 * Math.PI) / 180;
const CUBE = [
  { dir: [1, 0, 0], up: [0, 1, 0] }, { dir: [-1, 0, 0], up: [0, 1, 0] },
  { dir: [0, 1, 0], up: [0, 0, 1] }, { dir: [0, -1, 0], up: [0, 0, 1] },
  { dir: [0, 0, 1], up: [0, 1, 0] }, { dir: [0, 0, -1], up: [0, 1, 0] },
];

// The cube face (0..5: +x -x +y -y +z -z) that sees direction v from the light;
// the shader (gen/glsl/lights.js) picks the same way.
export function cubeFace(v) {
  const ax = Math.abs(v[0]), ay = Math.abs(v[1]), az = Math.abs(v[2]);
  if (ax >= ay && ax >= az) return v[0] > 0 ? 0 : 1;
  if (ay >= az) return v[1] > 0 ? 2 : 3;
  return v[2] > 0 ? 4 : 5;
}

// The views a shadowed light renders: one for a spot (its cone plus a margin, at
// most 170 degrees), six for a point. Each {vp (world -> clip), texel: the
// shadow-map texel's size in metres per metre of distance at tileSize}.
export function shadowViews(l, tileSize) {
  const far = Math.max(l.range, SHADOW_NEAR * 10);
  if (l.kind === "spot") {
    const { dir, outer } = coneOf(l);
    const fov = Math.min(2 * outer + 0.15, (170 * Math.PI) / 180);
    const up = Math.abs(dir[1]) > 0.99 ? [0, 0, 1] : [0, 1, 0];
    const view = lookAt(l.pos, [l.pos[0] + dir[0], l.pos[1] + dir[1], l.pos[2] + dir[2]], up);
    return [{ vp: mul(perspective(fov, 1, SHADOW_NEAR, far), view), texel: (2 * Math.tan(fov / 2)) / tileSize }];
  }
  const proj = perspective(CUBE_FOV, 1, SHADOW_NEAR, far);
  return CUBE.map((f) => ({
    vp: mul(proj, lookAt(l.pos, [l.pos[0] + f.dir[0], l.pos[1] + f.dir[1], l.pos[2] + f.dir[2]], f.up)),
    texel: (2 * Math.tan(CUBE_FOV / 2)) / tileSize,
  }));
}

// ---- static shadow atlas + live player shadows ----

// Tiles a shadowed light needs: a spot one view, a point light six (cube faces).
export const tilesOf = (l) => (l.kind === "spot" ? 1 : 6);

// Static atlas layout: every shadowed light gets consecutive tiles (base[i], -1 for
// unshadowed lights). The tile size is the largest of sizes whose atlas fits in
// maxSize x maxSize; with too many tiles even at the smallest size, the lights that
// don't fit get none (base -1). Returns {tile, cols, rows, tiles, base, width, height}.
export function atlasLayout(lights, maxSize = 4096, sizes = [512, 256, 128]) {
  const need = lights.reduce((n, l) => n + (l.shadow ? tilesOf(l) : 0), 0);
  let tile = sizes[sizes.length - 1];
  for (const t of sizes) if (Math.floor(maxSize / t) ** 2 >= need) { tile = t; break; }
  const maxCols = Math.floor(maxSize / tile), cap = maxCols * maxCols;
  const base = new Int32Array(lights.length).fill(-1);
  let used = 0;
  lights.forEach((l, i) => {
    if (!l.shadow) return;
    const n = tilesOf(l);
    if (used + n > cap) return;
    base[i] = used; used += n;
  });
  const cols = Math.max(1, Math.min(maxCols, used)), rows = Math.max(1, Math.ceil(used / cols));
  return { tile, cols, rows, tiles: used, base, width: cols * tile, height: rows * tile };
}

// Where tile t sits in an atlas of cols columns: [x, y] in tiles.
export const tileXY = (t, cols) => [t % cols, Math.floor(t / cols)];

// How much a light's live shadow (players drawn into it) matters this frame: for
// each player inside its range, the light's strength there, more for players near
// the camera and on screen. 0 = nobody near: the cached map shadow is enough.
export function dynamicScore(l, players, eye, onScreen) {
  let s = 0;
  for (let i = 0; i < players.length; i++) {
    const p = players[i];
    const dl = Math.hypot(p[0] - l.pos[0], p[1] + 0.9 - l.pos[1], p[2] - l.pos[2]);
    if (dl > l.range + 0.5) continue;
    const dc = Math.hypot(p[0] - eye[0], p[1] - eye[1], p[2] - eye[2]);
    s += (l.intensity ?? 1) * falloff(Math.min(dl, l.range * 0.99), l.range) * (onScreen(p) ? 1 : 0.25) / (1 + dc / 8);
  }
  return s;
}

// The top n lights by score > 0 (held ones count 30% more: no flip-flop between
// near-equal lights), stable on ties by index.
export function pickDynamic(scores, n, held) {
  if (n <= 0) return [];
  const out = [];
  scores.forEach((sc, i) => { if (sc > 0) out.push([i, sc * (held.has(i) ? 1.3 : 1)]); });
  return out.sort((a, b) => b[1] - a[1] || a[0] - b[0]).slice(0, n).map((e) => e[0]);
}

// First run of n free tiles in a pool (used: Uint8Array, 1 = taken); marks and returns
// its start, or -1.
export function allocTiles(used, n) {
  for (let s = 0; s + n <= used.length; s++) {
    let ok = true;
    for (let k = 0; k < n && ok; k++) ok = !used[s + k];
    if (ok) { used.fill(1, s, s + n); return s; }
  }
  return -1;
}

// A fade weight stepped toward target (0 or 1) over secs.
export const fadeToward = (w, target, dt, secs) => (target > w ? Math.min(target, w + dt / secs) : Math.max(target, w - dt / secs));
