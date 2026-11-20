import { live, now, rnd, dest, env, osc, noise, vibrato } from "../../gen/synth/core.js";

// Squeaky rubber shoes. sp: optional {pan, gain} for other players.
export function footstep(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.08, "bandpass", rnd(350, 700), 1.2), t, 0.35, 0.005, 0.07).connect(o);
  const f = rnd(1100, 1600);
  env(osc("sine", f, f * rnd(1.15, 1.4), t, 0.06), t, 0.05, 0.005, 0.05).connect(o);
}

// Boing.
export function jump() {
  if (!live()) return;
  const t = now();
  const o = osc("sine", 180, 620, t, 0.28);
  vibrato(o, 28, 40, t, 0.35);
  env(o, t, 0.3, 0.01, 0.27).connect(dest());
}

// Thud, louder for harder landings (speed in units/s).
export function land(speed) {
  if (!live()) return;
  const t = now(), k = Math.min(1, speed / 15);
  env(osc("sine", 140, 40, t, 0.18), t, 0.3 + 0.4 * k, 0.005, 0.18).connect(dest());
  env(noise(t, 0.12, "lowpass", 500), t, 0.3 * k + 0.1, 0.005, 0.1).connect(dest());
}
