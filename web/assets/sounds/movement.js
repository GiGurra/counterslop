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

// Splash: a burst of filtered noise that falls in pitch, a hollow "bloop" and
// a few late droplets; bigger for a harder entry (speed in m/s).
export function splash(speed = 8) {
  if (!live()) return;
  const t = now(), k = Math.min(1, speed / 15), o = dest();
  env(noise(t, 0.5, "bandpass", 1800 - 900 * k, 0.7), t, 0.25 + 0.35 * k, 0.005, 0.45).connect(o);
  env(noise(t, 0.35, "lowpass", 700), t, 0.2 + 0.3 * k, 0.01, 0.3).connect(o);
  env(osc("sine", 420, 140, t, 0.16), t, 0.25, 0.005, 0.15).connect(o);
  for (let i = 0; i < 4; i++) {
    const d = t + 0.12 + i * rnd(0.05, 0.11), f = rnd(900, 1800);
    env(osc("sine", f, f * 1.6, d, 0.04), d, 0.06 * (1 - i / 5), 0.002, 0.04).connect(o);
  }
}
