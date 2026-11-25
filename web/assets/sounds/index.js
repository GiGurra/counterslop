// Every game sound, by name. Recipes live next to this file; primitives in gen/synth.
export { initAudio, setVolume, audioState } from "../../gen/synth/core.js";
import { footstep, jump, land, splash } from "./movement.js";
import { pew, click, reload, bonk, stubShot, promptShot, linterShot, reviewShot, keyboardSwing, deploy, boltCycle,
  duckShot, hotfixShot, autocompleteShot, copypasteShot, tokenShot, contextShot } from "./weapons.js";
import { honk } from "./ui.js";
import { hurtFlesh, hurtArmor, heartbeat } from "./hurt.js";
import { hitBody, hitHead, hitArmor, killConfirm } from "./hitconfirm.js";
import { plantClick, ciBeep, revertTick, reverted, mergeBoom, roundEnd } from "./bigdiff.js";
import { nadeThrow, nadeBounce, heBlast, flashBang, flashRing, smokePop, fireBurst, fireCrackle, fireOut } from "./grenades.js";
import { impactStone, impactMetal, impactWood, impactGlass, impactDirt, impactCardboard, bulletImpact, casingBrass, casingShell } from "./impacts.js";
import { gpuGrab, gpuRestock, gpuSecure, paChime, PA_CHIME_SECONDS, escalatorHum } from "./store.js";

export { footstep, jump, land, splash, pew, click, reload, bonk, honk, stubShot, promptShot, linterShot, reviewShot, keyboardSwing, deploy, boltCycle,
  duckShot, hotfixShot, autocompleteShot, copypasteShot, tokenShot, contextShot };
export { plantClick, ciBeep, revertTick, reverted, mergeBoom, roundEnd };
export { hurtFlesh, hurtArmor, heartbeat };
export { hitBody, hitHead, hitArmor, killConfirm };
export { nadeThrow, nadeBounce, heBlast, flashBang, flashRing, smokePop, fireBurst, fireCrackle, fireOut };
export { gpuGrab, gpuRestock, gpuSecure, paChime, PA_CHIME_SECONDS, escalatorHum };
export { impactStone, impactMetal, impactWood, impactGlass, impactDirt, impactCardboard, bulletImpact, casingBrass, casingShell };

// Catalog for the dev gallery: name -> play with representative arguments.
export const CATALOG = {
  hurtFlesh: () => hurtFlesh(), hurtArmor: () => hurtArmor(), heartbeat: () => heartbeat(),
  hitBody: () => hitBody(), hitHead: () => hitHead(), hitArmor: () => hitArmor(), killConfirm: () => killConfirm(), killConfirmHead: () => killConfirm(true),
  footstep: () => footstep(), jump: () => jump(), land: () => land(14), splash: () => splash(12), pew: () => pew(),
  click: () => click(), reload: () => reload(1.6), bonk: () => bonk(), honk: () => honk(),
  stubShot: () => stubShot(), promptShot: () => promptShot(), linterShot: () => linterShot(),
  reviewShot: () => reviewShot(), keyboardSwing: () => keyboardSwing(), deploy: () => deploy(), boltCycle: () => boltCycle(1.1),
  plantClick: () => plantClick(), ciBeep: () => ciBeep(undefined, 0.5), revertTick: () => revertTick(), reverted: () => reverted(),
  nadeThrow: () => nadeThrow(), nadeBounce: () => nadeBounce(), heBlast: () => heBlast(), flashBang: () => flashBang(),
  flashRing: () => flashRing(2), smokePop: () => smokePop(), fireBurst: () => fireBurst(), fireCrackle: () => fireCrackle(), fireOut: () => fireOut(),
  mergeBoom: () => mergeBoom(), roundWin: () => roundEnd(true), roundLoss: () => roundEnd(false),
  duckShot: () => duckShot(), hotfixShot: () => hotfixShot(), autocompleteShot: () => autocompleteShot(),
  copypasteShot: () => copypasteShot(), tokenShot: () => tokenShot(), contextShot: () => contextShot(),
  impactStone: () => impactStone(), impactMetal: () => impactMetal(undefined, 1, false), impactRicochet: () => impactMetal(undefined, 1, true),
  impactWood: () => impactWood(), impactGlass: () => impactGlass(), impactDirt: () => impactDirt(), impactCardboard: () => impactCardboard(),
  casingBrass: () => casingBrass(), casingBrassCarpet: () => casingBrass(undefined, false), casingShell: () => casingShell(), casingShellCarpet: () => casingShell(undefined, false),
  gpuGrab: () => gpuGrab(), gpuRestock: () => gpuRestock(), gpuSecure: () => gpuSecure(), paChime: () => paChime(),
  escalatorHum: () => { const h = escalatorHum(); h?.update({ pan: 0.6, gain: 0.4 }); setTimeout(() => h?.stop(), 3000); }, // 3 s, panning right
};
