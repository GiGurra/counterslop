// Dynamic point lights (muzzle flashes first; explosions, flashbangs, torches later):
// pure, no GL (gl.js uploads what this builds). Game-agnostic: a light is
// {pos, color, intensity, range (< MASK/2 cells), life (seconds, Infinity = until
// removed), curve (a CURVES name or a function (age, life) -> 0..1; default "flash")}.
//
// - CURVES.flash: a fast attack from half to full, then an exponential decay windowed to
//   exactly zero at the end, with zero slope there (no hard cut).
// - createLightPool(): a fixed number of slots (the shader's uniform arrays); a new
//   light takes a free slot, else evicts the one that matters least right now.
// - createLightFlood() / lightMask(): which grid cells a light may reach, flooded from
//   its cell through open faces like a map light (engine/lights.js floodLight), so a
//   light in one room never lights the next one through the wall. The flood is cached
//   by start cell: a spraying player floods once per cell, not per shot.

import { floodLight, addWalls, wallScratch, cellOf, cellIndex } from "./lights.js";
import { MAX_DYN_LIGHTS, MASK } from "../gen/glsl/lights.js";

export { MAX_DYN_LIGHTS, MASK };
export const ATTACK = 0.008; // seconds from half to full brightness
const DECAY = 2.5; // e-folds over the decay part (before the end window)

// Brightness 0..1 at age t (seconds) of a flash lasting life seconds.
export function envelope(t, life) {
  if (!(t >= 0) || t >= life) return 0;
  const atk = Math.min(ATTACK, life * 0.25);
  if (t < atk) { const u = t / atk; return 0.5 + 0.5 * u * u * (3 - 2 * u); }
  const u = (t - atk) / (life - atk), w = 1 - u * u;
  return Math.exp(-DECAY * u) * w * w;
}

// Brightness curves over a light's life: (age, life) -> 0..1, 0 outside [0, life).
export const CURVES = {
  flash: envelope,
  steady: (t, life) => (t >= 0 && t < life ? 1 : 0),
};
export const curveOf = (l) => (typeof l.curve === "function" ? l.curve : CURVES[l.curve || "flash"] || envelope);

// A pool of n light slots. add() returns the slot taken (-1: dropped); live(now) the
// slots still lit with their strength (intensity x curve) this instant.
export function createLightPool(n = MAX_DYN_LIGHTS) {
  const slots = Array.from({ length: n }, () => null);
  let seq = 0;
  // How much slot s matters at now: its strength, times its apparent size from eye.
  const weight = (f, now, eye) => {
    const k = f.intensity * (f.t0 === now ? 1 : f.curveFn(now - f.t0, f.life)); // a new light at its peak
    if (!eye) return k;
    const d = Math.hypot(f.pos[0] - eye[0], f.pos[1] - eye[1], f.pos[2] - eye[2]);
    return k * Math.min(4, f.range / Math.max(d, 0.5));
  };
  function add(f, now, eye) {
    const e = { ...f, t0: now, seq: seq++, curveFn: curveOf(f) };
    let pick = slots.findIndex((s) => !s || now - s.t0 >= s.life);
    if (pick < 0) {
      let best = Infinity;
      slots.forEach((s, i) => {
        const w = weight(s, now, eye);
        if (w < best || (w === best && pick >= 0 && s.seq < slots[pick].seq)) { best = w; pick = i; }
      });
      if (weight(e, now, eye) < best) return -1; // weaker than everything lit: drop it
    }
    slots[pick] = e;
    return pick;
  }
  function live(now) {
    const out = [];
    slots.forEach((s, i) => {
      if (!s) return;
      const k = s.curveFn(now - s.t0, s.life);
      if (k <= 0) { if (now - s.t0 >= s.life) slots[i] = null; return; }
      out.push({ slot: i, light: s, strength: s.intensity * k });
    });
    return out;
  }
  return { add, live, slots, clear: () => slots.fill(null) };
}

// The cells a light at pos with range reaches (open cells + the walls around them),
// as grid cell indices; cached per start cell and range. g is engine/lights.js buildGrid.
export function createLightFlood(g, cacheSize = 64) {
  const scratch = new Int32Array(g.n).fill(-1), walls = wallScratch(g.n, scratch);
  const cache = new Map();
  let stamp = 0, floods = 0, hits = 0, ms = 0;
  function cells(pos, range) {
    const c = cellOf(g, pos);
    const key = cellIndex(g, c[0], c[1], c[2]) * 64 + Math.round(range * 2);
    let v = cache.get(key);
    if (v) { cache.delete(key); cache.set(key, v); hits++; return v; }
    const t0 = performance.now();
    const centre = c.map((k, a) => g.origin[a] + (k + 0.5) * g.cell);
    // Flood from the cell centre (the cached result serves every shot from this cell),
    // with half a cell diagonal more range to cover the cell.
    const s = stamp++;
    v = Int32Array.from(addWalls(g, floodLight(g, { kind: "point", pos: centre, range: range + g.cell * 0.87 }, scratch, s), walls, s));
    ms += performance.now() - t0; floods++;
    cache.set(key, v);
    if (cache.size > cacheSize) cache.delete(cache.keys().next().value);
    return v;
  }
  return { cells, stats: () => ({ floods, hits, ms }) };
}

// The MASK^3 mask around pos's cell: 255 where the light may reach, laid out as MASK
// layers (y) of MASK x MASK (x, z), the 2D-array texture's layout. Returns the cell
// offset of the mask's corner (grid cells) and fills out.
export function lightMask(g, pos, cells, out = new Uint8Array(MASK ** 3)) {
  const c = cellOf(g, pos), h = MASK >> 1;
  const off = [c[0] - h, c[1] - h, c[2] - h];
  out.fill(0);
  const { nx, nz } = g, layer = nx * nz;
  for (let k = 0; k < cells.length; k++) {
    const i = cells[k];
    const x = (i % nx) - off[0], z = (((i / nx) | 0) % nz) - off[2], y = ((i / layer) | 0) - off[1];
    if (x < 0 || y < 0 || z < 0 || x >= MASK || y >= MASK || z >= MASK) continue;
    out[x + MASK * (z + MASK * y)] = 255;
  }
  return { off, data: out };
}
