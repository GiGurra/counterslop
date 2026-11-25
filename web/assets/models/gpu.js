// The boxed GPU (GPU Drop's objective): a glossy retail box with the card's three RGB
// fans printed on the front, a glowing "9090" in seven-segment digits, a brand stripe
// and a "LIMIT 1" sticker. Box-built; parts are {m, color, emit} like every other model.
//   gpuParts({pos, yaw, time, st, progress})   on its shelf / dropped (st: shelf|dropped|secured)
//   gpuCarriedParts(torso, crouch, time)        strapped to a carrier's back (third person)
//   gpuHandParts(eye, yaw, pitch, v, time)      first person: under the left arm, low left
import { chain, translate, scale, rotY, rotX, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";

// Box size in metres (chunky on purpose: it has to read across a store).
export const GPU_BOX = { w: 0.62, h: 0.36, d: 0.2 };
const { w: W, h: H, d: D } = GPU_BOX;
const FZ = -D / 2 - 0.004; // just in front of the front face (model -z is the front)

// Seven-segment digits: segments a b c d e f g, as [x, y, w, h] in a 1 x 2 cell.
const SEG = { a: [0.5, 1.9, 0.8, 0.2], b: [0.9, 1.45, 0.2, 0.8], c: [0.9, 0.55, 0.2, 0.8], d: [0.5, 0.1, 0.8, 0.2],
  e: [0.1, 0.55, 0.2, 0.8], f: [0.1, 1.45, 0.2, 0.8], g: [0.5, 1.0, 0.8, 0.2] };
const DIGITS = { 0: "abcdef", 9: "abcdfg" };
// "9090" with its left edge at x (as seen from the front), bottom at y, cell height h.
// The front faces model -z, so the viewer's right is model -x: x runs mirrored.
function label(text, x, y, h, col, emit) {
  const out = [], u = h / 2;
  [...text].forEach((ch, i) => {
    for (const s of DIGITS[ch] || "") {
      const [sx, sy, sw, sh] = SEG[s];
      out.push([-(x + (i * 1.3 + sx) * u), y + sy * u, FZ - 0.002, sw * u, sh * u, 0.004, col, emit]);
    }
  });
  return out;
}

// [x, y, z, sx, sy, sz, colour, emit]: centred boxes in model space, y up from the bottom.
function body(glow) {
  const fans = [-0.17, 0, 0.17].map((x, i) => [
    [x, 0.17, FZ, 0.15, 0.15, 0.004, "gunmetal", 0],
    [x, 0.17, FZ - 0.002, 0.11, 0.11, 0.004, ["neonCyan", "neonMagenta", "neonLime"][i], 0.35 + 0.5 * glow],
    [x, 0.17, FZ - 0.004, 0.07, 0.07, 0.004, "vibeDark", 0, "hub"],
  ]).flat();
  return [
    [0, H / 2, 0, W, H, D, "vibeDark", 0], // the box
    [0, H / 2, FZ + 0.001, W - 0.03, H - 0.03, 0.004, "gunmetal", 0], // glossy front panel
    [0, 0.06, FZ - 0.001, W - 0.03, 0.035, 0.004, "neonLime", 0.6], // brand stripe
    [0, H - 0.015, 0, W + 0.004, 0.012, D + 0.004, "neonLime", 0.4], // lid edge
    ...fans,
    ...label("9090", 0.04, 0.265, 0.07, "neonLime", 0.9), // upper right, seen from the front
    [0.23, 0.3, FZ - 0.002, 0.11, 0.045, 0.004, "reviewRed", 0.3], // LIMIT 1 sticker, upper left
    [-0.25, 0.07, FZ - 0.003, 0.08, 0.05, 0.004, "toyYellow", 0.2], // price sticker, lower right
    [0, H / 2, D / 2 + 0.002, W - 0.06, H - 0.08, 0.004, "gunmetal", 0], // the spec sheet on the back
  ];
}

const parts = (base, spec, spin = 0) => spec.map(([x, y, z, sx, sy, sz, col, emit = 0, tag]) => ({
  m: chain(base, translate(x, y, z), tag === "hub" ? rotZ(spin) : rotZ(0), scale(sx, sy, sz)),
  color: P[col] || P.gunmetal, emit,
}));

// On its shelf or dropped. pos = the floor point under the box, yaw = where the front faces
// (+ yaw turns it like players). progress 0..1: a grab / restock bar above it.
export function gpuParts({ pos, yaw = 0, time = 0, st = "shelf", progress = 0 }) {
  if (st === "secured" || st === "carried") return [];
  const glow = 0.5 + 0.5 * Math.sin(time * 3 + pos[0] + pos[2]);
  let base;
  if (st === "dropped") {
    // Lying on its back, fans up, a lime beacon above it so the attackers find it again.
    base = chain(translate(pos[0], pos[1] + D / 2, pos[2]), rotY(yaw), rotX(Math.PI / 2), translate(0, -H / 2, 0));
  } else {
    base = chain(translate(pos[0], pos[1], pos[2]), rotY(yaw));
  }
  const out = parts(base, body(glow), time * 6);
  if (st === "dropped") {
    const b = 0.5 + 0.5 * Math.sin(time * 4);
    out.push({ m: chain(translate(pos[0], pos[1] + 1.2, pos[2]), scale(0.03, 1.8, 0.03)), color: P.neonLime, emit: 0.4 + 0.6 * b });
  }
  if (progress > 0) {
    const top = chain(translate(pos[0], pos[1] + (st === "dropped" ? 0.55 : H + 0.25), pos[2]), rotY(yaw));
    const p = Math.min(1, progress), w = 0.6;
    out.push({ m: chain(top, translate(0, 0, 0), scale(w + 0.04, 0.05, 0.03)), color: P.gunmetal, emit: 0 });
    out.push({ m: chain(top, translate(-w / 2 + (p * w) / 2, 0, -0.02), scale(p * w, 0.04, 0.03)), color: P.neonLime, emit: 1 });
  }
  return out;
}

// Strapped to a carrier's back, front (the fans) facing backwards so enemies see it.
// torso: the carrier's torso frame (player.js torsoFrame: feet at the origin,
// facing -z, swaying with the upper body).
export function gpuCarriedParts(torso, crouch = false, time = 0) {
  const base = chain(torso, translate(0, crouch ? 0.8 : 1.1, 0.22), rotY(Math.PI), scale(0.8, 0.8, 0.8));
  const out = parts(base, body(0.5 + 0.5 * Math.sin(time * 3)), time * 6);
  // two straps over the shoulders
  for (const x of [-0.18, 0.18]) out.push({ m: chain(base, translate(x, H * 0.6, 0.12), scale(0.04, H * 1.3, 0.02)), color: P.gunmetal, emit: 0 });
  return out;
}

// First person: the box tucked under the left arm, its corner showing low left of the
// view, bobbing with the walk (v: {bobX, bobY} like the weapon viewmodel).
export function gpuHandParts(eye, yaw, pitch, v = {}, time = 0) {
  const cam = chain(translate(...eye), ...(v.frame ? [v.frame] : []), rotY(yaw), rotX(pitch)); // v.frame: the view's own (engine/gravity.js)
  // Front (fans, "9090") turned to the player, angled in: you see what you are carrying.
  const g = chain(cam, translate(-0.52 + (v.bobX || 0) * 0.6, -0.45 + (v.bobY || 0) * 0.6, -0.72), rotY(Math.PI + 0.6), rotX(-0.25), rotZ(0.1), scale(0.52, 0.52, 0.52), translate(0, -H / 2, 0));
  return parts(g, body(0.5 + 0.5 * Math.sin(time * 3)), time * 6);
}
