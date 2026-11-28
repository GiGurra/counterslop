// Announcer playback: renders lines with SlopVox (cached), plays them on an announcer
// bus with its own volume, one at a time. Lines and voices are data in lines.js.
import { render } from "../../gen/voice/index.js";
import { audioCtx, master, live } from "../../gen/synth/core.js";
import { playSamples } from "../../gen/synth/fx.js";
import { LINES, VOICES } from "./lines.js";
import { paChime, PA_CHIME_SECONDS } from "../sounds/store.js";

export { LINES, VOICES, MULTI, SPREES, TAUNTS, ROUND_END } from "./lines.js";
export { RADIO, RADIO_BY_ID } from "./radio.js";

let bus = null, volume = 0.8, enabled = true, playing = null, pending = [], onLine = null, clock = () => performance.now();
// How long a line may wait in the queue (s) before it is dropped unsaid: an
// announcement belongs to its moment (a kill, a gate, a point); said seconds
// later it's out of place. A line can set its own (wait: seconds) when it is
// queued on purpose behind another (the safety demo after the captain).
export const MAX_WAIT = 1;
const cache = new Map();

// onLine({ id, text, color, voice, dur }) fires as each queued line starts (captions).
// clock() (ms): when lines were queued (tests pass their own; default performance.now).
export function setAnnouncer({ volume: v = volume, enabled: e = enabled, onLine: f = onLine, clock: c = clock } = {}) {
  volume = v; enabled = !!e; onLine = f; clock = c;
  if (bus) bus.gain.value = volume;
}

function announcerBus() {
  if (!bus) {
    bus = audioCtx().createGain();
    bus.gain.value = volume;
    bus.connect(master());
  }
  return bus;
}

// Render (or fetch from cache) a phoneme string in a voice. mul scales the voice's own
// numbers ({ pitch: 1.2, rate: 0.8 }); set replaces them ({ robot: 1 }).
export function renderVoice(ph, voiceName = "announcer", mul = {}, set = {}) {
  const key = JSON.stringify([ph, voiceName, mul, set]);
  if (!cache.has(key)) {
    const v = VOICES[voiceName] || VOICES.announcer, o = { ...v, ...set };
    for (const [k, m] of Object.entries(mul)) if (m !== undefined && typeof o[k] === "number") o[k] *= m;
    if (cache.size > 64) cache.delete(cache.keys().next().value);
    cache.set(key, render(ph, o));
  }
  return cache.get(key);
}

// Play a phoneme string now, ignoring the queue (gallery, `vox`). Returns the render.
export function sayPhonemes(ph, voiceName = "announcer", mul = {}, set = {}) {
  const r = renderVoice(ph, voiceName, mul, set);
  if (!live()) return r;
  const v = VOICES[voiceName] || VOICES.announcer;
  const src = playSamples(r.samples, r.sampleRate, { dest: announcerBus(), rev: v.reverb, wet: v.reverb?.wet || 0 });
  return { ...r, src };
}

// Queue an announcer line by id. One line plays at a time and at most two wait; older
// waiting lines are dropped, so a burst of kills says the latest streak, not every one,
// and a line that has waited longer than its wait (MAX_WAIT) is dropped unsaid.
// With the voice off (or audio not started) lines still run through the queue on their
// own timing, so captions keep working. A line with chime (the store PA) rings the PA
// chime first and speaks PA_CHIME_SECONDS later.
export function say(id) {
  if (!LINES[id]) throw new Error(`no announcer line "${id}"`);
  if (playing) { pending = [...pending, { id, by: clock() + (LINES[id].wait ?? MAX_WAIT) * 1000 }].slice(-2); return true; }
  const l = LINES[id], r = renderVoice(l.ph, l.voice), pre = l.chime ? PA_CHIME_SECONDS : 0, dur = pre + r.samples.length / r.sampleRate;
  const next = () => {
    playing = null;
    const t = clock();
    pending = pending.filter((n) => n.by >= t); // too late now: out of place
    const n = pending.shift();
    if (n) say(n.id);
  };
  onLine?.({ id, text: l.text, color: l.color, voice: l.voice || "announcer", dur });
  if (enabled && live()) {
    const v = VOICES[l.voice] || VOICES.announcer;
    const speak = () => {
      playing = playSamples(r.samples, r.sampleRate, { dest: announcerBus(), rev: v.reverb, wet: v.reverb?.wet || 0 });
      playing.onended = next;
    };
    if (pre) { paChime(); playing = setTimeout(speak, pre * 1000); } else speak();
  } else {
    playing = setTimeout(next, dur * 1000);
  }
  return true;
}

// Render every line ahead of time, one per idle slot, so the first kill doesn't hitch.
export function warmup() {
  const ids = Object.keys(LINES);
  const idle = window.requestIdleCallback || ((f) => setTimeout(f, 50));
  const step = () => {
    const id = ids.shift();
    if (!id) return;
    renderVoice(LINES[id].ph, LINES[id].voice);
    idle(step);
  };
  idle(step);
}
