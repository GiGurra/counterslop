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
  mergeconflict: sfx.nadeThrow, stacktrace: sfx.nadeThrow, smoketest: sfx.nadeThrow,
  bigdiff: () => {}, // never fires (planting has its own sounds, assets/sounds/bigdiff.js)
};
// Every weapon needs a fire sound (and a model, assets/models/weapons.js MODELS).
for (const id of Object.keys(WEAPONS)) if (!FIRE_SOUND[id]) console.warn(`weapon ${id} has no fire sound`);
