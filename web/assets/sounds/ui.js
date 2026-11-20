import { live, now, dest, env, osc, filter } from "../../gen/synth/core.js";

// Clown honk when someone joins.
export function honk() {
  if (!live()) return;
  const t = now();
  for (const [f, dt] of [[330, 0], [262, 0.16]]) {
    const lp = filter(osc("sawtooth", f, f * 0.97, t + dt, 0.14), "lowpass", 1400);
    env(lp, t + dt, 0.12, 0.01, 0.13).connect(dest());
  }
}
