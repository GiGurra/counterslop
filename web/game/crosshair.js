// The crosshair tells the truth: it sits where the next shot goes and its gap is
// the cone that shot can land in, both projected onto the screen exactly as the
// renderer projects the world (vertical fov, viewport height). Presentation only:
// the inputs are the shared rules' own numbers (ShotSpread, the spray offset).
//
// ShotAngles (internal/game/recoil_shared.go) draws each ray's yaw and pitch
// offsets uniformly in [-spread/2, +spread/2] around the (recoiled) aim: a square
// in angle space. The gap is the radius of the circle around its centre that
// holds that whole square on screen (its corners), so every ray, every pellet,
// lands inside the four lines' tips.
import { basis } from "../engine/math.js";

export const MIN_GAP = 3; // px: the lines never merge into a blob
export const SHRINK_TAU = 0.012; // s: shrinking eases (settled in ~50-60 ms from fully open); growing is instant

const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];

// The screen offset (CSS px, x right, y down) of the direction (yaw, pitch) as
// seen by a camera looking along (camYaw, camPitch) with vertical fov `fov`
// (radians) in a viewport `h` px high. null when it is behind the camera.
export function project(camYaw, camPitch, yaw, pitch, fov, h) {
  const c = basis(camYaw, camPitch), d = basis(yaw, pitch).fwd;
  const z = dot(d, c.fwd);
  if (z <= 1e-6) return null;
  const k = (h / 2) / Math.tan(fov / 2);
  return [(dot(d, c.right) / z) * k, -(dot(d, c.up) / z) * k];
}

// The cone of a shot aimed at (yaw, pitch) with spread `spread`, seen from the
// camera: { x, y } its centre on screen (px from the screen centre), r the
// radius that covers every ray ShotAngles can draw.
export function shotCone({ camYaw, camPitch, yaw, pitch, spread, fov, h }) {
  const c = project(camYaw, camPitch, yaw, pitch, fov, h) || [0, 0];
  let r = 0;
  if (spread > 0) {
    const s = spread / 2;
    // The square's corners and edge midpoints: its farthest points on screen
    // (the projection bends edges by well under a pixel; the corners dominate).
    for (const [u, v] of [[-s, -s], [s, -s], [-s, s], [s, s], [s, 0], [-s, 0], [0, s], [0, -s]]) {
      const p = project(camYaw, camPitch, yaw + u, pitch + v, fov, h);
      if (!p) return { x: c[0], y: c[1], r: h }; // a cone past 90°: everything
      r = Math.max(r, Math.hypot(p[0] - c[0], p[1] - c[1]));
    }
  }
  return { x: c[0], y: c[1], r };
}

// The drawn crosshair: follows the cone's centre exactly, and its gap jumps
// out with the cone at once but eases back in (SHRINK_TAU), so it is never
// tighter than the truth, at most a frame or two wider while it settles.
export function createCrosshair({ min = MIN_GAP, tau = SHRINK_TAU } = {}) {
  const st = { x: 0, y: 0, gap: min, r: 0 };
  return {
    state: st,
    update(cone, dt) {
      const want = Math.max(min, cone.r);
      st.gap = want >= st.gap || tau <= 0 ? want : want + (st.gap - want) * Math.exp(-Math.max(0, dt) / tau);
      if (st.gap - want < 0.5) st.gap = want; // the last half pixel: done
      st.x = cone.x; st.y = cone.y; st.r = cone.r;
      return st;
    },
    reset() { st.x = 0; st.y = 0; st.gap = min; st.r = 0; return st; },
  };
}
