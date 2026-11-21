import { test } from "node:test";
import assert from "node:assert/strict";
import { aaPlan, aaChoices, clampSamples, scaledSize, AA_MODES, AA_LABELS } from "./aa.js";
import { CVARS, coerce } from "../ui/settings.js";

const GPU = { builtin: false, samples: [8, 4, 2] };

test("clampSamples picks the largest supported count not above the request", () => {
  assert.equal(clampSamples(8, [8, 4, 2]), 8);
  assert.equal(clampSamples(8, [4]), 4); // SwiftShader-style cap
  assert.equal(clampSamples(2, [4]), 0); // nothing small enough
  assert.equal(clampSamples(4, []), 0);
  assert.equal(clampSamples(4, [16, 8, 4, 2, 1]), 4);
});

test("auto on a multisampled canvas at full size is the old direct path", () => {
  assert.deepEqual(aaPlan("auto", { builtin: true, samples: [4] }, 1),
    { mode: "auto", direct: true, samples: 0, fxaa: false, scale: 1, label: "auto (browser MSAA)" });
});

test("auto without the browser's MSAA (or below full scale) uses our MSAA 4x", () => {
  const p = aaPlan("auto", GPU, 1);
  assert.equal(p.direct, false);
  assert.equal(p.samples, 4);
  const q = aaPlan("auto", { builtin: true, samples: [4] }, 0.75);
  assert.equal(q.direct, false);
  assert.equal(q.samples, 4);
  assert.equal(q.scale, 0.75);
});

test("off is direct on a plain canvas, offscreen (single-sample) on a multisampled one", () => {
  assert.equal(aaPlan("off", GPU).direct, true);
  const p = aaPlan("off", { builtin: true, samples: [4] });
  assert.equal(p.direct, false);
  assert.equal(p.samples, 0);
  assert.equal(p.fxaa, false);
});

test("msaa modes are clamped to what the GPU supports and say so", () => {
  assert.equal(aaPlan("msaa8", GPU).samples, 8);
  assert.equal(aaPlan("msaa8", GPU).label, "msaa8");
  const p = aaPlan("msaa8", { samples: [4] });
  assert.equal(p.samples, 4);
  assert.match(p.label, /GPU max 4x/);
  const none = aaPlan("msaa2", { samples: [] });
  assert.equal(none.samples, 0);
  assert.equal(none.direct, true);
  assert.match(none.label, /GPU max none/);
});

test("fxaa and msaa4_fxaa always take the post pass", () => {
  assert.deepEqual([aaPlan("fxaa", GPU).fxaa, aaPlan("fxaa", GPU).samples, aaPlan("fxaa", GPU).direct], [true, 0, false]);
  const p = aaPlan("msaa4_fxaa", GPU);
  assert.deepEqual([p.fxaa, p.samples, p.label], [true, 4, "msaa4+fxaa"]);
});

test("an unknown mode falls back to auto", () => {
  assert.equal(aaPlan("ssaa16", { builtin: true }).mode, "auto");
});

test("the menu offers only sample counts the GPU can do, plus the current value", () => {
  assert.deepEqual(aaChoices([8, 4, 2]), AA_MODES);
  assert.deepEqual(aaChoices([4]), ["auto", "off", "msaa2", "msaa4", "fxaa", "msaa4_fxaa"]);
  assert.deepEqual(aaChoices([]), ["auto", "off", "fxaa"]);
  assert.ok(aaChoices([4], "msaa8").includes("msaa8"));
  for (const m of AA_MODES) assert.ok(AA_LABELS[m], m);
});

test("render scale: percent or fraction, clamped, never zero", () => {
  assert.deepEqual(scaledSize(1920, 1080, 100), [1920, 1080]);
  assert.deepEqual(scaledSize(1920, 1080, 50), [960, 540]);
  assert.deepEqual(scaledSize(1920, 1080, 0.75), [1440, 810]);
  assert.deepEqual(scaledSize(1, 1, 50), [1, 1]);
  assert.deepEqual(aaPlan("off", GPU, 0.5).direct, false);
  assert.equal(aaPlan("off", GPU, 0.5).scale, 0.5);
});

test("r_aa and r_render_scale cvars: defaults and parsing", () => {
  assert.equal(CVARS.r_aa.def, "auto");
  assert.deepEqual(CVARS.r_aa.enum, AA_MODES);
  assert.equal(coerce("r_aa", "fxaa"), "fxaa");
  assert.equal(coerce("r_aa", "msaa16"), undefined);
  assert.equal(CVARS.r_render_scale.def, 100);
  assert.equal(coerce("r_render_scale", 10), 50);
  assert.equal(coerce("r_render_scale", "200"), 100);
  assert.equal(coerce("r_render_scale", 72.4), 72);
});
