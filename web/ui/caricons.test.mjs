import test from "node:test";
import assert from "node:assert/strict";
import { carIconRects, carShopIconSvg, carShopGlyph } from "./caricons.js";
import carsData from "../assets/cars.json" with { type: "json" };

test("every car type's icon is its model seen from the side, inside the box, in its colours", () => {
  for (const c of carsData.cars) {
    const rs = carIconRects(c.id);
    assert.ok(rs.length >= 4, c.id);
    for (const [x, y, w, h, col] of rs) {
      assert.ok(x >= -0.01 && y >= -0.01 && x + w <= 64.01 && y + h <= 28.01, `${c.id}: ${[x, y, w, h]}`);
      assert.match(col, /^#[0-9a-f]{6}$/);
    }
    assert.match(carShopIconSvg(`car:${c.id}`), /^<svg class="car-icon"/);
  }
  assert.deepEqual(carIconRects("no-such-car"), []);
});

test("every item and upgrade in the shop has a glyph", () => {
  for (const it of carsData.equipment) assert.ok(carShopGlyph(`${it.slot}:${it.id}`).startsWith("<svg"), it.id);
  for (const u of ["charges", "tank", "plating", "engine", "tyres"]) assert.ok(carShopGlyph(`up:${u}`).startsWith("<svg"), u);
  assert.equal(carShopIconSvg("prompt"), "");
  assert.equal(carShopIconSvg("up:nope"), "");
});
