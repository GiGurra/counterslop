// WebAudio side of the music engine: runs the sequencer clock (sequencer.js) every
// 25 ms and schedules its notes on the instruments, each track on its own fade gain
// on one music bus (volume, on/off) into the master. Game-agnostic.
import { audioCtx, master, live } from "../synth/core.js";
import { createSequencer } from "./sequencer.js";
import { INSTRUMENTS } from "./instruments.js";
import { mtof } from "./theory.js";

export function createMusicPlayer({ lookahead = 0.2, interval = 25, timers = globalThis } = {}) {
  const seq = createSequencer({ lookahead });
  const outs = new Map(); // track -> its gain node
  let bus = null, volume = 0.5, timer = null;

  const ctx = () => audioCtx();
  function musicBus() {
    if (!bus) { bus = ctx().createGain(); bus.gain.value = volume; bus.connect(master()); }
    return bus;
  }
  function pump() {
    if (!live()) return;
    for (const n of seq.pump(ctx().currentTime)) {
      const out = outs.get(n.track);
      if (out) INSTRUMENTS[n.voice]?.(out, n.time, mtof(n.midi), n.dur, n.vel, n.midi);
    }
    for (const tr of [...outs.keys()]) if (!seq.has(tr)) { const g = outs.get(tr); outs.delete(tr); timers.setTimeout(() => g.disconnect?.(), 3000); }
    if (!seq.size && timer) { timers.clearInterval(timer); timer = null; }
  }

  return {
    setVolume(v) { volume = v; if (bus) bus.gain.value = v; },
    // Play a pattern ({ bpm, beats, loop, events }). opts: { delay (s), gain, lowpass (Hz),
    // at (an AudioContext time to start on instead: a segue's bar line) }.
    // Returns the track, or null while audio hasn't started.
    play(pattern, { delay = 0, gain = 1, lowpass = 0, at = 0 } = {}) {
      if (!live()) return null;
      const c = ctx(), g = c.createGain();
      g.gain.value = gain;
      if (lowpass) { const lp = c.createBiquadFilter(); lp.type = "lowpass"; lp.frequency.value = lowpass; g.connect(lp).connect(musicBus()); }
      else g.connect(musicBus());
      const tr = seq.add(pattern, Math.max(at, c.currentTime + 0.05 + delay));
      outs.set(tr, g);
      pump();
      if (!timer) timer = timers.setInterval(pump, interval);
      return tr;
    },
    // Fade a track out over `fade` seconds from `at` (default now); no new notes start
    // after the fade (a segue: none from `at` on).
    stop(tr, fade = 1, at = 0) {
      if (!tr || !outs.has(tr)) return;
      const c = ctx(), t = Math.max(c.currentTime, at), g = outs.get(tr).gain;
      g.cancelScheduledValues?.(c.currentTime);
      g.setValueAtTime(g.value || 1, t);
      g.linearRampToValueAtTime(0.0001, t + fade);
      seq.stop(tr, at ? t : t + fade);
    },
    // The next bar line of a playing track (AudioContext time): where a segue cuts.
    nextBar: (tr) => seq.nextBar(tr, ctx().currentTime),
    pump,
    get tracks() { return seq.size; },
  };
}
