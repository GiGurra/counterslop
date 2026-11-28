// The cars' sounds (race mode, docs/racing.md phase 4). All synthesized; sp =
// spatial options (dest). One loop with a handle per car:
//   carEngine(sp, kind) -> {update({pan, gain, rev, boost}), stop()}   kind: buggy|racecar|tank|hovercraft,
//     rev 0 (idle) .. 1 (top speed), boost 0..1 (the jet roar on top)
// and one-shots for the items and the hits:
//   carBeam  the Linter Beam's zap        carRocket(sp, heavy)  a launch and its whoosh
//   carOrb   Synergy Orbs' chime            carDrop(sp, kind)     spikes rattle, oil glugs, a mine clunks and beeps
//   carBoom  a rocket or a mine going off   carWreck              a car exploding: bigger, with bits raining
//   carRam   a crunch of panels             carSkid               tyres squealing (spikes, oil)
//   carBoostOn  the boost lighting up       carPit                the pit lane's repair ratchet
import { live, now, rnd, dest, env, osc, noise, filter, audioCtx, stopLoop } from "../../gen/synth/core.js";

// Each type's engine: the base note at idle and at the top, the waveforms, the
// lowpass at idle and opened up, and the rasp (noise) on top.
export const ENGINES = {
  buggy: { lo: 42, hi: 120, wave: "square", over: "sawtooth", overK: 1.5, cut: [500, 1600], rasp: 0.05, putt: 0.3 }, // a lawnmower two-stroke
  racecar: { lo: 90, hi: 330, wave: "sawtooth", over: "sawtooth", overK: 2.01, cut: [900, 4200], rasp: 0.03, putt: 0 }, // a screaming V10-ish whine
  tank: { lo: 28, hi: 62, wave: "sawtooth", over: "square", overK: 0.5, cut: [260, 700], rasp: 0.1, putt: 0.45 }, // a diesel rumble with clatter
  hovercraft: { lo: 70, hi: 160, wave: "triangle", over: "sawtooth", overK: 3, cut: [700, 2400], rasp: 0.22, putt: 0 }, // fans: mostly air
};

export function carEngine(sp = { pan: 0, gain: 0.5, rev: 0, boost: 0 }, kind = "buggy") {
  if (!live()) return null;
  const e = ENGINES[kind] || ENGINES.buggy, ctx = audioCtx(), t = now();
  const g = ctx.createGain(), p = ctx.createStereoPanner();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(Math.max(0.0001, sp.gain), t + 0.3);
  p.pan.value = sp.pan;
  g.connect(p).connect(dest());
  const f = (rev) => e.lo + (e.hi - e.lo) * rev;
  const a = ctx.createOscillator(), b = ctx.createOscillator(), ga = ctx.createGain(), gb = ctx.createGain();
  a.type = e.wave; b.type = e.over;
  a.frequency.value = f(sp.rev || 0); b.frequency.value = f(sp.rev || 0) * e.overK;
  ga.gain.value = 0.2; gb.gain.value = 0.07;
  const lp = ctx.createBiquadFilter();
  lp.type = "lowpass"; lp.Q.value = 3; lp.frequency.value = e.cut[0] + (e.cut[1] - e.cut[0]) * (sp.rev || 0);
  a.connect(ga).connect(lp); b.connect(gb).connect(lp);
  // The putt: the volume chopped at the firing rate (two-strokes and diesels).
  const chop = ctx.createGain(), lfo = ctx.createOscillator(), lg = ctx.createGain();
  chop.gain.value = 1 - e.putt / 2; lfo.type = "square"; lfo.frequency.value = f(sp.rev || 0) / 4; lg.gain.value = e.putt / 2;
  lfo.connect(lg).connect(chop.gain);
  lp.connect(chop).connect(g);
  const air = noise(t, 3600, "bandpass", 1200, 0.7, true), ag = ctx.createGain();
  ag.gain.value = e.rasp * (0.4 + (sp.rev || 0));
  air.connect(ag).connect(g);
  // The boost: a jet's roar (lowpassed noise) and a high whistle, faded in and out.
  const roar = noise(t, 3600, "lowpass", 900, 0.6, true), rg = ctx.createGain(), whistle = ctx.createOscillator(), wg = ctx.createGain();
  rg.gain.value = 0.0001; whistle.type = "sine"; whistle.frequency.value = 1800; wg.gain.value = 0.0001;
  roar.connect(rg).connect(g); whistle.connect(wg).connect(g);
  a.start(t); b.start(t); lfo.start(t); whistle.start(t);
  const srcs = [a, b, lfo, whistle, air.src, roar.src];
  let last = { pan: sp.pan, gain: sp.gain, rev: sp.rev || 0, boost: 0 };
  return {
    // Every frame; only an audible change schedules anything (AudioParam events pile up).
    update(s) {
      const rev = s.rev ?? last.rev, boost = s.boost ?? last.boost;
      if (Math.abs(s.gain - last.gain) < 0.01 && Math.abs(s.pan - last.pan) < 0.01 && Math.abs(rev - last.rev) < 0.02 && Math.abs(boost - last.boost) < 0.05) return;
      last = { pan: s.pan, gain: s.gain, rev, boost };
      const k = audioCtx().currentTime;
      g.gain.setTargetAtTime(Math.max(0.0001, s.gain), k, 0.1);
      p.pan.setTargetAtTime(s.pan, k, 0.1);
      a.frequency.setTargetAtTime(f(rev), k, 0.12);
      b.frequency.setTargetAtTime(f(rev) * e.overK, k, 0.12);
      lfo.frequency.setTargetAtTime(f(rev) / 4, k, 0.12);
      lp.frequency.setTargetAtTime(e.cut[0] + (e.cut[1] - e.cut[0]) * rev, k, 0.15);
      ag.gain.setTargetAtTime(e.rasp * (0.4 + rev), k, 0.2);
      rg.gain.setTargetAtTime(Math.max(0.0001, 0.35 * boost), k, 0.08);
      wg.gain.setTargetAtTime(Math.max(0.0001, 0.03 * boost), k, 0.08);
      whistle.frequency.setTargetAtTime(1500 + 900 * rev, k, 0.2);
    },
    stop(fade = 0.5) { stopLoop(g.gain, srcs, fade); },
  };
}

