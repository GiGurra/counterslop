import { test } from "node:test";
import assert from "node:assert/strict";
import { NADE_KINDS, nadeKind, typeOf, CLASS_LABEL } from "./types.js";
import { WEAPONS } from "./index.js";

test("every grenade has a kind tag and colour, and the three kinds differ (GitHub #3)", () => {
  const nades = Object.values(WEAPONS).filter((w) => w.action === "throw");
  assert.ok(nades.length >= 3);
  for (const w of nades) {
    const k = nadeKind(w);
    assert.ok(k, w.id);
    assert.equal(typeOf(w.id).tag, k.tag);
  }
  assert.equal(typeOf("mergeconflict").tag, "HE");
  assert.equal(typeOf("stacktrace").tag, "FLASH");
  assert.equal(typeOf("smoketest").tag, "SMOKE");
  const colors = Object.values(NADE_KINDS).map((k) => k.color);
  assert.equal(new Set(colors).size, colors.length);
  for (const c of colors) assert.match(c, /^#[0-9a-f]{6}$/);
});

test("guns get a class line, no tag; unknown ids are empty", () => {
  for (const w of Object.values(WEAPONS)) if (!nadeKind(w)) assert.ok(CLASS_LABEL[w.class], `${w.id}: class ${w.class} has a label`);
  assert.deepEqual(typeOf("codereview"), { tag: "", long: "Sniper rifle", color: "" });
  assert.equal(nadeKind("prompt"), null);
  assert.deepEqual(typeOf("nope"), { tag: "", long: "", color: "" });
});
