import { test } from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_BINDS, bindKey, clearAction, keyFor, bindsDiff, applyDiff, keyName, migrateMouseCodes } from "./binds.js";

test("an action keeps one key; a taken key moves and reports who lost it", () => {
  let { binds, moved } = bindKey(DEFAULT_BINDS, "KeyR", "+jump");
  assert.equal(binds.KeyR, "+jump");
  assert.equal(binds.Space, undefined);
  assert.equal(moved, "reload");
  assert.equal(keyFor(binds, "reload"), "");
  ({ binds, moved } = bindKey(binds, "KeyF", "slop")); // custom command: no single-key rule
  assert.equal(binds.KeyF, "slop");
  assert.equal(moved, null);
});

test("clear and the stored diff round-trip", () => {
  const b = clearAction(bindKey(DEFAULT_BINDS, "KeyC", "+duck").binds, "drop");
  const diff = bindsDiff(b);
  assert.deepEqual(diff, { ControlLeft: null, KeyC: "+duck", KeyG: null });
  assert.deepEqual(applyDiff(diff), b);
  assert.deepEqual(bindsDiff(DEFAULT_BINDS), {});
});

test("mouse buttons are the raw DOM event.button: fire = primary (0), zoom = secondary (2)", () => {
  assert.equal(DEFAULT_BINDS.Mouse0, "+attack");
  assert.equal(DEFAULT_BINDS.Mouse2, "+zoom");
  assert.equal(DEFAULT_BINDS.Mouse1, undefined); // middle button: unbound by default
  assert.equal(keyName("Mouse2"), "Mouse 2");
  assert.equal(keyName("Mouse0"), "Mouse 0");
});

test("v3 mouse codes (MOUSE<button+1>) migrate to the same physical button", () => {
  assert.deepEqual(migrateMouseCodes({ MOUSE3: "+zoom", MOUSE4: "reload", KeyR: null }), { Mouse2: "+zoom", Mouse3: "reload", KeyR: null });
});
