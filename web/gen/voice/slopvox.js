// SlopVox: a small Klatt-style formant speech synthesizer. Pure and seeded: the same
// phoneme string and options give the same samples on every client (and in Node).
//
//   glottal pulse (jitter, shimmer, tilt) + aspiration noise
//        -> cascade resonators F1 F2 F3 (moving) F4 F5 (fixed)  ─┐
//   frication noise -> bandpass (moving centre/bandwidth)  ───────┴─> DC block -> out
//
// Input is ARPAbet-ish, one token per phoneme, separated by spaces:
//   "F ER1 S T _ B AH1 G"
// Vowels take a stress digit (1 primary, 2 secondary, 0 unstressed). A leading ^ on a
// vowel shouts it (higher, longer, louder). "_" is a short pause, "," a long one.
// "~3" before a syllable marks it for the stutter effect (see fx.js): the marked span
// runs from that phoneme to the end of the next vowel.
import { PHONEMES, isVowel } from "./phonemes.js";
import { mulberry32 } from "../rng.js";

export const DEFAULTS = {
  pitch: 110,      // base f0, Hz
  rate: 1,         // speaking rate (2 = twice as fast)
  shift: 1,        // formant scale: <1 bigger/deeper, >1 smaller
  robot: 0,        // 0 natural pitch movement and jitter .. 1 flat monotone buzz
  excite: 1,       // how far stressed syllables jump in pitch
  fall: 0.18,      // pitch drop over the last vowel
  breath: 0.04,    // aspiration mixed into voiced sounds
  tilt: 0.35,      // glottal low-pass (0 bright .. 0.9 dull)
  seed: 1,
  sampleRate: 32000,
};

// Tokenize into phoneme segments with timings (seconds) and stutter marks.
export function parse(str, rate = 1) {
  const segs = [], marks = [];
  let t = 0, pendingStutter = 0;
  for (const raw of String(str).trim().split(/\s+/)) {
    if (!raw) continue;
    const st = /^~(\d+)$/.exec(raw);
    if (st) { pendingStutter = +st[1]; continue; }
    const m = /^(\^?)([A-Z]+|_|,)([012]?)$/.exec(raw.toUpperCase());
    const ph = m && PHONEMES[m[2]];
    if (!ph) throw new Error(`SlopVox: unknown phoneme "${raw}"`);
    const shout = !!m[1], stress = m[3] === "" ? (isVowel(ph) ? 1 : 0) : +m[3];
    let dur = ph.kind === "s" ? ph.clo + 8 + ph.asp : ph.kind === "x" ? ph.clo + ph.dur : ph.dur;
    if (isVowel(ph)) dur *= shout ? 1.5 : stress === 1 ? 1.15 : stress === 2 ? 0.95 : 0.6;
    dur = dur / 1000 / rate;
    if (pendingStutter) { marks.push({ count: pendingStutter, seg: segs.length }); pendingStutter = 0; }
    segs.push({ name: m[2], ph, stress, shout, t0: t, t1: t + dur });
    t += dur;
  }
  return { segs, marks, dur: t };
}

// Piecewise-linear parameter track, sampled with a forward-moving cursor.
class Track {
  constructor(v0) { this.pts = [[0, v0]]; this.i = 0; }
  add(t, v) {
    const last = this.pts[this.pts.length - 1];
    if (t < last[0]) t = last[0];
    this.pts.push([t, v]);
  }
  // Hold v over [t0, t1], leaving edge e at both ends for the transition.
  hold(t0, t1, v, e) { e = Math.min(e, (t1 - t0) * 0.45); this.add(t0 + e, v); this.add(t1 - e, v); }
  at(t) {
    const p = this.pts;
    while (this.i < p.length - 1 && p[this.i + 1][0] <= t) this.i++;
    const a = p[this.i], b = p[this.i + 1];
    if (!b || b[0] === a[0]) return a[1];
    return a[1] + (b[1] - a[1]) * ((t - a[0]) / (b[0] - a[0]));
  }
}

