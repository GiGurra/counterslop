// What counterslop's music sounds like: loops (composed from a spec + seed by
// gen/music/compose.js) and stingers (short hand-written phrases). Voices are
// gen/music/instruments.js names. Loops never play during live rounds (game/music.js),
// but for a race's own track (RACE_TRACKS) and Star Slop's ride.
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

// Race tracks (race mode, live: the one loop that plays under live play,
// game/music.js). Each race map has its own (RACE_MAP_TRACKS); the races of a
// match play variations of it and the final lap lifts it (raceTrackLoop). Original
// race rock: power chords, a lead guitar, driving drums; parkour (a race on foot)
// gets breakbeat chiptune instead. lift: the drum style the final lap switches to.
export const RACE_TRACKS = {
  // rc_merge_train: E minor at full tilt, the lead high and busy.
  merge: { name: "Merge Conflict Motorway", seed: 0x4ace01, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 168, bars: 8, root: 40, scale: "minor", progression: [0, 0, 5, 6, 0, 0, 3, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.5, 1, 1.5, 2, 2.5, 3, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.7, octave: 24, range: 7 },
      { role: "drums", voice: "kit", style: "drive", gain: 0.9 },
    ] } },
  // rc_hotfix_highway: dorian at dusk, a little slower, a chip arp under the lead.
  hotfix: { name: "Hotfix Highway", seed: 0x407f1c, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 152, bars: 8, root: 45, scale: "dorian", progression: [0, 6, 3, 4, 0, 6, 2, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.75, 1.5, 2, 2.75, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.6, octave: 24, range: 6 },
      { role: "arp", voice: "chip", step: 0.25, shape: "updown", gain: 0.35 },
      { role: "drums", voice: "kit", style: "rock", gain: 0.9 },
    ] } },
  // lab_race and any other race map in cars: the fastest, a pad behind the guitars.
  friday: { name: "Friday Deploy Drift", seed: 0xf41da4, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 176, bars: 8, root: 38, scale: "minor", progression: [0, 3, 6, 4, 0, 3, 5, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.5, 1, 1.5, 2, 2.5, 3, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.8, octave: 24, range: 8 },
      { role: "chords", voice: "pad", style: "pad", gain: 0.3 },
      { role: "drums", voice: "kit", style: "drive", gain: 1 },
    ] } },
  // rc_ci_pipeline: a bright mixolydian rocker (the flat seventh: I bVII IV I), a
  // pipeline going green, a chip arp ticking like a build log.
  pipeline: { name: "Green Build Getaway", seed: 0x9ee7b1d, lowpass: 0, gain: 0.8, spec: {
    loop: true, bpm: 160, bars: 8, root: 40, scale: "mixolydian", progression: [0, 6, 3, 0, 0, 6, 4, 4], parts: [
      { role: "chords", voice: "gtr", style: "power", gain: 0.9 },
      { role: "bass", voice: "tri", rhythm: [0, 0.5, 1.5, 2, 2.5, 3.5], octave: -12 },
      { role: "lead", voice: "lead", density: 0.65, octave: 24, range: 7 },
      { role: "arp", voice: "chip", step: 0.5, shape: "up", octave: 24, gain: 0.3 },
      { role: "drums", voice: "kit", style: "rock", gain: 0.9 },
    ] } },
  // Parkour (pk_ maps, lab_parkour: on foot, hooks, low gravity): breakbeat chiptune,
  // an acid bassline under a square lead and a 16th arp, a pad for the floaty bits.
  parkour: { name: "Happy Path Sprint", seed: 0x9a7c0de, lowpass: 0, gain: 1.7, lift: "breakHeavy", spec: {
    loop: true, bpm: 146, bars: 8, root: 42, scale: "minor", progression: [0, 0, 5, 3, 0, 0, 6, 4], parts: [
      { role: "bass", voice: "acid", rhythm: [0, 0.5, 0.75, 1.5, 2, 2.5, 3, 3.25, 3.5], octave: -12 },
      { role: "arp", voice: "chip", step: 0.25, shape: "updown", octave: 12, gain: 0.9 },
      { role: "lead", voice: "pulse", density: 0.6, octave: 24, range: 7, gain: 1.3 },
      { role: "chords", voice: "pad", style: "pad", gain: 0.6 },
      { role: "drums", voice: "kit", style: "break", gain: 0.95 },
    ] } },
};

