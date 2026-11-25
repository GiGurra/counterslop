// The view in a gravity of one's own (the Gravity Override boots;
// internal/sim gravity.go): each gravity's frame as a rotation matrix, and
// the camera's roll from one frame to the next, eased (render only: the
// movement, the shots and the hit test use the exact frames, generated from Go).
import { FromFrame } from "./sim.gen.js";

// frameOf(g): gravity g's frame as a column-major mat4 rotation (world from
// frame: its columns are the frame's x, y (up) and z in the world). null for
// the world's own gravity (no turn).
const FRAMES = [0, 1, 2, 3, 4, 5].map((g) => {
  if (g === 0) return null;
  const [x, y, z] = [[1, 0, 0], [0, 1, 0], [0, 0, 1]].map((v) => FromFrame(g, v));
  return new Float32Array([...x, 0, ...y, 0, ...z, 0, 0, 0, 0, 1]);
});
export const frameOf = (g) => FRAMES[g | 0] ?? null;

// turn(frame, v): direction v (frame coordinates) in the world; v itself without a frame.
export const turn = (m, v) => (m ? [
  m[0] * v[0] + m[4] * v[1] + m[8] * v[2],
  m[1] * v[0] + m[5] * v[1] + m[9] * v[2],
  m[2] * v[0] + m[6] * v[1] + m[10] * v[2],
] : v);

// A rotation matrix (column-major mat4, or null: none) as a unit quaternion [x, y, z, w].
export function quatOf(m) {
  if (!m) return [0, 0, 0, 1];
  const [a, b, c, , d, e, f, , g, h, i] = m; // columns: (a b c) (d e f) (g h i)
  const tr = a + e + i;
  let q;
  if (tr > 0) { const s = Math.sqrt(tr + 1) * 2; q = [(f - h) / s, (g - c) / s, (b - d) / s, s / 4]; }
  else if (a > e && a > i) { const s = Math.sqrt(1 + a - e - i) * 2; q = [s / 4, (d + b) / s, (g + c) / s, (f - h) / s]; }
  else if (e > i) { const s = Math.sqrt(1 + e - a - i) * 2; q = [(d + b) / s, s / 4, (h + f) / s, (g - c) / s]; }
  else { const s = Math.sqrt(1 + i - a - e) * 2; q = [(g + c) / s, (h + f) / s, s / 4, (b - d) / s]; }
  const l = Math.hypot(...q);
  return q.map((v) => v / l);
}

// A unit quaternion as a column-major mat4 rotation.
export function matOf([x, y, z, w]) {
  return new Float32Array([
    1 - 2 * (y * y + z * z), 2 * (x * y + z * w), 2 * (x * z - y * w), 0,
    2 * (x * y - z * w), 1 - 2 * (x * x + z * z), 2 * (y * z + x * w), 0,
    2 * (x * z + y * w), 2 * (y * z - x * w), 1 - 2 * (x * x + y * y), 0,
    0, 0, 0, 1,
  ]);
}

export function slerp(a, b, t) {
  let d = a[0] * b[0] + a[1] * b[1] + a[2] * b[2] + a[3] * b[3];
  const bb = d < 0 ? b.map((v) => -v) : b; // the short way round
  d = Math.abs(d);
  if (d > 0.9995) { const q = a.map((v, i) => v + (bb[i] - v) * t), l = Math.hypot(...q); return q.map((v) => v / l); }
  const th = Math.acos(d), s = Math.sin(th), ka = Math.sin((1 - t) * th) / s, kb = Math.sin(t * th) / s;
  return a.map((v, i) => ka * v + kb * bb[i]);
}

// The camera's roll: eases toward gravity g's frame (ROLL_RATE per second,
// exponential: a wall flip turns most of the way in a quarter second).
// update(dt, g) -> the eased frame (mat4, null once settled on the world's);
// snap(g) jumps there (a spawn).
export const ROLL_RATE = 9;
export function createRoll() {
  let q = [0, 0, 0, 1], g = 0, settled = true;
  return {
    update(dt, to) {
      to |= 0;
      if (to !== g) { g = to; settled = false; }
      if (settled) return frameOf(g);
      const target = quatOf(frameOf(g));
      q = slerp(q, target, 1 - Math.exp(-ROLL_RATE * dt));
      if (Math.abs(q[0] * target[0] + q[1] * target[1] + q[2] * target[2] + q[3] * target[3]) > 0.999999) { q = target; settled = true; return frameOf(g); }
      return matOf(q);
    },
    snap(to) { g = to | 0; q = quatOf(frameOf(g)); settled = true; },
    get settled() { return settled; },
  };
}

// unturn(frame, v): world direction v in the frame's coordinates (turn undone).
export const unturn = (m, v) => (m ? [
  m[0] * v[0] + m[1] * v[1] + m[2] * v[2],
  m[4] * v[0] + m[5] * v[1] + m[6] * v[2],
  m[8] * v[0] + m[9] * v[1] + m[10] * v[2],
] : v);

// above(p, g, h): the point h up from feet p along gravity g's up (= Go sim.Above).
export function above(p, g, h) {
  const u = turn(frameOf(g), [0, 1, 0]);
  return [p[0] + u[0] * h, p[1] + u[1] * h, p[2] + u[2] * h];
}
