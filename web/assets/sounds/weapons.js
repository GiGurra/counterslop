import { live, now, rnd, dest, env, osc, noise } from "../../gen/synth/core.js";

// Pew: a falling square chirp, slightly different every time.
export function pew(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  const f = rnd(1300, 1700);
  env(osc("square", f, rnd(160, 240), t, 0.14), t, 0.18, 0.003, 0.13).connect(o);
  env(osc("sawtooth", f * 0.5, 90, t, 0.1), t, 0.08, 0.003, 0.09).connect(o);
  env(noise(t, 0.05, "highpass", 2500), t, 0.15, 0.002, 0.04).connect(o);
}

// Dry-fire click.
export function click() {
  if (!live()) return;
  const t = now();
  env(noise(t, 0.03, "bandpass", 3000, 4), t, 0.4, 0.001, 0.025).connect(dest());
}

// Clack, clack, ding (dur = reload time in s).
export function reload(dur = 1.6) {
  if (!live()) return;
  const t = now();
  for (const dt of [0.1, 0.55]) env(noise(t + dt, 0.04, "bandpass", 1800, 3), t + dt, 0.5, 0.002, 0.04).connect(dest());
  env(osc("triangle", 300, 120, t + 0.6, 0.15), t + 0.6, 0.2, 0.005, 0.12).connect(dest());
  env(osc("sine", 1760, 1760, t + dur - 0.15, 0.4), t + dur - 0.15, 0.12, 0.005, 0.35).connect(dest());
}

// Shot hit another player.
export function bonk() {
  if (!live()) return;
  const t = now();
  env(osc("triangle", 900, 300, t, 0.08), t, 0.12, 0.002, 0.08).connect(dest());
}
