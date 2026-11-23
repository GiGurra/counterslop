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

test("itemPrice matches the server: helmet upgrade, helmet already on, warmup charges too", async () => {
  const { itemPrice } = await import("./buyrules.js");
  const helmet = { id: "helmet", price: 1000, upgrade: 350 };
  assert.equal(itemPrice(helmet, { armor: 0 }), 1000);
  assert.equal(itemPrice(helmet, { armor: 100 }), 350); // full vest: just the helmet
  assert.equal(itemPrice(helmet, { armor: 40, helmet: true }), 650); // helmet on, vest worn: the vest's price
  assert.equal(itemPrice(helmet, { armor: 40, helmet: true, warmup: true }), 650, "warmup: real prices (full money instead)");
});

test("ammo cards: reserve against the gun's full reserve, economy.json's price, FULL, no gun, can't afford", async () => {
  const { ammoCards } = await import("./buyrules.js");
  const econ = (await import("../assets/economy.json", { with: { type: "json" } })).default;
  const slots = hudSlots({ cur: "primary", slots: { primary: { id: "prompt", ammo: 30, reserve: 30 }, sidearm: { id: "stub", ammo: 12, reserve: 24 } } });
  const [p, s] = ammoCards({ slots, money: 16000 });
  assert.deepEqual([p.item, p.cmd, p.name, p.reserve, p.max, p.missing, p.full, p.none, p.price], ["ammo1", "buyammo1", "The Prompt", 30, 90, 60, false, false, econ.ammo.primary]);
  assert.ok(Math.abs(p.frac - 1 / 3) < 1e-9);
  assert.deepEqual([s.item, s.name, s.reserve, s.max, s.frac, s.full, s.price], ["ammo2", "The Stub", 24, 24, 1, true, econ.ammo.sidearm]);
  const [none] = ammoCards({ slots: hudSlots({ cur: "sidearm", slots: { sidearm: { id: "stub", ammo: 12, reserve: 0 } } }), money: 0 });
  assert.deepEqual([none.none, none.full, none.frac, none.name], [true, false, 0, ""]);
  const [, broke] = ammoCards({ slots: hudSlots({ cur: "sidearm", slots: { sidearm: { id: "stub", ammo: 12, reserve: 3 } } }), money: econ.ammo.sidearm - 1 });
  assert.deepEqual([broke.poor, broke.full, broke.missing], [true, false, 21]);
});

test("item stats: guns show damage (pellets × damage for shotguns) and mag; grenades and gear their own", async () => {
  const { itemStats } = await import("./buyrules.js");
  const cr = itemStats({ id: "codereview" });
  const w = WEAPONS.codereview, tb = WEAPONS.tokenburner;
  assert.deepEqual(cr.bars.map((b) => [b.label, b.text]), [["DAMAGE", String(w.damage)], ["MAG", String(w.mag)]]);
  assert.equal(cr.line, `${w.damage} dmg · ${w.mag} / ${w.reserve} · scoped`);
  assert.equal(itemStats({ id: "tokenburner" }).bars[0].text, `${tb.damage}×${tb.pellets}`);
  assert.equal(itemStats({ id: "tokenburner" }).bars[0].frac, 1); // capped
  assert.equal(itemStats({ id: "mergeconflict" }).line, `${WEAPONS.mergeconflict.damage} dmg · carry 1`);
  assert.equal(itemStats({ id: "stacktrace" }).line, "carry 2");
  assert.equal(itemStats({ id: "smoketest" }).line, "18 s · carry 1");
  assert.deepEqual(itemStats({ id: "vest" }).bars.map((b) => b.label), ["ARMOR"]);
  assert.equal(itemStats({ id: "drink" }).line, "speed +10% · lasts 10 s");
  for (const b of ["prompt", "contextwindow", "helmet", "kit", "shades"].flatMap((id) => itemStats({ id }).bars)) assert.ok(b.frac >= 0 && b.frac <= 1, JSON.stringify(b));
});

test("key caps: punctuation binds show as the character (, and . for ammo), others by name", async () => {
  const { keyCap } = await import("./buyrules.js");
  assert.equal(keyCap("Comma"), ",");
  assert.equal(keyCap("Period"), ".");
  assert.equal(keyCap("KeyB"), "B");
});
