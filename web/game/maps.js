// The map catalog (/maps.json: every map the server has, with its geometry)
// and preview images rendered from it on the client: nothing is shipped as an
// image. Previews render one at a time in a worker (thumbs.worker.js), so the
// page never stalls on them; where a worker can't draw, on the page, yielding
// between maps.
import { createWorkerThumbnailer } from "../engine/thumbs.js";

export function createMapCatalog(look) {
  let list = null;      // Promise<[{name, desc, maxPlayers, size, boxes, ...}]>
  const previews = {};  // name -> data URL
  const thumbs = createWorkerThumbnailer(new URL("./thumbs.worker.js", import.meta.url), look);
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
      try { previews[name] = await thumbs.render(m, camera(m)); } catch (e) { console.warn("map preview", name, e); }
    });
    return queue.then(() => previews[name]);
  }

  // Card data for the menus: no geometry. custom: from the server's maps dir.
  const cards = async () => (await load()).map(({ name, desc, maxPlayers, size, sites, category, custom }) =>
    ({ name, desc, maxPlayers, size, category: category || "training", sites: (sites || []).map((s) => s.name), custom: !!custom, preview: previews[name] || null }));

  // Read /maps.json again (the Maps screen opens: custom maps come and go,
  // --maps-dir hot reload); a custom map's preview is drawn again (it may
  // have changed), a built-in's is kept.
  async function refresh() {
    const old = list ? await list.catch(() => []) : [];
    list = null;
    for (const m of old) if (m.custom) delete previews[m.name];
    return load();
  }

  return { load, refresh, preview, cards, previews, thumbStats: () => ({ ...thumbs.stats(), where: thumbs.where }) };
}
