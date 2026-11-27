// What counterslop's music sounds like: loops (composed from a spec + seed by
// gen/music/compose.js) and stingers (short hand-written phrases). Voices are
// gen/music/instruments.js names. Loops never play during live rounds (game/music.js),
// but for a race's own track.
import { compose, phrase } from "../../gen/music/compose.js";
import { hashString } from "../../gen/music/theory.js";
import { rng } from "../../gen/rng.js";

// id -> { name, seed, lowpass (lo-fi Hz), gain, spec }
export const LOOPS = {
  // Freeze / buy time: lo-fi chiptune elevator bossa (Imaj7 vi7 ii7 V7 in F).
  buytime: { name: "Code Freeze Elevator", seed: 0xe1e7a7, lowpass: 2200, gain: 0.9, spec: {
    loop: true, bpm: 92, bars: 8, root: 53, scale: "major", progression: [0, 5, 1, 4], swing: 0.35, parts: [
      { role: "bass", voice: "tri", rhythm: [0, 1.5, 2, 3.5] },
      { role: "chords", voice: "epiano", style: "comp", seventh: true },
      { role: "lead", voice: "pulse", density: 0.55, octave: 12 },
      { role: "drums", voice: "kit", style: "bossa", gain: 0.7 },
    ] } },
  // Warmup: a busy compile-loop arp in D dorian.
  warmup: { name: "Warmup Compile", seed: 0xc0a1e, lowpass: 3200, gain: 0.8, spec: {
    loop: true, bpm: 124, bars: 4, root: 50, scale: "dorian", progression: [0, 3, 6, 4], parts: [
      { role: "bass", voice: "tri", rhythm: [0, 0.75, 2, 2.5, 3] },
      { role: "arp", voice: "chip", step: 0.25, shape: "updown" },
      { role: "lead", voice: "pulse", density: 0.35, octave: 12 },
      { role: "drums", voice: "kit", style: "chip", gain: 0.8 },
    ] } },
  // GPU Drop's freeze / buy time: superstore muzak. Smooth, swung, a vibraphone-ish bell
  // melody over electric piano in Bb, a walking-ish bass and brushes (Imaj7 iii7 vi7 ii7 V7).
  muzak: { name: "Doorbuster Muzak", seed: 0xd00b57, lowpass: 2600, gain: 0.85, spec: {
    loop: true, bpm: 84, bars: 8, root: 58, scale: "major", progression: [0, 2, 5, 1, 4, 4, 0, 4], swing: 0.4, parts: [
      { role: "bass", voice: "tri", rhythm: [0, 1, 2, 3] },
      { role: "chords", voice: "epiano", style: "comp", seventh: true, octave: -12 },
      { role: "chords", voice: "pad", style: "pad", seventh: true, gain: 0.5 },
      { role: "lead", voice: "bell", density: 0.5, octave: 12, range: 6 },
      { role: "drums", voice: "kit", style: "bossa", gain: 0.45 },
    ] } },
  // Races (race mode, live: the one loop that plays under live play, game/music.js):
  // one track a race of the match, in turn. Original race rock: power chords, a lead
  // guitar, driving drums.
  race1: { name: "Merge Conflict Motorway", seed: 0x4ace01, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 168, bars: 8, root: 40, scale: "minor", progression: [0, 0, 5, 6, 0, 0, 3, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.5, 1, 1.5, 2, 2.5, 3, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.7, octave: 24, range: 7 },
      { role: "drums", voice: "kit", style: "drive", gain: 0.9 },
    ] } },
  race2: { name: "Hotfix Highway", seed: 0x407f1c, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 152, bars: 8, root: 45, scale: "dorian", progression: [0, 6, 3, 4, 0, 6, 2, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.75, 1.5, 2, 2.75, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.6, octave: 24, range: 6 },
      { role: "arp", voice: "chip", step: 0.25, shape: "updown", gain: 0.35 },
      { role: "drums", voice: "kit", style: "rock", gain: 0.9 },
    ] } },
  race3: { name: "Friday Deploy Drift", seed: 0xf41da4, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 176, bars: 8, root: 38, scale: "minor", progression: [0, 3, 6, 4, 0, 3, 5, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.5, 1, 1.5, 2, 2.5, 3, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.8, octave: 24, range: 8 },
      { role: "chords", voice: "pad", style: "pad", gain: 0.3 },
      { role: "drums", voice: "kit", style: "drive", gain: 1 },
    ] } },
  // Star Slop (tickets mode): an original space opera. Warmup and freeze: the overture,
  // brass over strings in lydian (the bright raised fourth: I-II), timpani and a march.
  overture: { name: "Minimum Viable Overture", seed: 0x57a2510, lowpass: 0, gain: 0.85, spec: {
    loop: true, bpm: 96, bars: 8, root: 50, scale: "lydian", progression: [0, 1, 0, 1, 5, 4, 1, 4], parts: [
      { role: "chords", voice: "strings", style: "pad", octave: -12 },
      { role: "bass", voice: "timp", rhythm: [0, 2.5, 3], octave: -12, gain: 0.8 },
      { role: "lead", voice: "brass", density: 0.75, octave: 12, range: 7 },
      { role: "arp", voice: "bell", step: 0.5, shape: "up", octave: 24, gain: 0.35 },
      { role: "drums", voice: "kit", style: "march", gain: 0.55 },
    ] } },
  // Aboard MVP-1 in flight (the one other loop under live play): the ride, a driving
  // string ostinato in minor, brass on top, timpani on the bar.
  liftoff: { name: "Escape Velocity Standup", seed: 0x11f70ff, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 132, bars: 8, root: 43, scale: "minor", progression: [0, 5, 6, 4, 0, 5, 3, 4], parts: [
      { role: "arp", voice: "strings", step: 0.25, shape: "updown", octave: 0, gain: 0.8 },
      { role: "chords", voice: "pad", style: "pad", gain: 0.45 },
      { role: "bass", voice: "timp", rhythm: [0, 0.5, 2], octave: -12, gain: 0.9 },
      { role: "lead", voice: "brass", density: 0.65, octave: 12, range: 7 },
      { role: "drums", voice: "kit", style: "rock", gain: 0.5 },
    ] } },
  // Match over: a slow brass anthem.
  matchend: { name: "Merged to Main", seed: 0x3e76ed, lowpass: 0, gain: 0.9, spec: {
    loop: true, bpm: 78, bars: 8, root: 55, scale: "major", progression: [0, 4, 5, 3, 0, 3, 4, 4], parts: [
      { role: "chords", voice: "pad", style: "pad", seventh: false },
      { role: "bass", voice: "tri", rhythm: [0, 2, 3] },
      { role: "lead", voice: "brass", density: 0.75, octave: 12, range: 6 },
      { role: "drums", voice: "kit", style: "march", gain: 0.8 },
    ] } },
};

