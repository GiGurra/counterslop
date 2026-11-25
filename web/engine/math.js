// Minimal column-major mat4 helpers (m[col*4+row]), enough for a camera.

export function perspective(fovy, aspect, near, far) {
  const f = 1 / Math.tan(fovy / 2), nf = 1 / (near - far);
  return new Float32Array([
    f / aspect, 0, 0, 0,
    0, f, 0, 0,
    0, 0, (far + near) * nf, -1,
    0, 0, 2 * far * near * nf, 0,
  ]);
}

export function mul(a, b) {
  const o = new Float32Array(16);
  for (let c = 0; c < 4; c++)
    for (let r = 0; r < 4; r++) {
      let s = 0;
      for (let k = 0; k < 4; k++) s += a[k * 4 + r] * b[c * 4 + k];
      o[c * 4 + r] = s;
    }
  return o;
}

export function translate(x, y, z) {
  return new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, x, y, z, 1]);
}

export function scale(x, y, z) {
  return new Float32Array([x, 0, 0, 0, 0, y, 0, 0, 0, 0, z, 0, 0, 0, 0, 1]);
}

export function rotY(a) {
  const c = Math.cos(a), s = Math.sin(a);
  return new Float32Array([c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, 0, 0, 0, 1]);
}

export function rotX(a) {
  const c = Math.cos(a), s = Math.sin(a);
  return new Float32Array([1, 0, 0, 0, 0, c, s, 0, 0, -s, c, 0, 0, 0, 0, 1]);
}

export const identity = () => translate(0, 0, 0);

// View matrix for a camera at eye looking along yaw/pitch (yaw 0 = -Z).
// frame: the view's own frame (engine/gravity.js: a rotation, world from
// frame, a player in a gravity of its own), yaw and pitch within it; null: the world's.
export function fpsView(eye, yaw, pitch, frame = null) {
  const t = translate(-eye[0], -eye[1], -eye[2]);
  return mul(rotX(-pitch), mul(rotY(-yaw), frame ? mul(transpose3(frame), t) : t));
}

// The rotation part of a mat4, transposed (a rotation's inverse).
function transpose3(m) {
  return new Float32Array([m[0], m[4], m[8], 0, m[1], m[5], m[9], 0, m[2], m[6], m[10], 0, 0, 0, 0, 1]);
}

// Camera basis vectors in world space.
// frame: as fpsView's (yaw and pitch within it).
export function basis(yaw, pitch, frame = null) {
  const cp = Math.cos(pitch), sp = Math.sin(pitch), cy = Math.cos(yaw), sy = Math.sin(yaw);
  const b = {
    fwd: [-sy * cp, sp, -cy * cp],
    right: [cy, 0, -sy],
    up: [sy * sp, cp, cy * sp],
  };
  if (!frame) return b;
  const t = (v) => [0, 1, 2].map((i) => frame[i] * v[0] + frame[4 + i] * v[1] + frame[8 + i] * v[2]);
  return { fwd: t(b.fwd), right: t(b.right), up: t(b.up) };
}

export const lerp = (a, b, t) => a + (b - a) * t;

export function lerpAngle(a, b, t) {
  let d = b - a;
  while (d > Math.PI) d -= 2 * Math.PI;
  while (d < -Math.PI) d += 2 * Math.PI;
  return a + d * t;
}

export function rotZ(a) {
  const c = Math.cos(a), s = Math.sin(a);
  return new Float32Array([c, s, 0, 0, -s, c, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]);
}

// Multiply a chain of matrices left to right: chain(A, B, C) = A·B·C.
export const chain = (...ms) => ms.reduce((a, b) => mul(a, b));

// Orthographic projection (GL clip space, z in -1..1).
export function ortho(l, r, b, t, n, f) {
  return new Float32Array([
    2 / (r - l), 0, 0, 0,
    0, 2 / (t - b), 0, 0,
    0, 0, -2 / (f - n), 0,
    -(r + l) / (r - l), -(t + b) / (t - b), -(f + n) / (f - n), 1,
  ]);
}

// View matrix looking from eye towards target.
export function lookAt(eye, target, up) {
  const sub = (a, b) => [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
  const cross = (a, b) => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
  const nrm = (v) => { const l = Math.hypot(...v); return v.map((x) => x / l); };
  const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
  const z = nrm(sub(eye, target)), x = nrm(cross(up, z)), y = cross(z, x);
  return new Float32Array([
    x[0], y[0], z[0], 0,
    x[1], y[1], z[1], 0,
    x[2], y[2], z[2], 0,
    -dot(x, eye), -dot(y, eye), -dot(z, eye), 1,
  ]);
}

// Look speed while zoomed: scale by the zoom so the crosshair moves the same
// screen distance per count at the centre (monitor-distance matching at 0%).
export const zoomSensitivity = (zoomFov, baseFov, ratio = 1) => (Math.tan(zoomFov / 2) / Math.tan(baseFov / 2)) * ratio;

// Fog density for a map: the base density (per metre) up to a FOG_REF_SIDE m
// map, thinned in proportion beyond it, so a CS-scale map's long sightlines
// keep the haze a training map has at its far wall. bounds: [min, max] or null.
export const FOG_REF_SIDE = 74;
export function fogFor(base, bounds) {
  if (!bounds) return base;
  const side = Math.max(bounds[1][0] - bounds[0][0], bounds[1][2] - bounds[0][2]);
  return side > FOG_REF_SIDE ? base * FOG_REF_SIDE / side : base;
}
