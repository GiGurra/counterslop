// Bullet impacts per surface and casings landing, all short and positional.
// p: a pitch factor (~0.9..1.1: no two the same). Surfaces: game/impactsfx.js.
import { live, now, rnd, dest, env, osc, noise } from "../../gen/synth/core.js";

// Concrete, brick, plaster: a dry chip with a gritty tail.
export function impactStone(sp, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.05, "bandpass", 2400 * p, 1.5), t, 0.5, 0.001, 0.05).connect(o);
  env(noise(t + 0.01, 0.12, "highpass", 3500 * p), t + 0.01, 0.12, 0.004, 0.1).connect(o); // grit falling
  env(osc("triangle", 260 * p, 120, t, 0.05), t, 0.18, 0.001, 0.04).connect(o);
}

// Metal: a bright ping; now and then (ricochet) a falling whine off into the distance.
export function impactMetal(sp, p = 1, ricochet = Math.random() < 0.3) {
  if (!live()) return;
  const t = now(), o = dest(sp), f = rnd(2600, 3400) * p;
  env(osc("sine", f, f * 0.98, t, 0.3), t, 0.22, 0.001, 0.25).connect(o);
  env(osc("sine", f * 1.51, f * 1.49, t, 0.15), t, 0.1, 0.001, 0.12).connect(o);
  env(noise(t, 0.03, "highpass", 5000), t, 0.3, 0.001, 0.025).connect(o);
  if (ricochet) {
    const w = rnd(3200, 4200) * p;
    env(osc("sine", w, w * 0.45, t + 0.02, 0.45), t + 0.02, 0.09, 0.03, 0.4).connect(o);
  }
}

// Wood: a hollow knock and a splintery crack.
export function impactWood(sp, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("triangle", 420 * p, 180 * p, t, 0.08), t, 0.35, 0.001, 0.07).connect(o);
  env(noise(t, 0.04, "bandpass", 1500 * p, 2), t, 0.35, 0.001, 0.04).connect(o);
}

// Glass (screens, panes): a hard tick and a short glassy tinkle.
export function impactGlass(sp, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.02, "highpass", 6000), t, 0.35, 0.001, 0.015).connect(o);
  for (let i = 0; i < 3; i++) {
    const dt = 0.01 + i * rnd(0.02, 0.04), f = rnd(4500, 7000) * p;
    env(osc("sine", f, f, t + dt, 0.08), t + dt, 0.06, 0.001, 0.06).connect(o);
  }
}

// Sand, dirt (and carpet): a soft puff.
export function impactDirt(sp, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.1, "lowpass", 900 * p, 0.7), t, 0.4, 0.003, 0.09).connect(o);
  env(noise(t, 0.06, "bandpass", 2000 * p, 1), t, 0.1, 0.002, 0.05).connect(o);
}

// Cardboard cartons, cloth: a papery thump.
export function impactCardboard(sp, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("sine", 150 * p, 70, t, 0.08), t, 0.35, 0.002, 0.07).connect(o);
  env(noise(t, 0.05, "bandpass", 700 * p, 1.2), t, 0.3, 0.002, 0.05).connect(o);
}

const IMPACT = { stone: impactStone, metal: impactMetal, wood: impactWood, glass: impactGlass, dirt: impactDirt, cardboard: impactCardboard };
// The impact for a sound surface (impactsfx.js impactSound).
export function bulletImpact(surface, sp, p) { (IMPACT[surface] || impactStone)(sp, p); }

// A brass casing landing: on a hard floor two or three bright tinkles, on carpet a dull tick.
export function casingBrass(sp, hard = true, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  if (!hard) { env(noise(t, 0.03, "bandpass", 1800 * p, 2), t, 0.12, 0.001, 0.025).connect(o); return; }
  let dt = 0;
  for (let i = 0; i < 3; i++) {
    const f = rnd(5200, 6800) * p, g = 0.07 / (i + 1);
    env(osc("sine", f, f * 0.99, t + dt, 0.1), t + dt, g, 0.001, 0.08).connect(o);
    env(osc("sine", f * 1.38, f * 1.37, t + dt, 0.06), t + dt, g * 0.5, 0.001, 0.05).connect(o);
    dt += rnd(0.06, 0.11) / (i + 1);
  }
}

// A plastic shotgun shell: a hollow clack (and a softer second one), muffled on carpet.
export function casingShell(sp, hard = true, p = 1) {
  if (!live()) return;
  const t = now(), o = dest(sp), k = hard ? 1 : 0.4;
  for (const [dt, g] of [[0, 0.16], [rnd(0.08, 0.13), 0.07]]) {
    env(osc("triangle", 900 * p, 600 * p, t + dt, 0.04), t + dt, g * k, 0.001, 0.035).connect(o);
    env(noise(t + dt, 0.025, "bandpass", (hard ? 2600 : 1200) * p, 2), t + dt, g * k, 0.001, 0.02).connect(o);
  }
}
