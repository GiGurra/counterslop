import { live, now, rnd, dest, env, osc, noise } from "../../gen/synth/core.js";

// The Big Diff (the round objective). All synthesized; sp = spatial options (dest).

// Plant: three mechanical-keyboard clacks and a commit "ding".
export function plantClick(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  for (const dt of [0, 0.09, 0.2]) {
    env(noise(t + dt, 0.035, "bandpass", rnd(2200, 2800), 5), t + dt, 0.5, 0.001, 0.03).connect(o);
    env(osc("triangle", 180, 90, t + dt, 0.05), t + dt, 0.2, 0.001, 0.04).connect(o);
  }
  env(osc("sine", 1320, 1320, t + 0.32, 0.25), t + 0.32, 0.15, 0.004, 0.22).connect(o);
}

// One CI beep: a failing-test buzz (urgency 0..1 raises it as the timer runs out).
export function ciBeep(sp, urgency = 0) {
  if (!live()) return;
  const t = now(), o = dest(sp), f = 1800 + urgency * 900;
  env(osc("square", f, f, t, 0.09), t, 0.14, 0.002, 0.08).connect(o);
  env(osc("square", f * 0.5, f * 0.5, t, 0.09), t, 0.05, 0.002, 0.08).connect(o);
}

// Defuse progress: a short descending "undo" blip (git revert).
export function revertTick(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 1100, 500, t, 0.08), t, 0.12, 0.002, 0.07).connect(o);
}

// Defused: a reversed-sounding rising sweep and a calm major third.
export function reverted() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("sawtooth", 200, 1400, t, 0.35), t, 0.08, 0.3, 0.05).connect(o);
  for (const [f, dt] of [[659, 0.38], [831, 0.5]]) env(osc("sine", f, f, t + dt, 0.6), t + dt, 0.2, 0.005, 0.55).connect(o);
}

// The merge goes through: a silly detuned major chord blared over a noise burst
// and a sub drop.
export function mergeBoom(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 1.6, "lowpass", 900, 0.8), t, 1.0, 0.004, 1.4).connect(o);
  env(noise(t, 0.4, "highpass", 3000), t, 0.4, 0.001, 0.3).connect(o);
  env(osc("sine", 110, 28, t, 1.3), t, 0.9, 0.005, 1.2).connect(o);
  for (const f of [261.6, 329.6, 392, 523.3]) {
    env(osc("sawtooth", f * rnd(0.985, 1.015), f * 0.94, t + 0.08, 1.4), t + 0.08, 0.09, 0.02, 1.3).connect(o);
  }
}

// Round over: a short fanfare, rising for a win, falling for a loss.
export function roundEnd(win = true) {
  if (!live()) return;
  const t = now(), o = dest();
  const notes = win ? [523, 659, 784, 1047] : [440, 370, 311, 262];
  notes.forEach((f, i) => env(osc("square", f, f, t + i * 0.11, 0.2), t + i * 0.11, 0.1, 0.004, 0.18).connect(o));
}
