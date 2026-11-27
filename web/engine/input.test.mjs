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

// After an auto-respawn (tickets mode) the game asks for the lock with no
// gesture and the browser may refuse; the first key press (user activation,
// as a click is) asks again when the game wants aim (aimOnKey), not Esc, not
// while captured, and not when nothing wants aim (a menu up).
test("aimOnKey: a key press asks for the lock again", () => {
  let asks = 0;
  const canvas = { addEventListener() {}, requestPointerLock() { asks++; return Promise.resolve(); } };
  const input = createInput(canvas);
  const key = (code) => { for (const l of listeners.filter((l) => l.type === "keydown")) l.fn({ code, target: {}, preventDefault() {} }); };
  let wants = true;
  input.aimOnKey = () => wants;
  key("KeyW");
  assert.equal(asks, 1, "uncaptured in play: W aims");
  key("Escape");
  assert.equal(asks, 1, "Esc is not activation (and means the menu)");
  input.locked = true;
  key("KeyA");
  assert.equal(asks, 1, "already captured");
  input.locked = false;
  wants = false;
  key("KeyD");
  assert.equal(asks, 1, "a menu wants the cursor");
});

// GitHub #18: right-click > Copy on the console log. The browser menu opens in
// text fields and on elements marked data-native-menu (the log), nowhere else,
// and never while the mouse is captured (right-click is a game button then).
test("contextmenu: only on text fields and data-native-menu, never in game", () => {
  const input = createInput({ addEventListener() {} });
  const menu = (sel) => {
    let prevented = false;
    const e = { target: { closest: (q) => (q.split(",").map((s) => s.trim()).includes(sel) ? {} : null) }, preventDefault() { prevented = true; } };
    listeners.filter((l) => l.type === "contextmenu").at(-1).fn(e); // this input's (earlier tests made others)
    return !prevented;
  };
  input.locked = false;
  assert.equal(menu("input"), true, "text field");
  assert.equal(menu("[data-native-menu]"), true, "the console log");
  assert.equal(menu("canvas"), false, "the game");
  input.locked = true;
  assert.equal(menu("[data-native-menu]"), false, "mouse captured");
  assert.equal(menu("input"), false, "mouse captured");
});
