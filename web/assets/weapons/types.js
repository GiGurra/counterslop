// What kind of thing a weapon is, in words a CS player knows, next to its
// coding-slang name: "Merge Conflict" is an HE grenade, "Code Review" a sniper
// rifle. The grenades' kinds also have a colour (the palette's nade* entries)
// shared by the HUD tag, the kill feed, the buy menu and the can's band.
// Pure data + lookups (node-tested); the buy menu's details (#15) build on it.
import { WEAPONS } from "./index.js";
import { PALETTE as P } from "../palette.js";
import { rgbHex } from "../../gen/color.js";

// Grenade kinds (weapons.json grenade.kind): a short tag and a long name.
export const NADE_KINDS = {
  he: { tag: "HE", long: "HE grenade", color: rgbHex(P.nadeHe), pal: "nadeHe" },
  flash: { tag: "FLASH", long: "Flashbang", color: rgbHex(P.nadeFlash), pal: "nadeFlash" },
  smoke: { tag: "SMOKE", long: "Smoke grenade", color: rgbHex(P.nadeSmoke), pal: "nadeSmoke" },
};

// Weapon classes (weapons.json class).
export const CLASS_LABEL = {
  pistol: "Pistol", smg: "SMG", rifle: "Rifle", sniper: "Sniper rifle", shotgun: "Shotgun",
  heavy: "Machine gun", melee: "Knife", bomb: "The bomb", grenade: "Grenade",
};

const weapon = (w) => (typeof w === "string" ? WEAPONS[w] : w);

// The grenade kind of a weapon (an id or the weapon), or null for anything else.
export function nadeKind(w) {
  const k = weapon(w)?.grenade?.kind;
  return (k && NADE_KINDS[k]) || null;
}

// A weapon's type in words: { tag (short, for a pill; "" when the name is
// enough), long ("HE grenade", "Sniper rifle"), color (grenades) }.
export function typeOf(w) {
  w = weapon(w);
  if (!w) return { tag: "", long: "", color: "" };
  const k = nadeKind(w);
  if (k) return { tag: k.tag, long: k.long, color: k.color };
  return { tag: "", long: CLASS_LABEL[w.class] || "", color: "" };
}

// Gear's type line (economy.json gear ids).
export const GEAR_TYPE = {
  vest: "Body armor", helmet: "Body armor + helmet", kit: "Defuse kit", shades: "Flash protection", drink: "Speed boost",
};

// What each thing is good for, one line in the buy menu (GitHub #15): the
// role a CS player would recognise, said plainly next to the slang name.
export const GOOD_FOR = {
  stub: "the free pistol: save rounds, finishing a weakened enemy",
  rubberduck: "eco rounds: cheap, more bullets, forgiving on the move",
  hotfix: "force buys: a headshot up close drops anyone",
  autocomplete: "close range on the move; a fat kill reward",
  copypaste: "a budget rifle: measured taps at mid range",
  prompt: "the all-rounder: sprays up close, taps at range",
  linter: "holding angles: the most accurate rifle",
  codereview: "one body shot kills at any range; scope in, hold long angles",
  tokenburner: "corners and tight sites: the spread wastes it at range; a fat kill reward",
  contextwindow: "holding a choke with a 100-round belt; you walk slowly",
  mergeconflict: "damage a group behind cover, finish weakened enemies",
  stacktrace: "blind them before you (or a teammate) peek",
  smoketest: "block a sightline for 18 s: cross, plant or retake behind it",
  vest: "every round you can afford it: body shots hurt far less",
  helmet: "full buys: headshots stop being instant",
  kit: "retakes: git revert in 4 s instead of 8",
  shades: "rounds full of Stack Traces",
  drink: "a fast rotation or a late entry",
};
