// Round-lived marks: bullet holes (decals) and what decides when marks clear.
// Pure: no GL, no game state; main.js feeds it impacts and the round state.

// A fixed-capacity ring of float records: adding past the cap overwrites the
// oldest. data is laid out for instanced drawing (stride floats per record).
export function createRing(cap, stride) {
  const data = new Float32Array(cap * stride);
  let head = 0, count = 0, version = 0;
  return {
    cap, stride, data,
    get count() { return count; },
    get version() { return version; }, // bumps on every change (re-upload when it moved)
    // The slot the next record goes in (the oldest once full); write it via data.
    add() {
      const slot = head;
      head = (head + 1) % cap;
      count = Math.min(count + 1, cap);
      version++;
      return slot;
    },
    // Slots oldest first.
    slots() {
      const out = [];
      for (let i = 0; i < count; i++) out.push((head - count + i + cap) % cap);
      return out;
    },
    touch() { version++; },
    clear() { head = 0; count = 0; version++; },
  };
}

export const HOLE_STRIDE = 8; // pos.xyz, seed, normal.xyz, size
export const MAX_HOLES = 1024;

// Bullet holes: a hole sits on a box face (n = an axis normal), its seed varies
// its shape, its size whether it was a pistol pip or a rifle punch.
export function createHoles(cap = MAX_HOLES) {
  const ring = createRing(cap, HOLE_STRIDE);
  return {
    ring,
    get count() { return ring.count; },
    add(p, n, size = 0.05) {
      const o = ring.add() * HOLE_STRIDE, d = ring.data;
      d[o] = p[0]; d[o + 1] = p[1]; d[o + 2] = p[2];
      d[o + 3] = seedOf(p);
      d[o + 4] = n[0]; d[o + 5] = n[1]; d[o + 6] = n[2];
      d[o + 7] = size;
    },
    clear: () => ring.clear(),
  };
}

// Deterministic per-position seed in [0, 1): the same hole looks the same on every client.
export function seedOf(p) {
  let h = 2166136261;
  for (const v of p) { h ^= Math.round(v * 1000) | 0; h = Math.imul(h, 16777619); }
  h ^= h >>> 13; h = Math.imul(h, 0x5bd1e995); h ^= h >>> 15;
  return (h >>> 0) / 4294967296;
}

// The map box a surface point lies on (p on the face, n its outward normal), or null.
export function boxAt(boxes, p, n) {
  const q = [p[0] - n[0] * 0.01, p[1] - n[1] * 0.01, p[2] - n[2] * 0.01], e = 0.002;
  for (const b of boxes) {
    if (q[0] >= b.min[0] - e && q[0] <= b.max[0] + e && q[1] >= b.min[1] - e && q[1] <= b.max[1] + e && q[2] >= b.min[2] - e && q[2] <= b.max[2] + e) return b;
  }
  return null;
}

// When marks clear: a new round (the round number in the round state), a new map.
// Warmup and deathmatch have no rounds (key null): marks stay, capped by their rings.
// update() returns true when everything should be cleared now.
export function roundKey(round) {
  if (!round || round.ph === "warmup" || round.n == null) return null;
  return round.n;
}
export function createLifetime() {
  let key, mapKey, phase;
  return {
    update(round, mapName) {
      const k = roundKey(round), ph = round?.ph ?? null, first = key === undefined;
      // A round starts: its number moves on, or it (re)enters freeze time (a match restart back to round 1).
      const clear = !first && (k !== key || mapName !== mapKey || (ph === "freeze" && phase !== "freeze"));
      key = k; mapKey = mapName; phase = ph;
      return clear;
    },
  };
}
