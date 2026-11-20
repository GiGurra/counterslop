// Sample-level voice effects. Pure functions on Float32Arrays, so a line renders the
// same everywhere; reverb is applied at playback by gen/synth/fx.js (ConvolverNode).

// Stutter / repeat: "mo-mo-mo-MONSTER". The span [s0, s1) plays `count` extra times
// before the full take continues from s0. Each repeat is faded out at its end and
// grows louder toward the real word (rise: gain of the first repeat, 0..1).
export function stutter(x, sr, { s0, s1, count }, { rise = 0.6, gap = 0.025, cut = 0.85 } = {}) {
  const len = Math.max(1, Math.floor((s1 - s0) * cut)), g = Math.floor(gap * sr);
  const fade = Math.min(len, Math.floor(0.012 * sr));
  const out = new Float32Array(x.length + count * (len + g));
  out.set(x.subarray(0, s0), 0);
  let w = s0;
  for (let r = 0; r < count; r++) {
    const gain = rise + (1 - rise) * (r / Math.max(1, count));
    for (let i = 0; i < len; i++) {
      const f = i > len - fade ? (len - i) / fade : 1;
      out[w + i] = x[s0 + i] * gain * f;
    }
    w += len + g;
  }
  out.set(x.subarray(s0), w);
  return out;
}

// Mix renders on top of each other (layers may differ in length).
export function mix(parts) {
  const n = Math.max(...parts.map((p) => p.x.length)), out = new Float32Array(n);
  for (const { x, gain } of parts) for (let i = 0; i < x.length; i++) out[i] += x[i] * gain;
  return out;
}

// Soft-clip drive: 0 clean .. 1 crunchy. Keeps the peak level.
export function drive(x, amount) {
  if (amount <= 0) return x;
  const k = 1 + amount * 8, norm = 1 / Math.tanh(k);
  for (let i = 0; i < x.length; i++) x[i] = Math.tanh(x[i] * k) * norm;
  return x;
}

// Bitcrush: quantise to `bits` and hold every `hold`-th sample (sample-rate reduction).
export function bitcrush(x, bits = 16, hold = 1) {
  if (bits >= 16 && hold <= 1) return x;
  const q = Math.pow(2, bits - 1);
  let h = 0;
  for (let i = 0; i < x.length; i++) {
    if (i % hold === 0) h = Math.round(x[i] * q) / q;
    x[i] = h;
  }
  return x;
}
