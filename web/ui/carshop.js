// The race shop in the buy menu (race phase 3; the server is internal/game
// carshop.go): on a race map, on the grid, three more categories after CS's
// six: 7 Cars (a tier), 8 Car weapons (swap the front or rear item), 9
// Upgrades (the next tier of each). Pure: the items are the menu's
// ({id, name, desc, price, car: true, have}); the server checks and charges.
// gar: me.gar (the garage: {car, f, r, ch, tk, pl, en, ty}); carType: the
// car we sit in (its built-ins when the garage names none).
import carsData from "../assets/cars.json" with { type: "json" };

const CARS = carsData.cars, ITEMS = carsData.equipment || [], SHOP = carsData.shop || { upgrades: {} };
export const UPGRADES = [
  { id: "charges", key: "ch", name: "Charge pack", desc: "more front and rear charges every lap" },
  { id: "tank", key: "tk", name: "Boost tank", desc: "more boost every lap" },
  { id: "plating", key: "pl", name: "Plating", desc: "more car HP, and your rams hit harder" },
  { id: "engine", key: "en", name: "Engine", desc: "more acceleration and a little top speed" },
  { id: "tyres", key: "ty", name: "Tyres", desc: "more grip and a quicker turn (not for the hovercraft)" },
];
const TIERS = ["I", "II", "III"];

export function carShopCategories(gar = {}, carType = "") {
  gar = gar || {};
  const type = gar.car || carType || CARS[0]?.id;
  const spec = CARS.find((c) => c.id === type) || CARS[0];
  const front = gar.f || spec?.front, rear = gar.r || spec?.rear;
  const cars = CARS.map((c) => ({ id: `car:${c.id}`, name: c.name, desc: `tier ${c.tier} · top ${Math.round(c.topSpeed * 3.6)} km/h · ${c.hp} HP · built in: ${itemName(c.front)}, ${itemName(c.rear)}`, price: c.price > 0 && (gar.own || []).includes(c.id) ? 0 : c.price, car: true, have: c.id === type })); // owned this match: free to drive again
  const gear = ITEMS.map((it) => {
    const builtin = it.id === spec?.front || it.id === spec?.rear;
    return { id: `${it.slot}:${it.id}`, name: `${it.slot === "front" ? "Front" : "Rear"}: ${it.name}`, desc: it.desc, price: builtin ? 0 : SHOP.swap, car: true, have: it.id === front || it.id === rear };
  });
  const ups = UPGRADES.map((u) => {
    const tier = gar[u.key] | 0, prices = SHOP.upgrades[u.id] || [], max = tier >= prices.length;
    const hover = u.id === "tyres" && spec?.drive === "hover";
    return { id: `up:${u.id}`, name: `${u.name} ${max ? TIERS[prices.length - 1] || "" : TIERS[tier] || ""}`, desc: hover ? "the hovercraft has no tyres" : `${u.desc} (tier ${Math.min(tier + 1, prices.length)} of ${prices.length})`, price: max ? 0 : prices[tier], car: true, have: max || hover };
  });
  return [{ name: "Cars", items: cars, car: true }, { name: "Car weapons", items: gear, car: true }, { name: "Upgrades", items: ups, car: true }];
}

function itemName(id) { return ITEMS.find((it) => it.id === id)?.name || id; }
