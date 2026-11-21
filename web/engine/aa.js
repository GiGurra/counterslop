// Anti-aliasing and render scale: which render path a mode takes on this GPU. Pure, runs in Node.
//
// Modes (r_aa): auto = the browser's own MSAA on the canvas (the context's antialias flag),
// off, msaa2/4/8 (our multisampled offscreen target, resolved by a blit), fxaa (a post pass,
// gen/glsl/fxaa.js), msaa4_fxaa (both). A sample count the GPU can't do falls back to the
// largest it can (SwiftShader and old GPUs cap it); none at all means no MSAA.
export const AA_MODES = ["auto", "off", "msaa2", "msaa4", "msaa8", "fxaa", "msaa4_fxaa"];
export const AA_LABELS = {
  auto: "Auto (browser MSAA)", off: "Off", msaa2: "MSAA 2x", msaa4: "MSAA 4x", msaa8: "MSAA 8x",
  fxaa: "FXAA", msaa4_fxaa: "MSAA 4x + FXAA",
};

const want = (mode) => ({
  auto: 4, off: 0, msaa2: 2, msaa4: 4, msaa8: 8, fxaa: 0, msaa4_fxaa: 4,
}[mode] ?? 0);

// Largest supported sample count <= n (supported: what the GPU lists for RGBA8 renderbuffers).
export function clampSamples(n, supported = []) {
  let best = 0;
  for (const s of supported) if (s <= n && s > best) best = s;
  return best;
}

// The render path for a mode.
// gpu: { builtin: the canvas itself is multisampled (context antialias granted), samples: [..] }
// scale: render scale 0.5..1 (offscreen target smaller than the canvas, upscaled).
// Returns { mode, direct (draw straight into the canvas), samples (offscreen MSAA, 0 = none),
//           fxaa, scale, label }.
export function aaPlan(mode, gpu = {}, scale = 1) {
  if (!AA_MODES.includes(mode)) mode = "auto";
  const builtin = !!gpu.builtin, full = scale >= 1;
  const fxaa = mode === "fxaa" || mode === "msaa4_fxaa";
  // auto on a multisampled canvas at full size: exactly the old path.
  if (mode === "auto" && builtin && full) return { mode, direct: true, samples: 0, fxaa: false, scale: 1, label: "auto (browser MSAA)" };
  const samples = clampSamples(want(mode), gpu.samples);
  // off on a plain canvas at full size: nothing to do either.
  const direct = !fxaa && !samples && full && !builtin;
  let label = mode === "auto" ? (samples ? `auto (MSAA ${samples}x)` : "auto (none)") : mode.replace("_", "+");
  if (want(mode) && mode !== "auto" && samples !== want(mode)) label += ` (GPU max ${samples ? `${samples}x` : "none"})`;
  return { mode, direct, samples, fxaa, scale: full ? 1 : scale, label };
}

// Modes the options menu offers on this GPU (MSAA counts it can't do are left out; the
// current value always stays so the menu shows what is set).
export function aaChoices(supported = [], current = "auto") {
  const max = clampSamples(64, supported);
  return AA_MODES.filter((m) => m === current || !want(m) || m === "auto" || want(m) <= max);
}

// Offscreen target size for a canvas and a render scale (percent 50..100 or fraction).
export function scaledSize(w, h, scale) {
  const s = Math.min(1, Math.max(0.25, scale > 1 ? scale / 100 : scale));
  return [Math.max(1, Math.round(w * s)), Math.max(1, Math.round(h * s))];
}
