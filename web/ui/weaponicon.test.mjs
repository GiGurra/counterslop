import test from "node:test";
import assert from "node:assert/strict";
import { iconRects, weaponIcon, KILL_GLYPH } from "./weaponicon.js";
import { MODELS } from "../assets/models/weapons.js";

test("every weapon model makes an icon that fills the box, muzzle on the right", () => {
  for (const id of Object.keys(MODELS)) {
    const r = iconRects(id, 64, 22, 1);
    assert.equal(r.length, MODELS[id].length, id);
    const x0 = Math.min(...r.map((b) => b[0])), x1 = Math.max(...r.map((b) => b[0] + b[2]));
    const y0 = Math.min(...r.map((b) => b[1])), y1 = Math.max(...r.map((b) => b[1] + b[3]));
    assert.ok(x0 >= 0.9 && x1 <= 63.1 && y0 >= 0.9 && y1 <= 21.1, `${id} inside: ${[x0, x1, y0, y1]}`);
    assert.ok(x1 - x0 > 61 || y1 - y0 > 19, `${id} fills one axis`);
    for (const b of r) assert.match(b[4], /^#[0-9a-f]{6}$/);
  }
  // the Prompt's muzzle tip (z = -0.45, the most negative) is the rightmost box
  const p = iconRects("prompt"), tipIdx = MODELS.prompt.findIndex((b) => b[2] === -0.45);
  assert.equal(Math.max(...p.map((b) => b[0])), p[tipIdx][0]);
});

test("no model, no icon: glyphs cover the rest; without a DOM the URL is empty", () => {
  assert.deepEqual(iconRects("fall"), []);
  assert.equal(weaponIcon("fall"), "");
  assert.equal(weaponIcon("prompt"), ""); // node: no canvas
  assert.ok(KILL_GLYPH.bigdiff && KILL_GLYPH.world);
});
