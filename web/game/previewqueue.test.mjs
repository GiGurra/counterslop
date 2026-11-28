import { test } from "node:test";
import assert from "node:assert/strict";
import { afterPaint, createPreviewQueue } from "./previewqueue.js";

// A queue whose work resolves when the test says so; tick() lets it move on.
function rig() {
  const jobs = [], got = [];
  const q = createPreviewQueue((n) => new Promise((res) => jobs.push({ n, res })), (n, r) => got.push([n, r]));
  const tick = () => new Promise((r) => setTimeout(r, 0));
  return { q, jobs, got, tick };
}

test("one at a time, lowest priority first, each handed over before the next starts", async () => {
  const { q, jobs, got, tick } = rig();
  q.want("c", 2); q.want("a", 0); q.want("b", 1); // one batch, any order
  await tick();
  assert.deepEqual(jobs.map((j) => j.n), ["a"]);
  assert.equal(q.busy(), "a");
  assert.deepEqual(q.pending(), ["b", "c"]);
  jobs[0].res("A");
  await tick();
  assert.deepEqual(got, [["a", "A"]]);
  assert.deepEqual(jobs.map((j) => j.n), ["a", "b"]); // b only after a was handed over
  jobs[1].res("B"); await tick();
  jobs[2].res("C"); await tick();
  assert.deepEqual(got.map((g) => g[0]), ["a", "b", "c"]);
  assert.ok(q.idle());
  assert.deepEqual(q.log.map((e) => e.join(" ")), ["start a", "done a", "start b", "done b", "start c", "done c"]);
});

test("drop: scrolled away before its turn, never drawn; the one in progress finishes", async () => {
  const { q, jobs, got, tick } = rig();
  q.want("a", 0); q.want("b", 1); q.want("c", 2);
  await tick();
  q.drop("a"); q.drop("b"); // a is being drawn, b is waiting
  jobs[0].res("A"); await tick();
  jobs[1].res("C"); await tick();
  assert.deepEqual(jobs.map((j) => j.n), ["a", "c"]);
  assert.deepEqual(got.map((g) => g[0]), ["a", "c"]);
});

test("a vote option jumps the queue; a done one is not drawn again until forgotten", async () => {
  const { q, jobs, got, tick } = rig();
  q.want("x", 5); q.want("y", 6);
  await tick();
  q.want("v", -3);
  jobs[0].res("X"); await tick();
  assert.equal(jobs[1].n, "v");
  jobs[1].res("V"); await tick();
  jobs[2].res("Y"); await tick();
  q.want("x", 0);
  await tick();
  assert.equal(jobs.length, 3);
  q.forget("x"); q.want("x", 0);
  await tick();
  assert.equal(jobs[3].n, "x");
  assert.deepEqual(got.map((g) => g[0]), ["x", "v", "y"]);
});

test("a failed preview is not handed over and can be asked for again", async () => {
  const { q, jobs, got, tick } = rig();
  q.want("a", 0);
  await tick();
  jobs[0].res(null); await tick();
  assert.deepEqual(got, []);
  assert.deepEqual(q.log.at(-1), ["fail", "a"]);
  q.want("a", 0); await tick();
  assert.equal(jobs.length, 2);
});

test("afterPaint waits for a frame (a paint), not just a timer: one card changes at a time", async () => {
  const frames = [];
  let done = false;
  afterPaint((fn) => frames.push(fn)).then(() => { done = true; });
  await new Promise((r) => setTimeout(r, 5));
  assert.equal(done, false, "a timer alone must not let the next preview start (the page hasn't painted)");
  frames.shift()();
  await new Promise((r) => setTimeout(r, 5));
  assert.equal(done, true, "after the frame it goes on");
});
