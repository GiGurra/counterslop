import { test } from "node:test";
import assert from "node:assert/strict";
import { hashString, previewKey, staleKeys, openPreviewCache } from "./previewcache.js";

test("the key: build, map, its data's hash, size; any of them changing is a new key", () => {
  const h = hashString(JSON.stringify({ name: "de_slop", boxes: [[0, 0, 0]] }));
  assert.match(h, /^[0-9a-f]{8}$/);
  assert.notEqual(h, hashString(JSON.stringify({ name: "de_slop", boxes: [[0, 0, 1]] })));
  assert.equal(hashString(""), "811c9dc5");
  const k = previewKey("abc", "de_slop", h, 384, 216);
  assert.equal(k, `abc|de_slop|${h}|384x216`);
  for (const other of [previewKey("abd", "de_slop", h, 384, 216), previewKey("abc", "de_stack", h, 384, 216), previewKey("abc", "de_slop", "00000000", 384, 216), previewKey("abc", "de_slop", h, 192, 108)]) assert.notEqual(other, k);
});

test("eviction: other builds' entries; on a store, this build's older entry for the same map", () => {
  const keys = ["old|de_slop|1|384x216", "old|de_stack|2|384x216", "new|de_slop|1|384x216", "new|de_stack|2|384x216", "new|custom|3|384x216"];
  assert.deepEqual(staleKeys(keys, "new"), ["old|de_slop|1|384x216", "old|de_stack|2|384x216"]);
  assert.deepEqual(staleKeys(keys, "new", "new|custom|4|384x216"), ["old|de_slop|1|384x216", "old|de_stack|2|384x216", "new|custom|3|384x216"]);
  assert.deepEqual(staleKeys(keys, "new", "new|de_slop|1|384x216"), ["old|de_slop|1|384x216", "old|de_stack|2|384x216"]);
});

test("storage blocked (no IndexedDB, or it throws): misses, never errors out", async () => {
  for (const idb of [undefined, { open() { throw new Error("SecurityError"); } }]) {
    const c = openPreviewCache("b", idb ?? null);
    assert.equal(await c.get("b|x|1|1x1"), null);
    await c.put("b|x|1|1x1", new Blob(["x"]));
    await c.evict();
    assert.equal(c.stats.hits, 0);
    assert.equal(c.stats.misses, 1);
  }
});