// The Linter Beam: a falling zap with a buzz under it.
export function carBeam(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("sawtooth", 2400, 600, t, 0.09), t, 0.12, 0.001, 0.08).connect(o);
  env(osc("square", 120, 118, t, 0.08), t, 0.06, 0.001, 0.07).connect(o);
  env(noise(t, 0.05, "highpass", 5000), t, 0.15, 0.001, 0.04).connect(o);
}

// A rocket off the rails: a pop, then the motor's whoosh (heavier: lower, longer).
export function carRocket(sp, heavy = false) {
  if (!live()) return;
  const t = now(), o = dest(sp), d = heavy ? 0.7 : 0.45;
  env(noise(t, 0.05, "bandpass", 900, 1.5), t, 0.5, 0.001, 0.04).connect(o);
  env(noise(t + 0.02, d, "bandpass", heavy ? 500 : 900, 0.9), t + 0.02, 0.45, 0.03, d).connect(o);
  env(osc("sawtooth", heavy ? 160 : 260, heavy ? 90 : 150, t, d), t, 0.08, 0.01, d).connect(o);
}

// Synergy Orbs: a rising, wobbling chime (corporate synergy, now airborne).
export function carOrb(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  for (const [k, f] of [[0, 660], [0.06, 990], [0.12, 1320]]) env(osc("sine", f, f * 1.03, t + k, 0.35), t + k, 0.1, 0.005, 0.3).connect(o);
  env(osc("triangle", 330, 660, t, 0.3), t, 0.06, 0.02, 0.28).connect(o);
}

// A rear drop: spikes rattle out, oil glugs, a mine clunks down and beeps as it arms.
export function carDrop(sp, kind = "mine") {
  if (!live()) return;
  const t = now(), o = dest(sp);
  if (kind === "spikes") {
    for (let i = 0; i < 6; i++) { const k = t + i * 0.035 + rnd(0, 0.01), f = rnd(2600, 3600); env(osc("square", f, f * 0.95, k, 0.03), k, 0.06, 0.001, 0.025).connect(o); }
    return;
  }
  if (kind === "oil") {
    for (let i = 0; i < 4; i++) { const k = t + i * 0.09; env(osc("sine", 180 + i * 30, 90, k, 0.1), k, 0.22, 0.01, 0.09).connect(o); }
    env(noise(t, 0.4, "lowpass", 600, 1), t, 0.15, 0.02, 0.35).connect(o);
    return;
  }
  env(osc("sine", 140, 60, t, 0.12), t, 0.5, 0.002, 0.11).connect(o); // the clunk
  env(noise(t, 0.05, "bandpass", 1200, 2), t, 0.2, 0.001, 0.04).connect(o);
  for (const k of [0.3, 0.45, 0.6]) env(osc("square", 1900, 1900, t + k, 0.05), t + k, 0.05, 0.001, 0.04).connect(o); // arming beeps
}

