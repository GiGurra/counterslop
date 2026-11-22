import test from "node:test";
import assert from "node:assert/strict";
import { boxMesh, lowestFloor } from "./boxmesh.js";

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
