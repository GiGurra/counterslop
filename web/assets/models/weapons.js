// Box-built weapon models, one spec per registry id (web/assets/weapons/weapons.json).
// A spec is a list of boxes in gun space (metres; -Z is the muzzle, +Y up):
//   [x, y, z, sx, sy, sz, colorName, emit?, tag?]
// tag "mag" dips with the reload, "flash" sits at the muzzle, "seed" boxes get
// a seeded per-box colour (the Prompt's RGB strip). The same spec draws the
// first-person viewmodel and the dropped item on the floor.
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";
import { mulberry32 } from "../../gen/rng.js";

// n seeded neon segments along the top of the Prompt (same on every client).
const rgbStrip = (n, seed) => {
  const r = mulberry32(seed), cols = ["neonCyan", "neonMagenta", "neonLime"];
  return Array.from({ length: n }, (_, i) => [0, 0.087, 0.07 - i * 0.07, 0.096, 0.012, 0.05, cols[Math.floor(r() * 3)], 0.9]);
};

export const MODELS = {
  // The Stub: a grey placeholder block with a "// TODO" stripe. Short.
  stub: [
    [0, 0, 0, 0.07, 0.09, 0.24, "stubGrey"],
    [0, -0.09, 0.08, 0.06, 0.13, 0.07, "keycapDark", 0, "mag"],
    [0, 0.05, -0.02, 0.072, 0.012, 0.18, "toyYellow", 0.2], // the TODO stripe
    [0, 0.01, -0.15, 0.03, 0.03, 0.06, "gunmetal"],
    [0, 0.01, -0.19, 0.035, 0.035, 0.02, "toyTip", 0.6],
  ],
  // The Prompt: long neon rifle with an RGB light strip and a banana mag.
  prompt: [
    [0, 0, 0, 0.09, 0.11, 0.42, "gunmetal"],
    [0, 0.02, 0.3, 0.07, 0.12, 0.2, "keycapDark"], // stock
    [0, 0.062, 0, 0.094, 0.02, 0.42, "neonMagenta", 0.5],
    ...rgbStrip(4, 7),
    [0, -0.13, -0.04, 0.055, 0.17, 0.08, "neonCyan", 0.3, "mag"],
    [0, -0.1, 0.13, 0.065, 0.14, 0.07, "keycapDark"], // grip
    [0, 0.01, -0.32, 0.035, 0.035, 0.24, "gunmetal"],
    [0, 0.01, -0.45, 0.05, 0.05, 0.04, "neonLime", 1],
  ],
  // The Linter: oak body, copper fittings, a tidy little scope. Accurate.
  linter: [
    [0, 0, 0, 0.085, 0.1, 0.4, "oak"],
    [0, 0.0, 0.29, 0.07, 0.11, 0.18, "oakDark"],
    [0, 0.085, -0.02, 0.045, 0.05, 0.16, "copper"], // scope
    [0, 0.085, -0.105, 0.04, 0.04, 0.01, "glassBlue", 0.5],
    [0, -0.12, -0.03, 0.05, 0.15, 0.07, "copper", 0, "mag"],
    [0, -0.1, 0.12, 0.06, 0.13, 0.065, "oakDark"],
    [0, 0.01, -0.31, 0.032, 0.032, 0.22, "copper"],
    [0, 0.01, -0.43, 0.045, 0.045, 0.03, "gunmetal"],
  ],
  // Code Review: a long sniper with a big scope and red/green diff bands.
  codereview: [
    [0, 0, 0.02, 0.08, 0.1, 0.5, "reviewGreen"],
    [0, 0.0, 0.33, 0.07, 0.13, 0.18, "gunmetal"],
    [0, 0.054, 0.1, 0.082, 0.012, 0.12, "reviewRed", 0.3],
    [0, 0.054, -0.06, 0.082, 0.012, 0.12, "neonLime", 0.3],
    [0, 0.11, 0.0, 0.06, 0.07, 0.26, "gunmetal"], // scope
    [0, 0.11, -0.135, 0.055, 0.055, 0.01, "glassBlue", 0.6],
    [0, -0.1, 0.0, 0.05, 0.1, 0.07, "gunmetal", 0, "mag"],
    [0, -0.1, 0.17, 0.06, 0.13, 0.065, "reviewGreen"],
    [0, 0.01, -0.45, 0.03, 0.03, 0.42, "gunmetal"],
    [0, 0.01, -0.67, 0.045, 0.045, 0.04, "reviewRed", 0.4],
  ],
  // Mechanical Keyboard: held like a club. A slab with rows of keycaps.
  keyboard: [
    [0, 0, -0.1, 0.06, 0.03, 0.46, "keycapDark"],
    ...Array.from({ length: 12 }, (_, i) => [((i % 2) - 0.5) * 0.026, 0.026, -0.3 + Math.floor(i / 2) * 0.07, 0.022, 0.022, 0.05, i === 3 ? "keycapAccent" : "keycap"]),
    [0, -0.02, 0.17, 0.04, 0.05, 0.1, "keycapDark"], // the "handle" (a cable bundle)
  ],
};

function partsFrom(spec, base, v) {
  const out = [];
  const dip = v.reload ? Math.sin(Math.min(v.reload, 1) * Math.PI) : 0;
  for (const [x, y, z, sx, sy, sz, col, emit = 0, tag] of spec) {
    const ty = tag === "mag" ? -dip * 0.15 : 0;
    out.push({ m: chain(base, translate(x, y + ty, z), scale(sx, sy, sz)), color: P[col] || P.gunmetal, emit });
  }
  return out;
}

// First-person viewmodel for weapon id. v: {bobX, bobY, kick, flash, reload 0..1, swing 0..1, raise 0..1}.
export function viewmodelParts(eye, yaw, pitch, v, id = "prompt") {
  const spec = MODELS[id] || MODELS.prompt;
  const cam = chain(translate(...eye), rotY(yaw), rotX(pitch));
  const dip = Math.sin(Math.min(v.reload || 0, 1) * Math.PI);
  const raise = 1 - (v.raise ?? 1); // weapon comes up from below after a switch
  const swing = Math.sin(Math.min(v.swing || 0, 1) * Math.PI); // melee swipe
  const g = chain(cam,
    translate(0.24 + v.bobX - swing * 0.2, -0.22 + v.bobY - dip * 0.25 - raise * 0.35, -0.5 + v.kick * 0.08 - swing * 0.15),
    rotX(v.kick * 0.25 - dip * 0.9 - raise * 0.6 - swing * 0.5), rotZ(dip * 0.6 + swing * 1.1), rotY(0.04 + swing * 0.6), scale(0.8, 0.8, 0.8));
  const out = partsFrom(spec, g, v);
  if (v.flash > 0 && id !== "keyboard") {
    const tip = Math.min(...spec.map((b) => b[2] - b[5] / 2));
    out.push({ m: chain(g, translate(0, 0.01, tip - 0.06), rotZ(v.flash * 7), scale(0.12, 0.12, 0.05)), color: P.flashHot, emit: 1 });
  }
  return out;
}

// A weapon lying on the floor: on its side, slowly turning, bobbing, faintly glowing.
export function itemParts(pos, id, time) {
  const spec = MODELS[id];
  if (!spec) return [];
  const base = chain(translate(pos[0], pos[1] + 0.25 + Math.sin(time * 2 + pos[0]) * 0.05, pos[2]), rotY(time * 0.8 + pos[2]), rotZ(Math.PI / 2), scale(1.3, 1.3, 1.3));
  return partsFrom(spec, base, {}).map((p) => ({ ...p, emit: Math.max(p.emit, 0.15) }));
}
