// The arc a map's pads throw along (content.Launcher; internal/sim zones.go),
// drawn as a faint dashed line flowing from the pad: a jump pad's to where it
// lands (PadVel from the pad's middle, the generated sim's), an accelerator
// or launch pad's first 1.5 s. Pads carried by a mover (a ship's drop pods)
// aren't drawn. Presentation only: the throw itself is the shared sim's.
import { PadVel, GravityAt, Grav } from "../engine/sim.gen.js";
import { segment } from "./rope.js";

const STEPS = 36; // points along an arc
const THICK = 0.03; // m
const COLOR = [0.35, 0.9, 1.0];
const OPEN_FLIGHT = 1.5; // s of a fixed-velocity pad's throw drawn

// padArcs: per pad, the points of its arc (feet on the pad's middle).
export function padArcs(map) {
  const out = [];
  for (const l of map.launchers || []) {
    if (l.mover) continue;
    const from = [(l.min[0] + l.max[0]) / 2, l.min[1] + 0.1, (l.min[2] + l.max[2]) / 2];
    const g = GravityAt(map.gravZones || [], from, Grav);
    if (!(g > 0)) continue;
    let v = l.vel, T = OPEN_FLIGHT;
    if (l.apex > 0) {
      v = PadVel(from, l.target, l.apex, g);
      T = v[1] / g + Math.sqrt((2 * (Math.max(from[1], l.target[1]) + l.apex - l.target[1])) / g);
    }
    const pts = [];
    for (let i = 0; i <= STEPS; i++) {
      const t = (T * i) / STEPS;
      pts.push([from[0] + v[0] * t, from[1] + v[1] * t - (g * t * t) / 2, from[2] + v[2] * t]);
    }
    out.push(pts);
  }
  return out;
}

// padArcParts: the dashes as render solids at time (they flow along the arc).
export function padArcParts(arcs, time) {
  const out = [], shift = Math.floor(time * 8);
  for (const pts of arcs) {
    for (let i = 0; i + 1 < pts.length; i++) {
      if ((((i - shift) % 3) + 3) % 3 !== 0) continue;
      const m = segment(pts[i], pts[i + 1], THICK);
      if (m) out.push({ m, color: COLOR, emit: 0.45 });
    }
  }
  return out;
}
