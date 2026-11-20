// Every sound is synthesized on the fly. Silly on purpose.
let ctx = null, out = null, noiseBuf = null;

export function initAudio() {
  if (ctx) { ctx.resume(); return; }
  ctx = new AudioContext();
  const comp = ctx.createDynamicsCompressor();
  out = ctx.createGain();
  out.gain.value = 0.5;
  out.connect(comp).connect(ctx.destination);
  noiseBuf = ctx.createBuffer(1, ctx.sampleRate * 0.5, ctx.sampleRate);
  const d = noiseBuf.getChannelData(0);
  for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
}

// Optional stereo/distance for remote sounds: {pan, gain}
function dest(sp) {
  if (!sp) return out;
  const g = ctx.createGain();
  g.gain.value = sp.gain;
  const p = ctx.createStereoPanner();
  p.pan.value = sp.pan;
  g.connect(p).connect(out);
  return g;
}

function env(node, t, peak, attack, decay) {
  const g = ctx.createGain();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(peak, t + attack);
  g.gain.exponentialRampToValueAtTime(0.0001, t + attack + decay);
  node.connect(g);
  return g;
}

function osc(type, f0, f1, t, dur) {
  const o = ctx.createOscillator();
  o.type = type;
  o.frequency.setValueAtTime(f0, t);
  o.frequency.exponentialRampToValueAtTime(f1, t + dur);
  o.start(t); o.stop(t + dur + 0.05);
  return o;
}

function noise(t, dur, type, freq, q = 1) {
  const s = ctx.createBufferSource();
  s.buffer = noiseBuf;
  const f = ctx.createBiquadFilter();
  f.type = type; f.frequency.value = freq; f.Q.value = q;
  s.connect(f);
  s.start(t, Math.random() * 0.2); s.stop(t + dur + 0.05);
  return f;
}

const live = () => ctx && ctx.state === "running";
const rnd = (a, b) => a + Math.random() * (b - a);

// Squeaky rubber shoes.
export function footstep(sp) {
  if (!live()) return;
  const t = ctx.currentTime, o = dest(sp);
  env(noise(t, 0.08, "bandpass", rnd(350, 700), 1.2), t, 0.35, 0.005, 0.07).connect(o);
  const f = rnd(1100, 1600);
  env(osc("sine", f, f * rnd(1.15, 1.4), t, 0.06), t, 0.05, 0.005, 0.05).connect(o);
}

export function jump() {
  if (!live()) return;
  const t = ctx.currentTime;
  const o = osc("sine", 180, 620, t, 0.28);
  const lfo = ctx.createOscillator(), depth = ctx.createGain();
  lfo.frequency.value = 28; depth.gain.value = 40;
  lfo.connect(depth).connect(o.frequency);
  lfo.start(t); lfo.stop(t + 0.35);
  env(o, t, 0.3, 0.01, 0.27).connect(out);
}

export function land(speed) {
  if (!live()) return;
  const t = ctx.currentTime, k = Math.min(1, speed / 15);
  env(osc("sine", 140, 40, t, 0.18), t, 0.3 + 0.4 * k, 0.005, 0.18).connect(out);
  env(noise(t, 0.12, "lowpass", 500), t, 0.3 * k + 0.1, 0.005, 0.1).connect(out);
}

// Pew: a falling square chirp, slightly different every time.
export function pew(sp) {
  if (!live()) return;
  const t = ctx.currentTime, o = dest(sp);
  const f = rnd(1300, 1700);
  env(osc("square", f, rnd(160, 240), t, 0.14), t, 0.18, 0.003, 0.13).connect(o);
  env(osc("sawtooth", f * 0.5, 90, t, 0.1), t, 0.08, 0.003, 0.09).connect(o);
  env(noise(t, 0.05, "highpass", 2500), t, 0.15, 0.002, 0.04).connect(o);
}

export function click() {
  if (!live()) return;
  const t = ctx.currentTime;
  env(noise(t, 0.03, "bandpass", 3000, 4), t, 0.4, 0.001, 0.025).connect(out);
}

// Clack, clack, ding.
export function reload(dur) {
  if (!live()) return;
  const t = ctx.currentTime;
  for (const dt of [0.1, 0.55]) env(noise(t + dt, 0.04, "bandpass", 1800, 3), t + dt, 0.5, 0.002, 0.04).connect(out);
  env(osc("triangle", 300, 120, t + 0.6, 0.15), t + 0.6, 0.2, 0.005, 0.12).connect(out);
  env(osc("sine", 1760, 1760, t + dur - 0.15, 0.4), t + dur - 0.15, 0.12, 0.005, 0.35).connect(out);
}

// Clown honk when someone joins.
export function honk() {
  if (!live()) return;
  const t = ctx.currentTime;
  for (const [f, dt] of [[330, 0], [262, 0.16]]) {
    const o = osc("sawtooth", f, f * 0.97, t + dt, 0.14);
    const lp = ctx.createBiquadFilter();
    lp.type = "lowpass"; lp.frequency.value = 1400;
    o.connect(lp);
    env(lp, t + dt, 0.12, 0.01, 0.13).connect(out);
  }
}

export function bonk() {
  if (!live()) return;
  const t = ctx.currentTime;
  env(osc("triangle", 900, 300, t, 0.08), t, 0.12, 0.002, 0.08).connect(out);
}
