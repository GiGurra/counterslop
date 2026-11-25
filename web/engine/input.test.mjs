import { test } from "node:test";
import assert from "node:assert/strict";

// A bare browser: records listeners (with their options) on window and document.
const listeners = [];
globalThis.addEventListener = (type, fn, opts) => listeners.push({ type, fn, opts });
globalThis.document = { addEventListener() {}, fullscreenElement: null, exitPointerLock() {}, pointerLockElement: null, documentElement: {} };
const { createInput } = await import("./input.js");

const wheel = (ctrlKey) => {
  let prevented = false;
  const e = { ctrlKey, deltaY: 100, preventDefault() { prevented = true; } };
  for (const l of listeners.filter((l) => l.type === "wheel")) l.fn(e);
  return prevented;
};

// Ctrl+wheel is the browser's page zoom: in game (crouch + wheel jump) or in
// fullscreen it must not zoom (operator report). preventDefault only works
// from a non-passive listener.
test("Ctrl+wheel doesn't zoom the page in game or in fullscreen", () => {
  const input = createInput({ addEventListener() {} });
  assert.ok(listeners.some((l) => l.type === "wheel" && l.opts?.passive === false), "a non-passive wheel listener");
  input.locked = true;
  assert.equal(wheel(true), true, "locked");
  input.locked = false;
  document.fullscreenElement = {};
  assert.equal(wheel(true), true, "fullscreen");
  document.fullscreenElement = null;
  assert.equal(wheel(true), false, "menus in a window: zoom stays (accessibility)");
  input.locked = true;
  assert.equal(wheel(false), false, "a plain wheel isn't cancelled");
});

// Raw input: lock with unadjustedMovement where the browser takes it, else
// lock plainly (never leave the mouse uncaptured) and report input.raw =
// false so Options can say why aim feels accelerated (Firefox < 152, Linux).
test("pointer lock: raw where supported, a plain lock otherwise", async () => {
  const run = async (impl) => {
    const calls = [];
    const canvas = { addEventListener() {}, requestPointerLock(o) { calls.push(o ?? null); return impl(o, calls.length); } };
    const input = createInput(canvas);
    const seen = [];
    input.onRaw = (r) => seen.push(r);
    input.lock();
    await new Promise((r) => setTimeout(r, 0));
    return { calls, raw: input.raw, seen };
  };
  const raw = { unadjustedMovement: true };
  assert.deepEqual(await run(() => Promise.resolve()), { calls: [raw], raw: true, seen: [true] }, "Chromium, Firefox 152+ on Windows/macOS");
  const notSup = Object.assign(new Error("no"), { name: "NotSupportedError" });
  assert.deepEqual(await run((o) => (o ? Promise.reject(notSup) : Promise.resolve())), { calls: [raw, null], raw: false, seen: [false] }, "rejected: lock plainly");
  assert.deepEqual(await run((o) => { if (o) throw notSup; }), { calls: [raw, null], raw: false, seen: [false] }, "thrown: lock plainly (was: no lock at all)");
  assert.deepEqual(await run(() => undefined), { calls: [raw], raw: false, seen: [false] }, "no promise: the option was ignored");
  const busy = Object.assign(new Error("busy"), { name: "SecurityError" });
  assert.deepEqual(await run(() => Promise.reject(busy)), { calls: [raw], raw: null, seen: [] }, "other failures: the next click retries");
});

// A click mid-flick (#8): takePress gives the mouse counts pending when the
// button went down (the view at the click is the frame's start plus these),
// once; the counts after it still reach takeMouse.
test("takePress: the mouse counts pending at a click, once", () => {
  const input = createInput({ addEventListener() {} });
  input.locked = true;
  const fire = (type, e) => { for (const l of listeners.filter((l) => l.type === type)) l.fn(e); };
  fire("mousemove", { movementX: 130, movementY: -4 });
  fire("mousedown", { button: 0 });
  fire("mouseup", { button: 0 });
  fire("mousemove", { movementX: 130, movementY: 0 });
  assert.deepEqual(input.takePress("Mouse0"), [130, -4]);
  assert.equal(input.takePress("Mouse0"), null, "once");
  assert.deepEqual(input.takeMouse(), [260, -4]);
  assert.equal(input.takePress("Mouse1"), null, "never pressed");
});