// An MVP's own anthem, seeded by their name (like a music kit): key, scale, tempo,
// voices and drum style all come from the name.
export function mvpLoop(name) {
  const seed = hashString(`mvp:${name}`), r = rng(seed);
  const lead = r.pick(["pulse", "brass", "bell"]), comp = r.pick(["pad", "epiano"]);
  const parts = [
    { role: "bass", voice: "tri" },
    { role: "chords", voice: comp, style: comp === "pad" ? "pad" : "comp", seventh: r.next() < 0.5 },
    { role: "lead", voice: lead, density: r.range(0.55, 0.85), octave: 12 },
    { role: "drums", voice: "kit", gain: 0.8 },
  ];
  if (r.next() < 0.5) parts.push({ role: "arp", voice: "chip", step: r.pick([0.25, 0.5]), gain: 0.7 });
  return { name: `${name}'s anthem`, seed, lowpass: lead === "pulse" ? 3000 : 0, gain: 0.9, spec: { loop: true, bars: 4, parts } };
}

// A loop's pattern by id ("buytime", "warmup", "matchend", "mvp:<name>").
export function loopTheme(id) {
  const def = id.startsWith("mvp:") ? mvpLoop(id.slice(4)) : LOOPS[id];
  if (!def) throw new Error(`no music loop "${id}"`);
  return { ...def, pattern: compose(def.spec, def.seed) };
}

