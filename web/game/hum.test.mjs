import test from "node:test";
import assert from "node:assert/strict";
import { humSources, createHums, HUM_RANGE } from "./hum.js";

const box = (x0, y0, z0, x1, y1, z1, kind) => ({ min: [x0, y0, z0], max: [x1, y1, z1], kind });
// Two escalators (an up and a down flight side by side count as one), far apart, and other boxes.
const steps = (x, kind) => Array.from({ length: 4 }, (_, i) => box(x, 0, i * 0.45, x + 1, 0.25 * (i + 1), (i + 1) * 0.45, kind));
const map = { boxes: [box(-50, -1, -50, 50, 0, 50, "storetile"), ...steps(0, "escalator"), ...steps(1.2, "escalator_rev"), ...steps(40, "escalator"), box(10, 0, 10, 12, 2, 12, "shelf")] };

test("escalator boxes group into one hum per escalator, at its centre", () => {
  const s = humSources(map);
  assert.equal(s.length, 2);
  const near = s.find((g) => g.centre[0] < 10), far = s.find((g) => g.centre[0] > 10);
  assert.equal(near.boxes.length, 8);
  assert.deepEqual(near.centre.map((v) => +v.toFixed(2)), [1.1, 0.5, 0.9]);
  assert.equal(far.boxes.length, 4);
  assert.deepEqual(humSources({ boxes: [box(0, 0, 0, 1, 1, 1, "sand")] }), []);
  assert.deepEqual(humSources(null), []);
});

test("hums start in range, follow the listener, stop out of range and on a new map", () => {
  const log = [];
  const start = (sp) => { const h = { sp, stopped: false, update(s) { h.sp = s; }, stop() { h.stopped = true; } }; log.push(h); return h; };
  const hums = createHums({ start, spatial: (p) => ({ pan: Math.sign(p[0]), gain: 1 }) });
  hums.update(map, [0, 0, 0]);
  assert.equal(hums.sources, 2);
  assert.equal(hums.playing, 1, "only the near escalator");
  assert.ok(log[0].sp.gain > 0 && log[0].sp.gain <= 0.35);
  hums.update(map, [5, 0, 0]);
  assert.equal(log.length, 1, "kept, not restarted");
  hums.update(map, [30, 0, 0]);
  assert.equal(hums.playing, 1, "now the far one");
  assert.ok(log[0].stopped);
  hums.update(map, [30 + HUM_RANGE * 3, 0, 0]);
  assert.equal(hums.playing, 0);
  hums.update(map, [0, 0, 0]);
  const before = log.filter((h) => !h.stopped).length;
  assert.equal(before, 1);
  hums.update({ boxes: [] }, [0, 0, 0]); // changelevel
  assert.equal(hums.playing, 0);
  assert.ok(log.every((h) => h.stopped));
  hums.update(map, null); // dead / spectating with no listener: nothing plays
  assert.equal(hums.playing, 0);
});

test("audio not started yet: start returns null and it retries next frame", () => {
  let live = false, n = 0;
  const hums = createHums({ start: () => (live ? { update() {}, stop() {} } : (n++, null)), spatial: () => ({ pan: 0, gain: 1 }) });
  hums.update(map, [0, 0, 0]);
  assert.equal(hums.playing, 0);
  live = true;
  hums.update(map, [0, 0, 0]);
  assert.equal(hums.playing, 1);
  assert.equal(n, 1);
});
