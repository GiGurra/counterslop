// Music voices on the synth primitives: each schedules one note into `out` at
// AudioContext time t (freq Hz, dur s, vel 0..1). Game-agnostic; themes pick them by name.
import { audioCtx, env, osc, noise, filter, vibrato } from "../synth/core.js";
import { KICK, RIM, SNARE, HAT, OPEN_HAT } from "./compose.js";

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
