// Music playback: one loop at a time (setLoop, idempotent, called every frame by
// game/music.js) and stingers on top, through the gen/music player. state() is what
// the test API and the gallery show. Definitions are in themes.js.
import { createMusicPlayer } from "../../gen/music/player.js";
import { seconds } from "../../gen/music/compose.js";
import { LOOPS, STINGERS, loopTheme, mvpLoop } from "./themes.js";

export { LOOPS, STINGERS, loopTheme, mvpLoop };

export const LOOP_FADE = 1.5; // seconds: loops fade out when the round goes live

export function createMusic({ player = createMusicPlayer() } = {}) {
  let enabled = true, want = null, cur = null, last = null, count = 0;
  const themes = new Map();
  const theme = (id) => {
    if (!themes.has(id)) { if (themes.size > 16) themes.delete(themes.keys().next().value); themes.set(id, loopTheme(id)); }
    return themes.get(id);
  };
  function sync(fade = LOOP_FADE) {
    const id = enabled ? want : null;
    if (cur && cur.id !== id) { player.stop(cur.track, fade); cur = null; }
    if (id && !cur?.track) {
      const t = theme(id);
      cur = { id, track: player.play(t.pattern, { lowpass: t.lowpass, gain: t.gain }) };
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
    state: () => ({ on: enabled, want, loop: enabled ? want : null, playing: !!cur?.track, last, stingers: count, tracks: player.tracks }),
  };
}

// Gallery / play_music: every loop and stinger by name with how long one pass lasts.
export function catalog() {
  return {
    loops: Object.fromEntries([...Object.keys(LOOPS), "mvp:alice", "mvp:bob"].map((id) => { const t = loopTheme(id); return [id, { name: t.name, seconds: +seconds(t.pattern).toFixed(2) }]; })),
    stingers: Object.fromEntries(Object.entries(STINGERS).map(([id, f]) => { const s = f(); return [id, { name: s.name, seconds: +(s.delay + seconds(s.pattern)).toFixed(2) }]; })),
  };
}
