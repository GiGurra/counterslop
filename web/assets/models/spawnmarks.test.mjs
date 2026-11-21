import test from "node:test";
import assert from "node:assert/strict";
import { spawnMarks, PAD } from "./spawnmarks.js";
import { team } from "../teams.js";

// de_stack-like: Vibe Coders on the ground, Artisans on the deck at 4 m.
const map = {
  teamSpawns: { vibe: [[-26, 0, 0], [-23, 0, 3]], artisan: [[24, 4, -10]] },
  buyZones: { vibe: [{ min: [-29, -0.5, -3], max: [-20, 3, 6] }], artisan: [{ min: [21, 3.5, -13], max: [27, 7, -7] }] },
};
const at = (m) => [m[12], m[13], m[14]]; // column-major translation
const size = (m) => [m[0], m[5], m[10]];

test("a pad in the team colour on the floor under every team spawn", () => {
  const pads = spawnMarks(map).filter((s) => s.kind === "pad");
  assert.equal(pads.length, 3);
  const deck = pads.find((p) => p.team === "artisan");
  const [x, y, z] = at(deck.m);
  assert.deepEqual([x, z], [24, -10]);
  assert.ok(y > 4 && y < 4.05, `on the deck, not under it: y ${y}`);
  assert.deepEqual(deck.color, team("artisan").rgb);
  assert.ok(Math.abs(size(deck.m)[0] - PAD) < 1e-6);
  assert.ok(pads.filter((p) => p.team === "vibe").every((p) => at(p.m)[1] < 0.05));
});

test("each team's buy zone is outlined at its spawns' floor", () => {
  const lines = spawnMarks(map).filter((s) => s.kind === "zone");
  assert.equal(lines.length, 8);
  for (const l of lines.filter((s) => s.team === "artisan")) assert.ok(Math.abs(at(l.m)[1] - 4.015) < 1e-6);
});

test("no map, unknown team: nothing", () => {
  assert.deepEqual(spawnMarks(null), []);
  assert.deepEqual(spawnMarks({ teamSpawns: { nobody: [[0, 0, 0]] } }), []);
});
