import { test } from "node:test";
import assert from "node:assert/strict";
import { indoorAmbient } from "./lighting.js";

// A map's own indoor ambient (content.Map.Indoor) wins; without one, lamp-lit
// maps keep 0.3 and maps without lamps 0.55 (cq_moonshot's 12 m terminal was a
// brown gloom at 0.3: its lamps barely reach the floor).
test("indoorAmbient: the map's own, else the defaults by whether it has lamps", () => {
  assert.equal(indoorAmbient(0, 12), 0.3);
  assert.equal(indoorAmbient(undefined, 0), 0.55);
  assert.equal(indoorAmbient(0.85, 12), 0.85);
});
