// The race garage (docs/racing.md "The garage"): a bay you visit before a
// race (key H, `garage`): your car on a turntable with its items and
// upgrades fitted, the other types parked in bays, the race shop in place.
// Client-side presentation only: nothing here is gameplay state. What you
// own is the server's (me.gar, internal/game carshop.go) and a buy is a
// request ({t:"buy", item}) the server checks and charges, like the buy menu.
//
// Pure (no DOM, no GL): game/garagescene.js draws it, ui/garage.js is its
// panel; tests in garage.test.mjs.
//   garageAllowed({mode, phase, team, dead})  "" or why the garage is closed now
//   garageView({gar, carType, money, shopOpen, preview})  everything the garage shows
//   SLOTS                                    the car's parts you can click
//   moveFocus(view, focus, key) / focusTarget(view, focus)  arrows + Enter
//   SLOT_FACING, presentAngle(slot, camYaw), orbitEye(...)   the turntable and camera
import carsData from "../assets/cars.json" with { type: "json" };
import { carShopCategories, UPGRADES } from "../ui/carshop.js";
import { FitMaxHP, FitCharges, FitTank, FitTopSpeed } from "./shared.gen.js";

const CARS = carsData.cars, ITEMS = carsData.equipment || [], SHOP = carsData.shop || { upgrades: {} };
const spec = (id) => CARS.find((c) => c.id === id) || null;
const item = (id) => ITEMS.find((it) => it.id === id) || null;
export const TIERS = ["–", "I", "II", "III"];

// The garage opens on a car race map at any time and from anywhere: every
// phase, on foot or in a car, alive or wrecked, no zones, no distances.
// Only the phase decides what it is (the server's rules, internal/game
// pitstop.go): before GO and over the results the shop; while the race is
// live a pit stop (your car leaves the track, the clock runs; leaving puts
// you on your last gate). Not on a parkour course (onFoot: no cars).
export function garageAllowed({ mode, team, onFoot } = {}) {
  if (mode !== "race" || onFoot) return "the garage is on car race maps";
  if (team !== "vibe" && team !== "artisan") return "join a team to get a garage";
  return "";
}

// A pit stop: the garage open while the race is live (what the server is
// asked for, {t:"pit"}).
export const pitWanted = ({ open, mode, onFoot } = {}) => !!open && mode === "race" && !onFoot;

// The parts of a car you can click, in keyboard order: the shop's item
// ("front:<id>", "rear:<id>") or upgrade ("up:<id>") behind each.
export const SLOTS = [
  { id: "front", name: "Front mount", kind: "front", desc: "the front weapon (Q while driving)" },
  { id: "rear", name: "Rear mount", kind: "rear", desc: "the rear drop (C while driving)" },
  ...UPGRADES.map((u) => ({ id: u.id, name: u.name, kind: "up", key: u.key, desc: u.desc })),
];

// What a buy of price p is for us: "owned" (yours, nothing to pay), "free",
// "buy" (you can afford it) or "poor" (you can't).
const stateOf = (have, price, money) => (have ? "owned" : price <= 0 ? "free" : price > money ? "poor" : "buy");
// A car type for us: "selected" (the one you drive), "owned" (bought this
// match, or free: driving it again costs nothing), else as stateOf.
export const carState = (id, own, gar, price, money) =>
  id === own ? "selected" : price <= 0 || (gar?.own || []).includes(id) ? "owned" : stateOf(false, price, money);

// The numbers on a car's spec card: type sp fitted from garage g (front,
// rear: the items on it). The same rules the server fits a car with
// (internal/game car_shared.go Fit*).
export function fitOf(sp, g, front, rear) {
  const hover = sp.drive === "hover";
  const it = (id) => {
    const x = item(id);
    return x ? { id, name: x.name, kind: x.kind, desc: x.desc, charges: FitCharges(x.charges | 0, g.ch | 0, SHOP.chargePack || 0) } : null;
  };
  return {
    id: sp.id, name: sp.name, tier: sp.tier, drive: sp.drive,
    topKmh: Math.round(FitTopSpeed(sp.topSpeed, g.en | 0) * 3.6),
    hp: FitMaxHP(sp.hp, g.pl | 0, SHOP.plating || 0),
    tank: Math.round(FitTank(SHOP.boost || 0, SHOP.tank || 0, g.tk | 0) * 10) / 10,
    front: it(front), rear: it(rear),
    tiers: { charges: g.ch | 0, tank: g.tk | 0, plating: g.pl | 0, engine: g.en | 0, tyres: hover ? 0 : g.ty | 0 },
  };
}

