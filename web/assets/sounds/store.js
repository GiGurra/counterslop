// GPU Drop: the hardware superstore. All synthesized; sp = spatial options (dest).
import { live, now, rnd, dest, env, osc, noise, filter, audioCtx, stopLoop } from "../../gen/synth/core.js";

// Grab: a cardboard box torn off the shelf: a scrape, a burst of fibrous rip
// grains (irregular, bright), and a hollow cardboard thump as it lands on the shoulder.
export function gpuGrab(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.12, "bandpass", 900, 1.5), t, 0.25, 0.01, 0.1).connect(o);
  let dt = 0.06;
  for (let i = 0; i < 14; i++) {
    const g = rnd(0.012, 0.035);
    env(noise(t + dt, g, "bandpass", rnd(1800, 4200), rnd(2, 5)), t + dt, rnd(0.15, 0.4), 0.002, g).connect(o);
    dt += g * rnd(0.5, 1.1);
  }
  const th = t + dt + 0.04;
  env(osc("triangle", 140, 70, th, 0.12), th, 0.5, 0.003, 0.11).connect(o);
  env(noise(th, 0.08, "lowpass", 500), th, 0.35, 0.002, 0.07).connect(o);
}

// Restock: the barcode scanner: a short laser chirp and the till's two-tone beep.
export function gpuRestock(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("sawtooth", 3200, 5200, t, 0.05), t, 0.03, 0.002, 0.045).connect(o);
  for (const [f, dt] of [[2730, 0.07], [2730, 0.19]]) {
    env(osc("square", f, f, t + dt, 0.08), t + dt, 0.09, 0.001, 0.07).connect(o);
    env(osc("sine", f * 2, f * 2, t + dt, 0.08), t + dt, 0.04, 0.001, 0.07).connect(o);
  }
}

// Secure: the cash register: key clacks, the drawer's mechanical clunk and slide,
// then the bell (inharmonic partials, long ring) that rings "cha-ching".
export function gpuSecure(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  for (const dt of [0, 0.07, 0.13]) env(noise(t + dt, 0.03, "bandpass", rnd(2500, 3200), 4), t + dt, 0.3, 0.001, 0.03).connect(o);
  const c = t + 0.22; // "cha": the drawer
  env(osc("triangle", 160, 60, c, 0.09), c, 0.6, 0.002, 0.08).connect(o);
  env(noise(c, 0.25, "bandpass", 1400, 0.8), c, 0.35, 0.005, 0.22).connect(o);
  const b = c + 0.16; // "ching": the bell
  for (const [f, g, d] of [[2093, 0.22, 1.4], [2093 * 2.76, 0.08, 0.6], [2093 * 5.4, 0.04, 0.3], [2637, 0.1, 1.1]]) {
    env(osc("sine", f, f * 0.998, b, d), b, g, 0.002, d).connect(o);
  }
}

// The store PA chime before an announcement: three soft bell tones, a rising
// major arpeggio (G5 B5 D6), each with a quieter octave on top and a long tail.
export function paChime(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  [784, 988, 1175].forEach((f, i) => {
    const s = t + i * 0.32;
    env(osc("sine", f, f, s, 1.2), s, 0.22, 0.008, 1.1).connect(o);
    env(osc("sine", f * 2, f * 2, s, 0.6), s, 0.05, 0.004, 0.5).connect(o);
    env(osc("triangle", f, f, s, 0.15), s, 0.04, 0.002, 0.12).connect(o);
  });
}
// The spaceport's walk-through security scanner (cq_moonshot: it beeps at
// everyone): three stern square-wave beeps, a little detuned, like a cheap
// metal detector that has found your opinions.
export function scannerBeep(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  for (let i = 0; i < 3; i++) {
    const s = t + i * 0.19;
    env(osc("square", 1320, 1310, s, 0.14), s, 0.09, 0.004, 0.12).connect(o);
    env(osc("square", 1327, 1320, s, 0.14), s, 0.05, 0.004, 0.12).connect(o);
  }
}
export const PA_CHIME_SECONDS = 0.9; // when the announcer may start talking after the chime

// Escalator hum: a continuous, positional sound. Mains hum (50/100 Hz) and a
// motor whine under a soft comb-plate clack once per tread. Returns a handle:
// update({pan, gain}) as the listener moves, stop() to fade it out.
export function escalatorHum(sp = { pan: 0, gain: 0.5 }) {
  if (!live()) return null;
  const ctx = audioCtx(), t = now();
  const g = ctx.createGain(), p = ctx.createStereoPanner();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(Math.max(0.0001, sp.gain), t + 0.4);
  p.pan.value = sp.pan;
  g.connect(p).connect(dest());
  const srcs = [];
  for (const [type, f, lv] of [["sine", 50, 0.25], ["sine", 100, 0.18], ["sawtooth", 300, 0.02]]) {
    const o = ctx.createOscillator(), og = ctx.createGain();
    o.type = type; o.frequency.value = f; og.gain.value = lv;
    o.connect(og).connect(g);
    o.start(t);
    srcs.push(o);
  }
  // Tread clacks: a short pulse train (an oscillator gating filtered noise).
  const tick = ctx.createOscillator(), tg = ctx.createGain(), base = ctx.createGain();
  tick.type = "square"; tick.frequency.value = 1.6; // treads per second at ~0.5 m/s, 0.3 m treads
  tg.gain.value = 0.03; base.gain.value = 0.03;
  const clack = filter(tick, "bandpass", 1200, 3);
  clack.connect(tg).connect(g);
  tick.start(t);
  srcs.push(tick);
  const hiss = noise(t, 3600, "bandpass", 600, 0.7, true); // belt rumble: the shared noise looped, an hour at most
  hiss.connect(base).connect(g);
  srcs.push(hiss.src);
  let last = { pan: sp.pan, gain: sp.gain };
  return {
    // Called every frame: only schedule a change when it is audible (each call adds an
    // AudioParam event, and they pile up).
    update(s) {
      if (Math.abs(s.gain - last.gain) < 0.01 && Math.abs(s.pan - last.pan) < 0.01) return;
      last = { pan: s.pan, gain: s.gain };
      const k = audioCtx().currentTime;
      g.gain.setTargetAtTime(Math.max(0.0001, s.gain), k, 0.1);
      p.pan.setTargetAtTime(s.pan, k, 0.1);
    },
    stop(fade = 0.4) { stopLoop(g.gain, srcs, fade); },
  };
}
