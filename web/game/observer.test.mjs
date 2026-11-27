import test from "node:test";
import assert from "node:assert/strict";
import { overview, BACKDROP_BELOW } from "./observer.js";

const box = (x0, y0, z0, x1, y1, z1) => ({ min: [x0, y0, z0], max: [x1, y1, z1] });

test("overview: framed on the boxes played on, not a backdrop far below (rc_ci_pipeline's 520 m data sea)", () => {
  const track = [box(-90, -1, -90, 90, 0, 90), box(-1, -30, -1, 1, 0, 1)]; // a road and a pillar down into the void
  const sea = box(-260, -50, -260, 260, -48, 260);
  const map = { boxes: [...track, sea], spawns: [[0, 0, 0]] };
  const o = overview(map);
  assert.ok(o.eye[1] < 200 && o.eye[1] > 150, `eye at ${o.eye[1]} m: inside the far plane (400 m), the track filling the view`);
  assert.deepEqual(overview({ boxes: track, spawns: [[0, 0, 0]] }), o);
  // No spawns, or only backdrop: everything counts.
  assert.ok(overview({ boxes: [...track, sea] }).eye[1] > 400);
  assert.ok(overview({ boxes: [sea], spawns: [[0, 0, 0]] }).eye[1] > 400);
  assert.ok(BACKDROP_BELOW > 0);
  assert.deepEqual(overview(null), { eye: [0, 75, 0.01], yaw: 0, pitch: -1.5 });
});
