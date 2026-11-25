// The Big Diff (the bomb): a chunky slab of a pull request, +green and -red
// diff lines on its face, a CI status light and, once planted, a CI progress bar.
// Box-built; parts are {m, color, emit} like every other model.
import { chain, translate, scale, rotY, rotX } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";
import { mulberry32 } from "../../gen/rng.js";
import { scaleRgb } from "../../gen/color.js";

const SIZE = 1.5; // world scale of the model below

// [x, y, z, sx, sy, sz, colour, emit]: centred boxes in model space (metres).
const BODY = [
  [0, 0.09, 0, 0.62, 0.18, 0.42, "vibeDark", 0],
  [0, 0.185, 0, 0.56, 0.015, 0.36, "gunmetal", 0],
  // diff lines on the top face: + green, - red, of different lengths
  [-0.08, 0.2, -0.13, 0.34, 0.012, 0.035, "reviewGreen", 0.9],
  [-0.03, 0.2, -0.07, 0.44, 0.012, 0.035, "reviewGreen", 0.9],
  [-0.12, 0.2, -0.01, 0.26, 0.012, 0.035, "reviewRed", 0.9],
  [-0.05, 0.2, 0.05, 0.4, 0.012, 0.035, "reviewGreen", 0.9],
  [-0.1, 0.2, 0.11, 0.3, 0.012, 0.035, "reviewRed", 0.9],
  // keycap "ENTER" and the merge handle
  [0.22, 0.22, 0.1, 0.09, 0.06, 0.09, "keycap", 0.1],
  [0, 0.21, -0.2, 0.3, 0.04, 0.03, "neonMagenta", 0.6],
];

// st: {pos, yaw, time, planted, left (s), total (s), progress (defuse 0..1),
//      bar (draw the CI timer bar: mp_c4_show_timer), beep (s since the last beep; null: blink on its own)}
export function bigDiffParts({ pos, yaw = 0, time = 0, planted = false, left = 0, total = 40, progress = 0, bar = true, beep = null }) {
  const base = chain(translate(pos[0], pos[1], pos[2]), rotY(yaw), scale(SIZE, SIZE, SIZE));
  const box = ([x, y, z, sx, sy, sz, col, emit]) => ({ m: chain(base, translate(x, y, z), scale(sx, sy, sz)), color: P[col], emit });
  const out = BODY.map(box);
  // CI status light: idle amber; planted it blinks red, faster as the timer runs out.
  const urgency = planted ? 1 - Math.max(0, Math.min(1, left / total)) : 0;
  const rate = planted ? 1 + urgency * 7 : 0.5;
  // With the beeps (rounds.js): lit for a moment after each one, so light and sound are one clock.
  const on = planted && beep != null ? beep < 0.1 : Math.sin(time * Math.PI * 2 * rate) > 0;
  out.push(box([0.22, 0.24, -0.1, 0.07, 0.07, 0.07, planted ? (on ? "reviewRed" : "vibeDark") : "artisanAmber", on || !planted ? 1 : 0]));
  if (planted) {
    // The CI pipeline bar: a lime bar filling up as the timer runs out, the revert (defuse) eating it from the other end.
    // Only with mp_c4_show_timer 1: the bar is a countdown too.
    const w = 0.56, filled = bar ? urgency * w : 0;
    if (filled > 0) out.push(box([-w / 2 + filled / 2, 0.36, 0, filled, 0.05, 0.05, "neonLime", 1]));
    out.push(box([0, 0.36, 0, w + 0.03, 0.035, 0.035, "gunmetal", 0]));
    if (progress > 0) out.push(box([w / 2 - (progress * w) / 2, 0.42, 0, progress * w, 0.05, 0.05, "artisanCopper", 1]));
  }
  return out;
}

