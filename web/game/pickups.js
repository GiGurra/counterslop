// Race power-ups drawn (internal/game pickups.go: the server's, in every
// snapshot's round as "pu": {k: kind, p: where on the road, up}). Each one up
// is a glowing icon floating over the road: a spinning, bobbing crystal in
// its kind's colour round a white core, a halo turning round it, and a ring
// of dashes on the road under it. A taken one is gone until the server says it is back.
import { segment } from "./rope.js";

export const PICKUP_COLORS = {
  bonus: [1, 0.85, 0.2], // Bonus Approved: gold
  hotfix: [0.3, 1, 0.5], // Hotfix Patch: green
  reorg: [1, 0.6, 0.2], // Reorg Budget: orange
  caffeine: [0.75, 0.42, 0.2], // Caffeine: coffee
  freeze: [0.5, 0.88, 1], // Code Freeze: ice
  flag: [1, 0.25, 0.95], // Feature Flag: magenta (a mystery)
};

export const PICKUP_NAMES = {
  bonus: "Bonus Approved", hotfix: "Hotfix Patch", reorg: "Reorg Budget",
  caffeine: "Caffeine", freeze: "Code Freeze", flag: "Feature Flag",
};

const FLOAT = 1.3, SIZE = 0.8, RING = 2.2, DASHES = 12, HALO = 10, HALO_R = 0.95;

// rotated: a box of size s at c, turned by yaw a about y and tipped 45 degrees (a crystal).
function rotated(c, s, a) {
  const ca = Math.cos(a), sa = Math.sin(a), k = Math.SQRT1_2;
  // x and z turned by a about y, then the whole tipped 45 degrees about the turned x.
  const x = [ca, 0, -sa], z0 = [sa, 0, ca], y0 = [0, 1, 0];
  const y = y0.map((v, i) => v * k + z0[i] * k), z = z0.map((v, i) => v * k - y0[i] * k);
  return new Float32Array([...x.map((v) => v * s), 0, ...y.map((v) => v * s), 0, ...z.map((v) => v * s), 0, c[0], c[1], c[2], 1]);
}

// pickupParts: render solids for every pickup up at time (seconds).
export function pickupParts(list, time) {
  const out = [];
  (list || []).forEach((pu, i) => {
    if (!pu.up) return;
    const col = PICKUP_COLORS[pu.k] || PICKUP_COLORS.flag, p = pu.p;
    const ph = time * 2 + i * 1.7, c = [p[0], p[1] + FLOAT + 0.18 * Math.sin(ph), p[2]];
    const pulse = 0.75 + 0.25 * Math.sin(ph * 2.3);
    out.push({ m: rotated(c, SIZE, time * 1.6 + i), color: col, emit: 0.9 * pulse });
    out.push({ m: rotated(c, SIZE * 0.45, -time * 2.4 - i), color: [1, 1, 1], emit: 1 });
    const spin = time * 2.2 + i; // a halo standing round the crystal, turning about y
    for (let d = 0; d < HALO; d++) {
      const a0 = (d / HALO) * Math.PI * 2, a1 = a0 + (Math.PI * 2) / HALO * 0.6, at = (a) => [c[0] + HALO_R * Math.cos(a) * Math.cos(spin), c[1] + HALO_R * Math.sin(a), c[2] + HALO_R * Math.cos(a) * Math.sin(spin)];
      const m = segment(at(a0), at(a1), 0.045);
      if (m) out.push({ m, color: col, emit: 0.8 * pulse });
    }
    for (let d = 0; d < DASHES; d++) { // the ring on the road, turning slowly
      const a0 = (d / DASHES) * Math.PI * 2 + time * 0.5, a1 = a0 + (Math.PI * 2) / DASHES * 0.55, y = p[1] + 0.06;
      const m = segment([p[0] + RING * Math.cos(a0), y, p[2] + RING * Math.sin(a0)], [p[0] + RING * Math.cos(a1), y, p[2] + RING * Math.sin(a1)], 0.06);
      if (m) out.push({ m, color: col, emit: 0.6 * pulse });
    }
  });
  return out;
}

// shieldParts: a Code Freeze on a car (its "sh" in the snapshot): two rings
// of ice-blue dashes round it, turning and flickering. pos: the car's (on
// the road); size: its wheelbase (m).
export function shieldParts(pos, size, time) {
  const out = [], col = PICKUP_COLORS.freeze, n = 16, flick = 0.6 + 0.4 * Math.sin(time * 23);
  for (const [h, r, dir] of [[0.5, size * 0.85 + 0.9, 1], [1.35, size * 0.55 + 0.6, -1]]) {
    for (let d = 0; d < n; d += 1) {
      const a0 = (d / n) * Math.PI * 2 + dir * time * 1.4, a1 = a0 + (Math.PI * 2) / n * 0.6;
      const m = segment([pos[0] + r * Math.cos(a0), pos[1] + h, pos[2] + r * Math.sin(a0)], [pos[0] + r * Math.cos(a1), pos[1] + h, pos[2] + r * Math.sin(a1)], 0.05);
      if (m) out.push({ m, color: col, emit: 0.8 * flick });
    }
  }
  return out;
}
