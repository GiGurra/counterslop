import { test } from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_BINDS, bindKey, clearAction, keyFor, bindsDiff, applyDiff } from "./binds.js";

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
