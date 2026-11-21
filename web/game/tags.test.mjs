import test from "node:test";
import assert from "node:assert/strict";
import { pickTags, segmentBlocked, aimedAt, tagAlpha, segPointDist, TAG_FAR } from "./tags.js";

// A pinhole camera looking down -z from the eye (yaw 0, pitch 0): x right, y down.
const eye = [0, 1.62, 0];
const project = ([x, y, z]) => { const d = eye[2] - z; return d > 0.1 ? [400 + (x - eye[0]) / d * 400, 300 - (y - eye[1]) / d * 400, d] : null; };
const view = (o = {}) => ({ eye, yaw: 0, pitch: 0, team: "vibe", observing: false, ...o });
const wall = { min: [-5, 0, -6], max: [5, 4, -5.5] };
const P = (id, team, p, name = `p${id}`) => ({ id, name, team, p, crouch: false });
const ids = (t) => t.map((x) => x.id).sort();

test("segmentBlocked: through a box, beside it, parallel to a face", () => {
  assert.ok(segmentBlocked([wall], [0, 1, 0], [0, 1, -10]));
  assert.ok(!segmentBlocked([wall], [0, 1, 0], [8, 1, -10].map((v, i) => (i === 0 ? 20 : v))));
  assert.ok(!segmentBlocked([wall], [0, 5, 0], [0, 5, -10]), "over the top");
  assert.ok(!segmentBlocked([wall], [0, 1, 0], [0, 1, -5]), "stops short of it");
});

test("aimedAt: the crosshair on the body, beside it, behind you", () => {
  const fwd = [0, 0, -1];
  assert.ok(aimedAt(eye, fwd, [0, 0, -10], 1.8));
  assert.ok(aimedAt(eye, fwd, [0.3, 0, -10], 1.8), "edge of the body");
  assert.ok(!aimedAt(eye, fwd, [1.5, 0, -10], 1.8), "a step to the side");
  assert.ok(!aimedAt(eye, fwd, [0, 0, 10], 1.8), "behind you");
  assert.ok(!aimedAt(eye, [0, 0.5, -0.866], [0, 0, -10], 1.8), "looking over their head");
});

test("playing: teammates always, enemies only aimed at and visible", () => {
  const players = [
    P(1, "vibe", [6, 0, -12]),        // teammate off-crosshair, behind the wall: still tagged
    P(2, "artisan", [-6, 0, -12]),    // enemy off-crosshair
    P(3, "artisan", [0, 0, -12]),     // enemy under the crosshair, behind the wall
  ];
  assert.deepEqual(ids(pickTags(view(), players, [wall], project)), [1]);
  // the same enemy without the wall: tagged
  assert.deepEqual(ids(pickTags(view(), players, [], project)), [1, 3]);
});

test("observing (spectator or dead): everyone, through walls", () => {
  const players = [P(1, "vibe", [6, 0, -12]), P(2, "artisan", [-6, 0, -12]), P(3, "artisan", [0, 0, -12])];
  assert.deepEqual(ids(pickTags(view({ observing: true, team: "spec" }), players, [wall], project)), [1, 2, 3]);
});

test("de-clutter: a far tag overlapping a nearer one is dropped; distance fades and cuts off", () => {
  const players = [P(1, "vibe", [0.2, 0, -30]), P(2, "vibe", [0, 0, -10]), P(3, "vibe", [0, 0, -(TAG_FAR + 5)])];
  const t = pickTags(view(), players, [], project);
  assert.deepEqual(t.map((x) => x.id), [2], "the nearer one wins; beyond TAG_FAR nothing");
  assert.equal(tagAlpha(5), 1);
  assert.ok(tagAlpha(40) < 1 && tagAlpha(40) > tagAlpha(59));
});

test("smoke: no enemy tag through a smoke's core, teammates and spectators unaffected", () => {
  const players = [P(1, "vibe", [3, 0, -12]), P(3, "artisan", [0, 0, -12])];
  const smoke = { c: [0, 1.5, -6], r: 3.4 }; // a 4.5 m smoke between you and the enemy
  assert.deepEqual(ids(pickTags(view(), players, [], project, [smoke])), [1]);
  assert.deepEqual(ids(pickTags(view(), players, [], project, [{ c: [10, 1.5, -6], r: 3.4 }])), [1, 3], "a smoke off to the side");
  assert.deepEqual(ids(pickTags(view({ observing: true }), players, [], project, [smoke])), [1, 3]);
  assert.equal(segPointDist([0, 0, 0], [0, 0, -10], [1, 0, -5]), 1);
  assert.equal(segPointDist([0, 0, 0], [0, 0, -10], [0, 0, 3]), 3, "past the end");
});
