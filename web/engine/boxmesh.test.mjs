import test from "node:test";
import assert from "node:assert/strict";
import { boxMesh, lowestFloor, wedge } from "./boxmesh.js";

// Downward faces built (4 vertices each).
const undersides = (m) => { let n = 0; for (let i = 1; i < m.nrm.length; i += 3) if (m.nrm[i] < 0) n++; return n / 4; };

test("undersides at the floor are skipped, a basement's ceiling is drawn", () => {
  const slab = { min: [-10, -1, -10], max: [10, 0, 10], kind: "concrete" };
  const crate = { min: [0, 0, 0], max: [1, 1, 1], kind: "crate" };
  const ledge = { min: [0, 2, 0], max: [1, 2.4, 1], kind: "metal" };
  const flat = [slab, crate, ledge];
  assert.equal(lowestFloor(flat), 0);
  assert.equal(undersides(boxMesh(flat, () => 0, lowestFloor(flat))), 1); // only the ledge
  // A garage under the slab: its floor is the lowest, the slab is its ceiling.
  const garage = { min: [-10, -5, -10], max: [10, -4, 10], kind: "concrete" };
  const withGarage = [...flat, garage];
  assert.equal(lowestFloor(withGarage), -4);
  assert.equal(undersides(boxMesh(withGarage, () => 0, lowestFloor(withGarage))), 3); // slab, crate, ledge
  assert.equal(undersides(boxMesh(flat, () => 0, -Infinity)), 3);
});

test("a wedge is 5 faces: the slope (its normal, uv along its length), the back, two triangles, the bottom", () => {
  for (const ramp of [1, 2, 3, 4]) {
    const b = { min: [0, 0, 0], max: [4, 3, 4], kind: "x", ramp };
    const m = boxMesh([b], () => 0, -Infinity);
    assert.equal(m.idx.length, 5 * 6, `ramp ${ramp}`);
    const w = wedge(b);
    // Every vertex is on or under the slope's plane (y <= its height there).
    for (let v = 0; v < m.pos.length / 3; v++) {
      const p = [m.pos[v * 3], m.pos[v * 3 + 1], m.pos[v * 3 + 2]];
      const u = w.sg > 0 ? p[w.ax] - b.min[w.ax] : b.max[w.ax] - p[w.ax];
      assert.ok(p[1] <= (3 * u) / 4 + 1e-6, `ramp ${ramp} vertex ${p}`);
    }
    // The slope: 4 vertices with the normal (n.y 0.8, 5 m long).
    let slope = 0;
    for (let v = 0; v < m.nrm.length / 3; v++) {
      if (Math.abs(m.nrm[v * 3 + 1] - 0.8) < 1e-6) {
        slope++;
        assert.ok(Math.abs(m.nrm[v * 3 + w.ax] + w.sg * 0.6) < 1e-6);
        assert.ok(Math.abs(Math.max(m.face[v * 4 + 2], m.face[v * 4 + 3]) - 5) < 1e-6, "uv along the slope");
      }
    }
    assert.equal(slope, 4, `ramp ${ramp}`);
  }
});
