// The sky from where you stand (content.Map.SkyZones): from the Earth the
// Moon, from the Moon the Earth (Star Slop: the lead saw the Moon in the sky
// while standing on it).
import { test } from "node:test";
import assert from "node:assert/strict";
import { SKIES, SKY, skyAt } from "./sky.js";

const map = {
  sky: "space",
  skyZones: [
    { name: "earth", min: [-400, -100, -200], max: [-125, 1000, 200] },
    { name: "moon", min: [75, -100, -200], max: [400, 1000, 200] },
  ],
};

test("skyAt: the zone holding the eye, else the map's sky", () => {
  assert.equal(skyAt(map, [-250, 1.6, 0]), "earth");
  assert.equal(skyAt(map, [170, 31.6, 0]), "moon");
  assert.equal(skyAt(map, [0, 120, 0]), "space"); // in flight: both
  assert.equal(skyAt({ sky: "" }, [0, 0, 0]), "");
  assert.equal(skyAt(null, [0, 0, 0]), "");
});

test("every sky a zone names is one the shader draws", () => {
  for (const z of map.skyZones) assert.ok(SKIES.indexOf(z.name) > 0, z.name);
  assert.match(SKY, /uSkyMode == 2/);
  assert.match(SKY, /uSkyMode == 3/);
});