// Everything the garage shows. gar: me.gar (null: stock, the car we sit in);
// carType: the car we sit in; money; shopOpen: the server would sell now
// (me.buy); preview: a type picked in a bay to look at ("" / ours: none).
//   {own, shown, previewing, card, bays, slots, shopOpen, money}
// card: the turntable car's fit (a previewed type: as it would come, its
// own built-ins and our upgrade tiers, which carry over). bays: every type
// with its state; slots: our car's parts, each with the options the shop
// has for it ({buy, name, desc, price, state}).
export function garageView({ gar = null, carType = "", money = 0, shopOpen = false, preview = "" } = {}) {
  const g = gar || {};
  const own = spec(g.car) || spec(carType) || CARS[0];
  const shown = spec(preview) || own, previewing = shown.id !== own.id;
  const front = g.f || own.front, rear = g.r || own.rear;
  const card = previewing ? fitOf(shown, g, shown.front, shown.rear) : fitOf(own, g, front, rear);
  const [cars, gear, ups] = carShopCategories(g, own.id).map((c) => c.items);
  const bays = CARS.map((c) => {
    const it = cars.find((x) => x.id === `car:${c.id}`);
    return { id: c.id, name: c.name, tier: c.tier, price: c.price, buy: it.id, state: carState(c.id, own.id, g, c.price, money), shown: c.id === shown.id,
      desc: it.desc };
  });
  const opt = (it, name = it.name) => ({ buy: it.id, name, desc: it.desc, price: it.price, state: stateOf(it.have, it.price, money) });
  const slots = SLOTS.map((s) => {
    if (s.kind === "front" || s.kind === "rear") {
      const fitted = s.kind === "front" ? front : rear;
      const options = gear.filter((it) => it.id.startsWith(`${s.kind}:`)).map((it) => {
        const id = it.id.slice(s.kind.length + 1), x = item(id);
        return { ...opt(it, x?.name || id), state: id === fitted ? "owned" : stateOf(false, it.price, money), builtin: id === own[s.kind] };
      });
      return { ...s, fitted: item(fitted)?.name || fitted, options, tier: null };
    }
    const it = ups.find((x) => x.id === `up:${s.id}`), tier = g[s.key] | 0, max = (SHOP.upgrades[s.id] || []).length;
    const none = s.id === "tyres" && own.drive === "hover";
    const options = none || tier >= max ? [] : [{ ...opt(it, `${s.name} ${TIERS[tier + 1]}`) }];
    return { ...s, tier: none ? 0 : tier, max, none, fitted: none ? "no tyres on a hovercraft" : tier ? `tier ${TIERS[tier]}` : "stock", options };
  });
  return { own: own.id, shown: shown.id, previewing, card, bays, slots, shopOpen: !!shopOpen, money };
}

// Keyboard: a focus {row: "bays" | "slots" | "options", i, slot} moved by
// arrows. Left/Right walk along the row; Down goes from the bays to the
// parts, Up back. Enter on a part opens its options (row "options"), on an
// option or a bay it acts (focusTarget). Backspace / Up from the options:
// back to the parts.
export function moveFocus(view, focus, key) {
  const f = { row: "bays", i: 0, slot: "", ...(focus || {}) };
  const len = f.row === "bays" ? view.bays.length : f.row === "slots" ? view.slots.length : (view.slots.find((s) => s.id === f.slot)?.options.length || 0);
  const wrap = (i) => (len ? ((i % len) + len) % len : 0);
  switch (key) {
    case "ArrowLeft": return { ...f, i: wrap(f.i - 1) };
    case "ArrowRight": return { ...f, i: wrap(f.i + 1) };
    case "ArrowDown":
      if (f.row === "bays") return { row: "slots", i: 0, slot: "" };
      if (f.row === "slots") return { row: "options", i: 0, slot: view.slots[f.i]?.id || "" };
      return f;
    case "ArrowUp":
    case "Backspace":
      if (f.row === "options") return { row: "slots", i: Math.max(0, view.slots.findIndex((s) => s.id === f.slot)), slot: "" };
      if (f.row === "slots" && key === "ArrowUp") return { row: "bays", i: Math.max(0, view.bays.findIndex((b) => b.shown)), slot: "" };
      return f;
    case "Enter":
      if (f.row === "slots") return { row: "options", i: 0, slot: view.slots[f.i]?.id || "" };
      return f;
  }
  return f;
}

