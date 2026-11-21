// Step sequencer clock: turns patterns (beat-timed note events) into absolute-time
// notes a little ahead of `now`, so a WebAudio player can schedule them sample-exactly.
// Pure: no WebAudio, the caller passes the clock. gen/music/player.js drives it.
//
// pattern: { bpm, beats, loop, events: [{ beat, dur, voice, midi, vel }] }  (compose.js)
// pump(now) returns [{ track, time, dur, voice, midi, vel }] for every note starting in
// [what was already scheduled, now + lookahead); each note is returned exactly once.
export function createSequencer({ lookahead = 0.2 } = {}) {
  const tracks = new Set();
  let nextId = 1;
  return {
    // Start a pattern at absolute time `at`. loop repeats it until stop().
    add(pattern, at, { loop = !!pattern.loop } = {}) {
      const tr = { id: nextId++, pattern, start: at, loop, until: Infinity, cursor: at, spb: 60 / pattern.bpm };
      tracks.add(tr);
      return tr;
    },
    // No notes from `at` on; the track goes away once its scheduled notes have started.
    stop(tr, at) { if (tr) tr.until = Math.min(tr.until, at); },
    pump(now) {
      const out = [], horizon = now + lookahead;
      for (const tr of tracks) {
        const len = tr.pattern.beats * tr.spb, end = Math.min(tr.until, tr.loop ? Infinity : tr.start + len);
        const to = Math.min(horizon, end);
        if (to > tr.cursor) {
          // Cycles overlapping [cursor, to).
          for (let c = Math.max(0, Math.floor((tr.cursor - tr.start) / len)); tr.start + c * len < to; c++) {
            if (!tr.loop && c > 0) break;
            const base = tr.start + c * len;
            for (const e of tr.pattern.events) {
              const time = base + e.beat * tr.spb;
              if (time >= tr.cursor && time < to) out.push({ track: tr, time, dur: e.dur * tr.spb, voice: e.voice, midi: e.midi, vel: e.vel });
            }
          }
          tr.cursor = to;
        }
        if (tr.cursor >= end) tracks.delete(tr);
      }
      return out.sort((a, b) => a.time - b.time);
    },
    get size() { return tracks.size; },
    has: (tr) => tracks.has(tr),
  };
}
