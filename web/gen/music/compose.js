// Seeded composer: a spec (key, scale, tempo, parts) and a seed -> a pattern of note
// events, the same for the same seed on every client. Pure; runs in Node.
//
// spec: { bpm, bars, root (MIDI), scale (SCALES name), progression ([degree per bar]),
//         loop, swing (0..1, delays off-beat 8ths), parts: [{ role, voice, ...options }] }
// Anything left out is chosen from the seed. Roles:
//   bass    { octave }                          chord roots (and fifths) on a seeded rhythm
//   chords  { style: pad|stab|comp, seventh, octave }
//   arp     { step (beats), octave, shape: up|down|updown }
//   lead    { density (0..1), octave, range (degrees) }   motif-based melody, lands on chord tones
//   drums   { style: chip|bossa|march|rock }  midi 36 kick, 37 rim, 38 snare, 42 hat, 46 open hat
// Every part draws from its own seeded stream, so adding a part leaves the others alone.
// Returns { bpm, beats, loop, root, scale, progression, events: [{ beat, dur, voice, midi, vel }] }.
import { rng } from "../rng.js";
import { SCALES, minorish, note, chord, hashString } from "./theory.js";

export const BEATS_PER_BAR = 4;
export const KICK = 36, RIM = 37, SNARE = 38, HAT = 42, OPEN_HAT = 46;

const PROGRESSIONS = {
  major: [[0, 4, 5, 3], [0, 5, 3, 4], [0, 3, 4, 4], [1, 4, 0, 0], [0, 5, 1, 4], [0, 3, 0, 4]],
  minor: [[0, 5, 2, 6], [0, 3, 4, 0], [0, 6, 5, 4], [0, 3, 6, 2], [0, 5, 3, 4]],
};
const BASS_RHYTHMS = [[0, 2], [0, 1.5, 2, 3.5], [0, 0.75, 2, 2.5, 3], [0, 1, 2, 3], [0, 1.5, 3]];
const COMP_RHYTHMS = [[0, 1.5, 3], [0.5, 1.5, 2.5, 3.5], [0, 2.5], [1, 3]];
const CELLS = [[1], [0.5, 0.5], [0.75, 0.25], [0.25, 0.25, 0.5], [0.5, 0.25, 0.25], [2]];

export function compose(spec = {}, seed = 1) {
  const r = rng(seed);
  const scaleName = spec.scale ?? r.pick(Object.keys(SCALES));
  const scale = SCALES[scaleName];
  if (!scale) throw new Error(`no scale "${scaleName}"`);
  const root = spec.root ?? 48 + r.int(0, 11);
  const bpm = spec.bpm ?? Math.round(r.range(84, 136));
  const bars = spec.bars ?? 4;
  const progression = spec.progression ?? r.pick(PROGRESSIONS[minorish(scale) ? "minor" : "major"]);
  const k = { root, scale, bars, progression, chordAt: (bar) => progression[bar % progression.length] };
  const events = [];
  (spec.parts || []).forEach((p, i) => {
    const pr = rng((seed ^ hashString(`${p.role}:${i}`)) >>> 0);
    const role = ROLES[p.role];
    if (!role) throw new Error(`no part role "${p.role}"`);
    for (const e of role(p, k, pr)) events.push({ ...e, voice: p.voice ?? p.role, vel: +(e.vel * (p.gain ?? 1)).toFixed(3) });
  });
  const swing = spec.swing ?? 0;
  for (const e of events) if (swing && Math.abs((e.beat % 1) - 0.5) < 1e-9) e.beat += swing * 0.25;
  events.sort((a, b) => a.beat - b.beat || a.midi - b.midi || (a.voice < b.voice ? -1 : 1));
  return { bpm, beats: bars * BEATS_PER_BAR, loop: !!spec.loop, root, scale: scaleName, progression, events };
}