// What Enter does at focus f: {kind: "preview", id} (a bay you aren't
// looking at yet: it rolls onto the turntable), {kind: "buy", item} (a
// bay's car shown on the turntable: bought, or an owned one driven again
// for free; an option), {kind: "open", slot} (a part) or null (the car
// you drive, an option fitted: nothing to do).
export function focusTarget(view, f) {
  if (!f) return null;
  if (f.row === "bays") {
    const b = view.bays[f.i];
    if (!b) return null;
    if (!b.shown) return { kind: "preview", id: b.id };
    return b.state === "selected" ? null : { kind: "buy", item: b.buy };
  }
  if (f.row === "slots") { const s = view.slots[f.i]; return s ? { kind: "open", slot: s.id } : null; }
  const o = view.slots.find((s) => s.id === f.slot)?.options[f.i];
  return o && o.state !== "owned" ? { kind: "buy", item: o.buy } : null;
}

// The scene's geometry (game/garagescene.js draws with it; pure, so tested here).
// Which way each slot faces on the car (model space, x right, z aft): the
// turntable turns it towards the camera when you pick it, and a target
// facing away is drawn faded.
export const SLOT_FACING = { front: [0, -1], rear: [0, 1], tyres: [-1, -0.4], engine: [0.5, 1], plating: [1, 0], tank: [-1, 0.2], charges: [0.7, -1] };

// The turntable angle that faces slot s at the camera (camera at yaw camYaw round the table).
export function presentAngle(s, camYaw) {
  const f = SLOT_FACING[s];
  return f ? camYaw - Math.atan2(f[0], f[1]) : null;
}

// The orbit camera's eye and view angles around target (yaw: round the
// table, 0 from +z; pitch: up from level).
export function orbitEye(target, yaw, pitch, dist) {
  const cp = Math.cos(pitch);
  return { eye: [target[0] + dist * Math.sin(yaw) * cp, target[1] + dist * Math.sin(pitch), target[2] + dist * Math.cos(yaw) * cp], yaw, pitch: -pitch };
}

// The bay labels laid out along a strip of screen [lo, hi] (px, between the
// side panels): items [{key, x (wanted centre), w (width)}] -> [{key, x, row}].
// In one row each as near its bay as it can be without leaving the strip or
// overlapping the next; when they don't fit side by side, every other one
// goes up a row (row 1) and each row is laid out on its own.
export function layoutLabels(items, lo, hi, gap = 8) {
  const row = (list) => {
    const s = [...list].sort((a, b) => a.x - b.x).map((it) => ({ ...it, x: Math.max(lo + it.w / 2, Math.min(hi - it.w / 2, it.x)) }));
    for (let i = 1; i < s.length; i++) s[i].x = Math.max(s[i].x, s[i - 1].x + s[i - 1].w / 2 + gap + s[i].w / 2);
    for (let i = s.length - 1; i >= 0; i--) {
      s[i].x = Math.min(s[i].x, hi - s[i].w / 2);
      if (i < s.length - 1) s[i].x = Math.min(s[i].x, s[i + 1].x - s[i + 1].w / 2 - gap - s[i].w / 2);
    }
    return s;
  };
  const fits = (s) => s.every((it) => it.x - it.w / 2 >= lo - 0.5);
  const one = row(items);
  if (fits(one) || items.length < 2) return one.map(({ key, x }) => ({ key, x, row: 0 }));
  const sorted = [...items].sort((a, b) => a.x - b.x);
  return [...row(sorted.filter((_, i) => i % 2 === 0)).map(({ key, x }) => ({ key, x, row: 0 })),
    ...row(sorted.filter((_, i) => i % 2 === 1)).map(({ key, x }) => ({ key, x, row: 1 }))];
}
