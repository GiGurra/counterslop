import test from "node:test";
import assert from "node:assert/strict";
import { chaseEye, convergeAim } from "./carcam.js";
import { basis } from "../engine/math.js";

const spec = { size: [1.7, 1.2, 3] };

test("the chase camera sits behind and over the car, looking where we aim", () => {
  const e = chaseEye(spec, [0, 0, 0], 0, 0); // facing -z
  assert.deepEqual(e.map((v) => Math.round(v * 100) / 100), [0, 2.3, 6.5]);
  assert.ok(chaseEye(spec, [0, 0, 0], 0, -1.4)[1] >= 0.5, "never under the car");
});

test("the shot converges on the crosshair's point: from the seat to where the camera's centre ray hits", () => {
  const cam = chaseEye(spec, [0, 0, 0], 0, 0), seat = [0, 1.5, 0.3];
  const wall = (t) => () => ({ t }); // a wall t metres down the camera's ray
  const [y, p] = convergeAim(cam, 0, 0, seat, wall(16.5)); // the point: 10 m ahead of the car, at 2.3 m
  const d = basis(y, p).fwd, k = (10 + 0.3) / -d[2];
  const hitAt = seat.map((v, i) => v + d[i] * k);
  assert.ok(Math.abs(hitAt[0]) < 1e-9 && Math.abs(hitAt[1] - 2.3) < 1e-9 && Math.abs(hitAt[2] + 10) < 1e-9, `${hitAt}`);
  const [y2, p2] = convergeAim(cam, 0.3, 0, seat, () => null, 250); // nothing hit: converge at the far distance
  assert.ok(Math.abs(y2 - 0.3) < 0.01 && Math.abs(p2) < 0.01);
});
