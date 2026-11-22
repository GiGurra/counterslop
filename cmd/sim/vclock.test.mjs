// vclock.js on a stand-in window: timers fire in order at their virtual
// times, animation frames at 1/60 s boundaries, nothing moves without advance.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

function page() {
  const w = { performance: { now: () => 12345 }, setTimeout, clearTimeout, setInterval, clearInterval, requestAnimationFrame: () => 0, cancelAnimationFrame: () => {} };
  const ctx = vm.createContext({ globalThis: w, Date: class extends Date {}, Promise, Math, String, Map });
  w.globalThis = w;
  vm.runInContext(readFileSync(new URL("./vclock.js", import.meta.url), "utf8"), ctx);
  return { w, Date: ctx.Date };
}

test("timers fire in order at their virtual times, only when advanced", async () => {
  const { w } = page();
  const log = [];
  const t0 = w.performance.now();
  w.setTimeout(() => log.push(["b", w.performance.now() - t0]), 50);
  w.setTimeout(() => log.push(["a", w.performance.now() - t0]), 20);
  const iv = w.setInterval(() => log.push(["i", w.performance.now() - t0]), 30);
  await new Promise((r) => setTimeout(r, 20)); // real time passes: nothing fires
  assert.deepEqual(log, []);
  await w.__vclock.advance(60);
  w.clearInterval(iv);
  await w.__vclock.advance(100);
  assert.deepEqual(log, [["a", 20], ["i", 30], ["b", 50], ["i", 60]]);
  assert.equal(w.performance.now() - t0, 160);
});

test("animation frames run at 1/60 s boundaries; settle runs what is due without moving the clock", async () => {
  const { w } = page();
  const frames = [];
  const raf = (t) => { frames.push(Math.round(t - w.performance.now() + w.__vclock.now())); w.requestAnimationFrame(raf); };
  w.requestAnimationFrame(raf);
  await w.__vclock.advance(50);
  assert.deepEqual(frames, [17, 33, 50]);
  let ran = false;
  w.setTimeout(() => { ran = true; }, 0);
  const before = w.__vclock.now();
  await w.__vclock.settle();
  assert.ok(ran);
  assert.equal(w.__vclock.now(), before);
  assert.equal(typeof w.__real.setTimeout, "function");
});
