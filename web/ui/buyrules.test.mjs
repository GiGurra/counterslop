import { test } from "node:test";
import assert from "node:assert/strict";
import { owned } from "./buyrules.js";
import { WEAPONS } from "../assets/weapons/index.js";
import { createArsenal } from "../game/arsenal.js";

// The HUD's slot rows as arsenal.js shows them.
function hudSlots(inv) {
  let last = {};
  const a = createArsenal({ net: { send() {} }, hud: { set(v) { Object.assign(last, v); }, get: () => last }, zoomMode: () => "toggle" });
  a.setInv(inv);
  return last.slots;
}

test("a gun you hold shows as owned (the HUD's slot rows carry the weapon id)", () => {
  const slots = hudSlots({ cur: "primary", slots: { primary: { id: "prompt", ammo: 30, reserve: 90 }, sidearm: { id: "stub", ammo: 12, reserve: 24 } } });
  assert.equal(owned(WEAPONS.prompt, { slots }), true);
  assert.equal(owned(WEAPONS.stub, { slots }), true);
  assert.equal(owned(WEAPONS.linter, { slots }), false);
});

test("grenades are owned at their carry limit: one flash of two is not, two are", () => {
  const one = hudSlots({ cur: "primary", slots: { flash: { id: "stacktrace", ammo: 1, reserve: 0 }, he: { id: "mergeconflict", ammo: 1, reserve: 0 } } });
  assert.equal(owned(WEAPONS.stacktrace, { slots: one }), false);
  assert.equal(owned(WEAPONS.mergeconflict, { slots: one }), true);
  assert.equal(owned(WEAPONS.smoketest, { slots: one }), false);
  const two = hudSlots({ cur: "primary", slots: { flash: { id: "stacktrace", ammo: 2, reserve: 0 } } });
  assert.equal(owned(WEAPONS.stacktrace, { slots: two }), true);
});

test("sunglasses are owned while worn; a drink while it lasts (buy the next when it wears off)", () => {
  const shades = { id: "shades" }, drink = { id: "drink" };
  assert.equal(owned(shades, { shades: true }), true);
  assert.equal(owned(shades, {}), false);
  assert.equal(owned(drink, { boost: 4 }), true);
  assert.equal(owned(drink, { boost: 0 }), false);
});

test("itemPrice matches the server: helmet upgrade, helmet already on, warmup", async () => {
  const { itemPrice } = await import("./buyrules.js");
  const helmet = { id: "helmet", price: 1000, upgrade: 350 };
  assert.equal(itemPrice(helmet, { armor: 0 }), 1000);
  assert.equal(itemPrice(helmet, { armor: 100 }), 350); // full vest: just the helmet
  assert.equal(itemPrice(helmet, { armor: 40, helmet: true }), 650); // helmet on, vest worn: the vest's price
  assert.equal(itemPrice(helmet, { armor: 40, helmet: true, warmup: true }), 0);
});
