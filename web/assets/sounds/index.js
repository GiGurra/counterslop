// Every game sound, by name. Recipes live next to this file; primitives in gen/synth.
export { initAudio, setVolume } from "../../gen/synth/core.js";
import { footstep, jump, land } from "./movement.js";
import { pew, click, reload, bonk } from "./weapons.js";
import { honk } from "./ui.js";

export { footstep, jump, land, pew, click, reload, bonk, honk };

// Catalog for the dev gallery: name -> play with representative arguments.
export const CATALOG = {
  footstep: () => footstep(), jump: () => jump(), land: () => land(14), pew: () => pew(),
  click: () => click(), reload: () => reload(1.6), bonk: () => bonk(), honk: () => honk(),
};