// id -> (arg) => { name, delay (s after the event), pattern }. A few seconds at most.
export const STINGERS = {
  // A rising arpeggio that starts a step higher for every kill in a multi-kill.
  kill: (multi = 1) => {
    const m = Math.max(1, Math.min(5, multi | 0)), root = 72 + 2 * (m - 1), steps = [0, 4, 7, 12, 16, 19, 24];
    const notes = steps.slice(0, 2 + m).map((s, i) => [i * 0.5, 0.5, "chip", root + s, 0.5 + 0.08 * i]);
    notes.push([(1 + m) * 0.5, 1.5, "bell", root + steps[1 + m], 0.7]);
    return { name: `kill x${m}`, delay: 0, pattern: phrase(240, (1 + m) * 0.5 + 2, notes) };
  },
  // A bright two-bell ping on top of the kill arp.
  headshot: () => ({ name: "headshot", delay: 0.05, pattern: phrase(240, 3, [[0, 0.5, "bell", 96, 0.8], [0.5, 1.5, "bell", 103, 0.6], [0, 0.25, "kit", 37, 0.8]]) }),
  // The Big Diff is force-pushed: a low tritone pulse under a diminished pad.
  plant: () => ({ name: "Big Diff planted", delay: 0.45, pattern: phrase(100, 4, [
    [0, 0.5, "tri", 40, 0.9], [0.5, 0.5, "tri", 41, 0.8], [1, 0.5, "tri", 40, 0.9], [1.5, 2, "tri", 46, 0.8],
    [1, 2.5, "pad", 52, 0.8], [1, 2.5, "pad", 55, 0.8], [1, 2.5, "pad", 58, 0.8],
  ]) }),
  // git revert: an undo run downwards, then a calm major chord.
  defuse: () => ({ name: "revert", delay: 0.9, pattern: phrase(160, 4, [
    [0, 0.25, "chip", 84], [0.25, 0.25, "chip", 81], [0.5, 0.25, "chip", 77], [0.75, 0.25, "chip", 74],
    [1.25, 2.5, "bell", 72, 0.6], [1.25, 2.5, "bell", 76, 0.5], [1.25, 2.5, "bell", 79, 0.5], [1.25, 2.5, "pad", 60, 0.7],
  ]) }),
  // Round won: a short brass fanfare with a snare pickup.
  roundWon: () => ({ name: "round won", delay: 0, pattern: phrase(150, 5, [
    [0, 0.3, "brass", 60], [0.33, 0.3, "brass", 64], [0.67, 0.3, "brass", 67],
    [1, 2, "brass", 72, 0.9], [1, 2, "brass", 64, 0.6], [1, 2, "brass", 67, 0.6], [1, 2, "tri", 48, 0.9],
    [0.25, 0.25, "kit", 38, 0.4], [0.5, 0.25, "kit", 38, 0.5], [0.75, 0.25, "kit", 38, 0.6], [1, 0.25, "kit", 36, 0.9],
  ]) }),
  // GPU Drop, a GPU secured: the till rings "ka-ching" in bells over a brass stab.
  secure: () => ({ name: "GPU secured", delay: 0.35, pattern: phrase(180, 4, [
    [0, 0.25, "bell", 84, 0.6], [0.25, 0.25, "bell", 88, 0.6], [0.5, 2, "bell", 91, 0.8], [0.5, 2, "bell", 96, 0.6],
    [0.5, 0.6, "brass", 60, 0.8], [0.5, 0.6, "brass", 64, 0.6], [0.5, 0.6, "brass", 67, 0.6], [0.5, 0.25, "kit", 36, 0.8],
  ]) }),
  // GPU Drop, round won: the Black Friday doorbuster jingle: "DOOR-BUS-TER DEALS!"
  // on brass and bells, a cash-register rim shot and a big sale chord.
  blackFriday: () => ({ name: "Black Friday", delay: 0, pattern: phrase(160, 7, [
    [0, 0.45, "brass", 67], [0.5, 0.45, "brass", 72], [1, 0.45, "brass", 76], [1.5, 1, "brass", 79, 0.9],
    [0, 0.45, "bell", 79, 0.4], [0.5, 0.45, "bell", 84, 0.4], [1, 0.45, "bell", 88, 0.4], [1.5, 1, "bell", 91, 0.5],
    [2.75, 0.25, "brass", 77, 0.7], [3, 0.25, "brass", 76, 0.7], [3.25, 0.25, "brass", 74, 0.7],
    [3.5, 3, "brass", 72, 0.9], [3.5, 3, "brass", 76, 0.7], [3.5, 3, "brass", 79, 0.7], [3.5, 3, "pad", 60, 0.8], [3.5, 3, "tri", 48, 0.9],
    [3.5, 3, "bell", 96, 0.5], [0, 0.25, "kit", 36, 0.8], [1.5, 0.25, "kit", 38, 0.6], [3.25, 0.25, "kit", 37, 0.7], [3.5, 0.25, "kit", 36, 0.9],
  ]) }),
  // Star Slop: the doors shut with us aboard: a timpani roll under rising brass fifths.
  launch: () => ({ name: "liftoff", delay: 0.2, pattern: phrase(120, 6, [
    [0, 0.25, "timp", 43, 0.5], [0.25, 0.25, "timp", 43, 0.6], [0.5, 0.25, "timp", 43, 0.7], [0.75, 0.25, "timp", 43, 0.8],
    [1, 0.75, "brass", 55, 0.8], [1.75, 0.75, "brass", 62, 0.85], [2.5, 2.5, "brass", 67, 0.95],
    [2.5, 2.5, "brass", 71, 0.7], [2.5, 2.5, "brass", 74, 0.7], [2.5, 3, "strings", 55, 0.9], [2.5, 3, "strings", 62, 0.8],
    [2.5, 1, "timp", 31, 1], [2.5, 0.25, "kit", 36, 0.9],
  ]) }),
  // Round lost: the sad trombone.
  roundLost: () => ({ name: "round lost", delay: 0, pattern: phrase(90, 4.5, [
    [0, 0.7, "wah", 62], [0.75, 0.7, "wah", 61], [1.5, 0.7, "wah", 60], [2.25, 2, "wah", 59, 0.9],
  ]) }),
};