// Build every parameter track from the segments.
function tracks(segs, o, dur) {
  const sh = o.shift, fsh = Math.sqrt(o.shift);
  const F = [new Track(500 * sh), new Track(1500 * sh), new Track(2500 * sh)];
  const av = new Track(0), af = new Track(0), ah = new Track(0);
  const ff = new Track(4000), fb = new Track(2000), f0 = new Track(1);
  const R = 0.004; // amplitude ramp
  const setF = (t0, t1, f, e) => f.forEach((v, k) => F[k].hold(t0, t1, v * sh, e));
  const setFric = (t0, t1, fr) => { ff.hold(t0, t1, fr[0] * fsh, 0.001); fb.hold(t0, t1, fr[1] * fsh, 0.001); };
  const nextF = (i) => { for (let j = i + 1; j < segs.length; j++) if (segs[j].ph.f) return segs[j].ph.f; return [500, 1500, 2500]; };
  const vowels = segs.filter((s) => isVowel(s.ph));
  const lastVowel = vowels[vowels.length - 1];

  segs.forEach((s, i) => {
    const { ph, t0, t1 } = s, d = t1 - t0;
    switch (ph.kind) {
      case "v": case "d": {
        const e = Math.min(d * 0.3, 0.04);
        const amp = s.shout ? 1.2 : s.stress === 1 ? 1 : s.stress === 2 ? 0.9 : 0.75;
        if (ph.kind === "v") setF(t0, t1, ph.f, e);
        else { ph.f.forEach((v, k) => F[k].add(t0 + e, v * sh)); ph.f2.forEach((v, k) => F[k].add(t1 - e, v * sh)); }
        av.hold(t0, t1, amp, R); af.hold(t0, t1, 0, R); ah.hold(t0, t1, 0, R);
        // Pitch accent: a peak at the vowel's middle, by stress.
        const acc = s.shout ? 1 + 0.42 * o.excite : s.stress === 1 ? 1 + 0.2 * o.excite : s.stress === 2 ? 1 + 0.07 * o.excite : 0.97;
        f0.add(t0 + e, acc * 0.97); f0.add((t0 + t1) / 2, acc);
        f0.add(t1 - e, s === lastVowel ? acc * (1 - o.fall) : acc * 0.98);
        break;
      }
      case "a": case "n": {
        setF(t0, t1, ph.f, ph.kind === "n" ? 0.012 : d * 0.3);
        av.hold(t0, t1, ph.av, R); af.hold(t0, t1, 0, R); ah.hold(t0, t1, 0, R);
        break;
      }
      case "f": {
        setF(t0, t1, ph.f, 0.015); setFric(t0, t1, ph.fric);
        av.hold(t0, t1, ph.av, R); af.hold(t0, t1, ph.af, R); ah.hold(t0, t1, 0, R);
        break;
      }
      case "h": {
        setF(t0, t1, nextF(i), 0.005);
        av.hold(t0, t1, 0, R); af.hold(t0, t1, 0, R); ah.hold(t0, t1, ph.ah, R);
        break;
      }
      case "s": case "x": {
        const tr = t0 + (ph.clo / 1000) * (d / ((ph.kind === "s" ? ph.clo + 8 + ph.asp : ph.clo + ph.dur) / 1000));
        setF(t0, t1, ph.f, 0.002);
        // closure: silence, or a low voice bar for voiced stops
        av.hold(t0, tr, ph.voiced ? 0.18 : 0, R); af.hold(t0, tr, 0, R); ah.hold(t0, tr, 0, R);
        if (ph.kind === "s") {
          const tb = Math.min(t1, tr + 0.008 * (d / ((ph.clo + 8 + ph.asp) / 1000)));
          setFric(tr, t1, ph.fric);
          af.add(tr + 0.0005, ph.burst); af.add(tb, ph.burst * 0.3); af.add(Math.min(t1, tb + 0.006), 0);
          if (ph.asp) { ah.add(tb, 0.5); ah.add(t1 - 0.003, 0.4); }
          if (ph.voiced) { av.add(tr + 0.003, 0.5); av.add(t1 - 0.002, 0.8); }
        } else {
          setFric(tr, t1, ph.fric);
          af.add(tr + 0.002, ph.af); af.add(t1 - R, ph.af * 0.8);
          av.add(tr + 0.003, ph.av); av.add(t1 - R, ph.av);
        }
        break;
      }
      case "_": {
        av.hold(t0, t1, 0, R); af.hold(t0, t1, 0, R); ah.hold(t0, t1, 0, R);
        break;
      }
    }
  });
  for (const tr of [...F, av, af, ah, ff, fb, f0]) tr.add(dur + 1, tr.pts[tr.pts.length - 1][1]);
  return { F, av, af, ah, ff, fb, f0 };
}

// Klatt digital resonator (unity gain at DC; cascading these gives natural formant levels).
function resonator() {
  let a = 1, b = 0, c = 0, y1 = 0, y2 = 0;
  return {
    set(f, bw, sr) {
      const r = Math.exp(-Math.PI * bw / sr);
      c = -r * r; b = 2 * r * Math.cos(2 * Math.PI * f / sr); a = 1 - b - c;
    },
    run(x) { const y = a * x + b * y1 + c * y2; y2 = y1; y1 = y; return y; },
  };
}

