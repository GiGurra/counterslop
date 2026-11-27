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

test("item stats: guns show damage (pellets × damage for shotguns), fire rate and accuracy on the card, every stat in the detail; grenades and gear their own", async () => {
  const { itemStats } = await import("./buyrules.js");
  const cr = itemStats({ id: "codereview" });
  const w = WEAPONS.codereview, tb = WEAPONS.tokenburner;
  assert.deepEqual(cr.bars.map((b) => [b.label, b.text]), [["DAMAGE", String(w.damage)], ["FIRE RATE", `${Math.round(60 / w.interval)} rpm`], ["ACCURACY", `±${(w.spread.base / 2 * 180 / Math.PI).toFixed(2)}°`]]);
  assert.deepEqual(cr.all.map((b) => b.label), ["DAMAGE", "FIRE RATE", "ACCURACY", "RANGE", "MOVE SPEED", "ARMOR PEN", "KILL REWARD"]);
  assert.equal(cr.line, `${w.damage} dmg · ${Math.round(60 / w.interval)} rpm · ${w.mag} / ${w.reserve} · scoped`);
  assert.deepEqual(cr.facts, [`mag ${w.mag} / ${w.reserve}`, `reload ${w.reload} s`, "scoped"]);
  assert.equal(cr.type, "Sniper rifle");
  assert.ok(cr.good.length > 10);
  assert.equal(itemStats({ id: "tokenburner" }).bars[0].text, `${tb.damage}×${tb.pellets}`);
  assert.equal(itemStats({ id: "tokenburner" }).bars[0].frac, 1); // capped
  assert.equal(itemStats({ id: "mergeconflict" }).line, `${WEAPONS.mergeconflict.damage} dmg · carry 1`);
  assert.equal(itemStats({ id: "stacktrace" }).line, "carry 2");
  assert.equal(itemStats({ id: "smoketest" }).line, "18 s · carry 1");
  assert.deepEqual(itemStats({ id: "vest" }).bars.map((b) => b.label), ["ARMOR"]);
  assert.equal(itemStats({ id: "drink" }).line, "speed +10% · lasts 10 s");
  for (const b of ["prompt", "contextwindow", "helmet", "kit", "shades"].flatMap((id) => itemStats({ id }).bars)) assert.ok(b.frac >= 0 && b.frac <= 1, JSON.stringify(b));
});

// GitHub #15: everything in the menu says what it is and what it is good for,
// and the bars rank the guns the way their numbers do.
test("every buyable item has a type line and a good-for line; grenades say their kind", async () => {
  const { itemStats, buyCategories } = await import("./buyrules.js");
  for (const team of ["vibe", "artisan"]) {
    for (const it of buyCategories(team, { mp_gravityboots: "1" }).flatMap((c) => c.items)) {
      const st = itemStats(it);
      assert.ok(st.type, `${it.id}: type`);
      assert.ok(st.good, `${it.id}: good for`);
      for (const b of st.all) assert.ok(b.frac >= 0 && b.frac <= 1 && b.text, `${it.id} ${JSON.stringify(b)}`);
    }
  }
  assert.equal(itemStats({ id: "mergeconflict" }).type, "HE grenade");
  assert.equal(itemStats({ id: "stacktrace" }).type, "Flashbang");
  assert.equal(itemStats({ id: "smoketest" }).type, "Smoke grenade");
  assert.equal(itemStats({ id: "vest" }).type, "Body armor");
});

test("the bars order the guns as their stats do: rate, accuracy, range, speed, pen, reward", async () => {
  const { gunBars } = await import("./buyrules.js");
  const b = (id) => gunBars(WEAPONS[id]);
  assert.ok(b("autocomplete").rate.frac > b("prompt").rate.frac, "the SMG fires faster");
  assert.ok(b("linter").accuracy.frac > b("autocomplete").accuracy.frac, "the Linter is more accurate");
  assert.ok(b("codereview").accuracy.frac > b("prompt").accuracy.frac);
  assert.ok(b("prompt").range.frac > b("tokenburner").range.frac, "a shotgun has no range");
  assert.ok(b("stub").speed.frac > b("contextwindow").speed.frac, "the belt-fed is slow");
  assert.ok(b("codereview").pen.frac > b("autocomplete").pen.frac);
  assert.ok(b("tokenburner").reward.frac > b("prompt").reward.frac, "shotgun kills pay more");
  assert.equal(b("prompt").rate.text, "600 rpm");
});

test("key caps: punctuation binds show as the character (, and . for ammo), others by name", async () => {
  const { keyCap } = await import("./buyrules.js");
  assert.equal(keyCap("Comma"), ",");
  assert.equal(keyCap("Period"), ".");
  assert.equal(keyCap("KeyB"), "B");
});

