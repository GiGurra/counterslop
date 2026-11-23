import test from "node:test";
import assert from "node:assert/strict";
import { gpuParts, gpuCarriedParts, gpuHandParts, GPU_BOX } from "./gpu.js";
import { torsoFrame } from "./player.js";

// World-space corners of a part (a unit cube scaled by its matrix).
function corners(m) {
  const out = [];
  for (let i = 0; i < 8; i++) {
    const p = [i & 1 ? 0.5 : -0.5, i & 2 ? 0.5 : -0.5, i & 4 ? 0.5 : -0.5];
    out.push([0, 1, 2].map((r) => m[r] * p[0] + m[4 + r] * p[1] + m[8 + r] * p[2] + m[12 + r]));
  }
  return out;
}
function bounds(parts) {
  const mn = [Infinity, Infinity, Infinity], mx = [-Infinity, -Infinity, -Infinity];
  for (const pt of parts) for (const c of corners(pt.m)) for (let a = 0; a < 3; a++) { mn[a] = Math.min(mn[a], c[a]); mx[a] = Math.max(mx[a], c[a]); }
  return { mn, mx };
}
const finite = (parts) => parts.every((p) => [...p.m].every(Number.isFinite) && p.color.every(Number.isFinite) && p.emit >= 0 && p.emit <= 1);

test("on its shelf: standing on the spot's floor point, box-sized, with glowing parts", () => {
  const pos = [3, 1.2, -4], parts = gpuParts({ pos, yaw: 0.7, time: 2 });
  assert.ok(parts.length > 20 && finite(parts));
  const { mn, mx } = bounds(parts);
  assert.ok(Math.abs(mn[1] - pos[1]) < 0.01, `bottom at the floor point: ${mn[1]}`);
  assert.ok(mx[1] - mn[1] < GPU_BOX.h + 0.02, "no taller than the box");
  assert.ok(Math.hypot(mx[0] - mn[0], mx[2] - mn[2]) < 0.9, "box-sized footprint (the rotated box's bounds)");
  assert.ok(parts.some((p) => p.emit >= 0.9), "the 9090 label glows");
});

test("dropped: lying flat with a beacon above; secured or carried draws nothing here", () => {
  const pos = [0, 0, 0], flat = gpuParts({ pos, st: "dropped", time: 1 });
  const box = flat.filter((p) => bounds([p]).mx[1] < 1);
  const { mn, mx } = bounds(box);
  assert.ok(mn[1] > -0.01 && mx[1] < GPU_BOX.d + 0.03, `flat on the floor: ${mn[1]}..${mx[1]}`);
  assert.ok(bounds(flat).mx[1] > 1.5, "the beacon reaches up");
  assert.deepEqual(gpuParts({ pos, st: "secured" }), []);
  assert.deepEqual(gpuParts({ pos, st: "carried" }), []);
});

test("a grab / restock in progress adds a bar that fills", () => {
  const base = gpuParts({ pos: [0, 0, 0], time: 0 }).length;
  const half = gpuParts({ pos: [0, 0, 0], time: 0, progress: 0.5 }), full = gpuParts({ pos: [0, 0, 0], time: 0, progress: 1 });
  assert.equal(half.length, base + 2);
  const w = (ps) => { const b = bounds([ps.at(-1)]); return Math.hypot(b.mx[0] - b.mn[0], b.mx[2] - b.mn[2]); };
  assert.ok(w(full) > w(half) * 1.8);
});

test("carried: on the carrier's back (behind them, at torso height), lower when crouched", () => {
  const pos = [10, 0, 5];
  for (const yaw of [0, 1.2, -2.5]) {
    const { mn, mx } = bounds(gpuCarriedParts(torsoFrame({ p: pos, yaw }), false));
    const c = [(mn[0] + mx[0]) / 2, (mn[1] + mx[1]) / 2, (mn[2] + mx[2]) / 2];
    // the player looks along (-sin yaw, -cos yaw); the back is the other way
    const back = [Math.sin(yaw), Math.cos(yaw)];
    const along = (c[0] - pos[0]) * back[0] + (c[2] - pos[2]) * back[1];
    assert.ok(along > 0.1 && along < 0.5, `behind the player at yaw ${yaw}: ${along}`);
    assert.ok(c[1] > 0.9 && c[1] < 1.6, `torso height: ${c[1]}`);
  }
  assert.ok(bounds(gpuCarriedParts(torsoFrame({ p: pos, yaw: 0 }), true)).mn[1] < bounds(gpuCarriedParts(torsoFrame({ p: pos, yaw: 0 }), false)).mn[1]);
});

test("first person: low left in front of the eye, turning with the view", () => {
  const eye = [0, 1.6, 0];
  for (const yaw of [0, 2]) {
    const { mn, mx } = bounds(gpuHandParts(eye, yaw, 0, {}, 0));
    const c = [(mn[0] + mx[0]) / 2 - eye[0], (mn[1] + mx[1]) / 2 - eye[1], (mn[2] + mx[2]) / 2 - eye[2]];
    const fwd = [-Math.sin(yaw), -Math.cos(yaw)], left = [-Math.cos(yaw), Math.sin(yaw)];
    assert.ok(c[0] * fwd[0] + c[2] * fwd[1] > 0.3, "in front");
    assert.ok(c[0] * left[0] + c[2] * left[1] > 0.2, "to the left");
    assert.ok(c[1] < -0.2, "below the eye");
  }
});
