import { live, now, rnd, dest, env, osc, noise } from "../../gen/synth/core.js";

// Grenades (Merge Conflict, Stack Trace, Smoke Test). All synthesized; sp = spatial options (dest).

// The pin and the throw: a ring ping, then an airy whoosh.
export function nadeThrow(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 2600, 2400, t, 0.06), t, 0.12, 0.001, 0.05).connect(o);
  env(noise(t + 0.05, 0.3, "bandpass", rnd(700, 900), 1.2), t + 0.05, 0.35, 0.06, 0.22).connect(o);
}

// A bounce off a wall or the floor: a dull metal clink.
export function nadeBounce(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp), f = rnd(1500, 1900);
  env(osc("square", f, f * 0.96, t, 0.05), t, 0.08, 0.001, 0.04).connect(o);
  env(noise(t, 0.04, "bandpass", 3500, 4), t, 0.2, 0.001, 0.03).connect(o);
}

// Merge Conflict goes off: a crack, a low thump and a dissonant chord (both sides at once).
export function heBlast(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.12, "highpass", 2000), t, 0.7, 0.001, 0.1).connect(o);
  env(noise(t, 1.1, "lowpass", 700, 0.9), t, 1.0, 0.003, 1.0).connect(o);
  env(osc("sine", 90, 30, t, 0.8), t, 0.9, 0.004, 0.7).connect(o);
  for (const f of [311, 330]) env(osc("sawtooth", f, f * 0.9, t + 0.05, 0.6), t + 0.05, 0.07, 0.01, 0.55).connect(o);
}

// Stack Trace goes off: a sharp bang and the ringing of a thousand-line error.
export function flashBang(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.15, "highpass", 1500), t, 0.9, 0.001, 0.12).connect(o);
  env(osc("sine", 140, 50, t, 0.25), t, 0.6, 0.002, 0.2).connect(o);
}

// The ringing in your ears when you are blinded (secs long).
export function flashRing(secs = 3) {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("sine", 3520, 3460, t, secs), t, 0.06, 0.05, secs * 0.9).connect(o);
  env(osc("sine", 3530, 3470, t, secs), t, 0.04, 0.05, secs * 0.9).connect(o);
}

// A fire grenade bursts (Hot Reload, Burndown Chart): glass-ish tink, a
// low whump of catching fuel and a roar settling into the burn.
export function fireBurst(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 3100, 2700, t, 0.07), t, 0.18, 0.001, 0.06).connect(o);
  env(osc("sine", 110, 55, t + 0.02, 0.4), t + 0.02, 0.7, 0.01, 0.35).connect(o);
  env(noise(t + 0.02, 1.2, "lowpass", 1400, 0.8), t + 0.02, 0.6, 0.05, 1.1).connect(o);
}

// A fire pool burning: one short crackle (played again and again while it burns).
export function fireCrackle(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.5, "bandpass", rnd(500, 800), 0.8), t, 0.16, 0.05, 0.4).connect(o);
  for (let i = 0; i < 4; i++) {
    const at = t + rnd(0, 0.45);
    env(noise(at, 0.03, "highpass", rnd(2500, 5000)), at, rnd(0.1, 0.25), 0.001, 0.02).connect(o);
  }
}

// A fire put out by a smoke: a wet hiss.
export function fireOut(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 1.0, "highpass", 3000, 0.6), t, 0.3, 0.02, 0.9).connect(o);
}

// Smoke Test pops: a long hiss that settles.
export function smokePop(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 500, 200, t, 0.08), t, 0.25, 0.002, 0.06).connect(o);
  env(noise(t, 2.6, "bandpass", 2400, 0.7), t, 0.35, 0.15, 2.3).connect(o);
}
