// Playback effects on the WebAudio graph: generated-impulse reverb, buffer playback.
import { audioCtx, master } from "./core.js";
import { mulberry32 } from "../rng.js";

// Stereo hall impulse response: seeded noise with exponential decay, a few early
// reflections and a high-frequency roll-off that grows with time. Pure; no context needed.
export function reverbIR(sampleRate, { seconds = 2.5, decay = 3, seed = 1 } = {}) {
  const n = Math.floor(seconds * sampleRate), rand = mulberry32(seed);
  const chans = [new Float32Array(n), new Float32Array(n)];
  for (const ch of chans) {
    let lp = 0;
    for (let i = 0; i < n; i++) {
      const t = i / n, k = 0.15 + 0.8 * t; // darker as it decays
      lp += (1 - k) * ((rand() * 2 - 1) - lp);
      ch[i] = lp * Math.pow(1 - t, decay);
    }
    for (let e = 0; e < 6; e++) { // early reflections
      const at = Math.floor((0.01 + rand() * 0.07) * sampleRate);
      if (at < n) ch[at] += (rand() - 0.5) * 0.8;
    }
  }
  return chans;
}

const irs = new Map(), convolvers = new WeakMap();
// A ConvolverNode for these reverb settings feeding `dest`, built once per (settings, dest).
export function reverb(opts, dest) {
  const ctx = audioCtx(), key = JSON.stringify([opts.seconds, opts.decay, opts.seed]);
  if (!convolvers.has(dest)) convolvers.set(dest, new Map());
  const mine = convolvers.get(dest);
  if (!mine.has(key)) {
    if (!irs.has(key)) irs.set(key, reverbIR(ctx.sampleRate, opts));
    const ir = irs.get(key), buf = ctx.createBuffer(2, ir[0].length, ctx.sampleRate);
    buf.copyToChannel(ir[0], 0); buf.copyToChannel(ir[1], 1);
    const c = ctx.createConvolver();
    c.buffer = buf;
    c.connect(dest);
    mine.set(key, c);
  }
  return mine.get(key);
}

// Play mono samples into `dest` (default master) with an optional reverb send.
// Returns the source node (onended, stop).
export function playSamples(samples, sampleRate, { dest = master(), rev, wet = 0, gain = 1 } = {}) {
  const ctx = audioCtx();
  const buf = ctx.createBuffer(1, samples.length, sampleRate);
  buf.copyToChannel(samples, 0);
  const src = ctx.createBufferSource(), g = ctx.createGain();
  src.buffer = buf; g.gain.value = gain;
  src.connect(g).connect(dest);
  if (rev && wet > 0) {
    const s = ctx.createGain();
    s.gain.value = wet;
    g.connect(s).connect(reverb(rev, dest));
  }
  src.start();
  return src;
}