// Carried on a player's back, worn upright like a backpack with its diff face
// turned to whoever is behind: the same +green/-red lines, amber CI light and
// magenta merge handle as the dropped/planted slab, but bigger lines, lit, on a
// dark face inside a light keycap-coloured casing, so it reads against the dark
// hoodie from 20+ m and from any side. torso: the carrier's torso frame
// (player.js torsoFrame: feet at the origin, facing -z, swaying with the upper body).
export const PACK = { w: 0.6, h: 0.52, d: 0.16, back: 0.23 }; // casing size; back = the hoodie's back face (z)
const LIT = { reviewGreen: scaleRgb(P.reviewGreen, 1.7), reviewRed: scaleRgb(P.reviewRed, 1.35) }; // the slab's colours, brighter
export function carriedParts(torso, crouch = false) {
  const { w, h, d, back } = PACK;
  const base = chain(torso, translate(0, crouch ? 0.56 : 0.98, back + d / 2));
  const box = (x, y, z, sx, sy, sz, color, emit = 0) => ({ m: chain(base, translate(x, y, z), scale(sx, sy, sz)), color, emit });
  const face = d / 2 + 0.012, out = [
    box(0, 0, 0, w, h, d, P.keycap, 0.15), // casing
    box(0, -0.015, face - 0.006, w - 0.08, h - 0.11, 0.012, P.vibeDark), // the dark diff face
    box(0, h / 2 + 0.03, 0, 0.34, 0.06, 0.06, P.neonMagenta, 0.9), // merge handle on top
    box(w / 2 - 0.065, h / 2 - 0.05, face, 0.06, 0.06, 0.02, P.artisanAmber, 1), // CI status light
  ];
  // The slab's diff lines, rows top to bottom, thicker and fully lit.
  for (const [x, , z, sx, , , col] of BODY) {
    if (!LIT[col]) continue;
    out.push(box(x * 1.1 + 0.02, -0.015 - z * 1.25, face + 0.004, sx * 1.05, 0.06, 0.012, LIT[col], 1));
  }
  // Lit side stripes: green on the left, red on the right (seen from behind), so
  // the pack reads as a diff from the side too.
  for (const [x, c] of [[-1, LIT.reviewGreen], [1, LIT.reviewRed]]) out.push(box(x * (w / 2 + 0.006), 0, 0.01, 0.012, h - 0.08, d - 0.05, c, 1));
  // Shoulder straps over the hoodie.
  for (const x of [-0.18, 0.18]) out.push(box(x, h / 2 - 0.02, -d / 2 - 0.2, 0.05, 0.03, 0.42, P.gunmetal));
  return out;
}

// The merge t seconds after the blast: an expanding ring of glowing slabs, a
// flash core, then a plume of +/- diff glyphs drifting up and shrinking (~4 s).
export const BLAST_TIME = 4;
export function shockwaveParts(pos, t) {
  if (t < 0 || t > BLAST_TIME) return [];
  const out = plume(pos, t);
  if (t > 1.2) return out;
  const r = 1 + t * 22, h = Math.max(0.02, 0.8 * (1 - t / 1.2)), n = 24;
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2;
    out.push({ m: chain(translate(pos[0] + Math.sin(a) * r, pos[1] + 0.4, pos[2] + Math.cos(a) * r), rotY(a), scale(r * 0.27, h, 0.15)), color: i % 2 ? P.neonMagenta : P.neonLime, emit: 1 });
  }
  const c = Math.max(0, 4 * (1 - t / 0.5));
  if (c > 0) out.push({ m: chain(translate(pos[0], pos[1] + c / 2, pos[2]), scale(c, c, c)), color: P.flashHot, emit: 1 });
  return out;
}

// Seeded so every client sees the same plume: 40 glyphs, "+" (green) or "-" (red).
function plume(pos, t) {
  const rnd = mulberry32(0xd1ff), out = [];
  for (let i = 0; i < 40; i++) {
    const a = rnd() * Math.PI * 2, sp = 0.6 + rnd() * 2.2, up = 1.5 + rnd() * 3.5, spin = (rnd() - 0.5) * 6;
    const plus = rnd() < 0.55, life = 2.5 + rnd() * 1.5;
    if (t > life) continue;
    const k = 1 - t / life, sz = 1.1 * k * (0.6 + rnd() * 0.8);
    const c = [pos[0] + Math.sin(a) * sp * t * 2, pos[1] + 0.5 + up * t * 1.3 - 0.3 * t * t, pos[2] + Math.cos(a) * sp * t * 2];
    const base = chain(translate(...c), rotY(a + spin * t), rotX(spin * t * 0.5));
    const col = plus ? P.reviewGreen : P.reviewRed;
    out.push({ m: chain(base, scale(sz, sz * 0.28, sz * 0.28)), color: col, emit: 1 });
    if (plus) out.push({ m: chain(base, scale(sz * 0.28, sz, sz * 0.28)), color: col, emit: 1 });
  }
  return out;
}