// The menu's grenade items are owned at the server's carry limit (grenades.go
// buyGrenade: "can't carry more" at the weapon's mag): one HE, two flashes.
// The menu's items lacked the grenade flag, so a second HE stayed buyable.
test("the buy menu's grenades show owned at the carry limit, as the server refuses them", async () => {
  const { buyCategories } = await import("./buyrules.js");
  const nades = buyCategories("vibe").find((c) => c.items.some((it) => it.id === "mergeconflict")).items;
  const item = (id) => nades.find((it) => it.id === id);
  const one = hudSlots({ cur: "primary", slots: { he: { id: "mergeconflict", ammo: 1, reserve: 0 }, flash: { id: "stacktrace", ammo: 1, reserve: 0 } } });
  assert.equal(owned(item("mergeconflict"), { slots: one }), true);
  assert.equal(owned(item("stacktrace"), { slots: one }), false);
  assert.equal(owned(item("smoketest"), { slots: one }), false);
  const two = hudSlots({ cur: "primary", slots: { flash: { id: "stacktrace", ammo: 2, reserve: 0 } } });
  assert.equal(owned(item("stacktrace"), { slots: two }), true);
});

test("after a buy the menu goes back to the categories (buys chain); cl_buy_close closes it; owned or too dear stays", async () => {
  const { afterPick, buyCategories } = await import("./buyrules.js");
  const rifle = buyCategories("vibe").flatMap((c) => c.items).find((it) => it.id === "prompt");
  const rich = { money: 16000, slots: [] };
  assert.equal(afterPick(rifle, rich, false), "categories");
  assert.equal(afterPick(rifle, rich, true), "close");
  assert.equal(afterPick(rifle, { money: 100, slots: [] }, false), "stay");
  assert.equal(afterPick(rifle, { money: 16000, slots: [{ slot: "primary", id: "prompt" }] }, true), "stay");
});

test("the confirmation line names what the server sold: an item, ammo, or nothing", async () => {
  const { boughtLine, buyCategories } = await import("./buyrules.js");
  const cats = buyCategories("vibe");
  const vest = cats.flatMap((c) => c.items).find((i) => i.id === "vest");
  assert.equal(boughtLine("vest", cats), `bought ${vest.name}`);
  assert.equal(boughtLine("ammo1", cats), "bought primary ammo");
  assert.equal(boughtLine("", cats), "");
  assert.equal(boughtLine("nope", cats), "");
});

// The Gravity Override is on sale only while mp_gravityboots is "1" (the
// server's or the map's pin), as the server's purchase() refuses it otherwise.
test("the Gravity Override is in the menu only under mp_gravityboots 1; owned while worn", async () => {
  const { buyCategories } = await import("./buyrules.js");
  const gear = (rules) => buyCategories("vibe", rules).flatMap((c) => c.items).filter((it) => it.id === "boots");
  assert.equal(gear().length, 0);
  assert.equal(gear({ mp_gravityboots: "0" }).length, 0);
  const [b] = gear({ mp_gravityboots: "1" });
  assert.equal(b.price, 1000);
  assert.equal(owned(b, { boots: true }), true);
  assert.equal(owned(b, {}), false);
});

// mp_buy_guns 0 (he_merge_hell): the menu sells grenades and gear only, as
// the server's purchase() does; the gun categories stay, empty (numbers fixed).
test("mp_buy_guns 0: no guns in the menu, grenades and gear still", async () => {
  const { buyCategories } = await import("./buyrules.js");
  const all = buyCategories("vibe", { mp_buy_guns: "0" });
  const items = all.flatMap((c) => c.items);
  assert.equal(all.length, buyCategories("vibe").length);
  assert.ok(items.some((it) => it.id === "mergeconflict") && items.some((it) => it.id === "vest"));
  assert.ok(!items.some((it) => it.id === "prompt" || it.id === "stub" || it.id === "hotfix"));
  assert.ok(buyCategories("vibe").flatMap((c) => c.items).some((it) => it.id === "prompt"));
});

test("priceNote: says why the combo costs less than its list price", async () => {
  const { priceNote, itemPrice } = await import("./buyrules.js");
  const helmet = { id: "helmet", price: 1000, upgrade: 350 }, vest = { id: "vest", price: 650 };
  assert.equal(priceNote(helmet, { armor: 0 }), "");
  assert.equal(priceNote(vest, { armor: 40, helmet: true }), "");
  assert.match(priceNote(helmet, { armor: 100 }), /hoodie/i);
  assert.equal(itemPrice(helmet, { armor: 40, helmet: true }), 650);
  assert.match(priceNote(helmet, { armor: 40, helmet: true }), /headphones/i);
});

test("boughtLine: a car-shop buy says where it shows (no hidden 'garage')", async () => {
  const { boughtLine } = await import("./buyrules.js");
  const cats = [{ name: "Cars", items: [{ id: "car:tank", name: "Legacy Tank" }] }];
  assert.equal(boughtLine("car:tank", cats, { warmup: true, driving: true }), "bought Legacy Tank · fitted for a test drive: the match starts stock");
  assert.equal(boughtLine("car:tank", cats, { warmup: true, driving: false }), "bought Legacy Tank · a warmup try-out: the match starts stock");
  assert.equal(boughtLine("car:tank", cats, { warmup: false, driving: true }), "bought Legacy Tank · fitted to your car");
  assert.equal(boughtLine("car:tank", cats, { warmup: false, driving: false }), "bought Legacy Tank · yours on the next grid");
  assert.equal(boughtLine("car:tank", cats), "bought Legacy Tank · yours on the next grid");
});
