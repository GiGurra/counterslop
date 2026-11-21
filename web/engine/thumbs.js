// Offscreen previews: one hidden renderer (created on first use) draws a map
// from a camera into an image URL. Game-agnostic: the caller passes the look
// (palette, materials, sky, sun), the map and the camera.
import { createRenderer } from "./gl.js";

export function createThumbnailer(look, w = 384, h = 216) {
  let gfx = null, canvas = null;
  return {
    // cam: {eye, yaw, pitch, fov?}; returns a JPEG data URL.
    render(map, cam) {
      if (!gfx) {
        canvas = document.createElement("canvas");
        canvas.width = w; canvas.height = h;
        gfx = createRenderer(canvas, look, { fixedSize: true, preserveDrawingBuffer: true });
        gfx.setOptions({ shadowSize: 1024, shadowQuality: 1, fog: 0.004 });
      }
      gfx.setMap(map, { preview: true });
      gfx.frame({ ...cam, time: 40, solids: [] });
      return canvas.toDataURL("image/jpeg", 0.85);
    },
    // Renderer counters (tests: previews compile one shader and build no light grid).
    stats() { return gfx ? gfx.stats() : null; },
  };
}
