import { test } from "node:test";
import assert from "node:assert/strict";
import { mapFor } from "./casemap.mjs";
import { carriedRemotes, createScannerGag, departureHud, signValues } from "./ships.js";

const moon = mapFor("lab_moon");
const dep = moon.departures[0];

test("a remote in MVP-1's cabin is drawn where the cabin is drawn", () => {
  // At 17 s MVP-1 climbs; a passenger seen at the view time (16.9 s) stands on its floor.
  const tView = 16.9, tDraw = 17.0;
  const climbed = (t) => { const k = Math.min(1, Math.max(0, (t - 14) / 6)); return 110 * k * k * (3 - 2 * k); };
  const passenger = { id: 2, p: [dep.board[0], climbed(tView), dep.board[2]] };
  const outside = { id: 3, p: [-130, 0, 0] };
  const [a, b] = carriedRemotes([passenger, outside], moon, tView, tDraw);
  assert.ok(Math.abs(a.p[1] - climbed(tDraw)) < 1e-9, `passenger drawn at ${a.p[1]}, the floor at ${climbed(tDraw)}`);
  assert.equal(b, outside);
  assert.deepEqual(carriedRemotes([passenger], moon, 5, 5), [passenger]);
});

test("the departure board: boarding, in flight, aboard, far away", () => {
  const near = [dep.door[0] - 5, 0, 0];
  assert.deepEqual(departureHud(moon, near, 3.2), { name: "MVP-1", state: "boarding", in: 11, aboard: false });
  assert.deepEqual(departureHud(moon, near, 20), { name: "MVP-1", state: "flight", in: 10, aboard: false });
  assert.equal(departureHud(moon, dep.board, 5).aboard, true);
  assert.equal(departureHud(moon, [90, 20, 0], 5), null); // on the Moon: no board
  assert.equal(departureHud(mapFor("de_slop"), [0, 0, 0], 5), null);
});

test("live sign values: each team's backlog, the seconds to MVP-1's doors closing", () => {
  const shot = mapFor("cq_moonshot");
  assert.deepEqual(signValues(shot, 3.2, { tk: { vibe: 287, artisan: 9 } }), { "backlog:vibe": "287", "backlog:artisan": "  9", "departure:MVP-1": "09" });
  assert.equal(signValues(shot, 20, null)["departure:MVP-1"], "GO");
  assert.equal(signValues(shot, 20, null)["backlog:vibe"], "---");
  assert.equal(signValues(shot, 35 + 1.5, null)["departure:MVP-1"], "11"); // the next one
});

test("the security scanner beeps as you walk in, not while you stand in it, not twice in a row", () => {
  const shot = mapFor("cq_moonshot"), z = shot.scanners[1], gag = createScannerGag(8);
  const inside = [(z.min[0] + z.max[0]) / 2, 0, (z.min[2] + z.max[2]) / 2], out = [-260, 0, 0];
  assert.equal(gag.enter(shot, out, 0), false);
  assert.equal(gag.enter(shot, inside, 1), true);
  assert.equal(gag.enter(shot, inside, 1.1), false);
  assert.equal(gag.enter(shot, out, 2), false);
  assert.equal(gag.enter(shot, inside, 3), false, "cooling down");
  gag.enter(shot, out, 9);
  assert.equal(gag.enter(shot, inside, 10), true);
});