// Which track a race map plays (by map id). Any other race map: on foot (a
// parkour course) plays "parkour", in cars the default.
export const RACE_MAP_TRACKS = {
  rc_ci_pipeline: "pipeline", rc_merge_train: "merge", rc_hotfix_highway: "hotfix", lab_race: "friday",
  pk_happy_path: "parkour", lab_parkour: "parkour",
};
export const DEFAULT_RACE_TRACK = "friday", PARKOUR_TRACK = "parkour";
export const RACE_VARIATIONS = 3; // races 1, 2, 3 of a match (then round again)

// The final-lap lift: this much faster, a whole step up, drums at their heaviest
// and an extra arp on top. The parts keep their seeded streams, so the lifted
// melody is the same tune, higher and faster.
export const LIFT_TEMPO = 1.07, LIFT_KEY = 2;

// A race track's loop id: "<track>:<variation 1..3>" and ":lift" for the final lap.
export const raceLoopId = (track, v = 1, lift = false) => `${track}:${v}${lift ? ":lift" : ""}`;

const rotate = (a, k) => a.map((_, i) => a[(i + k) % a.length]);
const swapDrums = { drive: "rock", rock: "drive", break: "break", heavy: "heavy" };

// A race track, variation v (the race of the match), lifted or not. 1: as written.
// 2: a new melody (its own seed), the progression's halves swapped and an arp in or
// out (parkour: its arp at half speed, falling). 3: another melody, the progression turned by two bars, the other drum feel
// and a pad under it.
export function raceTrack(track, v = 1, lift = false) {
  const def = RACE_TRACKS[track];
  if (!def) throw new Error(`no race track "${track}"`);
  let { seed, spec } = def, parts = spec.parts.map((p) => ({ ...p })), name = def.name;
  let progression = spec.progression;
  if (v === 2) {
    seed = (seed ^ 0x5eed0002) >>> 0; progression = rotate(progression, progression.length / 2);
    const arp = parts.findIndex((p) => p.role === "arp");
    if (track === PARKOUR_TRACK) parts[arp] = { ...parts[arp], shape: "down", step: 0.5 };
    else if (arp >= 0) parts.splice(arp, 1);
    else parts.push({ role: "arp", voice: "chip", step: 0.5, shape: "down", octave: 24, gain: 0.3 });
    name += " (second heat)";
  } else if (v === 3) {
    seed = (seed ^ 0x5eed0003) >>> 0; progression = rotate(progression, 2);
    parts = parts.map((p) => p.role === "drums" ? { ...p, style: swapDrums[p.style] ?? p.style } : p.role === "lead" ? { ...p, density: Math.min(0.9, (p.density ?? 0.7) + 0.1) } : p);
    if (!parts.some((p) => p.voice === "pad")) parts.push({ role: "chords", voice: "pad", style: "pad", gain: 0.3 });
    name += " (final heat)";
  }
  let bpm = spec.bpm, root = spec.root;
  if (lift) {
    bpm = Math.round(bpm * LIFT_TEMPO); root += LIFT_KEY;
    parts = parts.map((p) => p.role === "drums" ? { ...p, style: def.lift ?? "heavy", gain: Math.min(1, (p.gain ?? 1) + 0.05) } : p);
    parts.push({ role: "arp", voice: "chip", step: 0.25, shape: "up", octave: 24, gain: 0.32 });
    name += ", final lap";
  }
  return { name, seed, lowpass: def.lowpass, gain: def.gain, segue: track, spec: { ...spec, bpm, root, progression, parts } };
}

// Every race loop (the gallery, catalog()): each track's variations and its lift.
export const RACE_LOOP_IDS = Object.keys(RACE_TRACKS).flatMap((t) => [1, 2, 3].map((v) => raceLoopId(t, v)).concat(raceLoopId(t, 1, true)));

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

