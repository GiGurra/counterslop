// Corpses (visual only: no collision, hitboxes, tags or radar dots). A dead
// player's body topples over stiffly, like a felled plank: one rigid rotation
// of the whole box model about its feet, falling the way the server says (fy:
// away from the killer, else backwards), then a small comic bounce, and it lies
// there until the player respawns. In warmup and with timed respawns (dv) it
// shrinks away instead. Everything follows from the snapshot (the death tick
// dk), so a client that joins late sees bodies already lying flat.
import { mul, translate, rotX, rotY, scale } from "../engine/math.js";

export const FALL_S = 0.6; // the topple: ease-in, like gravity
export const BOUNCE_S = 0.35; // two little hops on landing, each smaller
export const VANISH_S = 0.3; // warmup: shrink away
const FLAT = Math.PI / 2;
const BOUNCES = [[0, 0.2, 0.16], [0.2, 0.35, 0.05]]; // [from, to (s after landing), lift (rad)]
export const LIFT = 0.005; // lying bodies sit a few mm above the floor (no z-fighting)

// The pose `age` seconds after death: {angle (rad from upright), scale}, or
// null once a vanishing body is gone.
export function corpsePose(age, vanish) {
  const t = Math.max(0, age);
  if (vanish) {
    if (t >= VANISH_S) return null;
    const k = t / VANISH_S;
    return { angle: 0, scale: 1 - k * k };
  }
  if (t < FALL_S) {
    const k = t / FALL_S;
    return { angle: FLAT * k * k, scale: 1 };
  }
  const b = t - FALL_S;
  for (const [from, to, lift] of BOUNCES) {
    if (b >= from && b < to) return { angle: FLAT - lift * Math.sin(Math.PI * (b - from) / (to - from)), scale: 1 };
  }
  return { angle: FLAT, scale: 1 };
}

// Seconds since landing crossed in (age0, age1]: true once, for the dust puff.
export const landedBetween = (age0, age1) => age0 < FALL_S && age1 >= FALL_S;

// Render parts [{m, ...}] of an upright body standing at feet (x, y, z), toppled
// by pose toward yaw `fy` (its forward: yaw 0 = -z). The body is lifted so the
// lowest corner of the parts that `rests` on the floor stays on it: it rolls
// over its lowest edge rather than sinking into the ground. Parts that don't
// rest (a held gun) may poke into the floor rather than prop the body up.
export function corpseParts(parts, feet, fy, pose, rests = () => true) {
  const [x, y, z] = feet;
  // about the feet: face the fall direction, tip forward (-z), turn back
  const tip = mul(translate(x, y, z), mul(rotY(fy), mul(rotX(-pose.angle), mul(rotY(-fy), mul(scale(pose.scale, pose.scale, pose.scale), translate(-x, -y, -z))))));
  const out = parts.map((pt) => ({ ...pt, m: mul(tip, pt.m) }));
  let low = Infinity;
  for (const pt of out) {
    if (!rests(pt)) continue;
    const { m } = pt;
    for (const cx of [-0.5, 0.5]) for (const cy of [-0.5, 0.5]) for (const cz of [-0.5, 0.5]) {
      low = Math.min(low, m[1] * cx + m[5] * cy + m[9] * cz + m[13]);
    }
  }
  const up = y + LIFT - low;
  if (up !== 0) for (const pt of out) { pt.m = mul(translate(0, up, 0), pt.m); }
  return out;
}

// The player model: everything but the held gun rests on the floor.
export const bodyRests = (pt) => !String(pt.name ?? "").startsWith("gun");

// The death age in seconds of a remote drawn at server tick `viewTick`.
export const deathAge = (r, viewTick, tickHz = 100) => (viewTick - r.dk) / tickHz;
