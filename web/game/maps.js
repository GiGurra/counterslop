// The map catalog (/maps.json: every map the server has, with its geometry)
// and preview images rendered from it on the client: nothing is shipped as an
// image. Previews render one at a time, yielding between maps.
import { createThumbnailer } from "../engine/thumbs.js";

export function createMapCatalog(look) {
  let list = null;      // Promise<[{name, desc, maxPlayers, size, boxes, ...}]>
  const previews = {};  // name -> data URL
  const thumbs = createThumbnailer(look);
  let queue = Promise.resolve();

  const load = () => (list ??= fetch("/maps.json").then((r) => r.json()).catch((e) => { list = null; throw e; }));

  // Three-quarter view from the south, the whole map in frame.
  function camera(m) {
    let x0 = Infinity, x1 = -Infinity, z0 = Infinity, z1 = -Infinity, y1 = 0;
    for (const b of m.boxes) {
      x0 = Math.min(x0, b.min[0]); x1 = Math.max(x1, b.max[0]);
      z0 = Math.min(z0, b.min[2]); z1 = Math.max(z1, b.max[2]);
      y1 = Math.max(y1, b.max[1]);
    }
    // Fit the width (16:9 frame) or the depth, whichever needs more distance.
    const cx = (x0 + x1) / 2, cz = (z0 + z1) / 2, d = Math.max((x1 - x0) * 0.62, (z1 - z0) * 0.95);
    return { eye: [cx, d * 0.78 + y1 * 0.4, cz + d * 0.62], yaw: 0, pitch: -0.9, fov: 0.85 };
  }

  // Resolves to the preview of one map (rendered on first request).
  function preview(name) {
    if (previews[name]) return Promise.resolve(previews[name]);
    queue = queue.then(() => load()).then(async (all) => {
      if (previews[name]) return;
      const m = all.find((x) => x.name === name);
      if (!m) return;
      await new Promise((r) => setTimeout(r, 0)); // let the page breathe between maps
      try { previews[name] = thumbs.render(m, camera(m)); } catch (e) { console.warn("map preview", name, e); }
    });
    return queue.then(() => previews[name]);
  }

  // Card data for the menus: no geometry.
  const cards = async () => (await load()).map(({ name, desc, maxPlayers, size, sites }) =>
    ({ name, desc, maxPlayers, size, sites: (sites || []).map((s) => s.name), preview: previews[name] || null }));

  return { load, preview, cards, previews };
}
