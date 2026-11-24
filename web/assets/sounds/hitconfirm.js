// Hit confirmation (the shooter's ears only, on the server's word, never on a
// prediction): a dry tick for the body, a bright ding for the head, a muffled
// thunk when armour soaked it, and a two-note confirm on a kill.
import { live, now, rnd, dest, env, osc, noise } from "../../gen/synth/core.js";

// Body hit: a short, dry woodblock tick.
export function hitBody() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("triangle", rnd(1150, 1250), 700, t, 0.05), t, 0.16, 0.001, 0.045).connect(o);
  env(noise(t, 0.02, "bandpass", 2600, 3), t, 0.12, 0.001, 0.015).connect(o);
}

// Headshot: a higher, ringing ding (two partials, a little shimmer).
export function hitHead() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("sine", 2093, 2093, t, 0.32), t, 0.16, 0.001, 0.3).connect(o);
  env(osc("sine", 3140, 3120, t, 0.18), t, 0.06, 0.001, 0.16).connect(o);
  env(osc("triangle", 1400, 900, t, 0.04), t, 0.1, 0.001, 0.035).connect(o);
}

// Armour took part of it: the tick through a pillow, low and dull.
export function hitArmor() {
  if (!live()) return;
  const t = now(), o = dest();
  env(osc("sine", rnd(300, 330), 160, t, 0.08), t, 0.22, 0.002, 0.07).connect(o);
  env(noise(t, 0.05, "lowpass", 700), t, 0.18, 0.002, 0.045).connect(o);
}

// Kill confirm: a quick rising two-note blip (headshot kills ring brighter).
export function killConfirm(head = false) {
  if (!live()) return;
  const t = now(), o = dest();
  const f = head ? 1318 : 988;
  env(osc("square", f, f, t, 0.07), t, 0.07, 0.002, 0.06).connect(o);
  env(osc("square", f * 1.5, f * 1.5, t + 0.075, 0.14), t + 0.075, 0.07, 0.002, 0.13).connect(o);
  env(osc("sine", f * 3, f * 3, t + 0.075, 0.2), t + 0.075, 0.05, 0.002, 0.18).connect(o);
}
