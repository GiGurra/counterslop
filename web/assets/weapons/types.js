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
