// Map previews off the page's thread (engine/thumbs.js createWorkerThumbnailer):
// this game's look, drawn on an OffscreenCanvas in a worker.
import { serveThumbnails } from "../engine/thumbs.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SKIES, SUN_DIR } from "../assets/sky.js";

serveThumbnails({ palette: PALETTE, materials: MATERIALS, sky: SKY, skies: SKIES, sun: SUN_DIR });
