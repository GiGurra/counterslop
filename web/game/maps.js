// The map catalog (/maps.json: every map the server has, with its geometry)
// and preview images rendered from it on the client: nothing is shipped as an
// image. Previews are drawn only for the cards in view (want/drop, from the
// cards' IntersectionObserver), one at a time in a worker (thumbs.worker.js),
// each handed over the moment it is ready (onPreview); where a worker can't
// draw, on the page, yielding between maps. Each finished one is kept in this
// browser (previewcache.js, per build + map data), so the next visit shows it
// at once (cache reads skip the queue). Until then a card shows a top-down placeholder drawn from the
// map's boxes (placeholder()).
import { createWorkerThumbnailer } from "../engine/thumbs.js";
import { afterPaint, createPreviewQueue } from "./previewqueue.js";
import { openPreviewCache, previewKey, hashString } from "./previewcache.js";
import { radarLayout, radarShade } from "./radar.js";

const W = 384, H = 216;


export function createMapCatalog(look, { build = "", onPreview = () => {}, onStart = null } = {}) {
  let list = null;      // Promise<[{name, desc, maxPlayers, size, boxes, ...}]>
  const previews = {};  // name -> object URL
  const hashes = {};    // name -> hash of the map's data (the cache key)
  const holders = {};   // name -> placeholder data URL
  const thumbs = createWorkerThumbnailer(new URL("./thumbs.worker.js", import.meta.url), look, W, H);
  const cache = openPreviewCache(build);
  const counts = { rendered: 0, cached: 0, renderMs: [] };
  let evicted = false;

  const load = () => (list ??= fetch("/maps.json").then((r) => r.json()).catch((e) => { list = null; throw e; }));
  const find = async (name) => (await load()).find((x) => x.name === name);
  const hashOf = (m) => (hashes[m.name] ??= hashString(JSON.stringify(m)));

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

  // One preview drawn (and stored in the background: the card doesn't wait
  // for storage). The queue runs one at a time.
  async function work(name) {
    const m = await find(name);
    if (!m) return null;
    const t0 = performance.now();
    let blob;
    try { blob = await thumbs.render(m, camera(m)); } catch (e) { console.warn("map preview", name, e); return null; }
    counts.rendered++;
    counts.renderMs.push([name, Math.round(performance.now() - t0)]);
    const t = thumbs.stats()?.times?.at(-1); // where it went (the first one compiles the preview shader): for real-GPU reports
    if (t) console.info(`map preview ${name}: ${Math.round(performance.now() - t0)} ms (setup ${Math.round(t.setup)} · build ${Math.round(t.build)} · frame ${Math.round(t.frame)} · gpu+encode ${Math.round(t.encode ?? 0)}) on the ${thumbs.where}'s thread`);
    cache.put(previewKey(build, name, hashOf(m), W, H), blob);
    return (previews[name] = URL.createObjectURL(blob));
  }
  const queue = createPreviewQueue(work, (name, url) => onPreview(name, url), {
    yieldFn: afterPaint, // the finished one is painted on its card before the next starts
    onStart,
  });

  // A card came into view (prio: its place on the page; the vote's options go
  // first) / left it. The cache is asked first, for every card at once (a read
  // is quick, nothing to queue for); only a miss joins the drawing queue.
  const DROPPED = Infinity, checking = new Map(), missed = new Set(); // checking: name -> prio while its cache read runs
  async function want(name, prio = 0) {
    if (!evicted) { evicted = true; cache.evict(); }
    if (previews[name]) return;
    if (missed.has(name)) return queue.want(name, prio);
    if (checking.has(name)) { checking.set(name, prio); return; }
    checking.set(name, prio);
    let blob = null;
    try {
      const m = await find(name);
      if (m) blob = await cache.get(previewKey(build, name, hashOf(m), W, H));
    } catch {}
    const p = checking.get(name);
    checking.delete(name);
    if (previews[name]) return;
    if (blob) { counts.cached++; onPreview(name, (previews[name] = URL.createObjectURL(blob))); return; }
    missed.add(name);
    if (p !== DROPPED) queue.want(name, p);
  }
  function drop(name) {
    if (checking.has(name)) checking.set(name, DROPPED);
    queue.drop(name);
  }

  // A card's stand-in until its preview is ready: the map from above (radar.js's
  // floor plan), drawn on a small 2D canvas. Sync once the catalog is loaded.
  async function placeholder(name) {
    if (name in holders) return holders[name];
    const m = await find(name), lay = m && radarLayout(m);
    if (!lay || typeof document === "undefined") return (holders[name] = null);
    const c = Object.assign(document.createElement("canvas"), { width: W / 2, height: H / 2 }), g = c.getContext("2d");
    g.fillStyle = "#14181f"; g.fillRect(0, 0, c.width, c.height);
    const k = Math.min((c.width - 8) / lay.w, (c.height - 8) / lay.d), ox = (c.width - lay.w * k) / 2, oz = (c.height - lay.d * k) / 2;
    const rect = (b) => g.fillRect(ox + (b.min[0] - lay.x0) * k, oz + (b.min[2] - lay.z0) * k, Math.max(1, (b.max[0] - b.min[0]) * k), Math.max(1, (b.max[2] - b.min[2]) * k));
    for (const b of lay.order) { const v = radarShade(b.max[1]); g.fillStyle = `rgb(${Math.round(v * 150)},${Math.round(v * 175)},${Math.round(v * 205)})`; rect(b); }
    g.globalAlpha = 0.35; g.fillStyle = "#c8d4e6";
    for (const b of lay.over) rect(b);
    return (holders[name] = c.toDataURL("image/png"));
  }

  // Card data for the menus: no geometry. custom: from the server's maps dir.
  const cards = async () => (await load()).map(({ name, desc, maxPlayers, size, sites, category, custom }) =>
    ({ name, desc, maxPlayers, size, category: category || "training", sites: (sites || []).map((s) => s.name), custom: !!custom, preview: previews[name] || null }));

  // Read /maps.json again (the Maps screen opens: custom maps come and go,
  // --maps-dir hot reload); a custom map whose data changed is drawn again
  // (its hash, so its cache key, changed too), a built-in's preview is kept.
  async function refresh() {
    const old = list ? await list.catch(() => []) : [];
    list = null;
    const fresh = await load();
    const now = Object.fromEntries(fresh.map((m) => [m.name, m]));
    for (const m of old) {
      if (!m.custom) continue;
      const n = now[m.name];
      if (n && hashString(JSON.stringify(n)) === hashes[m.name]) continue;
      delete previews[m.name]; delete hashes[m.name]; delete holders[m.name];
      missed.delete(m.name); queue.forget(m.name);
      onPreview(m.name, null);
    }
    return fresh;
  }

  return {
    load, refresh, cards, previews, want, drop, placeholder,
    // Tests: what the queue did (log: [start|done|fail, name]) and where previews came from.
    previewStats: () => ({ log: queue.log.map((e) => e.join(" ")), pending: queue.pending(), busy: queue.busy(), idle: queue.idle(), ...counts, cache: { ...cache.stats } }),
    thumbStats: () => ({ ...thumbs.stats(), where: thumbs.where }),
  };
}
