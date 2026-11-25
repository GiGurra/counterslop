// The Gravity Override's hook line (internal/sim hook.go), drawn: a thin
// glowing beam from the player's chest to the anchor, wobbling a little (a
// tractor beam on a budget), and a small clamp where it holds.
import { above } from "../engine/gravity.js";

const THICK = 0.035; // m
const CHEST = 1.25; // m up from the feet, along the player's up

// segment: a column-major mat4 taking the unit cube (-0.5..0.5) to a box
// THICK wide from a to b.
export function segment(a, b, thick = THICK) {
  const d = [b[0] - a[0], b[1] - a[1], b[2] - a[2]], len = Math.hypot(...d);
  if (len < 1e-6) return null;
  const y = d.map((v) => v / len);
  const ref = Math.abs(y[1]) < 0.9 ? [0, 1, 0] : [1, 0, 0];
  let x = [y[1] * ref[2] - y[2] * ref[1], y[2] * ref[0] - y[0] * ref[2], y[0] * ref[1] - y[1] * ref[0]];
  const xl = Math.hypot(...x);
  x = x.map((v) => v / xl);
  const z = [x[1] * y[2] - x[2] * y[1], x[2] * y[0] - x[0] * y[2], x[0] * y[1] - x[1] * y[0]];
  const c = [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2, (a[2] + b[2]) / 2];
  return new Float32Array([x[0] * thick, x[1] * thick, x[2] * thick, 0, y[0] * len, y[1] * len, y[2] * len, 0, z[0] * thick, z[1] * thick, z[2] * thick, 0, c[0], c[1], c[2], 1]);
}

// ropeParts: the beam for a player at feet p (gravity gv) hooked at anchor,
// as render solids; time wobbles its glow. [] when not hooked. start: where
// the beam leaves (default its chest; our own: the gun hand).
export function ropeParts(p, gv, anchor, rgb, time = 0, start = null) {
  if (!anchor) return [];
  const from = start ?? above(p, gv | 0, CHEST), m = segment(from, anchor);
  if (!m) return [];
  const glow = 0.7 + 0.3 * Math.sin(time * 40);
  const clamp = new Float32Array([0.14, 0, 0, 0, 0, 0.14, 0, 0, 0, 0, 0.14, 0, anchor[0], anchor[1], anchor[2], 1]);
  return [{ m, color: rgb, emit: glow }, { m: clamp, color: [0.8, 0.85, 0.9], emit: 0.2 }];
}
