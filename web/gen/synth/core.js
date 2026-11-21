// WebAudio synthesis primitives. Sound recipes in assets/sounds build on these;
// nothing here knows about the game.
let ctx = null, out = null, noiseBuf = null, volume = 0.5;

// Must run from a user gesture (browsers start audio suspended).
export function initAudio() {
  if (ctx) { ctx.resume(); return; }
  ctx = new AudioContext();
  const comp = ctx.createDynamicsCompressor();
  out = ctx.createGain();
  out.gain.value = volume;
  out.connect(comp).connect(ctx.destination);
  noiseBuf = ctx.createBuffer(1, ctx.sampleRate * 0.5, ctx.sampleRate);
  const d = noiseBuf.getChannelData(0);
  for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
}

export function setVolume(v) {
  volume = v;
  if (out) out.gain.value = v;
}

export const live = () => ctx && ctx.state === "running";
// "none" until the first gesture creates the context, then its state.
export const audioState = () => (ctx ? ctx.state : "none");
// The context and master bus, for gen/synth/fx.js and buffer playback.
export const audioCtx = () => ctx;
export const master = () => out;
export const now = () => ctx.currentTime;
export const rnd = (a, b) => a + Math.random() * (b - a);

// Output bus: the master, or a panned/attenuated branch for positional sounds ({pan, gain}).
export function dest(sp) {
  if (!sp) return out;
  const g = ctx.createGain();
  g.gain.value = sp.gain;
  const p = ctx.createStereoPanner();
  p.pan.value = sp.pan;
  g.connect(p).connect(out);
  return g;
}

// Exponential attack/decay envelope around node; returns the envelope gain to connect onward.
export function env(node, t, peak, attack, decay) {
  const g = ctx.createGain();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(peak, t + attack);
  g.gain.exponentialRampToValueAtTime(0.0001, t + attack + decay);
  node.connect(g);
  return g;
}

// Oscillator sweeping f0 -> f1 (exponential) over dur.
export function osc(type, f0, f1, t, dur) {
  const o = ctx.createOscillator();
  o.type = type;
  o.frequency.setValueAtTime(f0, t);
  o.frequency.exponentialRampToValueAtTime(f1, t + dur);
  o.start(t); o.stop(t + dur + 0.05);
  return o;
}

// Filtered white noise burst; returns the filter node. The noise is one shared 0.5 s
// buffer: a burst longer than that needs loop (a continuous bed).
export function noise(t, dur, type, freq, q = 1, loop = false) {
  const s = ctx.createBufferSource();
  s.buffer = noiseBuf;
  s.loop = loop;
  const f = filter(s, type, freq, q);
  s.start(t, Math.random() * 0.2); s.stop(t + dur + 0.05);
  return f;
}

export function filter(node, type, freq, q = 1) {
  const f = ctx.createBiquadFilter();
  f.type = type; f.frequency.value = freq; f.Q.value = q;
  node.connect(f);
  return f;
}

// Frequency wobble on an oscillator (rate Hz, depth Hz).
export function vibrato(o, rate, depth, t, dur) {
  const lfo = ctx.createOscillator(), g = ctx.createGain();
  lfo.frequency.value = rate; g.gain.value = depth;
  lfo.connect(g).connect(o.frequency);
  lfo.start(t); lfo.stop(t + dur);
}