// A loop's pattern by id ("buytime", "warmup", "matchend", "mvp:<name>", a race
// track "hotfix:1", "parkour:3:lift").
function loopDef(id) {
  if (id.startsWith("mvp:")) return mvpLoop(id.slice(4));
  const [track, v, lift] = id.split(":");
  if (RACE_TRACKS[track] && v && (!lift || lift === "lift")) return raceTrack(track, +v, !!lift);
  return LOOPS[id];
}
export function loopTheme(id) {
  const def = loopDef(id);
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
  // Race mode (game/music.js, from the race state): over the line into a new lap (not
  // the last): a quick chip pickup into a brass-and-guitar stab, a step higher each lap.
  lap: (lap = 2) => {
    const r = 64 + 2 * Math.max(0, Math.min(4, (lap | 0) - 2));
    return { name: `lap ${Math.max(2, lap | 0)}`, delay: 0, pattern: phrase(200, 4, [
      [0, 0.25, "chip", r + 7, 0.5], [0.25, 0.25, "chip", r + 12, 0.6],
      [0.5, 1.5, "brass", r + 12, 0.85], [0.5, 1.5, "brass", r + 16, 0.6], [0.5, 1.5, "brass", r + 19, 0.6],
      [0.5, 0.4, "gtr", r - 12, 0.7], [0.5, 0.4, "gtr", r - 5, 0.6],
      [0, 0.25, "kit", 38, 0.5], [0.25, 0.25, "kit", 38, 0.6], [0.5, 0.25, "kit", 36, 0.9], [0.5, 0.25, "kit", 49, 0.5],
    ]) };
  },
  // Over the line into the final lap (or a parkour run's last stretch): a snare roll
  // under a chromatic climb to a held brass chord a whole step up, and a crash.
  finalLap: () => ({ name: "final lap", delay: 0.1, pattern: phrase(180, 6, [
    ...[0, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75].map((b, i) => [b, 0.25, "kit", 38, 0.3 + 0.07 * i]),
    [0, 0.5, "lead", 64, 0.7], [0.5, 0.5, "lead", 65, 0.75], [1, 0.5, "lead", 66, 0.8], [1.5, 0.5, "lead", 67, 0.85],
    [2, 3, "brass", 69, 0.95], [2, 3, "brass", 73, 0.7], [2, 3, "brass", 76, 0.7], [2, 1.5, "gtr", 45, 0.8], [2, 1.5, "gtr", 52, 0.7],
    [2, 0.25, "kit", 36, 1], [2, 0.25, "kit", 49, 0.7], [2, 3, "bell", 88, 0.4],
  ]) }),
  // Over the finish line: on the podium (place 1..3) a fanfare with a high bell for a
  // win; off it a shrugging little run down to a plain chord.
  finish: (place = 1) => {
    const p = Math.max(1, place | 0);
    if (p > 3) return { name: `finish ${p}th`, delay: 0.2, pattern: phrase(150, 5, [
      [0, 0.5, "pulse", 76, 1], [0.5, 0.5, "pulse", 74, 1], [1, 0.5, "pulse", 72, 1], [1.5, 0.5, "pulse", 71, 1],
      [2, 2.5, "brass", 60, 0.6], [2, 2.5, "brass", 64, 0.5], [2, 2.5, "brass", 67, 0.5], [2, 2.5, "epiano", 72, 0.9], [2, 2, "tri", 48, 1],
      [2, 0.25, "kit", 37, 0.8], [2, 0.25, "kit", 36, 0.7],
    ]) };
    const notes = [
      [0, 0.3, "brass", 67], [0.33, 0.3, "brass", 67], [0.67, 0.3, "brass", 67], [1, 0.9, "brass", 72, 0.9],
      [2, 2.5, "brass", 76, 0.9], [2, 2.5, "brass", 72, 0.7], [2, 2.5, "brass", 79, 0.7], [2, 2.5, "gtr", 48, 0.8], [2, 2.5, "gtr", 55, 0.7],
      [2, 2.5, "tri", 36, 0.9], [0, 0.25, "kit", 38, 0.5], [0.33, 0.25, "kit", 38, 0.55], [0.67, 0.25, "kit", 38, 0.6],
      [1, 0.25, "kit", 36, 0.9], [2, 0.25, "kit", 36, 1], [2, 0.25, "kit", 49, 0.8],
    ];
    if (p === 1) notes.push([2, 2.5, "bell", 91, 0.6], [2.5, 2, "bell", 96, 0.5]);
    return { name: p === 1 ? "finish: winner" : `finish: podium ${p}`, delay: 0.2, pattern: phrase(150, 5, notes) };
  },
  // Round lost: the sad trombone.
  roundLost: () => ({ name: "round lost", delay: 0, pattern: phrase(90, 4.5, [
    [0, 0.7, "wah", 62], [0.75, 0.7, "wah", 61], [1.5, 0.7, "wah", 60], [2.25, 2, "wah", 59, 0.9],
  ]) }),
};
