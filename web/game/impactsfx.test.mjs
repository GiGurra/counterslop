import test from "node:test";
import assert from "node:assert/strict";
import { surfaceOf, impactSound, hardFloor, createLimiter, SURFACES } from "./impactsfx.js";
import { readFileSync } from "node:fs";

test("every map material has a surface", () => {
  const src = readFileSync(new URL("../../internal/content/content.go", import.meta.url), "utf8");
  const list = src.match(/var Materials = \[\]string\{([^}]*)\}/)[1].split(",").map((s) => s.trim()).filter(Boolean);
  const names = Object.fromEntries([...src.matchAll(/^\s*(\w+)\s*=\s*"([a-z_]+)"/gm)].map((m) => [m[1], m[2]]));
  assert.ok(list.length > 20);
  for (const id of list) {
    const kind = names[id];
    assert.ok(kind, id);
    assert.ok(SURFACES.includes(surfaceOf(kind)), kind);
  }
  assert.equal(surfaceOf("metal"), "metal");
  assert.equal(surfaceOf("carton"), "cardboard");
  assert.equal(surfaceOf("glass"), "glass");
  assert.equal(surfaceOf("sand"), "dirt");
  assert.equal(surfaceOf("crate"), "wood");
  assert.equal(surfaceOf("brick"), "stone");
  assert.equal(surfaceOf("nope"), "stone");
});

test("carpet mutes casings and takes bullets like dirt", () => {
  assert.equal(hardFloor(surfaceOf("carpet")), false);
  assert.equal(hardFloor(surfaceOf("lancarpet")), false);
  assert.equal(hardFloor(surfaceOf("concrete")), true);
  assert.equal(hardFloor(surfaceOf("metal")), true);
  assert.equal(impactSound("soft"), "dirt");
});

test("limiter: a burst, then the rate, per key", () => {
  const l = createLimiter({ rate: 10, burst: 3 });
  const at = (t, n) => Array.from({ length: n }, () => l.allow("a", t)).filter(Boolean).length;
  assert.equal(at(0, 10), 3); // the burst
  assert.equal(at(0.05, 5), 0); // half a token
  assert.equal(at(0.1, 5), 1);
  assert.equal(at(1.1, 10), 3); // refilled, capped at burst
  assert.equal(l.allow("b", 1.1), true); // keys are independent
  // Full auto at 600 RPM for 2 s: about rate*2 + burst sounds, not 20.
  const m = createLimiter({ rate: 4, burst: 2 });
  let n = 0;
  for (let i = 0; i < 20; i++) if (m.allow("x", i * 0.1)) n++;
  assert.ok(n >= 8 && n <= 10, `${n}`);
});
