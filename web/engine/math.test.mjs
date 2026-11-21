// node --test web/  (also run by `go test ./web`)
import test from "node:test";
import assert from "node:assert/strict";
import { zoomSensitivity, fogFor, FOG_REF_SIDE } from "./math.js";

test("zoomSensitivity matches the screen-centre motion", () => {
  assert.equal(zoomSensitivity(1.3, 1.3), 1);
  const k = zoomSensitivity(0.32, 1.3);
  assert.ok(Math.abs(k - Math.tan(0.16) / Math.tan(0.65)) < 1e-12);
  assert.ok(k > 0.2 && k < 0.25); // Code Review zoom ≈ 4.4x slower
  assert.equal(zoomSensitivity(0.32, 1.3, 2), k * 2);
  // A small angle d moves the centre by f*tan(d), f = (h/2)/tan(fov/2); scaling d by k keeps it equal.
  const d = 0.01, px = (fov, a) => 360 / Math.tan(fov / 2) * Math.tan(a);
  assert.ok(Math.abs(px(1.3, d) - px(0.32, d * k)) < 0.01);
});

test("fogFor: training-size maps keep the base fog, bigger ones thin it by size", () => {
  assert.equal(fogFor(0.014, null), 0.014);
  assert.equal(fogFor(0.014, [[-30, -1, -30], [30, 13, 30]]), 0.014);
  assert.equal(fogFor(0.014, [[-37, 0, -10], [37, 5, 10]]), 0.014); // exactly the reference side
  const big = fogFor(0.014, [[-71, -1, -61], [71, 10, 61]]);
  assert.ok(Math.abs(big - 0.014 * FOG_REF_SIDE / 142) < 1e-12, String(big));
  // The haze at the far wall is the same as a reference map's at its far wall.
  assert.ok(Math.abs(big * 142 - 0.014 * FOG_REF_SIDE) < 1e-9);
});
