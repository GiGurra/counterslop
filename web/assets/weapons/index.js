// The weapon registry for the client: stats from weapons.json (shared with the
// server, which validates fire with the same numbers) plus the generated model
// and fire sound for each id. Adding a weapon: an entry in weapons.json, a box
// spec in assets/models/weapons.js and a sound in assets/sounds/weapons.js.
import data from "./weapons.json" with { type: "json" };
import * as sfx from "../sounds/index.js";

export const WEAPONS = Object.fromEntries(data.weapons.map((w) => [w.id, w]));
export const SLOTS = data.slots; // primary, sidearm, melee (keys 1, 2, 3)

export const FIRE_SOUND = {
  stub: sfx.stubShot, prompt: sfx.promptShot, linter: sfx.linterShot,
  codereview: sfx.reviewShot, keyboard: sfx.keyboardSwing,
  rubberduck: sfx.duckShot, hotfix: sfx.hotfixShot, autocomplete: sfx.autocompleteShot,
  copypaste: sfx.copypasteShot, tokenburner: sfx.tokenShot, contextwindow: sfx.contextShot,
  mergeconflict: sfx.nadeThrow, stacktrace: sfx.nadeThrow, smoketest: sfx.nadeThrow, hotreload: sfx.nadeThrow, burndown: sfx.nadeThrow,
  bigdiff: () => {}, // never fires (planting has its own sounds, assets/sounds/bigdiff.js)
};
// Every weapon needs a fire sound (and a model, assets/models/weapons.js MODELS).
for (const id of Object.keys(WEAPONS)) if (!FIRE_SOUND[id]) console.warn(`weapon ${id} has no fire sound`);

// Muzzle light per weapon class (engine/flashes.js): a warm point light for a few
// frames at the muzzle. intensity at its peak, range in metres (under 10), life in
// seconds. Knives, grenades and the bomb make none.
const WARM = [1, 0.7, 0.38];
export const MUZZLE_LIGHT = {
  pistol: { color: WARM, intensity: 9, range: 6, life: 0.055 },
  smg: { color: WARM, intensity: 9, range: 6.5, life: 0.05 },
  rifle: { color: WARM, intensity: 12, range: 7.5, life: 0.065 },
  shotgun: { color: [1, 0.64, 0.32], intensity: 18, range: 8.5, life: 0.085 },
  heavy: { color: [1, 0.66, 0.34], intensity: 16, range: 8.5, life: 0.07 },
  sniper: { color: [1, 0.74, 0.45], intensity: 26, range: 9.5, life: 0.09 },
};
export const muzzleLight = (id) => MUZZLE_LIGHT[WEAPONS[id]?.class] || null;
