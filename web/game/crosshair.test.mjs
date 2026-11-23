// The crosshair helper (crosshair.js): the projection matches the renderer's
// perspective, and every ray the real ShotAngles draws lands inside the gap.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { project, shotCone, createCrosshair, MIN_GAP } from "./crosshair.js";
import { shotAngles, recoilOffset, recoilAngles } from "./arsenal.js";

const WEAPONS = JSON.parse(readFileSync(new URL("../assets/weapons/weapons.json", import.meta.url))).weapons;
const FOV = 1.3, H = 1080;

test("project: the centre is 0, an angle a off-axis lands at tan(a)/tan(fov/2) * h/2", () => {
  assert.deepEqual(project(0.3, -0.2, 0.3, -0.2, FOV, H).map((v) => Math.round(v * 1e9) / 1e9 + 0), [0, 0]);
  const a = 0.1, want = Math.tan(a) / Math.tan(FOV / 2) * H / 2;
  const up = project(0, 0, 0, a, FOV, H), right = project(0, 0, -a, 0, FOV, H);
  assert.ok(Math.abs(up[0]) < 1e-9 && Math.abs(up[1] + want) < 1e-9, `up ${up}`); // up is -y on screen
  assert.ok(Math.abs(right[0] - want) < 1e-9 && Math.abs(right[1]) < 1e-9, `right ${right}`); // yaw decreases to the right
  assert.equal(project(0, 0, Math.PI, 0, FOV, H), null); // behind
  // A narrower fov (a scope) magnifies.
  assert.ok(project(0, 0, 0, a, 0.32, H)[1] < up[1]);
});

test("shotCone: r is the spread's corner on screen (half-width x sqrt 2), 0 without spread", () => {
  const s = 0.05, c = shotCone({ camYaw: 0, camPitch: 0, yaw: 0, pitch: 0, spread: s, fov: FOV, h: H });
  const half = Math.tan(s / 2) / Math.tan(FOV / 2) * H / 2;
  assert.ok(c.r >= half * Math.SQRT2 * 0.999 && c.r <= half * Math.SQRT2 * 1.01, `r ${c.r} half ${half}`);
  assert.equal(shotCone({ camYaw: 0, camPitch: 0, yaw: 0, pitch: 0, spread: 0, fov: FOV, h: H }).r, 0);
});

// Seeded shots of every weapon in every stance-ish spread, aimed through a spray
// offset from a camera showing half of it (the view punch), looking level, up and
// down: every ray projects inside the circle the centred crosshair draws.
test("every ShotAngles ray lands inside the centred gap (all weapons, pellets, recoil, pitch)", () => {
  let n = 0;
  for (const w of WEAPONS) {
    const sp = w.spread || {};
    const spreads = [sp.base, sp.base + sp.move * 3, sp.base + sp.air, (sp.base + (sp.unscoped || 0)) * (sp.crouch || 1)].filter((s) => s > 0);
    for (const spread of spreads) for (const pitch of [0, 0.7, -0.9, 1.3]) for (const idx of [0, 3, 12]) for (const fov of [FOV, w.scope?.fov].filter(Boolean)) {
      const off = recoilOffset(w.recoil?.pattern || [], idx);
      const [ay, ap] = recoilAngles(0.4, pitch, off, 1); // the shot's aim (full offset)
      const [cy, cp] = recoilAngles(0.4, pitch, off, 0.5); // the camera (half: VIEW_PUNCH)
      const cone = shotCone({ camYaw: cy, camPitch: cp, yaw: ay, pitch: ap, spread, fov, h: H });
      const gap = Math.max(MIN_GAP, cone.reach);
      for (let seed = 1; seed <= 60; seed++) {
        for (const [y, p] of shotAngles(ay, ap, spread, seed * 2654435761, w.pellets)) {
          const q = project(cy, cp, y, p, fov, H);
          const d = Math.hypot(q[0], q[1]); // from the screen centre: the reticle stays there
          assert.ok(d <= gap + 1e-6, `${w.id} spread ${spread} pitch ${pitch} idx ${idx}: ray at ${d.toFixed(2)} px, gap ${gap.toFixed(2)}`);
          n++;
        }
      }
    }
  }
  assert.ok(n > 10000, `rays checked: ${n}`);
});

test("a spray opens the gap by the offset the view doesn't show; recovery closes it to the spread", () => {
  const w = WEAPONS.find((x) => x.id === "prompt"), spread = w.spread.base, gaps = [];
  for (const idx of [0, 4, 10, 4, 0]) {
    const off = recoilOffset(w.recoil.pattern, idx);
    const [ay, ap] = recoilAngles(0, 0, off, 1), [cy, cp] = recoilAngles(0, 0, off, 0.5);
    const cone = shotCone({ camYaw: cy, camPitch: cp, yaw: ay, pitch: ap, spread, fov: FOV, h: H });
    const hidden = project(cy, cp, ay, ap, FOV, H); // the unshown half of the offset, on screen
    assert.ok(cone.reach >= Math.hypot(...hidden) + cone.r * 0.7 - 1e-9 && cone.reach <= Math.hypot(...hidden) + cone.r + 1e-9, `idx ${idx}`);
    gaps.push(cone.reach);
  }
  assert.ok(gaps[1] > gaps[0] + 5 && gaps[2] > gaps[1], `opens: ${gaps}`);
  assert.ok(Math.abs(gaps[3] - gaps[1]) < 1e-9 && Math.abs(gaps[4] - gaps[0]) < 1e-9, `closes: ${gaps}`);
  assert.ok(Math.abs(gaps[0] - shotCone({ camYaw: 0, camPitch: 0, yaw: 0, pitch: 0, spread, fov: FOV, h: H }).r) < 1e-9, "recovered: the pure spread");
});

test("the gap is tight: the cone's corners are reached by some ray", () => {
  // 2000 seeded single shots: the farthest one is near the drawn radius.
  const spread = 0.1, cone = shotCone({ camYaw: 0, camPitch: 0, yaw: 0, pitch: 0, spread, fov: FOV, h: H });
  let far = 0;
  for (let seed = 1; seed <= 2000; seed++) {
    const [[y, p]] = shotAngles(0, 0, spread, seed * 2654435761);
    const q = project(0, 0, y, p, FOV, H);
    far = Math.max(far, Math.hypot(q[0], q[1]));
  }
  assert.ok(far > cone.r * 0.95, `farthest ${far} vs r ${cone.r}`);
});

test("createCrosshair: grows at once, eases back within ~50 ms, keeps the minimum", () => {
  const x = createCrosshair();
  assert.equal(x.update({ x: 0, y: 0, r: 0, reach: 0 }, 0.016).gap, MIN_GAP);
  assert.equal(x.update({ x: 1, y: -4, r: 70, reach: 80 }, 0.016).gap, 80); // instant, to the reach from the screen centre
  const g1 = x.update({ x: 0, y: 0, r: 0 }, 0.016).gap;
  assert.ok(g1 < 80 && g1 > MIN_GAP, `one frame: ${g1}`);
  let g = g1;
  for (let t = 0.016; t < 0.1; t += 0.016) g = x.update({ x: 0, y: 0, r: 0 }, 0.016).gap;
  assert.equal(g, MIN_GAP, "settled after 100 ms");
  // Never tighter than the truth while easing.
  x.update({ x: 0, y: 0, r: 50 }, 0.016);
  assert.ok(x.update({ x: 0, y: 0, r: 30 }, 0.016).gap >= 30);
});
