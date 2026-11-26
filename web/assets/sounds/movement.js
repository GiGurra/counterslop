import { live, now, rnd, dest, env, osc, noise, filter, vibrato } from "../../gen/synth/core.js";

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

// A launch pad's throw (internal/sim zones.go, Star Slop): a heavy catapult
// thunk, then a whoosh rising in pitch as you leave the ground.
export function launch(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("sine", 90, 35, t, 0.25), t, 0.55, 0.004, 0.24).connect(o);
  env(noise(t, 0.08, "lowpass", 900), t, 0.35, 0.002, 0.07).connect(o);
  const w = noise(t + 0.03, 1.1, "bandpass", 400, 1.4);
  env(w, t + 0.03, 0.3, 0.25, 0.8).connect(o);
  const z = osc("triangle", 220, 880, t + 0.05, 0.9);
  vibrato(z, 7, 20, t + 0.05, 0.9);
  env(z, t + 0.05, 0.07, 0.2, 0.7).connect(o);
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

// The Gravity Override's hook (internal/sim hook.go). Fired and latched: a
// zappy tractor-beam chirp with a metallic clack at the end.
export function hookFire(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  const z = osc("sawtooth", 1800, 240, t, 0.18);
  vibrato(z, 60, 90, t, 0.18);
  env(filter(z, "bandpass", 1400, 2), t, 0.22, 0.004, 0.17).connect(o);
  env(noise(t + 0.15, 0.05, "highpass", 3000, 1.5), t + 0.15, 0.25, 0.002, 0.04).connect(o);
}

// Fired at nothing in reach: the beam fizzles out, a little embarrassed.
export function hookMiss(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(filter(osc("sawtooth", 900, 120, t, 0.3), "lowpass", 1200), t, 0.15, 0.005, 0.28).connect(o);
  env(noise(t, 0.25, "bandpass", 2500, 4), t, 0.08, 0.01, 0.22).connect(o);
}

// Let go: the line snaps back.
export function hookRelease(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 500, 1100, t, 0.08), t, 0.12, 0.003, 0.07).connect(o);
}

// Gravity flipped onto the hooked face: a rising whoomp, then the boots clamp on.
export function gravFlip(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  const w = osc("sine", 70, 260, t, 0.35);
  vibrato(w, 9, 25, t, 0.35);
  env(w, t, 0.45, 0.02, 0.33).connect(o);
  env(osc("square", 160, 90, t + 0.3, 0.08), t + 0.3, 0.2, 0.003, 0.07).connect(o);
  env(noise(t + 0.3, 0.06, "lowpass", 800), t + 0.3, 0.3, 0.003, 0.05).connect(o);
}
