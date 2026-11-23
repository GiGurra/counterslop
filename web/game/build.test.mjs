import { test } from "node:test";
import assert from "node:assert/strict";
import { buildAction, nextAttempt, bustURL, RELOAD_GUARD_MS } from "./build.js";

test("same or unknown build: carry on", () => {
  assert.equal(buildAction("a", "a", null, 0), "ok");
  assert.equal(buildAction("", "b", null, 0), "ok");
  assert.equal(buildAction("a", undefined, null, 0), "ok");
});

test("a stale page never keeps playing: reload, then bust the cache, then stop", () => {
  let last = null, t = 1000;
  const seen = [];
  for (let i = 0; i < 4; i++) {
    const a = buildAction("old", "new", last, t);
    seen.push(a);
    if (a === "reload" || a === "bust") last = nextAttempt(a, "new", last, t);
    t += 500; // the reload came back on the same old page
  }
  assert.deepEqual(seen, ["reload", "bust", "stale", "stale"]);
});

test("an old attempt or another build starts over with a plain reload", () => {
  const last = { build: "new", at: 0, tries: 2 };
  assert.equal(buildAction("old", "new", last, RELOAD_GUARD_MS + 1), "reload");
  assert.equal(buildAction("old", "newer", last, 10), "reload");
  // records from before tries existed count as one reload
  assert.equal(buildAction("old", "new", { build: "new", at: 0 }, 10), "bust");
});

test("bust URL keeps the query and adds the build", () => {
  assert.equal(bustURL("http://h/?test=1", "abc"), "http://h/?test=1&build=abc");
  assert.equal(bustURL("http://h/?build=x", "y"), "http://h/?build=y");
});
