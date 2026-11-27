// Music voices on the synth primitives: each schedules one note into `out` at
// AudioContext time t (freq Hz, dur s, vel 0..1). Game-agnostic; themes pick them by name.
import { audioCtx, env, osc, noise, filter, vibrato } from "../synth/core.js";
import { KICK, RIM, SNARE, HAT, OPEN_HAT } from "./compose.js";

// A soft-clipping curve (tanh, driven hard), made once.
let curve = null;
function driveCurve() {
  if (!curve) { curve = new Float32Array(1024); for (let i = 0; i < 1024; i++) { const x = (i / 1023) * 2 - 1; curve[i] = Math.tanh(x * 6); } }
  return curve;
}

const tone = (type, f, t, a, d, peak, out) => { const o = osc(type, f, f, t, a + d); env(o, t, peak, a, d).connect(out); return o; };

export const INSTRUMENTS = {
  // Square lead with a lazy vibrato: the chiptune melody.
  pulse(out, t, f, dur, vel) {
    const o = tone("square", f, t, 0.008, dur, 0.11 * vel, out);
    vibrato(o, 5.5, f * 0.008, t + 0.08, dur + 0.1);
  },
  // Short square blip (arps).
  chip(out, t, f, dur, vel) { tone("square", f, t, 0.003, Math.min(dur, 0.18), 0.08 * vel, out); },
  // Triangle bass.
  tri(out, t, f, dur, vel) { tone("triangle", f, t, 0.006, dur, 0.4 * vel, out); },
  // Soft electric piano: sine + a quick triangle overtone.
  epiano(out, t, f, dur, vel) {
    tone("sine", f, t, 0.006, dur * 1.2, 0.16 * vel, out);
    tone("triangle", f * 2, t, 0.003, 0.25, 0.04 * vel, out);
  },
  // Detuned saw pad through a lowpass, slow attack.
  pad(out, t, f, dur, vel) {
    const lp = audioCtx().createBiquadFilter();
    lp.type = "lowpass"; lp.frequency.value = 1400; lp.connect(out);
    for (const det of [0.996, 1.004]) tone("sawtooth", f * det, t, Math.min(0.35, dur * 0.4), dur, 0.045 * vel, lp);
  },
  // Brass-ish fanfare: saw through a lowpass that opens on the attack.
  brass(out, t, f, dur, vel) {
    const lp = audioCtx().createBiquadFilter();
    lp.type = "lowpass"; lp.Q.value = 2;
    lp.frequency.setValueAtTime(400, t);
    lp.frequency.exponentialRampToValueAtTime(3200, t + 0.06);
    lp.frequency.exponentialRampToValueAtTime(1200, t + 0.06 + dur);
    lp.connect(out);
    tone("sawtooth", f, t, 0.02, dur, 0.12 * vel, lp);
  },
  // Bell: sine with an inharmonic partial, long ring.
  bell(out, t, f, dur, vel) {
    tone("sine", f, t, 0.002, dur + 0.6, 0.18 * vel, out);
    tone("sine", f * 2.76, t, 0.001, 0.35, 0.06 * vel, out);
  },
  // Overdriven guitar: two detuned saws through a soft-clipping shaper and a
  // cabinet-ish lowpass; palm-muted short notes chug, long ones ring.
  gtr(out, t, f, dur, vel) {
    const ctx = audioCtx(), sh = ctx.createWaveShaper(), lp = ctx.createBiquadFilter();
    sh.curve = driveCurve(); lp.type = "lowpass"; lp.frequency.value = 2600; lp.Q.value = 0.8;
    const g = ctx.createGain();
    g.gain.value = 0.5;
    sh.connect(lp).connect(g).connect(out);
    for (const det of [0.997, 1.003]) tone("sawtooth", f * det, t, 0.004, dur, 0.5 * vel, sh);
  },
  // Lead guitar: one saw, driven, with a singing vibrato.
  lead(out, t, f, dur, vel) {
    const ctx = audioCtx(), sh = ctx.createWaveShaper(), lp = ctx.createBiquadFilter();
    sh.curve = driveCurve(); lp.type = "lowpass"; lp.frequency.value = 3400;
    const g = ctx.createGain();
    g.gain.value = 0.35;
    sh.connect(lp).connect(g).connect(out);
    const o = tone("sawtooth", f, t, 0.01, dur, 0.55 * vel, sh);
    vibrato(o, 6, f * 0.012, t + 0.12, dur + 0.1);
  },
  // A string section: three detuned saws through a lowpass, a bowed attack and a
  // slow vibrato (Star Slop's space opera).
  strings(out, t, f, dur, vel) {
    const lp = audioCtx().createBiquadFilter();
    lp.type = "lowpass"; lp.frequency.value = 2400; lp.Q.value = 0.5; lp.connect(out);
    for (const det of [0.994, 1, 1.006]) {
      const o = tone("sawtooth", f * det, t, Math.min(0.12, dur * 0.3), dur, 0.04 * vel, lp);
      vibrato(o, 5, f * 0.006, t + 0.15, dur + 0.1);
    }
  },
  // Timpani: a low sine that sags a little as it rings, with a felt thump of noise.
  timp(out, t, f, dur, vel) {
    env(osc("sine", f * 1.04, f, t, Math.max(0.6, dur)), t, 0.55 * vel, 0.004, Math.max(0.6, dur)).connect(out);
    env(noise(t, 0.08, "lowpass", 300), t, 0.3 * vel, 0.002, 0.07).connect(out);
  },
  // Sad trombone: a saw that sags in pitch with a wide wobble.
  wah(out, t, f, dur, vel) {
    const o = osc("sawtooth", f, f * 0.94, t, dur);
    const lp = filter(o, "lowpass", 900, 3);
    env(lp, t, 0.14 * vel, 0.03, dur).connect(out);
    vibrato(o, 6, f * 0.03, t + dur * 0.3, dur);
  },
  // Drum kit by MIDI note (compose.js KICK/RIM/SNARE/HAT/OPEN_HAT).
  kit(out, t, _f, _dur, vel, midi) {
    if (midi === KICK) { env(osc("sine", 150, 42, t, 0.14), t, 0.7 * vel, 0.002, 0.16).connect(out); return; }
    if (midi === SNARE) {
      env(noise(t, 0.14, "bandpass", 1800, 0.8), t, 0.5 * vel, 0.001, 0.13).connect(out);
      env(osc("triangle", 220, 160, t, 0.08), t, 0.25 * vel, 0.001, 0.07).connect(out);
      return;
    }
    if (midi === RIM) { env(osc("square", 1700, 1700, t, 0.02), t, 0.1 * vel, 0.001, 0.02).connect(out); return; }
    const open = midi === OPEN_HAT;
    if (midi === HAT || open) env(noise(t, open ? 0.25 : 0.04, "highpass", 7000), t, 0.25 * vel, 0.001, open ? 0.22 : 0.035).connect(out);
  },
};
