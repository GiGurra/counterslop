import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { PATTERNS, aaStepRef, aaBandRef, aaPulseRef } from "./patterns.js";

const near = (a, b, msg) => assert.ok(Math.abs(a - b) < 1e-9, `${msg}: ${a} != ${b}`);

// The shader helpers and their JS twins are the same maths (the GLSL is checked by eye
// in the anti-aliasing-serverless scenario; the formulas are pinned here).
test("aaStep/aaBand/aaPulse are in PATTERNS", () => {
  for (const f of ["float aaStep(float e, float x)", "float aaBand(float x, float lo, float hi)", "float aaLine(float x, float w)", "float aaPulse(float t, float a, float b)"])
    assert.ok(PATTERNS.includes(f), f);
});

test("aaStep: the pixel's coverage of x >= e; a hard step when the footprint is ~0", () => {
  near(aaStepRef(0.5, 0.5, 0.1), 0.5, "centred on the edge: half");
  near(aaStepRef(0.5, 0.55, 0.1), 1, "footprint just inside");
  near(aaStepRef(0.5, 0.45, 0.1), 0, "footprint just outside");
  near(aaStepRef(0.5, 0.525, 0.1), 0.75, "linear in between");
  near(aaStepRef(0.5, 0.5001, 0), 1, "no footprint: step()");
  near(aaStepRef(0.5, 0.4999, 0), 0, "no footprint: step()");
});

test("aaBand: a band thinner than the pixel fades to its share instead of breaking up", () => {
  near(aaBandRef(0.5, 0.4, 0.6, 0.01), 1, "inside");
  near(aaBandRef(0.9, 0.4, 0.6, 0.01), 0, "outside");
  near(aaBandRef(0.5, 0.45, 0.55, 1), 0.1, "0.1 wide band in a footprint of 1");
  near(aaBandRef(0.6, 0.4, 0.6, 0.1), 0.5, "on the upper edge: half");
});

test("aaPulse: the box filter of a repeating band, exact across fract's wrap", () => {
  // Brute force: the share of 10^4 samples over the footprint with a <= fract(t) <= b.
  const brute = (t, a, b, fw) => { let n = 0; const N = 10000;
    for (let i = 0; i < N; i++) { const u = t - fw / 2 + (i + 0.5) / N * fw, f = u - Math.floor(u); if (f >= a && f <= b) n++; }
    return n / N; };
  for (const [t, a, b, fw] of [[0.0, 0.6, 1, 0.1], [0.98, 0.6, 1, 0.1], [3.62, 0.6, 1, 0.08], [-1.3, 0.5, 1, 0.3], [7.01, 0, 0.03, 0.05], [2.2, 0.55, 1, 4.5]])
    assert.ok(Math.abs(aaPulseRef(t, a, b, fw) - brute(t, a, b, fw)) < 1e-3, `t=${t} a=${a} b=${b} fw=${fw}`);
  near(aaPulseRef(0.8, 0.6, 1, 1e-9), 1, "no footprint: inside");
  near(aaPulseRef(0.3, 0.6, 1, 1e-9), 0, "no footprint: outside");
  near(aaPulseRef(12.34, 0.5, 1, 50), 0.5, "far away: the band's average");
});

// MSAA smooths only triangle edges; a hard step() on a surface coordinate inside a face
// stays jagged (de_serverless's neon trim was). These materials draw their lines with the
// aa helpers only: step() is left for per-cell choices (step(hash(...), p)).
test("de_serverless's neon materials draw no unfiltered step() lines", () => {
  for (const m of ["lambda", "jumppad", "accelpad", "accelpadr", "chevron", "chevronr"]) {
    const src = readFileSync(new URL(`../../assets/materials/${m}.js`, import.meta.url), "utf8");
    const bad = [...src.matchAll(/(?<![a-zA-Z])step\(([^,]*)/g)].map((x) => x[0]).filter((x) => !/step\(hash/.test(x));
    assert.deepEqual(bad, [], `${m}: use aaStep/aaBand/aaLine/aaPulse (gen/glsl/patterns.js)`);
  }
});
