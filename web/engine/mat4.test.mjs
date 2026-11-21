// The camera matrix helpers (math.js): identities a renderer relies on.
import test from "node:test";
import assert from "node:assert/strict";
import * as m from "./math.js";

const apply = (M, v) => [0, 1, 2, 3].map((r) => M[r] * v[0] + M[4 + r] * v[1] + M[8 + r] * v[2] + M[12 + r] * v[3]);
const close = (a, b, eps = 1e-5) => a.every((x, i) => Math.abs(x - b[i]) < eps);

test("identity, translate, scale and mul compose as column-major mat4", () => {
  assert.ok(close(apply(m.identity(), [1, 2, 3, 1]), [1, 2, 3, 1]));
  assert.ok(close(apply(m.translate(1, 2, 3), [0, 0, 0, 1]), [1, 2, 3, 1]));
  assert.ok(close(apply(m.scale(2, 3, 4), [1, 1, 1, 1]), [2, 3, 4, 1]));
  // chain(T, S) applies S first
  assert.ok(close(apply(m.chain(m.translate(1, 0, 0), m.scale(2, 2, 2)), [1, 0, 0, 1]), [3, 0, 0, 1]));
  assert.ok(close(m.mul(m.identity(), m.rotY(0.3)), m.rotY(0.3)));
});

test("rotations turn the axes the right way", () => {
  assert.ok(close(apply(m.rotY(Math.PI / 2), [0, 0, -1, 0]), [-1, 0, 0, 0]), "yaw +90°: -Z (forward) goes to -X");
  assert.ok(close(apply(m.rotX(Math.PI / 2), [0, 0, -1, 0]), [0, 1, 0, 0]), "pitch +90°: forward goes up");
  assert.ok(close(apply(m.rotZ(Math.PI / 2), [1, 0, 0, 0]), [0, 1, 0, 0]));
});

test("fpsView puts the camera at the origin looking down -Z; basis matches it", () => {
  const eye = [3, 1.6, -2], yaw = 0.7, pitch = 0.2;
  const V = m.fpsView(eye, yaw, pitch);
  assert.ok(close(apply(V, [...eye, 1]), [0, 0, 0, 1]));
  const { fwd, right, up } = m.basis(yaw, pitch);
  const ahead = eye.map((e, i) => e + fwd[i] * 5);
  assert.ok(close(apply(V, [...ahead, 1]), [0, 0, -5, 1]), "basis.fwd is the view's -Z");
  assert.ok(close(apply(V, [...right, 0]), [1, 0, 0, 0]));
  assert.ok(close(apply(V, [...up, 0]), [0, 1, 0, 0]));
});

test("lookAt sees the target straight ahead", () => {
  const V = m.lookAt([0, 5, 10], [0, 0, 0], [0, 1, 0]);
  const t = apply(V, [0, 0, 0, 1]);
  assert.ok(Math.abs(t[0]) < 1e-5 && Math.abs(t[1]) < 1e-5 && t[2] < 0);
});

test("perspective and ortho map the frustum to clip space", () => {
  const P = m.perspective(Math.PI / 2, 2, 0.1, 100);
  const near = apply(P, [0, 0, -0.1, 1]), far = apply(P, [0, 0, -100, 1]);
  assert.ok(Math.abs(near[2] / near[3] + 1) < 1e-4 && Math.abs(far[2] / far[3] - 1) < 1e-4);
  const O = m.ortho(-1, 1, -2, 2, 0, 10);
  assert.ok(close(apply(O, [1, 2, -10, 1]), [1, 1, 1, 1]));
});

test("lerp, lerpAngle (the short way round) and zoomSensitivity", () => {
  assert.equal(m.lerp(2, 4, 0.25), 2.5);
  assert.ok(Math.abs(m.lerpAngle(3, -3, 0.5) - (3 + (2 * Math.PI - 6) / 2)) < 1e-9);
  assert.ok(Math.abs(m.lerpAngle(-3, 3, 1) - (-3 - (2 * Math.PI - 6))) < 1e-9);
  assert.equal(m.zoomSensitivity(1, 1), 1);
  assert.ok(m.zoomSensitivity(0.3, 1.2, 1) < 0.3);
});
