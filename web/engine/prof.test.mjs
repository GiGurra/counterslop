import { test } from "node:test";
import assert from "node:assert/strict";
import { createProf, stat, verdict, formatProf } from "./prof.js";

// A scripted clock: t advances only when told.
function clock() {
  let t = 1000;
  const c = () => t;
  c.add = (ms) => { t += ms; };
  return c;
}
// One frame of `ms`: work(c) runs inside, the boundary at the end.
function on() {
  const c = clock(), p = createProf({ now: c });
  p.enable(true); p.frame();
  return { c, p };
}
const row = (t, path) => t.rows.find((r) => r.path === path);

test("off: nothing is measured, begin/end are no-ops", () => {
  const c = clock(), p = createProf({ now: c });
  p.begin("a"); c.add(5); p.end(); p.frame();
  assert.equal(p.on, false);
  assert.equal(p.table().frames, 0);
  assert.equal(p.cur(), null);
  assert.equal(p.scope("x", () => 7), 7);
});

test("nesting: totals per frame, self time, calls per frame, idle", () => {
  const { c, p } = on();
  for (let f = 0; f < 10; f++) {
    p.begin("frame");
    c.add(1);
    p.begin("render"); c.add(3);
    p.begin("shadow"); c.add(2); p.end();
    p.end();
    p.begin("update"); c.add(1); p.end();
    p.begin("update"); c.add(1); p.end(); // twice a frame
    p.end();
    p.begin("net"); c.add(0.5); p.end(); // outside the frame: a top-level sibling
    c.add(8.5); // idle
    p.frame();
  }
  const t = p.table();
  assert.equal(t.frames, 10);
  assert.equal(t.interval.mean, 17);
  assert.equal(t.busy.mean, 8.5);
  assert.equal(t.idle.mean, 8.5);
  assert.equal(row(t, "frame").mean, 8);
  assert.equal(row(t, "frame").self, 1);
  assert.equal(row(t, "frame/render").mean, 5);
  assert.equal(row(t, "frame/render").self, 3);
  assert.equal(row(t, "frame/render/shadow").depth, 2);
  assert.equal(row(t, "frame/update").calls, 2);
  assert.equal(row(t, "frame/update").mean, 2);
  assert.equal(row(t, "net").mean, 0.5);
  // depth-first, children largest first
  assert.deepEqual(t.rows.map((r) => r.path), ["frame", "frame/render", "frame/render/shadow", "frame/update", "net"]);
});

test("percentiles and max over the window", () => {
  const { c, p } = on();
  for (let f = 0; f < 100; f++) { p.begin("w"); c.add(f === 99 ? 50 : f < 95 ? 1 : 10); p.end(); p.frame(); }
  const w = row(p.table(), "w");
  assert.equal(w.max, 50);
  assert.equal(w.p95, 1);
  assert.equal(w.p99, 10);
  assert.ok(Math.abs(w.mean - (95 + 40 + 50) / 100) < 1e-9);
});

test("window roll-off: frames older than the window drop out", () => {
  const c = clock(), p = createProf({ now: c, windowMs: 100 });
  p.enable(true); p.frame();
  for (let f = 0; f < 10; f++) { p.begin("slow"); c.add(20); p.end(); p.frame(); } // 200 ms of slow frames
  for (let f = 0; f < 10; f++) { p.begin("fast"); c.add(2); p.end(); c.add(3); p.frame(); } // 50 ms of fast ones
  const t = p.table();
  // the window (100 ms back from the last boundary) holds the 10 fast frames and the last 3 slow ones (ending at 1160, 1180, 1200)
  assert.equal(t.frames, 13);
  assert.equal(row(t, "fast").calls, 10 / 13);
  c.add(1000);
  assert.equal(p.table().frames, 0);
});

test("enable/disable apply at the frame boundary; a section left open closes there", () => {
  const c = clock(), p = createProf({ now: c });
  p.begin("a");
  p.enable(true);
  assert.equal(p.on, false); // not mid-frame
  p.end();
  p.frame();
  assert.equal(p.on, true);
  p.begin("leak"); c.add(4); // never ended (an exception)
  p.frame();
  assert.equal(row(p.table(), "leak").mean, 4);
  p.begin("x"); c.add(1); p.end();
  assert.equal(p.cur(), null);
  p.enable(false); p.frame();
  assert.equal(p.on, false);
  p.reset();
  assert.equal(p.table().frames, 0);
});

test("scope returns and closes on throw", () => {
  const { c, p } = on();
  assert.equal(p.scope("s", () => { c.add(2); return 3; }), 3);
  assert.throws(() => p.scope("t", () => { c.add(1); throw new Error("x"); }));
  p.frame();
  const t = p.table();
  assert.equal(row(t, "s").mean, 2);
  assert.equal(row(t, "t").mean, 1);
});

test("GPU samples per section and per frame, in their own window", () => {
  const { c, p } = on();
  p.begin("gfx"); const g = p.cur(); p.begin("shadow"); const s = p.cur(); p.end(); p.end();
  p.frame();
  for (let i = 0; i < 4; i++) { p.gpuSample(s, 2 + i, c()); p.gpuSample(g, 5, c()); p.gpuFrameSample(5, c()); }
  const t = p.table();
  assert.equal(row(t, "gfx/shadow").gpu.mean, 3.5);
  assert.equal(row(t, "gfx/shadow").gpu.max, 5);
  assert.equal(t.gpu.mean, 5);
  c.add(5000);
  assert.equal(p.table().gpu, null);
});

