import test from "node:test";
import assert from "node:assert/strict";
import { mulberry32, rng } from "./rng.js";

test("mulberry32 is seeded and in [0, 1)", () => {
  const a = mulberry32(42), b = mulberry32(42);
  for (let i = 0; i < 100; i++) {
    const x = a();
    assert.equal(x, b());
    assert.ok(x >= 0 && x < 1);
  }
  assert.notEqual(mulberry32(1)(), mulberry32(2)());
});

test("rng helpers: range, int (inclusive), pick", () => {
  const r = rng(7);
  for (let i = 0; i < 200; i++) {
    const x = r.range(2, 3);
    assert.ok(x >= 2 && x < 3);
    const n = r.int(1, 3);
    assert.ok(Number.isInteger(n) && n >= 1 && n <= 3);
    assert.ok(["a", "b"].includes(r.pick(["a", "b"])));
  }
});
