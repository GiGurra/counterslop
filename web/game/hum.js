// Ambient positional loops placed by the map's materials: every escalator (boxes of the
// escalator / escalator_rev materials, grouped when they touch) hums from its centre while
// the listener is within HUM_RANGE. Pure apart from the start() it is given.
//   createHums({ start(sp) -> handle {update(sp), stop()} | null, spatial(pos) -> {pan, gain} })
//   .update(map, listenerAlive)   every frame (cheap: a map change regroups, the rest is a few hypots)
export const HUM_MATERIALS = new Set(["escalator", "escalator_rev"]);
export const HUM_RANGE = 22; // metres
const GAP = 1.0; // boxes closer than this belong to one escalator

// Group the hum boxes of a map: returns [{ centre: [x, y, z], boxes }].
export function humSources(map) {
  const boxes = (map?.boxes || []).filter((b) => HUM_MATERIALS.has(b.kind));
  const groups = [];
  const touch = (a, b) => [0, 1, 2].every((i) => a.min[i] - GAP <= b.max[i] && b.min[i] - GAP <= a.max[i]);
  for (const b of boxes) {
    const hit = groups.filter((g) => g.boxes.some((o) => touch(o, b)));
    const merged = { boxes: [b, ...hit.flatMap((g) => g.boxes)] };
    for (const g of hit) groups.splice(groups.indexOf(g), 1);
    groups.push(merged);
  }
  return groups.map((g) => {
    const mn = [0, 1, 2].map((i) => Math.min(...g.boxes.map((b) => b.min[i])));
    const mx = [0, 1, 2].map((i) => Math.max(...g.boxes.map((b) => b.max[i])));
    return { centre: mn.map((v, i) => (v + mx[i]) / 2), boxes: g.boxes };
  });
}

export function createHums({ start, spatial, range = HUM_RANGE }) {
  let mapRef = null, sources = [];
  const stopAll = () => { for (const s of sources) { s.handle?.stop(); s.handle = null; } };
  return {
    update(map, listener) {
      if (map !== mapRef) { stopAll(); mapRef = map; sources = humSources(map).map((s) => ({ ...s, handle: null })); }
      for (const s of sources) {
        const d = listener ? Math.hypot(s.centre[0] - listener[0], s.centre[1] - listener[1], s.centre[2] - listener[2]) : Infinity;
        if (d > range) { if (s.handle) { s.handle.stop(); s.handle = null; } continue; }
        const sp = spatial(s.centre), fade = Math.min(1, (range - d) / 4); // fade in over the last 4 m
        const out = { pan: sp.pan, gain: sp.gain * 0.35 * fade };
        if (s.handle) s.handle.update(out);
        else s.handle = start(out);
      }
    },
    get playing() { return sources.filter((s) => s.handle).length; },
    get sources() { return sources.length; },
    stop: stopAll,
  };
}
