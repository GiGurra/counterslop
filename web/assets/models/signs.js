// Signs on the map (content.Sign, docs/starslop.md phase 4): text in our own
// dot font (../font.js), each glyph a few boxes standing a few centimetres out
// of the surface it is on.
//   signBoxes(signs)              still signs as map boxes {min, max, kind}: built
//                                 into the map's mesh with everything else (free to draw)
//   signSolids(map, ctx)          the rest each frame as solids: signs on a mover
//                                 (where it is drawn; not while its box is hidden) and live ones (a template the
//                                 game fills in: backlog counters, the T-minus clock)
//                                 on split-flap tiles that flip when a character changes
//   signText(text, values)        a live template filled in
import { chain, translate, scale, rotX, rotZ } from "../../engine/math.js";
import { FONT_H, FONT_W, glyphRects } from "../font.js";
import { MoverOffset, MoverHidden } from "../../engine/sim.gen.js";

const ADV = FONT_W + 1, LINE = FONT_H + 2; // dots per character and per line

// The surface's frame for a face: n (out of the surface, towards the reader),
// right (the reader's right, looking at it) and up (the text's up). "+y" is
// on a floor, read standing on it facing +x (its top towards +x).
const UP = [0, 1, 0];
const FRAMES = {
  "+x": { n: [1, 0, 0], r: [0, 0, -1], u: UP },
  "-x": { n: [-1, 0, 0], r: [0, 0, 1], u: UP },
  "+z": { n: [0, 0, 1], r: [1, 0, 0], u: UP },
  "-z": { n: [0, 0, -1], r: [-1, 0, 0], u: UP },
  "+y": { n: [0, 1, 0], r: [0, 0, 1], u: [1, 0, 0] },
};
const frameOf = (face) => FRAMES[face] || FRAMES["+z"];

// Where each character cell of text sits on the sign, in metres in the sign's
// plane (u right, v up, centred on Pos): [{ch, u, v}] (u, v: the cell's
// top-left), plus the dot size.
export function layout(text, size) {
  const px = size / FONT_H, lines = text.split("\n"), cells = [];
  const h = (lines.length * LINE - 2) * px;
  lines.forEach((line, i) => {
    const w = ([...line].length * ADV - 1) * px;
    [...line].forEach((ch, k) => cells.push({ ch, u: -w / 2 + k * ADV * px, v: h / 2 - i * LINE * px }));
  });
  return { px, cells };
}

// One rectangle (u0..u1, v0..v1 in the plane, d0..d1 out of it) as a world box.
function worldBox(pos, f, u0, u1, v0, v1, d0, d1, kind) {
  const c = (u, v, d) => [0, 1, 2].map((a) => pos[a] + f.r[a] * u + f.u[a] * v + f.n[a] * d);
  const a = c(u0, v0, d0), b = c(u1, v1, d1);
  return { min: a.map((x, i) => Math.min(x, b[i])), max: a.map((x, i) => Math.max(x, b[i])), kind };
}

// The letter boxes of sign s with text (its own by default).
export function letterBoxes(s, text = s.text, at = s.pos) {
  const f = frameOf(s.face), { px, cells } = layout(text, s.size), d0 = 0.006, d1 = d0 + Math.max(0.012, px * 0.35), out = [];
  for (const { ch, u, v } of cells) {
    for (const r of glyphRects(ch)) out.push(worldBox(at, f, u + r.x * px, u + (r.x + r.w) * px, v - (r.y + r.h) * px, v - r.y * px, d0, d1, s.mat));
  }
  return out;
}

// Still signs (no mover, not live) as map boxes, for the map's mesh.
export const signBoxes = (signs) => (signs || []).filter((s) => !s.mover && !s.live).flatMap((s) => letterBoxes(s));

// A live template filled in: "{key}" -> values[key] ("" for one it lacks).
export const signText = (text, values) => text.replace(/\{([^}]+)\}/g, (_, k) => String(values?.[k] ?? ""));

// Colours of a sign drawn as solids, by material.
const LOOK = { led: { color: [1, 0.68, 0.16], emit: 1 }, ink: { color: [0.05, 0.05, 0.06], emit: 0 } };
const TILE = [0.07, 0.07, 0.08], FLIP_S = 0.22;
const boxSolid = (b, look) => ({ m: chain(translate((b.min[0] + b.max[0]) / 2, (b.min[1] + b.max[1]) / 2, (b.min[2] + b.max[2]) / 2), scale(b.max[0] - b.min[0], b.max[1] - b.min[1], b.max[2] - b.min[2])), ...look });

// The split-flap state: when each live cell last changed character.
const flips = new Map();

// Every sign drawn per frame, as solids. ctx: {t: the movers' draw time, now:
// seconds (the flips' clock), values: the live templates' values}.
export function signSolids(map, { t = 0, now = 0, values = {} } = {}) {
  const out = [];
  (map?.signs || []).forEach((s, i) => {
    if (!s.mover && !s.live) return;
    let at = s.pos;
    if (s.mover) {
      const m = map.movers?.[s.mover - 1];
      if (!m || MoverHidden(m, t)) return; // gone with its mover's box (the cabin floor's markings as the pods open it)
      const o = MoverOffset(m, t);
      at = [at[0] + o[0], at[1] + o[1], at[2] + o[2]];
    }
    const look = LOOK[s.mat] || { color: [0.92, 0.92, 0.9], emit: 0.2 };
    if (!s.live) { for (const b of letterBoxes(s, s.text, at)) out.push(boxSolid(b, look)); return; }
    // Live: a tile per character, the character on it, a flap falling over
    // it for FLIP_S after it changes (the top half folding down at the split).
    const text = signText(s.text, values), f = frameOf(s.face), { px, cells } = layout(text, s.size);
    const tileH = (FONT_H + 1.2) * px, tileW = (FONT_W + 0.8) * px;
    cells.forEach(({ ch, u, v }, k) => {
      const key = `${i}:${k}`, st = flips.get(key);
      if (!st || st.ch !== ch) flips.set(key, { ch, at: st ? now : -1 });
      const age = now - flips.get(key).at;
      const cu = u + (FONT_W * px) / 2, cv = v - (FONT_H * px) / 2;
      out.push(boxSolid(worldBox(at, f, cu - tileW / 2, cu + tileW / 2, cv - tileH / 2, cv + tileH / 2, 0, 0.004, "flap"), { color: TILE, emit: 0 }));
      if (age >= 0 && age < FLIP_S) { // the flap: half a tile turning down about the split (shrinks to an edge, then shows its back)
        const a = (age / FLIP_S) * Math.PI, hinge = [0, 1, 2].map((ax) => at[ax] + f.r[ax] * cu + f.u[ax] * cv + f.n[ax] * 0.012);
        const turn = f.n[0] !== 0 ? rotZ(f.n[0] > 0 ? -a : a) : rotX(f.n[2] > 0 ? a : -a); // the top half towards the reader
        out.push({ m: chain(translate(...hinge), turn, translate(0, tileH / 4, 0), scale(Math.abs(f.r[0]) * tileW + Math.abs(f.n[0]) * 0.01, tileH / 2, Math.abs(f.r[2]) * tileW + Math.abs(f.n[2]) * 0.01)), color: [0.12, 0.12, 0.13], emit: 0 });
      }
      for (const r of glyphRects(ch)) out.push(boxSolid(worldBox(at, f, u + r.x * px, u + (r.x + r.w) * px, v - (r.y + r.h) * px, v - r.y * px, 0.006, 0.006 + Math.max(0.012, px * 0.35), s.mat), look));
    });
  });
  return out;
}

