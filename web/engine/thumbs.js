// Offscreen previews: one hidden renderer (created on first use) draws a map
// from a camera into an image. Game-agnostic: the caller passes the look
// (palette, materials, sky, sun), the map and the camera.
//
// createThumbnailer runs the renderer on the page's thread (it blocks while it
// compiles and draws); createWorkerThumbnailer runs the same code in a Web
// Worker on an OffscreenCanvas (thumbs.worker.js in the caller's tree), so the
// page never stalls on a preview, and falls back to the page's thread where a
// worker can't draw (no OffscreenCanvas WebGL2).
import { createRenderer } from "./gl.js";

// The renderer half, usable on any canvas (a page canvas or an OffscreenCanvas).
export function previewRenderer(look, makeCanvas, w = 384, h = 216) {
  let gfx = null, canvas = null;
  const times = []; // per preview (ms): {map, setup, build, frame}
  return {
    // Draws map from cam ({eye, yaw, pitch, fov?}) and resolves to the canvas
    // (once the map's shader has compiled: in the background where it can).
    async draw(map, cam) {
      const t0 = performance.now();
      if (!gfx) {
        canvas = makeCanvas(w, h);
        gfx = createRenderer(canvas, look, { fixedSize: true, preserveDrawingBuffer: true });
        gfx.setOptions({ shadowSize: 1024, shadowQuality: 1, fog: 0.004 });
      }
      const t1 = performance.now();
      gfx.setMap(map, { preview: true });
      const t2 = performance.now();
      gfx.frame({ ...cam, time: 40, solids: [] });
      while (gfx.loading()) {
        await new Promise((r) => setTimeout(r, 10));
        gfx.frame({ ...cam, time: 40, solids: [] });
      }
      times.push({ map: map.name, setup: t1 - t0, build: t2 - t1, frame: performance.now() - t2 });
      return canvas;
    },
    // Renderer counters (tests: previews compile one shader and build no light grid).
    stats() { return gfx ? { ...gfx.stats(), times } : null; },
  };
}

// On the page's thread: render(map, cam) -> Promise<image URL>.
export function createThumbnailer(look, w = 384, h = 216) {
  const r = previewRenderer(look, (cw, ch) => Object.assign(document.createElement("canvas"), { width: cw, height: ch }), w, h);
  return {
    async render(map, cam) {
      const canvas = await r.draw(map, cam);
      const t0 = performance.now();
      const url = canvas.toDataURL("image/jpeg", 0.85);
      r.stats().times.at(-1).encode = performance.now() - t0;
      return url;
    },
    stats: () => r.stats(),
    where: "page",
  };
}

// In a worker (workerUrl: a module that calls serveThumbnails with the look);
// the page's thread only receives the finished JPEG. Falls back to
// createThumbnailer(fallbackLook) if the worker can't draw.
export function createWorkerThumbnailer(workerUrl, fallbackLook, w = 384, h = 216) {
  let worker = null, fallback = null, stats = null, seq = 0;
  const waiting = new Map(); // id -> {resolve, reject}
  const useFallback = () => (fallback ??= createThumbnailer(fallbackLook, w, h));
  const canWork = typeof Worker !== "undefined" && typeof OffscreenCanvas !== "undefined";
  function start() {
    worker = new Worker(workerUrl, { type: "module" });
    worker.onmessage = ({ data }) => {
      const p = waiting.get(data.id);
      waiting.delete(data.id);
      if (data.stats) stats = data.stats;
      if (data.blob) p?.resolve(URL.createObjectURL(data.blob));
      else p?.reject(new Error(data.error || "no preview"));
    };
    worker.onerror = (e) => { // the module failed to load: everything waiting goes to the page's thread
      e.preventDefault?.();
      worker = null;
      for (const p of waiting.values()) p.reject(new Error("worker failed"));
      waiting.clear();
    };
  }
  return {
    async render(map, cam) {
      if (!canWork || fallback) return useFallback().render(map, cam);
      if (!worker) start();
      try {
        return await new Promise((resolve, reject) => {
          const id = ++seq;
          waiting.set(id, { resolve, reject });
          worker.postMessage({ id, map, cam, w, h });
        });
      } catch (e) {
        if (!/WebGL2|worker failed/.test(e.message)) throw e;
        worker?.terminate(); worker = null;
        return useFallback().render(map, cam); // this browser's workers can't draw
      }
    },
    stats: () => (fallback ? fallback.stats() : stats),
    get where() { return fallback ? "page" : canWork ? "worker" : "page"; },
  };
}

// The worker's side: answer {id, map, cam, w, h} with {id, blob, stats}.
export function serveThumbnails(look) {
  let r = null;
  self.onmessage = async ({ data: { id, map, cam, w, h } }) => {
    try {
      r ??= previewRenderer(look, (cw, ch) => new OffscreenCanvas(cw, ch), w, h); // createRenderer throws "WebGL2 not supported" where workers can't draw
      const canvas = await r.draw(map, cam);
      const t0 = performance.now();
      const blob = await canvas.convertToBlob({ type: "image/jpeg", quality: 0.85 });
      const st = r.stats();
      st.times.at(-1).encode = performance.now() - t0;
      self.postMessage({ id, blob, stats: JSON.parse(JSON.stringify(st)) });
    } catch (e) {
      self.postMessage({ id, error: String(e?.message || e) });
    }
  };
}