test("stat: empty and nearest rank", () => {
  assert.deepEqual(stat([]), { n: 0, mean: 0, p95: 0, p99: 0, max: 0 });
  assert.deepEqual(stat([3, 1, 2]), { n: 3, mean: 2, p95: 3, p99: 3, max: 3 });
});

// A fake table for the verdict: frame interval, main-thread busy time, GPU, sections.
const fake = (frame, cpu, gpu, rows = []) => ({
  frames: 60, interval: stat([frame]), busy: stat([cpu]), idle: stat([Math.max(0, frame - cpu)]), gpu: gpu == null ? null : stat([gpu]), rows,
});

test("verdict: GPU-bound names the heaviest GPU pass", () => {
  const rows = [
    { path: "frame", name: "frame", depth: 0, self: 1, mean: 3, gpu: null },
    { path: "frame/render/gfx", name: "gfx", depth: 2, self: 0, mean: 2, gpu: stat([30]) },
    { path: "frame/render/gfx/world", name: "world", depth: 3, self: 1, mean: 1, gpu: stat([20]) },
    { path: "frame/render/gfx/shadow", name: "shadow", depth: 3, self: 1, mean: 1, gpu: stat([8]) },
  ];
  const v = verdict(fake(33, 3, 30, rows), { refreshMs: 16.7 });
  assert.equal(v.kind, "gpu");
  assert.equal(v.top, "world");
  assert.match(v.text, /GPU-bound/);
});

test("verdict: CPU-bound names the heaviest section by self time", () => {
  const rows = [
    { path: "frame", name: "frame", depth: 0, self: 2, mean: 25, gpu: null },
    { path: "frame/sim", name: "sim", depth: 1, self: 18, mean: 18, gpu: null },
    { path: "frame/render", name: "render", depth: 1, self: 5, mean: 5, gpu: null },
  ];
  const v = verdict(fake(28, 26, 4, rows), { refreshMs: 16.7 });
  assert.equal(v.kind, "cpu");
  assert.equal(v.top, "frame/sim");
});

test("verdict: a bench's wait for the GPU counts as GPU time, not CPU work", () => {
  const rows = [
    { path: "gpu-wait", name: "gpu-wait", depth: 0, self: 90, mean: 90, gpu: null },
    { path: "frame", name: "frame", depth: 0, self: 2, mean: 2, gpu: null },
  ];
  const v = verdict(fake(92, 92, null, rows), { refreshMs: 16.7, gpuTimer: false });
  assert.equal(v.kind, "gpu");
  assert.ok(!v.guess);
  // the timer under-reports (software GL): the wait is the better measure
  assert.equal(verdict(fake(92, 92, 20, rows), { refreshMs: 16.7 }).kind, "gpu");
});

test("verdict: GPU time counts per drawn frame (frames that drew nothing scale it down)", () => {
  const rows = (calls) => [
    { path: "frame", name: "frame", depth: 0, self: 1, mean: 3, calls: 1, gpu: null },
    { path: "frame/render/gfx", name: "gfx", depth: 2, self: 0, mean: 2, calls, gpu: stat([30]) },
    { path: "frame/render/gfx/world", name: "world", depth: 3, self: 1, mean: 1, calls, gpu: stat([25]) },
  ];
  assert.equal(verdict(fake(33, 3, 30, rows(1)), { refreshMs: 16.7 }).kind, "gpu");
  // drew every 4th frame: 7.5 ms of GPU a frame in a 33 ms frame is not GPU-bound
  assert.notEqual(verdict(fake(33, 3, 30, rows(0.25)), { refreshMs: 16.7 }).kind, "gpu");
});

test("verdict: vsync, the fps cap, throttling, background, no GPU timer", () => {
  assert.equal(verdict(fake(16.7, 4, 5), { refreshMs: 16.7 }).kind, "vsync");
  assert.equal(verdict(fake(6.94, 2, 2), { refreshMs: 6.94 }).kind, "vsync");
  assert.equal(verdict(fake(13.9, 4, 5), { refreshMs: 6.94, capMs: 13.9 }).kind, "cap");
  assert.equal(verdict(fake(100, 3, 2), { refreshMs: 16.7 }).kind, "throttled");
  assert.equal(verdict(fake(16.7, 4, 5), { hidden: true }).kind, "background");
  const g = verdict(fake(25, 5, null), { refreshMs: 16.7, gpuTimer: false });
  assert.equal(g.kind, "gpu");
  assert.equal(g.guess, true);
  assert.equal(verdict({ frames: 0 }, {}).kind, "unknown");
  // a GPU timer but no samples yet, frames slower than vsync for no clear reason
  assert.equal(verdict(fake(25, 12, null), { refreshMs: 16.7, gpuTimer: true }).kind, "unknown");
  assert.equal(verdict(fake(25, 12, 10), { refreshMs: 16.7 }).kind, "unknown");
});

test("formatProf: level 1 compact, level 2 the tree with a GPU column", () => {
  const { c, p } = on();
  for (let f = 0; f < 5; f++) { p.begin("frame"); p.begin("render"); c.add(2); p.end(); c.add(1); p.end(); c.add(13.7); p.frame(); }
  const t = p.table();
  const one = formatProf(t, 1, { refreshMs: 16.7, gpu: "TestGPU", res: "800×600 @1", hz: 60 });
  assert.match(one, /Holding the display's 60 Hz/);
  assert.match(one, /TestGPU · 800×600 @1 · 60 Hz/);
  assert.match(one, /gpu n\/a/);
  assert.match(one, /frame\/render/);
  const two = formatProf(t, 2, { refreshMs: 16.7 });
  assert.match(two, /cpu: {2}mean/);
  assert.match(two, /│ gpu:/);
  assert.match(two, /^ {2}render/m);
});
