// Every game sound, by name. Recipes live next to this file; primitives in gen/synth.
export { initAudio, setVolume, audioState } from "../../gen/synth/core.js";
import { footstep, jump, land } from "./movement.js";
import { pew, click, reload, bonk, stubShot, promptShot, linterShot, reviewShot, keyboardSwing, deploy, boltCycle } from "./weapons.js";
import { honk } from "./ui.js";

export { footstep, jump, land, pew, click, reload, bonk, honk, stubShot, promptShot, linterShot, reviewShot, keyboardSwing, deploy, boltCycle };

// Catalog for the dev gallery: name -> play with representative arguments.
export const CATALOG = {
  footstep: () => footstep(), jump: () => jump(), land: () => land(14), pew: () => pew(),
  click: () => click(), reload: () => reload(1.6), bonk: () => bonk(), honk: () => honk(),
  stubShot: () => stubShot(), promptShot: () => promptShot(), linterShot: () => linterShot(),
  reviewShot: () => reviewShot(), keyboardSwing: () => keyboardSwing(), deploy: () => deploy(), boltCycle: () => boltCycle(1.1),
};
