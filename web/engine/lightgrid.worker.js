// A map's light grid off the main thread (engine/gl.js createLightGrid): the
// same pure build (lights.js computeLightGrid) the main thread runs when there
// is no worker. A big lit map (cq_moonshot) takes seconds; on the main thread
// that froze the page, its loading screen and its socket.
//
// In:  {gen, map: {boxes, lights, indoor}}
// Out: {gen, step, frac} while it works, then {gen, done: result} (buffers transferred).
import { computeLightGrid, lightGridBuffers } from "./lights.js";

self.onmessage = (e) => {
  const { gen, map } = e.data;
  let last = -1;
  const r = computeLightGrid(map, (step, frac) => {
    const pct = Math.floor(frac * 100);
    if (step === "lights" && pct === last) return; // a message per percent, not per light
    last = pct;
    self.postMessage({ gen, step, frac });
  });
  self.postMessage({ gen, done: r }, lightGridBuffers(r));
};
