// Buy menu rules that need no UI (tested in node): what you already own.
// s: the HUD state (armor, helmet, kit, slots from arsenal.js hudSlots: gun rows
// carry the weapon id, the grenade row "nades" {id: count}).
import econ from "../assets/economy.json" with { type: "json" };
import { WEAPONS } from "../assets/weapons/index.js";
import { keyName } from "./binds.js";

// Owned items show greyed out as "owned". Grenades count as owned once you
// carry as many as you may (weapons.json mag: 1 HE, 2 flashes, 1 smoke).
export function owned(it, s) {
  if (it.id === "vest") return s.armor >= 100;
  if (it.id === "helmet") return s.armor >= 100 && s.helmet;
  if (it.id === "kit") return s.kit;
  if (it.id === "shades") return !!s.shades;
  if (it.id === "drink") return s.boost > 0; // one at a time: buy the next when it wears off
  if (it.grenade) return ((s.slots || []).find((x) => x?.nades)?.nades[it.id] || 0) >= (it.mag || 1);
  return (s.slots || []).some((x) => x && x.id === it.id);
}

// What an item costs you now (the server's purchase() charges the same).
export function itemPrice(it, s) {
  if (it.id === "helmet" && s.armor >= 100 && it.upgrade) return it.upgrade;
  if (it.id === "helmet" && s.helmet) return econ.gear.find((g) => g.id === "vest")?.price ?? it.price; // only the vest is worn: CS charges the vest
  return it.price;
}

// The ammo strip: one card per gun slot, as CS 1.6's buyammo1 (",", the
// primary) and buyammo2 (".", the pistol). A buy fills the reserve back to
// what a new gun comes with, for economy.json's ammo price (internal/game
// buy.go buyAmmo). s.slots: arsenal.js hudSlots rows (slot, id, reserve).
export const AMMO_SLOTS = [
  { item: "ammo1", cmd: "buyammo1", slot: "primary", label: "primary" },
  { item: "ammo2", cmd: "buyammo2", slot: "sidearm", label: "pistol" },
];
export function ammoCards(s) {
  return AMMO_SLOTS.map((a) => {
    const row = (s.slots || []).find((x) => x?.slot === a.slot && x.id);
    const w = row ? WEAPONS[row.id] : null;
    const max = w?.mag ? w.reserve : 0, reserve = max ? Math.min(max, row.reserve ?? 0) : 0;
    const price = econ.ammo?.[a.slot] ?? 0;
    return {
      ...a, id: row?.id || "", name: w?.name || "", reserve, max,
      frac: max ? reserve / max : 0, missing: max - reserve, price,
      none: !max, full: !!max && reserve >= max, poor: price > (s.money ?? 0),
    };
  });
}

// What a buy-menu card shows under the name: bars [{label, frac 0..1, text}]
// and a one-line summary (the narrow layout's stat line).
const DMG_FULL = 120, MAG_FULL = 40; // a bar is full here (Code Review's one-shot, a big mag)
const clamp01 = (x) => Math.max(0, Math.min(1, x));
export function itemStats(it) {
  const w = WEAPONS[it.id];
  if (w?.action === "throw") {
    const bars = [];
    if (w.damage > 0) bars.push({ label: "DAMAGE", frac: clamp01(w.damage / DMG_FULL), text: String(w.damage) });
    if (w.grenade?.duration) bars.push({ label: "LASTS", frac: clamp01(w.grenade.duration / 20), text: `${w.grenade.duration} s` });
    bars.push({ label: "CARRY", frac: clamp01((w.mag || 1) / 2), text: String(w.mag || 1) });
    return { bars, line: [w.damage > 0 && `${w.damage} dmg`, w.grenade?.duration && `${w.grenade.duration} s`, `carry ${w.mag || 1}`].filter(Boolean).join(" · ") };
  }
  if (w?.mag) {
    const dmg = w.damage * (w.pellets || 1), dtext = w.pellets ? `${w.damage}×${w.pellets}` : String(w.damage);
    return {
      bars: [{ label: "DAMAGE", frac: clamp01(dmg / DMG_FULL), text: dtext }, { label: "MAG", frac: clamp01(w.mag / MAG_FULL), text: String(w.mag) }],
      line: `${dtext} dmg · ${w.mag} / ${w.reserve}${w.scope ? " · scoped" : ""}`,
    };
  }
  const g = econ.gear.find((x) => x.id === it.id);
  if (!g) return { bars: [], line: it.desc || "" };
  const bars = {
    vest: [{ label: "ARMOR", frac: 1, text: "100" }],
    helmet: [{ label: "ARMOR", frac: 1, text: "100" }, { label: "HELMET", frac: 1, text: "yes" }],
    kit: [{ label: "REVERT", frac: 0.5, text: "2× fast" }],
    shades: [{ label: "BLIND", frac: g.flash ?? 1, text: `−${Math.round((1 - (g.flash ?? 1)) * 100)}%` }],
    drink: [{ label: "SPEED", frac: clamp01(((g.speed ?? 1) - 1) * 5), text: `+${Math.round(((g.speed ?? 1) - 1) * 100)}%` }, { label: "LASTS", frac: clamp01((g.secs ?? 0) / 20), text: `${g.secs ?? 0} s` }],
  }[it.id] || [];
  return { bars, line: bars.map((b) => `${b.label.toLowerCase()} ${b.text}`).join(" · ") };
}

// A key cap's label: punctuation as itself (the ammo binds are , and .).
const PUNCT = { Comma: ",", Period: ".", Slash: "/", Semicolon: ";", Quote: "'", Minus: "-", Equal: "=", BracketLeft: "[", BracketRight: "]" };
export const keyCap = (code) => PUNCT[code] || keyName(code);

// The menu's items: weapons from the registry by class (team-specific ones
// only for that team), gear from economy.json.
export function buyCategories(team) {
  return econ.buyCategories.map((c) => ({
    name: c.name,
    items: c.gear
      ? econ.gear.filter((g) => !g.team || g.team === team).map((g) => ({ id: g.id, name: g.name, desc: g.desc, price: g.price, upgrade: g.upgrade }))
      : Object.values(WEAPONS)
          .filter((w) => c.classes.includes(w.class) && w.slot !== "melee" && (!w.team || w.team === team))
          .sort((a, b) => a.price - b.price)
          // grenade + mag: owned() counts a grenade as owned at its carry limit (the server's rule)
          .map((w) => ({ id: w.id, name: w.name, desc: w.flavor, price: w.price, slot: w.slot, grenade: w.action === "throw", mag: w.mag })),
  })); // fixed numbers, CS-style: an empty category stays (greyed) so the keys never shift
}

// What the menu does after you pick an item (the request goes to the server
// either way): something you own or can't afford stays where it is (the
// server's answer says why); a buy goes back to the categories so the next
// digits buy again (B 4 2 6 1), or closes the menu with cl_buy_close (CS 1.6).
export function afterPick(it, s, closeOnBuy) {
  if (owned(it, s) || itemPrice(it, s) > (s.money ?? 0)) return "stay";
  return closeOnBuy ? "close" : "categories";
}

// The menu's confirmation line for the server's last ok answer (an item id,
// or ammo1/ammo2), "" when there is none.
export function boughtLine(id, cats) {
  if (!id) return "";
  const a = AMMO_SLOTS.find((x) => x.item === id);
  if (a) return `bought ${a.label} ammo`;
  const it = cats.flatMap((c) => c.items).find((x) => x.id === id);
  return it ? `bought ${it.name}` : "";
}
