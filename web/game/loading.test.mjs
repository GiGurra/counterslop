// The map change's client side (loading.js): what we report and when, and
// what the loading screen says.
import test from "node:test";
import assert from "node:assert/strict";
import { createLoadReporter, loadView, ownProgress, RESEND_MS, PCT_EVERY_MS } from "./loading.js";

const load = (pl, step = "ready", secs = 0) => ({ id: 7, map: "cq_moonshot", step, secs, pl });

test("reports a step at once, resends until the snapshot shows it, then stops", () => {
  const r = createLoadReporter();
  const l = load([{ id: 1, step: "" }]);
  assert.deepEqual(r.next(l, { step: "geometry", pct: 0 }, 1, 0), { t: "loading", lid: 7, step: "geometry", pct: 0 });
  assert.equal(r.next(l, { step: "geometry", pct: 0 }, 1, 10), null, "not every frame");
  assert.ok(r.next(l, { step: "geometry", pct: 0 }, 1, RESEND_MS), "lost? resent after RESEND_MS");
  const shown = load([{ id: 1, step: "geometry" }]);
  assert.equal(r.next(shown, { step: "geometry", pct: 0 }, 1, 5 * RESEND_MS), null, "the server has it");
  assert.ok(r.next(shown, { step: "lights", pct: 0 }, 1, 5 * RESEND_MS + 1), "a new step goes at once");
});

test("percent updates are rate limited, a new load id goes at once", () => {
  const r = createLoadReporter();
  const l = load([{ id: 1, step: "" }]);
  r.next(l, { step: "lights", pct: 10 }, 1, 0);
  assert.equal(r.next(l, { step: "lights", pct: 11 }, 1, PCT_EVERY_MS - 1), null);
  assert.equal(r.next(l, { step: "lights", pct: 12 }, 1, PCT_EVERY_MS).pct, 12);
  assert.equal(r.next({ ...l, id: 8 }, { step: "lights", pct: 12 }, 1, PCT_EVERY_MS + 1).lid, 8);
});

test("no load or no id yet: nothing to report", () => {
  const r = createLoadReporter();
  assert.equal(r.next(null, { step: "ready" }, 1, 0), null);
  assert.equal(r.next(load([]), { step: "ready" }, -1, 0), null);
});

test("the screen: the server building, us loading, someone waiting", () => {
  const v = loadView(load([{ id: 1, step: "lights", pct: 40 }, { id: 2, step: "geometry" }, { id: 3, step: "ready" }], "nav", 6),
    { step: "lights", pct: 40 }, { 2: "bob", 3: "carol" }, 1, "cq_moonshot");
  assert.equal(v.map, "cq_moonshot");
  assert.deepEqual(v.server, { step: "nav", text: "building the bots' nav graph", secs: 6 });
  assert.equal(v.me.text, "building lights");
  assert.equal(v.me.pct, 40);
  assert.deepEqual(v.waiting.map((w) => [w.name, w.text]), [["bob", "building geometry"]]);
  assert.equal(v.status, "Loading with the server…");
  assert.equal(v.total, 3 + 1 - 1);
});

test("the screen: we're done, waiting for the server, then for others with a countdown", () => {
  const names = { 2: "bob" };
  assert.equal(loadView(load([{ id: 2, step: "" }], "nav"), null, names, 1, "x").status, "Waiting for the server");
  const v = loadView(load([{ id: 1, step: "ready" }, { id: 2, step: "shaders" }], "ready", 14), null, names, 1, "x");
  assert.equal(v.status, "Waiting for bob");
  assert.equal(v.startsIn, 14);
  assert.ok(v.me.done);
  assert.equal(loadView(load([{ id: 1, step: "ready" }]), null, names, 1, "x").status, "Starting…");
});

test("our own build without a server load (joining): only our line", () => {
  assert.equal(loadView(null, null, {}, 1, "de_slop"), null);
  const v = loadView(null, { step: "shaders", pct: 0 }, {}, 1, "de_slop");
  assert.equal(v.map, "de_slop");
  assert.equal(v.me.text, "compiling shaders");
  assert.equal(v.server, undefined);
});

test("own progress from the renderer", () => {
  assert.deepEqual(ownProgress({ have: false }), { step: "map", pct: 0 });
  assert.deepEqual(ownProgress({ have: true, due: true }), { step: "geometry", pct: 0 });
  assert.deepEqual(ownProgress({ have: true, gfxStep: { step: "lights", frac: 0.426 } }), { step: "lights", pct: 42 });
  assert.deepEqual(ownProgress({ have: true, gfxStep: { step: "shaders", frac: 0 } }), { step: "shaders", pct: 0 });
  assert.deepEqual(ownProgress({ have: true, gfxStep: null }), { step: "ready", pct: 100 });
});
