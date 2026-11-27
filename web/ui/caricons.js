// The race shop's icons (ui/carshop.js items): a car type is drawn from its
// own model (assets/models/car.js carParts, at rest, seen from the side, nose
// to the right), so the icon is the car you get; the items and upgrades get
// a glyph by kind. carIconRects is pure (tested in Node).
//   carIconRects(type) -> [[x, y, w, h, "#rrggbb"], ...] in a 64 x 28 box
//   carShopIconSvg(id) -> SVG markup (our own constants) for "car:<type>" |
//                         "front:<item>" | "rear:<item>" | "up:<upgrade>"; "" if none
import carsData from "../assets/cars.json" with { type: "json" };
import { carParts } from "../assets/models/car.js";
import { rgbHex } from "../gen/color.js";

const CORNERS = [];
for (const x of [-0.5, 0.5]) for (const y of [-0.5, 0.5]) for (const z of [-0.5, 0.5]) CORNERS.push([x, y, z]);

export function carIconRects(type, w = 64, h = 28, pad = 1) {
  const spec = carsData.cars.find((c) => c.id === type);
  if (!spec) return [];
  const parts = carParts({ spec, pos: [0, 0, 0] });
  const rects = parts.map((p) => {
    const m = p.m;
    let u0 = Infinity, u1 = -Infinity, v0 = Infinity, v1 = -Infinity, depth = 0;
    for (const [x, y, z] of CORNERS) {
      const wx = m[0] * x + m[4] * y + m[8] * z + m[12], wy = m[1] * x + m[5] * y + m[9] * z + m[13], wz = m[2] * x + m[6] * y + m[10] * z + m[14];
      u0 = Math.min(u0, -wz); u1 = Math.max(u1, -wz); v0 = Math.min(v0, wy); v1 = Math.max(v1, wy); depth += wx;
    }
    return { u0, u1, v0, v1, depth: depth / 8, col: p.color };
  }).filter((r) => r.u1 - r.u0 > 1e-3 && r.v1 - r.v0 > 1e-3);
  if (!rects.length) return [];
  // The far side first (x < 0 is the far side seen from +x): the near wheels over the hull.
  rects.sort((a, b) => a.depth - b.depth);
  const U0 = Math.min(...rects.map((r) => r.u0)), U1 = Math.max(...rects.map((r) => r.u1));
  const V0 = Math.min(...rects.map((r) => r.v0)), V1 = Math.max(...rects.map((r) => r.v1));
  const k = Math.min((w - 2 * pad) / (U1 - U0), (h - 2 * pad) / (V1 - V0));
  const ox = (w - (U1 - U0) * k) / 2, oy = (h - (V1 - V0) * k) / 2;
  const r2 = (v) => Math.round(v * 100) / 100;
  return rects.map((r) => [r2(ox + (r.u0 - U0) * k), r2(oy + (V1 - r.v1) * k), r2(Math.max(1, (r.u1 - r.u0) * k)), r2(Math.max(1, (r.v1 - r.v0) * k)), rgbHex(r.col)]);
}

// A glyph per item kind (cars.json equipment "kind") and per upgrade.
const GLYPH = {
  beam: `<svg viewBox="0 0 40 28"><rect x="3" y="10" width="10" height="8" rx="2" /><path d="M13 14h24" fill="none" stroke-width="3" /><path d="M13 11l24 2v2l-24 2z" opacity=".5" /></svg>`,
  rocket: `<svg viewBox="0 0 40 28"><path d="M8 11h20l7 3-7 3H8z" /><path d="M8 11l-4-4v14l4-4z" /><path d="M2 14h-2" fill="none" stroke-width="2" /></svg>`,
  orb: `<svg viewBox="0 0 40 28"><circle cx="24" cy="14" r="8" /><path d="M4 14q6-7 12 0t12 0" fill="none" stroke-width="2" opacity=".6" /></svg>`,
  ram: `<svg viewBox="0 0 40 28"><path d="M6 6h8v16H6zM14 10l18-4v16l-18-4z" /><path d="M34 4v20" fill="none" stroke-width="3" /></svg>`,
  spikes: `<svg viewBox="0 0 40 28"><path d="M3 24l5-14 5 14 5-14 5 14 5-14 5 14 5-14 1 14z" /></svg>`,
  oil: `<svg viewBox="0 0 40 28"><path d="M20 3c5 7 8 11 8 15a8 8 0 0 1-16 0c0-4 3-8 8-15z" /><ellipse cx="20" cy="25" rx="16" ry="2.5" opacity=".5" /></svg>`,
  mine: `<svg viewBox="0 0 40 28"><ellipse cx="20" cy="18" rx="13" ry="6" /><rect x="17" y="7" width="6" height="6" rx="1" /><path d="M20 3v4" fill="none" stroke-width="2" /></svg>`,
  charges: `<svg viewBox="0 0 40 28"><rect x="5" y="6" width="8" height="18" rx="1" /><rect x="16" y="6" width="8" height="18" rx="1" /><rect x="27" y="6" width="8" height="18" rx="1" /><path d="M5 3h30" fill="none" stroke-width="2" /></svg>`,
  tank: `<svg viewBox="0 0 40 28"><rect x="6" y="7" width="26" height="15" rx="7" /><path d="M32 14h5M14 11l-3 4h4l-2 4 6-6h-4l2-2z" fill="none" stroke-width="2" /></svg>`,
  plating: `<svg viewBox="0 0 40 28"><path d="M20 3l13 4v7c0 6-6 10-13 12C13 24 7 20 7 14V7z" /><path d="M13 11h14M13 16h14" fill="none" stroke="#000" stroke-width="1.5" /></svg>`,
  engine: `<svg viewBox="0 0 40 28"><rect x="9" y="9" width="20" height="13" rx="2" /><rect x="12" y="4" width="5" height="5" /><rect x="21" y="4" width="5" height="5" /><path d="M29 15h6M3 15h6" fill="none" stroke-width="3" /></svg>`,
  tyres: `<svg viewBox="0 0 40 28"><circle cx="20" cy="14" r="11" /><circle cx="20" cy="14" r="5" fill="#000" /><path d="M20 3v4M20 21v4M9 14h4M27 14h4" fill="none" stroke="#000" stroke-width="2" /></svg>`,
};

// The glyph for a race-shop item id; "" for an unknown one.
export function carShopGlyph(id) {
  const [k, v] = String(id).split(":");
  if (k === "up") return GLYPH[v] || "";
  if (k === "front" || k === "rear") return GLYPH[carsData.equipment?.find((e) => e.id === v)?.kind] || "";
  return "";
}

export function carShopIconSvg(id) {
  const [k, v] = String(id).split(":");
  if (k !== "car") return carShopGlyph(id);
  const rs = carIconRects(v);
  return rs.length ? `<svg class="car-icon" viewBox="0 0 64 28">${rs.map(([x, y, w, h, c]) => `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="${c}" />`).join("")}</svg>` : "";
}
