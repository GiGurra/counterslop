// Music playback: one loop at a time (setLoop, idempotent, called every frame by
// game/music.js) and stingers on top, through the gen/music player. state() is what
// the test API and the gallery show. Definitions are in themes.js.
import { createMusicPlayer } from "../../gen/music/player.js";
import { seconds } from "../../gen/music/compose.js";
import { LOOPS, STINGERS, RACE_LOOP_IDS, loopTheme, mvpLoop } from "./themes.js";

export { LOOPS, STINGERS, RACE_LOOP_IDS, loopTheme, mvpLoop };
export { RACE_TRACKS, RACE_MAP_TRACKS, raceLoopId, raceTrack } from "./themes.js";

export const LOOP_FADE = 1.5; // seconds: loops fade out when the round goes live
export const SEGUE_FADE = 0.06; // seconds: a segue's old loop fades this fast at the bar line (no click)

export function createMusic({ player = createMusicPlayer() } = {}) {
  let enabled = true, want = null, cur = null, last = null, count = 0, segues = 0;
  const themes = new Map();
  const theme = (id) => {
    if (!themes.has(id)) { if (themes.size > 16) themes.delete(themes.keys().next().value); themes.set(id, loopTheme(id)); }
    return themes.get(id);
  };
  // A change within one segue family (a race track: its heats and its final-lap lift,
  // themes.js segue) waits for the old loop's next bar line: the old one stops there
  // and the new one starts on it, in time. Any other change fades the old one out and
  // starts now.
  function sync(fade = LOOP_FADE) {
    const id = enabled ? want : null;
    let at = 0;
    if (cur && cur.id !== id) {
      if (id && cur.track && theme(cur.id).segue && theme(cur.id).segue === theme(id).segue) { at = player.nextBar(cur.track); player.stop(cur.track, SEGUE_FADE, at); }
      else player.stop(cur.track, fade);
      cur = null;
      segues += at ? 1 : 0;
    }
    if (id && !cur?.track) {
      const t = theme(id);
      cur = { id, track: player.play(t.pattern, { lowpass: t.lowpass, gain: t.gain, at }) };
    }
  }
  return {
    set({ enabled: e = enabled, volume } = {}) {
      if (volume !== undefined) player.setVolume(volume);
      if (!!e !== enabled) { enabled = !!e; sync(0.3); }
    },
    // The loop that should play now (an id or null); a change fades the old one out.
    setLoop(id) { want = id || null; sync(); },
    // Play a stinger now (after its own delay). Returns whether music is on.
    stinger(id, arg) {
      const def = STINGERS[id];
      if (!def) throw new Error(`no music stinger "${id}"`);
      if (!enabled) return false;
      const s = def(arg);
      player.play(s.pattern, { delay: s.delay });
      last = id; count++;
      return true;
    },
    // Gallery / play_music: one pass of a loop or a stinger, whatever the phase.
    preview(id, arg) {
      if (STINGERS[id]) { const s = STINGERS[id](arg); player.play(s.pattern, { delay: 0 }); return s.name; }
      const t = theme(id);
      player.play({ ...t.pattern, loop: false }, { lowpass: t.lowpass, gain: t.gain });
      return t.name;
    },
    state: () => ({ on: enabled, want, loop: enabled ? want : null, playing: !!cur?.track, last, stingers: count, segues, tracks: player.tracks }),
  };
}

// Gallery / play_music: every loop and stinger by name with how long one pass lasts.
export function catalog() {
  return {
    loops: Object.fromEntries([...Object.keys(LOOPS), ...RACE_LOOP_IDS, "mvp:alice", "mvp:bob"].map((id) => { const t = loopTheme(id); return [id, { name: t.name, seconds: +seconds(t.pattern).toFixed(2) }]; })),
    stingers: Object.fromEntries(Object.entries(STINGERS).map(([id, f]) => { const s = f(); return [id, { name: s.name, seconds: +(s.delay + seconds(s.pattern)).toFixed(2) }]; })),
  };
}
