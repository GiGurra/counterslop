import test from "node:test";
import assert from "node:assert/strict";
import { createDebris } from "./debris.js";
import { createPhysics } from "../engine/physics.js";

const floor = { min: [-20, -1, -20], max: [20, 0, 20], kind: "carpet" };
const wall = { min: [5, 0, -5], max: [6, 3, 5], kind: "metal" };
function setup(over = {}) {
  const phys = createPhysics(), boxes = [floor, wall], played = [], cfg = { r_decals: 1, r_casings: 1 };
  phys.setMap({ boxes });
  const sfx = {
    bulletImpact: (surface) => played.push(`impact:${surface}`),
    casingBrass: (sp, hard) => played.push(`brass:${hard ? "hard" : "soft"}`),
    casingShell: (sp, hard) => played.push(`shell:${hard ? "hard" : "soft"}`),
  };
  const d = createDebris({ sfx, spatial: () => ({ pan: 0, gain: 1 }), listener: () => over.listener ?? [0, 0, 0], cfg: () => cfg, boxes: () => boxes, raycast: phys.raycast, ...over.deps });
  return { d, played, cfg };
}
const rifle = { class: "rifle", action: "auto" }, sniper = { class: "sniper", action: "bolt", cycle: 1.1 }, knife = { class: "melee" };

test("a remote shot leaves holes where the server says, with the surface's sound", () => {
  const { d, played } = setup();
  d.remoteShot({ id: 2, w: "prompt", o: [0, 1.6, 0], h: [[5, 1.6, 0], [5, 1.0, 0.5]] }, rifle, null);
  assert.equal(d.stats().holes, 2);
  assert.deepEqual(played, ["impact:metal", "impact:metal"]);
  // An impact point with no map under it (a stale map, a moved box) leaves nothing.
  d.remoteShot({ id: 2, w: "prompt", o: [0, 1.6, 0], h: [[3, 1.6, 0]] }, rifle, null);
  assert.equal(d.stats().holes, 2);
});

test("impact sounds are rate-limited and only heard nearby", () => {
  const { d, played } = setup();
  for (let i = 0; i < 30; i++) d.impact([5, 1, i * 0.01], [-1, 0, 0], rifle);
  assert.equal(d.stats().holes, 30);
  assert.ok(played.length <= 5, `${played.length} sounds at once`);
  const far = setup({ listener: [-15, 0, -15] });
  far.d.impact([5, 1, 0], [-1, 0, 0], rifle);
  assert.equal(far.played.length, 0);
});

test("casings: one per shot, a muted clink on carpet; bolt actions eject when the bolt cycles; no casing from a knife", () => {
  const { d, played } = setup();
  d.localShot(rifle, [0, 1.6, 0], 0, 0, [0, 0, 0]);
  d.localShot(knife, [0, 1.6, 0], 0, 0, [0, 0, 0]);
  assert.equal(d.stats().casings, 1);
  for (let i = 0; i < 120; i++) d.update(1 / 60, null, "m");
  assert.equal(d.stats().flying, 0);
  assert.deepEqual(played, ["brass:soft"]);
  d.localShot(sniper, [0, 1.6, 0], 0, 0, [0, 0, 0]);
  assert.equal(d.stats().pending, 1);
  d.update(0.3, null, "m");
  assert.equal(d.stats().casings, 1);
  d.update(0.5, null, "m");
  assert.equal(d.stats().casings, 2);
  // A remote shooter's casing comes from its pose.
  d.remoteShot({ id: 3, w: "prompt" }, rifle, { p: [2, 0, 2], yaw: 0, pitch: 0, c: false });
  assert.equal(d.stats().casings, 3);
});

test("everything clears when the next round starts, and with the cvars off", () => {
  const { d, cfg } = setup();
  d.update(0, { ph: "live", n: 1 }, "m");
  d.impact([5, 1, 0], [-1, 0, 0], rifle);
  d.localShot(rifle, [0, 1.6, 0], 0, 0, [0, 0, 0]);
  d.update(0.1, { ph: "live", n: 1 }, "m");
  assert.equal(d.stats().holes, 1);
  assert.equal(d.stats().casings, 1);
  d.update(0.1, { ph: "freeze", n: 2 }, "m");
  assert.deepEqual(d.stats(), { holes: 0, casings: 0, flying: 0, pending: 0 });
  d.impact([5, 1, 0], [-1, 0, 0], rifle);
  d.localShot(rifle, [0, 1.6, 0], 0, 0, [0, 0, 0]);
  cfg.r_decals = 0; cfg.r_casings = 0;
  d.update(0.1, { ph: "freeze", n: 2 }, "m");
  assert.equal(d.stats().holes + d.stats().casings, 0);
  d.impact([5, 1, 0], [-1, 0, 0], rifle);
  d.localShot(rifle, [0, 1.6, 0], 0, 0, [0, 0, 0]);
  assert.equal(d.stats().holes + d.stats().casings, 0);
  d.testFill([0, 0, 0], 10, 5);
  assert.equal(d.stats().holes, 10);
});
