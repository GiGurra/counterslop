// Gait: the cartoon walk cycle and idle sway. The rules are Go
// (internal/game/gait_shared.go, translated into game/shared.gen.js): the
// server steps every player's gait per command, sends it in each snapshot
// ("g": [phase, amp, air, crouch]) and ray-traces shots against the pose it
// gives, so the drawn pose is the hit pose. This file adapts it:
//
//   gaitFromWire(g)              snapshot "g" (ints in 1/GAIT_Q) -> Gait (null when absent)
//   newGait() / resetGait(g)     a Gait at rest (the local body in free cam)
//   gaitStep(g, dx, dy, dz, o)   one command tick: o = {onGround, crouch, id}
//   lerpGait(a, b, k)            between two states (phase forward only)
//   gaitBetween(ta, ga, tb, gb, t)  at tick t between two full snapshots
//
// The pose itself (feet, legs, upper body, head frames) is Model.Frames,
// reached through gen/boxmodel.js from a pose state's {gait, t}.
import * as G from "../game/shared.gen.js";

// The wire form: integers in 1/GAIT_Q, trailing zeros left out (Go gait.go).
export const GAIT_Q = 50;
export const gaitFromWire = (g) => (Array.isArray(g) ? { phase: (g[0] || 0) / GAIT_Q, amp: (g[1] || 0) / GAIT_Q, air: (g[2] || 0) / GAIT_Q, crouch: (g[3] || 0) / GAIT_Q } : null);

export const newGait = () => ({ phase: 0, amp: 0, air: 0, crouch: 0 });

export function resetGait(g) {
  g.phase = g.amp = g.air = g.crouch = 0;
  return g;
}

// The per-player factors (Go GaitKOf), cached by id.
const kCache = new Map();
export function gaitK(id) {
  const key = id | 0;
  let k = kCache.get(key);
  if (!k) kCache.set(key, (k = G.GaitKOf(key)));
  return k;
}

export function gaitStep(g, dx, dy, dz, o = {}) {
  G.Gait_Step(g, dx, dy, dz, !!o.onGround, !!o.crouch, gaitK(o.id).swing);
  return g;
}

export const lerpGait = (a, b, k) => G.LerpGait(a, b, k);

// The gait at tick t from the full snapshots around it (ticks ta <= tb, gaits
// ga, gb or null): Go GaitBetween, as the server's lag-compensation history.
export const gaitBetween = (ta, ga, tb, gb, t) => G.GaitBetween(ta, ga, tb, gb, t);

// The pose time (s) between snapshots at ticks a and b, k of the way (Go PoseTime).
export const poseTime = (a, b, k) => G.PoseTime(a, b, k);
