import { test } from "node:test";
import assert from "node:assert/strict";
import { firefoxVersion, rawInputHint, hasKeyboardLock } from "./browser.js";

const FF = (v, os = "Windows NT 10.0; Win64; x64") => `Mozilla/5.0 (${os}; rv:${v}.0) Gecko/20100101 Firefox/${v}.0`;
const CHROME = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36";

test("firefoxVersion", () => {
  assert.equal(firefoxVersion(FF(151)), 151);
  assert.equal(firefoxVersion(CHROME), 0);
});

test("rawInputHint names the fix for each browser", () => {
  assert.match(rawInputHint(FF(140)), /Firefox 140 has no raw mouse input .*update Firefox.*Enhance pointer precision/);
  assert.match(rawInputHint(FF(153, "X11; Linux x86_64")), /Firefox on Linux.*flat/);
  assert.match(rawInputHint(FF(153)), /This browser gave no raw mouse input/);
  assert.match(rawInputHint(CHROME), /Enhance pointer precision/);
});

test("hasKeyboardLock: Chromium's navigator.keyboard.lock only", () => {
  assert.equal(hasKeyboardLock({ keyboard: { lock() {} } }), true);
  assert.equal(hasKeyboardLock({}), false);
  assert.equal(hasKeyboardLock(undefined), false);
});
