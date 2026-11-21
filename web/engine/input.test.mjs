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