const ROLES = {
  bass(p, k, r) {
    const out = [], rh = p.rhythm ?? r.pick(BASS_RHYTHMS), base = k.root - 12 + (p.octave ?? 0);
    for (let bar = 0; bar < k.bars; bar++) {
      const d = k.chordAt(bar);
      rh.forEach((b, i) => {
        const next = rh[i + 1] ?? BEATS_PER_BAR;
        const deg = i > 0 && r.next() < 0.3 ? d + 4 : d; // the fifth now and then
        out.push({ beat: bar * BEATS_PER_BAR + b, dur: (next - b) * 0.85, midi: note(base, k.scale, deg), vel: i === 0 ? 0.9 : 0.7 });
      });
    }
    return out;
  },
  chords(p, k, r) {
    const out = [], style = p.style ?? r.pick(["pad", "stab", "comp"]), base = k.root + (p.octave ?? 0);
    const rh = style === "pad" ? [0] : style === "stab" ? [0.5, 1.5, 2.5, 3.5] : p.rhythm ?? r.pick(COMP_RHYTHMS);
    for (let bar = 0; bar < k.bars; bar++) {
      const tones = chord(base, k.scale, k.chordAt(bar), p.seventh ? 4 : 3);
      rh.forEach((b, i) => {
        const dur = style === "pad" ? BEATS_PER_BAR : style === "stab" ? 0.3 : ((rh[i + 1] ?? BEATS_PER_BAR) - b) * 0.8;
        for (const m of tones) out.push({ beat: bar * BEATS_PER_BAR + b, dur, midi: m, vel: style === "pad" ? 0.35 : 0.5 });
      });
    }
    return out;
  },
  arp(p, k, r) {
    const out = [], step = p.step ?? 0.25, shape = p.shape ?? r.pick(["up", "down", "updown"]), base = k.root + (p.octave ?? 12);
    const per = Math.round(BEATS_PER_BAR / step);
    for (let bar = 0; bar < k.bars; bar++) {
      const d = k.chordAt(bar), tones = [...chord(base, k.scale, d, 3), note(base, k.scale, d + 7)];
      const seq = shape === "up" ? tones : shape === "down" ? [...tones].reverse() : [...tones, ...tones.slice(1, -1).reverse()];
      for (let s = 0; s < per; s++) out.push({ beat: bar * BEATS_PER_BAR + s * step, dur: step * 0.8, midi: seq[s % seq.length], vel: s % 4 === 0 ? 0.6 : 0.4 });
    }
    return out;
  },
  // A one-bar rhythm motif, repeated (or re-rolled) per bar; pitches walk the scale and
  // land on a chord tone on beats 0 and 2. The last note resolves to the tonic.
  lead(p, k, r) {
    const out = [], density = p.density ?? 0.7, base = k.root + (p.octave ?? 12), range = p.range ?? 7;
    const motif = () => {
      const cells = [];
      for (let b = 0; b < BEATS_PER_BAR;) {
        const c = r.pick(CELLS.filter((x) => x.reduce((a, y) => a + y, 0) <= BEATS_PER_BAR - b));
        cells.push({ at: b, steps: c, rest: r.next() > density });
        b += c.reduce((a, y) => a + y, 0);
      }
      return cells;
    };
    let m = motif(), deg = 0;
    for (let bar = 0; bar < k.bars; bar++) {
      if (bar > 0 && r.next() < 0.4) m = motif();
      const c = k.chordAt(bar), tones = [c, c + 2, c + 4];
      for (const cell of m) {
        let t = cell.at;
        for (const len of cell.steps) {
          const beat = bar * BEATS_PER_BAR + t;
          if (!cell.rest) {
            deg += r.pick([-2, -1, -1, 1, 1, 2, 0, 3, -3]);
            if (t === 0 || t === 2) deg = tones.reduce((best, x) => { const y = x + 7 * Math.round((deg - x) / 7); return Math.abs(y - deg) < Math.abs(best - deg) ? y : best; }, tones[0]);
            deg = Math.max(-2, Math.min(range, deg));
            out.push({ beat, dur: len * 0.9, midi: note(base, k.scale, deg), vel: t % 1 === 0 ? 0.7 : 0.55 });
          }
          t += len;
        }
      }
    }
    if (out.length) out[out.length - 1].midi = note(base, k.scale, 0);
    return out;
  },
  drums(p, k, r) {
    const out = [], style = p.style ?? r.pick(["chip", "bossa", "march", "rock"]);
    const hit = (beat, midi, vel) => out.push({ beat, dur: 0.25, midi, vel });
    for (let bar = 0; bar < k.bars; bar++) {
      const b0 = bar * BEATS_PER_BAR, fill = bar === k.bars - 1 && r.next() < 0.6;
      if (style === "bossa") {
        for (const b of [0, 1.5, 2, 3.5]) hit(b0 + b, KICK, 0.6);
        for (const b of [0, 0.75, 1.5, 2.5, 3]) hit(b0 + b, RIM, 0.45); // clave-ish
        for (let s = 0; s < 8; s++) hit(b0 + s / 2, HAT, s % 2 ? 0.2 : 0.3);
      } else if (style === "march") {
        hit(b0, KICK, 0.8); hit(b0 + 2, KICK, 0.7);
        for (const b of [1, 3, 3.5, 3.75]) hit(b0 + b, SNARE, b === 1 || b === 3 ? 0.6 : 0.35);
      } else {
        const kicks = style === "rock" ? [0, 1.5, 2] : [0, 2, 2.5];
        for (const b of kicks) hit(b0 + b, KICK, 0.8);
        hit(b0 + 1, SNARE, 0.6); hit(b0 + 3, SNARE, 0.6);
        for (let s = 0; s < 8; s++) hit(b0 + s / 2, s === 7 && style === "rock" ? OPEN_HAT : HAT, s % 2 ? 0.25 : 0.4);
      }
      if (fill) for (const b of [3.25, 3.5, 3.75]) hit(b0 + b, SNARE, 0.5);
    }
    return out;
  },
};

// A hand-written phrase as a pattern: notes are [beat, dur, voice, midi, vel].
export const phrase = (bpm, beats, notes, loop = false) => ({
  bpm, beats, loop, events: notes.map(([beat, dur, voice, midi, vel = 0.7]) => ({ beat, dur, voice, midi, vel })).sort((a, b) => a.beat - b.beat),
});

// Length of a pattern in seconds (one cycle).
export const seconds = (p) => (p.beats * 60) / p.bpm;
