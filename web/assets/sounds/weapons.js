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

// ---- per-weapon fire sounds (keyed by registry id in assets/weapons/index.js) ----

// The Stub: a thin, apologetic pip.
export function stubShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("square", rnd(1900, 2100), rnd(900, 1000), t, 0.08), t, 0.12, 0.002, 0.07).connect(o);
  env(noise(t, 0.03, "highpass", 4000), t, 0.08, 0.001, 0.025).connect(o);
}

// The Prompt: a fat, slightly detuned zap with a glitchy upper partial (it hallucinates).
export function promptShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  const f = rnd(700, 820);
  env(osc("sawtooth", f, f * 0.3, t, 0.16), t, 0.2, 0.002, 0.15).connect(o);
  env(osc("square", f * 1.01, f * 0.32, t, 0.14), t, 0.12, 0.002, 0.13).connect(o);
  env(osc("square", rnd(2500, 4200), rnd(300, 3000), t + 0.01, 0.05), t + 0.01, 0.05, 0.001, 0.05).connect(o);
  env(noise(t, 0.08, "bandpass", 1200, 0.7), t, 0.3, 0.001, 0.07).connect(o);
}

// The Linter: crisp, tidy and consistent: a clean click-thump with a tick on top.
export function linterShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 520, 140, t, 0.12), t, 0.3, 0.001, 0.11).connect(o);
  env(noise(t, 0.04, "bandpass", 3200, 2), t, 0.25, 0.001, 0.035).connect(o);
  env(osc("sine", 2400, 2400, t + 0.005, 0.03), t + 0.005, 0.05, 0.001, 0.03).connect(o);
}

// Code Review: a heavy crack with a long descending tail ("changes requested").
export function reviewShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.35, "lowpass", 1800, 0.8), t, 0.6, 0.001, 0.3).connect(o);
  env(osc("sawtooth", 260, 40, t, 0.5), t, 0.35, 0.002, 0.45).connect(o);
  env(osc("square", 1600, 200, t, 0.12), t, 0.12, 0.001, 0.1).connect(o);
}

// Rubber Duck: a pistol pop and a squeak.
export function duckShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.04, "bandpass", 2600, 1.5), t, 0.3, 0.001, 0.035).connect(o);
  env(osc("sine", rnd(900, 1000), rnd(1500, 1700), t + 0.01, 0.09), t + 0.01, 0.2, 0.005, 0.08).connect(o);
  env(osc("square", 1400, 700, t, 0.05), t, 0.08, 0.001, 0.045).connect(o);
}

// Hotfix: a heavy boom with a metallic ring ("deployed to prod").
export function hotfixShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.25, "lowpass", 1400, 0.7), t, 0.6, 0.001, 0.22).connect(o);
  env(osc("sawtooth", 180, 45, t, 0.3), t, 0.4, 0.002, 0.28).connect(o);
  env(osc("triangle", 1900, 1850, t + 0.02, 0.35), t + 0.02, 0.07, 0.003, 0.33).connect(o);
}

// Autocomplete: a soft tick and an upward suggestion chirp.
export function autocompleteShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.03, "bandpass", 3000, 2), t, 0.22, 0.001, 0.025).connect(o);
  env(osc("square", rnd(1100, 1250), rnd(2100, 2400), t, 0.045), t, 0.07, 0.001, 0.04).connect(o);
}

// Copy-Paste: the same crisp zap every time (it's a copy).
export function copypasteShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("sawtooth", 880, 260, t, 0.09), t, 0.18, 0.001, 0.085).connect(o);
  env(noise(t, 0.04, "bandpass", 2200, 1.5), t, 0.25, 0.001, 0.035).connect(o);
}

// Token Burner: a wide blast and a scatter of coin jingles.
export function tokenShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.3, "lowpass", 2200, 0.6), t, 0.7, 0.001, 0.26).connect(o);
  env(osc("sawtooth", 140, 50, t, 0.2), t, 0.35, 0.002, 0.18).connect(o);
  for (let i = 0; i < 4; i++) {
    const dt = 0.06 + i * 0.045;
    env(osc("sine", rnd(2600, 3600), rnd(2600, 3600), t + dt, 0.06), t + dt, 0.05, 0.001, 0.05).connect(o);
  }
}

// Context Window: a low thump with a whirr of the belt.
export function contextShot(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", rnd(300, 340), 90, t, 0.09), t, 0.35, 0.001, 0.08).connect(o);
  env(noise(t, 0.06, "bandpass", 900, 1), t, 0.25, 0.001, 0.05).connect(o);
  env(osc("sawtooth", 60, 70, t, 0.07), t, 0.08, 0.005, 0.06).connect(o);
}

// Mechanical Keyboard: a swish plus three clacky switches (blue switches, obviously).
export function keyboardSwing(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.18, "bandpass", 900, 1.5), t, 0.25, 0.03, 0.12).connect(o);
  for (const [dt, f] of [[0.04, 4200], [0.08, 3600], [0.12, 4600]]) {
    env(noise(t + dt, 0.02, "bandpass", f + rnd(-300, 300), 6), t + dt, 0.5, 0.001, 0.015).connect(o);
  }
}

// Weapon swap: a quick slide-and-click.
export function deploy() {
  if (!live()) return;
  const t = now();
  env(noise(t, 0.1, "bandpass", 1400, 2), t, 0.15, 0.02, 0.07).connect(dest());
  env(noise(t + 0.1, 0.02, "bandpass", 2600, 5), t + 0.1, 0.35, 0.001, 0.02).connect(dest());
}

// Bolt-action cycle (dur = cycle time): lift-pull clack, then push-lock clunk.
export function boltCycle(dur = 1.1) {
  if (!live()) return;
  const t = now(), o = dest();
  const a = t + dur * 0.25, b = t + dur * 0.6;
  env(noise(a, 0.05, "bandpass", 2200, 4), a, 0.45, 0.001, 0.04).connect(o);
  env(noise(a + 0.06, 0.09, "bandpass", 1300, 2), a + 0.06, 0.25, 0.01, 0.08).connect(o);
  env(noise(b, 0.07, "bandpass", 1600, 3), b, 0.3, 0.005, 0.06).connect(o);
  env(osc("triangle", 260, 140, b + 0.05, 0.1), b + 0.05, 0.35, 0.002, 0.09).connect(o);
}
