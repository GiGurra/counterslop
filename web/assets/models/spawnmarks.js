// Team start markers on the floor: a glowing pad under every team spawn and
// a painted outline around each team's buy zone, in the team's colour, so
// you can see where Vibe Coders and Artisans start (and where you may buy).
// Pure: map in, render parts out ({m, color, emit}); built once per map.
import { chain, translate, scale } from "../../engine/math.js";
import { team as teamInfo } from "../teams.js";

export const PAD = 1.1;     // pad side (m)
export const LINE = 0.15;   // buy-zone outline width (m)
const THICK = 0.03;          // proud of the floor, under a player's feet

const slab = (x0, y, z0, x1, z1, color, emit) => ({
  m: chain(translate((x0 + x1) / 2, y + THICK / 2, (z0 + z1) / 2), scale(x1 - x0, THICK, z1 - z0)),
  color, emit,
});

export function spawnMarks(map) {
  const out = [];
  for (const [id, spawns] of Object.entries(map?.teamSpawns || {})) {
    const t = teamInfo(id);
    if (!t) continue;
    for (const [x, y, z] of spawns) out.push({ ...slab(x - PAD / 2, y, z - PAD / 2, x + PAD / 2, z + PAD / 2, t.rgb, 0.9), team: id, kind: "pad" });
    for (const zn of map.buyZones?.[id] || []) {
      // BuyAroundSpawns puts the zone's floor 0.5 m under the spawns.
      const [x0, y0, z0] = zn.min, [x1, , z1] = zn.max, y = y0 + 0.5;
      const dim = t.rgb.map((c) => c * 0.8);
      for (const s of [
        slab(x0, y, z0, x1, z0 + LINE, dim, 0.5), slab(x0, y, z1 - LINE, x1, z1, dim, 0.5),
        slab(x0, y, z0, x0 + LINE, z1, dim, 0.5), slab(x1 - LINE, y, z0, x1, z1, dim, 0.5),
      ]) out.push({ ...s, team: id, kind: "zone" });
    }
  }
  return out;
}
