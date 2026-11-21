// Kill-feed weapon icons, drawn from the same box models as the viewmodels
// (assets/models/weapons.js): a side-view silhouette, muzzle to the right,
// CS-style. iconRects is pure (tested in Node); weaponIcon renders it once
// per weapon to a data URL. Kills without a gun get a glyph instead.
import { MODELS } from "../assets/models/weapons.js";
import { PALETTE } from "../assets/palette.js";
import { rgbHex } from "../gen/color.js";

// Glyphs for kills that aren't a model in hand.
export const KILL_GLYPH = { bigdiff: "💥", fall: "🪂", world: "☠" };

// Side-view rectangles in icon pixels for a weapon id: [x, y, w, h, colour]
// within a w x h box (keeps the aspect, centred). Muzzle (-Z) on the right.
export function iconRects(id, w = 64, h = 22, pad = 1) {
  const boxes = MODELS[id];
  if (!boxes?.length) return [];
  let u0 = Infinity, u1 = -Infinity, v0 = Infinity, v1 = -Infinity;
  const flat = boxes.map(([, y, z, , sy, sz, col]) => {
    const u = -z, r = { u0: u - sz / 2, u1: u + sz / 2, v0: y - sy / 2, v1: y + sy / 2, col };
    u0 = Math.min(u0, r.u0); u1 = Math.max(u1, r.u1); v0 = Math.min(v0, r.v0); v1 = Math.max(v1, r.v1);
    return r;
  });
  const k = Math.min((w - 2 * pad) / (u1 - u0), (h - 2 * pad) / (v1 - v0));
  const ox = (w - (u1 - u0) * k) / 2, oy = (h - (v1 - v0) * k) / 2;
  return flat.map((r) => [
    ox + (r.u0 - u0) * k, oy + (v1 - r.v1) * k, // y down
    Math.max(1, (r.u1 - r.u0) * k), Math.max(1, (r.v1 - r.v0) * k),
    rgbHex(PALETTE[r.col] || [0.8, 0.8, 0.8]),
  ]);
}

const cache = new Map();
// A data URL of the icon (light silhouette with a hint of the model's colours).
export function weaponIcon(id) {
  if (cache.has(id)) return cache.get(id);
  let url = "";
  const rects = iconRects(id, 64, 22);
  if (rects.length && typeof document !== "undefined") {
    const c = document.createElement("canvas");
    c.width = 128; c.height = 44; // 2x for crisp edges
    const g = c.getContext("2d");
    g.scale(2, 2);
    g.fillStyle = "#f2f2f2";
    for (const [x, y, rw, rh] of rects) g.fillRect(x - 0.5, y - 0.5, rw + 1, rh + 1); // outline-ish body
    g.globalAlpha = 0.45;
    for (const [x, y, rw, rh, col] of rects) { g.fillStyle = col; g.fillRect(x, y, rw, rh); }
    url = c.toDataURL();
  }
  cache.set(id, url);
  return url;
}
