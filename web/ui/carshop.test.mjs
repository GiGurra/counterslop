import test from "node:test";
import assert from "node:assert/strict";
import { carShopCategories } from "./carshop.js";
import { owned, itemPrice, itemStats } from "./buyrules.js";

test("the race shop: cars by tier, item swaps (built-ins free), upgrades tier by tier; what you have is owned", () => {
  const [cars, gear, ups] = carShopCategories({}, "buggy");
  assert.deepEqual(cars.items.map((c) => [c.id, c.price, c.have]), [["car:buggy", 0, true], ["car:racecar", 2500, false], ["car:tank", 4500, false], ["car:hovercraft", 6500, false]]);
  const beam = gear.items.find((x) => x.id === "front:linter_beam"), rockets = gear.items.find((x) => x.id === "front:hotfix_missiles");
  assert.ok(beam.have && owned(beam, {}) && itemPrice(beam, {}) === 0);
  assert.ok(!rockets.have && rockets.price === 500);
  assert.deepEqual(ups.items.map((u) => [u.id, u.price]), [["up:charges", 600], ["up:tank", 400], ["up:plating", 500], ["up:engine", 700], ["up:tyres", 400]]);
  const g = { car: "hovercraft", f: "hotfix_missiles", pl: 3, en: 1 };
  const [, gear2, ups2] = carShopCategories(g, "buggy");
  assert.ok(gear2.items.find((x) => x.id === "front:hotfix_missiles").have);
  assert.equal(gear2.items.find((x) => x.id === "front:synergy_orbs").price, 0, "the hovercraft's own built-in: free back");
  const plating = ups2.items.find((x) => x.id === "up:plating"), engine = ups2.items.find((x) => x.id === "up:engine"), tyres = ups2.items.find((x) => x.id === "up:tyres");
  assert.ok(plating.have && plating.name === "Plating III", "maxed");
  assert.equal(engine.price, 1500, "tier II");
  assert.ok(tyres.have && /no tyres/.test(tyres.desc), "no tyres on a hovercraft");
  assert.equal(itemStats(engine).type, engine.desc);
});
