// Things worn on the torso (the Big Diff and a GPU on a carrier's back) move
// with the upper body's draw-only sway, bob and lean, rigidly, like the hoodie
// and the apron do: the operator saw the Big Diff hang still while its carrier
// swayed under it.
import test from "node:test";
import assert from "node:assert/strict";
import { playerParts, torsoFrame } from "./player.js";
import { carriedParts } from "./bigdiff.js";
import { gpuCarriedParts } from "./gpu.js";
import { newGait, gaitStep } from "../../gen/gait.js";

// Column-major 4x4: a*b and the inverse of an affine matrix.
function mul(a, b) {
  const o = new Array(16).fill(0);
  for (let c = 0; c < 4; c++) for (let r = 0; r < 4; r++) for (let k = 0; k < 4; k++) o[c * 4 + r] += a[k * 4 + r] * b[c * 4 + k];
  return o;
}
function invAffine(m) {
  const [a, b, c, d, e, f, g, h, i] = [m[0], m[4], m[8], m[1], m[5], m[9], m[2], m[6], m[10]];
  const det = a * (e * i - f * h) - b * (d * i - f * g) + c * (d * h - e * g);
  const r = [(e * i - f * h) / det, (c * h - b * i) / det, (b * f - c * e) / det,
    (f * g - d * i) / det, (a * i - c * g) / det, (c * d - a * f) / det,
    (d * h - e * g) / det, (b * g - a * h) / det, (a * e - b * d) / det]; // row-major 3x3 inverse
  const t = [m[12], m[13], m[14]];
  const o = [r[0], r[3], r[6], 0, r[1], r[4], r[7], 0, r[2], r[5], r[8], 0, 0, 0, 0, 1];
  for (let k = 0; k < 3; k++) o[12 + k] = -(r[k * 3] * t[0] + r[k * 3 + 1] * t[1] + r[k * 3 + 2] * t[2]);
  return o;
}
const close = (a, b, eps = 1e-4) => a.every((v, j) => Math.abs(v - b[j]) < eps);
const moved = (a, b) => a.some((v, j) => Math.abs(v - b[j]) > 1e-3);

// A body walking for a while, one standing (idle sway only), one crouch-walking.
function gaits() {
  const out = [];
  for (const [v, o, n] of [[7, {}, 137], [0, {}, 260], [2.3, { crouch: true }, 91]]) {
    const g = newGait();
    for (let i = 0; i < n; i++) gaitStep(g, 0, 0, -v * 0.01, 0.01, o);
    out.push([g, !!o.crouch]);
  }
  return out;
}

const WORN = [
  ["Big Diff", (torso, crouch) => carriedParts(torso, crouch)],
  ["GPU", (torso, crouch) => gpuCarriedParts(torso, crouch, 1.3)],
];

test("worn on the back: follows the torso's sway exactly as the hoodie does", () => {
  for (const [g, crouch] of gaits()) for (const alpha of [0.2, 0.9]) {
    const still = { p: [4, 1, -3], yaw: 0.8, pitch: 0.1, crouch, team: "vibe", id: 5, rgb: [1, 0, 0] };
    const swayed = { ...still, gait: g, alpha };
    // The torso's draw-only motion this frame: drawn body = M · still body.
    const body = (p) => playerParts(p).find((pt) => pt.name === "body").m;
    const M = mul(body(swayed), invAffine(body(still)));
    assert.ok(moved(M, [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]), "the gait really moves the torso");
    for (const [name, worn] of WORN) {
      const at = worn(torsoFrame(still), crouch), drawn = worn(torsoFrame(swayed), crouch);
      for (let i = 0; i < at.length; i++) {
        assert.ok(close(drawn[i].m, mul(M, at[i].m)), `${name} part ${i} rides the torso (crouch ${crouch}, alpha ${alpha})`);
        assert.ok(moved(drawn[i].m, at[i].m), `${name} part ${i} isn't left behind`);
      }
    }
  }
});

test("without a gait the torso frame is the hit pose's (feet, facing)", () => {
  const f = torsoFrame({ p: [1, 2, 3], yaw: Math.PI / 2 });
  assert.ok(close(f, [0, 0, -1, 0, 0, 1, 0, 0, 1, 0, 0, 0, 1, 2, 3, 1]));
});

// World-space centre of a part (unit cube scaled by its matrix).
const centre = (m) => [m[12], m[13], m[14]];

test("the carried Big Diff reads from behind: lit +green/-red lines on the back, a light casing", async () => {
  const { PALETTE: P } = await import("../palette.js");
  for (const crouch of [false, true]) for (const yaw of [0, 1.1, -2.4]) {
    const st = { p: [2, 0, -1], yaw, crouch };
    const parts = carriedParts(torsoFrame(st), crouch);
    const back = [Math.sin(yaw), Math.cos(yaw)]; // the player faces (-sin, -cos)
    const behind = (c) => (c[0] - st.p[0]) * back[0] + (c[2] - st.p[2]) * back[1];
    for (const [col, hue] of [["reviewGreen", 1], ["reviewRed", 0]]) {
      // the slab's colours (its hue, at least as bright)
      const lines = parts.filter((p) => p.color[hue] >= P[col][hue] && p.color[hue] > 1.5 * Math.max(...p.color.filter((_, i) => i !== hue)));
      assert.ok(lines.length >= 2 && lines.every((p) => p.emit === 1), `${col} diff lines, lit`);
      // on the face turned backwards: further back than the casing's centre
      assert.ok(lines.filter((p) => behind(centre(p.m)) > behind(centre(parts[0].m)) + 0.05).length >= 2, "the diff face looks backwards");
    }
    const casing = parts[0];
    assert.ok(casing.color.reduce((a, b) => a + b) > 2, "a light casing against the dark hoodie");
    const c = centre(casing.m);
    assert.ok(behind(c) > 0.2 && behind(c) < 0.45, `on the back: ${behind(c)}`);
    assert.ok(crouch ? c[1] > 0.3 && c[1] < 0.8 : c[1] > 0.6 && c[1] < 1.35, `at the hoodie's height: ${c[1]}`);
  }
});
