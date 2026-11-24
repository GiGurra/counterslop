import test from "node:test";
import assert from "node:assert/strict";
import { createLink, linkView, sameView, BLIP_MS, LOST_MS } from "./link.js";

const up = { wanted: true, open: true, attempt: 0, retryAt: 0 };

test("snapshots flowing: ok; a late packet or two is not a blip", () => {
  const l = createLink();
  for (let t = 0; t <= 3000; t += 30) { l.snap(t); assert.equal(l.status(t + 29, up).mode, "ok"); }
  assert.equal(l.status(3000 + BLIP_MS - 1, up).mode, "ok", "just under the threshold");
});

test("thresholds: blip from BLIP_MS, lost (stall) from LOST_MS, back to ok on the next snapshot", () => {
  const l = createLink();
  l.snap(1000);
  assert.deepEqual(l.status(1000 + BLIP_MS, up), { mode: "blip", age: BLIP_MS });
  assert.equal(l.status(1000 + LOST_MS - 1, up).mode, "blip");
  assert.deepEqual(l.status(1000 + LOST_MS, up), { mode: "lost", why: "stall", age: LOST_MS });
  l.snap(5000);
  assert.equal(l.status(5000, up).mode, "ok");
});

test("a socket that opens but never sends counts from when it opened", () => {
  const l = createLink();
  assert.equal(l.status(0, { ...up, open: false }).mode, "ok", "first connect under way: nothing to say yet");
  assert.equal(l.status(100, up).mode, "ok", "just opened");
  assert.equal(l.status(100 + BLIP_MS, up).mode, "blip");
  assert.equal(l.status(100 + LOST_MS, up).mode, "lost");
});

test("socket closed: lost at once, with the reconnect attempt and countdown", () => {
  const l = createLink();
  l.snap(0);
  const st = l.status(10, { wanted: true, open: false, attempt: 3, retryAt: 5000 }, 1200);
  assert.deepEqual(st, { mode: "lost", why: "closed", attempt: 3, nextIn: 4 });
  assert.equal(l.status(10, { wanted: true, open: false, attempt: 3, retryAt: 0 }, 1200).nextIn, 0, "a try under way");
  // reconnected: the clock restarts at the open, not at the old snapshot
  assert.equal(l.status(60000, up).mode, "ok");
});

test("deliberate disconnect: never an overlay, however long", () => {
  const l = createLink();
  l.snap(0);
  const off = { wanted: false, open: false, attempt: 0, retryAt: 0 };
  assert.equal(l.status(60000, off).mode, "ok");
  // and connecting again afterwards starts clean
  assert.equal(l.status(60001, up).mode, "ok");
});

test("linkView: what the HUD draws, and when it changes", () => {
  assert.equal(linkView({ mode: "ok" }), null);
  const b = linkView({ mode: "blip", age: 1449 });
  assert.deepEqual(b, { mode: "blip", why: "", secs: 1.4 });
  assert.ok(sameView(b, linkView({ mode: "blip", age: 1401 })), "same tenth: no re-render");
  assert.ok(!sameView(b, linkView({ mode: "blip", age: 1500 })));
  assert.deepEqual(linkView({ mode: "lost", why: "closed", attempt: 2, nextIn: 1 }), { mode: "lost", why: "closed", attempt: 2, nextIn: 1 });
  assert.ok(sameView(null, null) && !sameView(null, b));
});