// A rocket or a mine going off: a crack, a boom and a gritty tail.
export function carBoom(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.1, "highpass", 2500), t, 0.6, 0.001, 0.08).connect(o);
  env(noise(t, 0.9, "lowpass", 900, 0.8), t, 0.9, 0.003, 0.85).connect(o);
  env(osc("sine", 110, 32, t, 0.6), t, 0.9, 0.004, 0.55).connect(o);
}

// A car exploding: a double boom, a long rumble, bits of bodywork raining down.
export function carWreck(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  carBoom(sp);
  env(noise(t + 0.12, 1.6, "lowpass", 500, 0.8), t + 0.12, 1.0, 0.01, 1.5).connect(o);
  env(osc("sine", 70, 24, t + 0.1, 1.2), t + 0.1, 0.9, 0.01, 1.1).connect(o);
  for (let i = 0; i < 9; i++) { const k = t + 0.5 + rnd(0, 1.2), f = rnd(900, 2600); env(osc("square", f, f * 0.9, k, 0.05), k, 0.05, 0.001, 0.04).connect(o); }
}

// A ram: crumpling panels.
export function carRam(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(noise(t, 0.25, "bandpass", 700, 1.2), t, 0.8, 0.001, 0.22).connect(o);
  env(osc("sine", 90, 50, t, 0.2), t, 0.6, 0.002, 0.18).connect(o);
  for (const f of [430, 610]) env(osc("sawtooth", f, f * 0.8, t, 0.3), t, 0.06, 0.003, 0.28).connect(o);
}

// Tyres squealing: a narrow, wavering band of noise.
export function carSkid(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp), n = noise(t, 0.9, "bandpass", 2300, 12);
  const g = env(n, t, 0.35, 0.03, 0.8);
  const f = filter(g, "peaking", 2600, 8);
  f.connect(o);
}

// The boost lighting up: a whump and a rising whoosh.
export function carBoostOn(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  env(osc("sine", 70, 140, t, 0.25), t, 0.5, 0.005, 0.22).connect(o);
  env(noise(t, 0.6, "bandpass", 800, 0.8), t, 0.35, 0.15, 0.4).connect(o);
}

// The pit lane: an impact wrench's ratchet.
export function carPit(sp) {
  if (!live()) return;
  const t = now(), o = dest(sp);
  for (let i = 0; i < 10; i++) { const k = t + i * 0.045; env(noise(k, 0.03, "bandpass", 3000, 3), k, 0.25, 0.001, 0.025).connect(o); }
  env(osc("sawtooth", 210, 230, t, 0.45), t, 0.05, 0.01, 0.42).connect(o);
}

// A power-up driven through (internal/game pickups.go): a bright two-note
// chime up and a sparkle, its colour by kind (money rings, a repair ratchets,
// the freeze shimmers low and glassy, caffeine fizzes).
const PICKUP_NOTES = { bonus: [880, 1320], hotfix: [660, 990], reorg: [587, 880], caffeine: [740, 1110], freeze: [523, 784], flag: [698, 1047] };
export function carPickup(sp, kind = "bonus") {
  if (!live()) return;
  const t = now(), o = dest(sp), [a, b] = PICKUP_NOTES[kind] || PICKUP_NOTES.bonus;
  env(osc("triangle", a, a, t, 0.12), t, 0.3, 0.003, 0.11).connect(o);
  env(osc("triangle", b, b, t + 0.08, 0.25), t + 0.08, 0.3, 0.003, 0.24).connect(o);
  env(osc("sine", b * 2, b * 2.02, t + 0.08, 0.3), t + 0.08, 0.08, 0.01, 0.28).connect(o);
  env(noise(t, 0.35, "highpass", 6000), t, 0.08, 0.05, 0.3).connect(o); // the sparkle
  if (kind === "bonus") for (let i = 0; i < 4; i++) { const k = t + 0.18 + i * 0.05; env(osc("square", 2400 + i * 300, 2400 + i * 300, k, 0.04), k, 0.05, 0.001, 0.035).connect(o); } // coins
  else if (kind === "hotfix") for (let i = 0; i < 5; i++) { const k = t + 0.15 + i * 0.04; env(noise(k, 0.025, "bandpass", 3000, 3), k, 0.2, 0.001, 0.02).connect(o); } // a ratchet
  else if (kind === "caffeine") env(noise(t + 0.1, 0.5, "bandpass", 4500, 2), t + 0.1, 0.15, 0.05, 0.4).connect(o); // fizz
  else if (kind === "freeze") env(osc("sine", 3100, 2900, t + 0.05, 0.6), t + 0.05, 0.06, 0.1, 0.5).connect(o); // a glassy ring
}
