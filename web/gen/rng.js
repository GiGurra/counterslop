// Seeded PRNG (mulberry32). The algorithm is internal/content/rng (Go),
// translated into rng.gen.js by cmd/gojs, so anything generated from a seed at
// build time, on the server or in the browser matches bit for bit.
import { New, Rand_Float, Rand_Range, Rand_Int } from "./rng.gen.js";

// A generator function: each call returns the next value in [0, 1).
export function mulberry32(seed) {
  const r = New(seed >>> 0);
  return () => Rand_Float(r);
}

// Convenience helpers over a generator.
export function rng(seed) {
  const r = New(seed >>> 0);
  const next = () => Rand_Float(r);
  return {
    next,
    range: (a, b) => Rand_Range(r, a, b),
    int: (a, b) => Rand_Int(r, a, b),
    pick: (arr) => arr[Math.floor(next() * arr.length)],
  };
}
