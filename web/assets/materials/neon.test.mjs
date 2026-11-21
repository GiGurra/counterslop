import { test } from "node:test";
import assert from "node:assert/strict";
import neon, { SEGMENTS, WORDS } from "./neon.js";

test("neon signs only spell words whose letters read as themselves (no seven-segment D)", () => {
  for (const w of WORDS) {
    assert.ok(w.length <= 4, `${w}: a sign has 4 cells`);
    for (const ch of w) assert.ok(SEGMENTS[ch] > 0, `${w}: no readable seven-segment shape for ${ch}`);
  }
  assert.equal(SEGMENTS.D, undefined, "a seven-segment d reads as a backwards E");
});

test("neonGlyph in the shader spells WORDS", () => {
  const src = neon.glsl;
  WORDS.forEach((w, k) => {
    for (const [i, ch] of [...w].entries()) assert.ok(src.includes(`i == ${i} ? ${SEGMENTS[ch]} :`), `${w}[${i}]`);
    assert.ok(src.includes(`if (w == ${k}) return ${w.length};`), `${w} length`);
  });
});