// RBJ bandpass biquad (0 dB peak), for the frication branch.
function bandpass() {
  let b0 = 0, b2 = 0, a1 = 0, a2 = 0, x1 = 0, x2 = 0, y1 = 0, y2 = 0;
  return {
    set(f, bw, sr) {
      f = Math.min(f, sr * 0.45);
      const w = 2 * Math.PI * f / sr, q = Math.max(0.3, f / bw), al = Math.sin(w) / (2 * q), n = 1 + al;
      b0 = al / n; b2 = -al / n; a1 = -2 * Math.cos(w) / n; a2 = (1 - al) / n;
    },
    run(x) { const y = b0 * x + b2 * x2 - a1 * y1 - a2 * y2; x2 = x1; x1 = x; y2 = y1; y1 = y; return y; },
  };
}

const FRIC_GAIN = 1.4; // balances the frication branch against the voiced cascade

// Render a phoneme string. Returns { samples: Float32Array, sampleRate, marks }, where
// marks are stutter spans in samples: [{ count, s0, s1 }]. Output is normalised to 0.9 peak.
export function speak(phonemes, opts = {}) {
  const o = { ...DEFAULTS, ...opts }, sr = o.sampleRate;
  const { segs, marks, dur } = parse(phonemes, o.rate);
  const tk = tracks(segs, o, dur);
  const tail = 0.06, n = Math.ceil((dur + tail) * sr), out = new Float32Array(n);
  const rand = mulberry32(o.seed * 7919 + 17);
  const R = [resonator(), resonator(), resonator(), resonator(), resonator()];
  const bp = bandpass(), bp2 = bandpass();
  const sh = o.shift, OQ = 0.6, nat = 1 - o.robot;
  R[3].set(3500 * sh, 250, sr); R[4].set(4500 * sh, 320, sr);
  let phase = 0, jit = 1, shim = 1, lp = 0, dcx = 0, dcy = 0, av = 0, af = 0, ah = 0, f0 = o.pitch;

  for (let i = 0; i < n; i++) {
    const t = i / sr;
    if ((i & 15) === 0) { // control rate: every 16 samples
      const f1 = tk.F[0].at(t), f2 = tk.F[1].at(t), f3 = tk.F[2].at(t);
      R[0].set(f1, 50 + f1 * 0.06, sr); R[1].set(f2, 70 + f2 * 0.025, sr); R[2].set(f3, 110 + f3 * 0.02, sr);
      const fc = tk.ff.at(t), fw = tk.fb.at(t);
      bp.set(fc, fw, sr); bp2.set(fc, fw * 1.3, sr);
      av = tk.av.at(t); af = tk.af.at(t); ah = tk.ah.at(t);
      // Declining baseline, stress accents scaled toward flat by robotness, light vibrato.
      const contour = tk.f0.at(t) * (1.05 - 0.15 * (t / Math.max(dur, 0.1)));
      f0 = o.pitch * (1 + (contour - 1) * nat) * (1 + 0.012 * nat * Math.sin(2 * Math.PI * 5.3 * t));
    }
    phase += (f0 * jit) / sr;
    if (phase >= 1) { // new glottal cycle: fresh jitter and shimmer
      phase -= 1;
      jit = 1 + (rand() - 0.5) * 0.035 * nat;
      shim = 1 + (rand() - 0.5) * 0.12 * nat;
    }
    const tg = phase / OQ, open = tg < 1;
    let g = open ? 2 * tg - 3 * tg * tg : 0;
    if (o.robot > 0) g = g * nat + (open ? 1 - 2 * tg : -0.25) * 0.6 * o.robot; // buzzier pulse
    lp += (1 - o.tilt) * (g - lp);
    const nz = rand() * 2 - 1, nz2 = rand() * 2 - 1;
    let x = lp * av * shim + nz * (ah * 0.35 + av * o.breath * (open ? 1 : 0.3));
    for (let k = 0; k < 5; k++) x = R[k].run(x);
    const fr = bp2.run(bp.run(nz2 * af * (av > 0.1 ? (open ? 1 : 0.5) : 1))) * FRIC_GAIN;
    const y = x + fr;
    dcy = y - dcx + 0.995 * dcy; dcx = y; // DC block
    out[i] = dcy;
  }
  normalize(out, 0.9);
  return {
    samples: out, sampleRate: sr,
    marks: marks.map((m) => {
      let j = m.seg;
      while (j < segs.length && !isVowel(segs[j].ph)) j++;
      const end = segs[Math.min(j, segs.length - 1)];
      return { count: m.count, s0: Math.floor(segs[m.seg].t0 * sr), s1: Math.floor(end.t1 * sr) };
    }),
  };
}

export function normalize(x, peak) {
  let m = 0;
  for (let i = 0; i < x.length; i++) m = Math.max(m, Math.abs(x[i]));
  if (m > 0) for (let i = 0; i < x.length; i++) x[i] *= peak / m;
  return x;
}
