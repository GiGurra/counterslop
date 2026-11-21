// Moving escalator steps: where each step of a map conveyor (content.Conveyor,
// map JSON "conveyors") is at time t, as boxes the renderer draws with the
// conveyor's material in place of the map's static treads. Pure: the same t
// (the server clock, so every client agrees) gives the same steps.
//
// A conveyor of n treads (Rise high, Run deep) rising towards `up` along
// `axis`: n + 1 steps, step j at u = j + phase treads from the low end
// (j = -1 .. n-1), covering u*run .. (u+1)*run with its top at (u+1)*rise.
// Clipped to the flight: at the foot a step rises out of the floor, at the
// head it flattens to the upper floor's level and slides under the comb plate.

const INSET = 0.005; // off the balustrades: no shared faces with them
const DEPTH = 2; // risers: each step reaches this many treads down, so no gaps

export const frac = (x) => x - Math.floor(x);

// The treads' phase (0..1) at time t: how far along a tread each step has moved.
export function phase(cv, t) {
  const speed = cv.vel[cv.axis] * cv.up; // > 0: towards the top
  return frac((t * speed) / cv.run);
}

export const treads = (cv) => Math.round((cv.max[1] - cv.min[1]) / cv.rise);

// The step boxes [{min, max}] of one conveyor at time t (out: reused array).
export function conveyorSteps(cv, t, out = []) {
  out.length = 0;
  const n = treads(cv), p = phase(cv, t), a = cv.axis, c = 2 - a; // c: across
  const base = cv.min[1], low = cv.up > 0 ? cv.min[a] : cv.max[a], len = n * cv.run;
  for (let j = -1; j < n; j++) {
    const u = j + p;
    const s0 = Math.max(0, u * cv.run), s1 = Math.min(len, (u + 1) * cv.run);
    const top = base + Math.min(u + 1, n) * cv.rise;
    if (s1 - s0 < 1e-3 || top - base < 0.01) continue; // not out of the floor yet / gone under the comb
    const min = [0, Math.max(base - cv.rise, top - DEPTH * cv.rise - 0.02), 0], max = [0, top, 0];
    const e0 = low + cv.up * s0, e1 = low + cv.up * s1;
    min[a] = Math.min(e0, e1); max[a] = Math.max(e0, e1);
    min[c] = cv.min[c] + INSET; max[c] = cv.max[c] - INSET;
    out.push({ min, max, kind: cv.kind });
  }
  return out;
}

// Every conveyor's steps at time t, and whether a map box is a static tread
// the moving steps replace (inside a conveyor's volume).
export function allSteps(conveyors, t) {
  const out = [];
  for (const cv of conveyors || []) out.push(...conveyorSteps(cv, t));
  return out;
}

export function insideConveyor(b, conveyors) {
  return (conveyors || []).some((cv) => [0, 1, 2].every((i) => b.min[i] >= cv.min[i] - 1e-6 && b.max[i] <= cv.max[i] + 1e-6));
}
