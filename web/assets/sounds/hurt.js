// Taking damage: a dull thud for flesh, a clank for the hoodie+headphones
// soaking it up, and a heartbeat at low HP.
import { live, now, rnd, dest, env, osc, noise } from "../../gen/synth/core.js";

// Hit where it hurts: a low body thump with a short muffled crunch.
export function hurtFlesh() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("sine", rnd(150, 180), 55, t, 0.16), t, 0.5, 0.002, 0.15).connect(o);
  env(noise(t, 0.06, "lowpass", 900), t, 0.25, 0.001, 0.05).connect(o);
}

// The armor took it: a dry plasticky clank (headphones are mostly plastic).
export function hurtArmor() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("square", rnd(620, 700), 380, t, 0.06), t, 0.12, 0.001, 0.06).connect(o);
  env(osc("triangle", rnd(1500, 1700), 1100, t, 0.09), t, 0.08, 0.001, 0.08).connect(o);
  env(noise(t, 0.04, "bandpass", 3000), t, 0.15, 0.001, 0.03).connect(o);
}

// One heartbeat (lub-dub) for low HP.
export function heartbeat() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("sine", 62, 48, t, 0.12), t, 0.55, 0.004, 0.11).connect(o);
  env(osc("sine", 58, 44, t + 0.18, 0.1), t + 0.18, 0.4, 0.004, 0.09).connect(o);
}
