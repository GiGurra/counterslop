// The dinghy's outboard: a two-stroke buzz (a sawtooth and a pulse a fifth
// over it, through a lowpass that opens with the revs), the prop's churn
// (filtered noise) and the hull slapping the water. A loop with a handle:
//   outboard(sp) -> {update({pan, gain, rev}), stop()}   rev 0 (idle) .. 1 (full ahead)
import { live, now, dest, noise, filter, audioCtx } from "../../gen/synth/core.js";

const freq = (rev) => 38 + 74 * rev;

export function outboard(sp = { pan: 0, gain: 0.5, rev: 0 }) {
  if (!live()) return null;
  const ctx = audioCtx(), t = now();
  const g = ctx.createGain(), p = ctx.createStereoPanner();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(Math.max(0.0001, sp.gain), t + 0.3);
  p.pan.value = sp.pan;
  g.connect(p).connect(dest());
  const saw = ctx.createOscillator(), pulse = ctx.createOscillator(), sg = ctx.createGain(), pg = ctx.createGain();
  saw.type = "sawtooth"; pulse.type = "square";
  saw.frequency.value = freq(sp.rev || 0); pulse.frequency.value = freq(sp.rev || 0) * 1.5;
  sg.gain.value = 0.22; pg.gain.value = 0.06;
  const lp = filter(saw, "lowpass", 400 + 1400 * (sp.rev || 0), 4);
  lp.connect(sg).connect(g);
  pulse.connect(pg).connect(g);
  const churn = noise(t, 3600, "bandpass", 500, 0.8, true), cg = ctx.createGain();
  cg.gain.value = 0.05 + 0.12 * (sp.rev || 0);
  churn.connect(cg).connect(g);
  saw.start(t); pulse.start(t);
  const srcs = [saw, pulse];
  let last = { pan: sp.pan, gain: sp.gain, rev: sp.rev || 0 };
  return {
    // Every frame; only an audible change schedules anything (AudioParam events pile up).
    update(s) {
      const rev = s.rev ?? last.rev;
      if (Math.abs(s.gain - last.gain) < 0.01 && Math.abs(s.pan - last.pan) < 0.01 && Math.abs(rev - last.rev) < 0.02) return;
      last = { pan: s.pan, gain: s.gain, rev };
      const k = audioCtx().currentTime;
      g.gain.setTargetAtTime(Math.max(0.0001, s.gain), k, 0.1);
      p.pan.setTargetAtTime(s.pan, k, 0.1);
      saw.frequency.setTargetAtTime(freq(rev), k, 0.15);
      pulse.frequency.setTargetAtTime(freq(rev) * 1.5, k, 0.15);
      lp.frequency?.setTargetAtTime(400 + 1400 * rev, k, 0.15);
      cg.gain.setTargetAtTime(0.05 + 0.12 * rev, k, 0.2);
    },
    stop(fade = 0.5) {
      const k = audioCtx().currentTime;
      g.gain.setTargetAtTime(0.0001, k, fade / 4);
      for (const s of srcs) s.stop(k + fade + 0.1);
    },
  };
}
