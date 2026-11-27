// Music engine: the seeded composer, the sequencer clock, the themes, the director's
// "no loops under live play" rule, and the WebAudio player on a fake graph.
import test from "node:test";
import assert from "node:assert/strict";
import { compose, phrase, seconds } from "./compose.js";
import { createSequencer } from "./sequencer.js";
import { note, chord, SCALES, hashString, mtof } from "./theory.js";

const SPEC = { loop: true, bars: 4, parts: [{ role: "bass", voice: "tri" }, { role: "chords", voice: "pad" }, { role: "arp", voice: "chip" }, { role: "lead", voice: "pulse" }, { role: "drums", voice: "kit" }] };

test("theory: degrees wrap octaves, chords stack thirds, A4 = 440", () => {
  assert.equal(note(60, SCALES.major, 7), 72);
  assert.equal(note(60, SCALES.major, -1), 59);
  assert.deepEqual(chord(60, SCALES.major, 0, 4), [60, 64, 67, 71]);
  assert.equal(mtof(69), 440);
  assert.equal(hashString("alice"), hashString("alice"));
  assert.notEqual(hashString("alice"), hashString("bob"));
});

test("compose: the same seed gives the same notes, another seed other notes", () => {
  assert.deepEqual(compose(SPEC, 42), compose(SPEC, 42));
  assert.notDeepEqual(compose(SPEC, 42).events, compose(SPEC, 43).events);
  const p = compose(SPEC, 42);
  assert.equal(p.beats, 16);
  assert.ok(p.events.length > 40);
  for (const e of p.events) {
    assert.ok(e.beat >= 0 && e.beat < p.beats, `beat ${e.beat} inside the loop`);
    assert.ok(e.dur > 0 && e.vel > 0 && Number.isInteger(e.midi));
  }
  for (const v of ["tri", "pad", "chip", "pulse", "kit"]) assert.ok(p.events.some((e) => e.voice === v), v);
});

test("compose: adding a part leaves the other parts' notes alone", () => {
  const fixed = { ...SPEC, root: 50, scale: "minor", bpm: 100, progression: [0, 5, 3, 4] };
  const a = compose(fixed, 7), b = compose({ ...fixed, parts: [...fixed.parts, { role: "arp", voice: "bell" }] }, 7);
  assert.deepEqual(b.events.filter((e) => e.voice !== "bell"), a.events);
});

test("compose: melody notes sit in the scale and the last lead note is the tonic", () => {
  const p = compose({ ...SPEC, root: 48, scale: "major" }, 9), lead = p.events.filter((e) => e.voice === "pulse");
  const pcs = new Set(SCALES.major.map((x) => (48 + x) % 12));
  for (const e of lead) assert.ok(pcs.has(e.midi % 12), `${e.midi} in C major`);
  assert.equal(lead.at(-1).midi % 12, 0);
});

test("sequencer: every note comes out once, at its time, and a loop repeats", () => {
  const p = phrase(120, 2, [[0, 0.5, "a", 60], [1, 0.5, "a", 62]], true); // 1 s per cycle
  const seq = createSequencer({ lookahead: 0.1 });
  seq.add(p, 10);
  const got = [];
  for (let t = 9.5; t < 13; t += 0.025) got.push(...seq.pump(t));
  assert.deepEqual(got.map((n) => +n.time.toFixed(3)), [10, 10.5, 11, 11.5, 12, 12.5, 13]);
  assert.ok(got.every((n) => Math.abs(n.dur - 0.25) < 1e-9));
});

test("sequencer: a one-shot ends after one pass; stop() ends a loop at its time", () => {
  const seq = createSequencer({ lookahead: 0.1 });
  const once = seq.add(phrase(120, 2, [[0, 0.5, "a", 60], [1, 0.5, "a", 62]]), 0);
  const loop = seq.add(phrase(120, 2, [[0, 0.5, "b", 60]], true), 0);
  const got = [];
  for (let t = 0; t < 2; t += 0.05) got.push(...seq.pump(t));
  assert.equal(got.filter((n) => n.voice === "a").length, 2);
  assert.equal(seq.has(once), false);
  seq.stop(loop, 2.6);
  for (let t = 2; t < 6; t += 0.05) got.push(...seq.pump(t));
  assert.deepEqual(got.filter((n) => n.voice === "b").map((n) => n.time), [0, 1, 2]);
  assert.equal(seq.size, 0);
});

// --- a fake WebAudio graph for the player ---
const started = [];
class Param { constructor() { this.value = 1; } setValueAtTime() {} linearRampToValueAtTime() {} exponentialRampToValueAtTime() {} cancelScheduledValues() {} }
class Node {
  constructor(kind) { this.kind = kind; this.gain = new Param(); this.frequency = new Param(); this.Q = new Param(); this.pan = new Param(); }
  connect(n) { return n; }
  disconnect() {}
  start(t) { started.push({ kind: this.kind, t }); }
  stop() {}
}
class FakeContext {
  constructor() { this.state = "running"; this.currentTime = 0; this.sampleRate = 8000; this.destination = new Node("destination"); }
  resume() {}
  createDynamicsCompressor() { return new Node("compressor"); }
  createGain() { return new Node("gain"); }
  createStereoPanner() { return new Node("panner"); }
  createOscillator() { return new Node("osc"); }
  createBufferSource() { return new Node("source"); }
  createBiquadFilter() { return new Node("filter"); }
  createConvolver() { return new Node("convolver"); }
  createWaveShaper() { return new Node("shaper"); }
  createBuffer(ch, len) { return { length: len, getChannelData: () => new Float32Array(len), copyToChannel() {} }; }
}
globalThis.AudioContext = FakeContext;
const { initAudio, audioCtx } = await import("../synth/core.js");
const { createMusicPlayer } = await import("./player.js");
const { createMusic, LOOPS, STINGERS, loopTheme, mvpLoop, catalog } = await import("../../assets/music/index.js");
const { createMusicDirector, loopFor, MVP_DELAY } = await import("../../game/music.js");
initAudio();
const timers = { setInterval: () => 1, clearInterval() {}, setTimeout() {} }; // the test pumps by hand

test("themes: every loop loops and every stinger is a few seconds", () => {
  for (const id of [...Object.keys(LOOPS), "mvp:alice"]) {
    const t = loopTheme(id);
    assert.equal(t.pattern.loop, true, id);
    assert.ok(seconds(t.pattern) >= 4 && seconds(t.pattern) <= 30, `${id}: ${seconds(t.pattern)} s`);
    assert.ok(t.pattern.events.length > 20, id);
  }
  for (const [id, f] of Object.entries(STINGERS)) {
    for (const arg of [1, 3, 9]) {
      const s = f(arg);
      assert.equal(s.pattern.loop, false, id);
      assert.ok(s.delay + seconds(s.pattern) <= 4, `${id}: ${s.delay + seconds(s.pattern)} s`);
      assert.ok(s.pattern.events.every((e) => e.beat < s.pattern.beats), id);
    }
  }
  assert.deepEqual(Object.keys(catalog().stingers), Object.keys(STINGERS));
});

test("themes: the kill arpeggio steps up per multi-kill", () => {
  const first = (m) => STINGERS.kill(m).pattern.events[0].midi, len = (m) => STINGERS.kill(m).pattern.events.length;
  for (let m = 1; m < 5; m++) { assert.ok(first(m + 1) > first(m)); assert.ok(len(m + 1) > len(m)); }
  assert.deepEqual(STINGERS.kill(5), STINGERS.kill(8)); // capped
});

test("themes: MVP anthems are seeded by the name", () => {
  assert.deepEqual(loopTheme("mvp:alice").pattern, loopTheme("mvp:alice").pattern);
  assert.notDeepEqual(loopTheme("mvp:alice").pattern.events, loopTheme("mvp:bob").pattern.events);
  assert.equal(mvpLoop("carol").seed, hashString("mvp:carol"));
});

test("director: loops only outside live play", () => {
  assert.equal(loopFor({ ph: "warmup" }), "warmup");
  assert.equal(loopFor({ ph: "warmup", buyMenu: true }), "buytime");
  assert.equal(loopFor({ ph: "freeze" }), "buytime");
  assert.equal(loopFor({ ph: "halftime" }), "buytime");
  assert.equal(loopFor({ ph: "matchover" }), "matchend");
  assert.equal(loopFor({ ph: "over", mvp: "alice", overFor: 0 }), null); // the win/loss stinger first
  assert.equal(loopFor({ ph: "over", mvp: "alice", overFor: MVP_DELAY }), "mvp:alice");
  for (const ph of ["live", "planted", null, undefined, "nonsense"]) {
    assert.equal(loopFor({ ph, buyMenu: true, mvp: "alice", overFor: 99 }), null, String(ph));
  }
});

test("director: a race plays its own rock track while it's on, one per race in turn; grid and results as ever", () => {
  assert.equal(loopFor({ ph: "live", mode: "race", n: 1 }), "race1");
  assert.equal(loopFor({ ph: "live", mode: "race", n: 2 }), "race2");
  assert.equal(loopFor({ ph: "live", mode: "race", n: 4 }), "race1");
  assert.equal(loopFor({ ph: "live", mode: "race" }), "race1");
  assert.equal(loopFor({ ph: "freeze", mode: "race", n: 2 }), "buytime");
  assert.equal(loopFor({ ph: "over", mode: "race", n: 2 }), null);
  for (const id of ["race1", "race2", "race3"]) {
    const t = loopTheme(id);
    assert.ok(t.name && t.pattern.events.length > 100, id);
    assert.ok(t.pattern.events.some((e) => e.voice === "gtr") && t.pattern.events.some((e) => e.voice === "lead"), `${id}: guitars`);
    assert.deepEqual(t.pattern, loopTheme(id).pattern, "seeded: the same on every client");
  }
});

test("director: GPU Drop plays store muzak in buy time and the Black Friday jingle for a win", () => {
  assert.equal(loopFor({ ph: "freeze", mode: "gpudrop" }), "muzak");
  assert.equal(loopFor({ ph: "halftime", mode: "gpudrop" }), "muzak");
  assert.equal(loopFor({ ph: "warmup", buyMenu: true, mode: "gpudrop" }), "muzak");
  assert.equal(loopFor({ ph: "warmup", mode: "gpudrop" }), "warmup");
  assert.equal(loopFor({ ph: "live", mode: "gpudrop" }), null);
  const played = [];
  const fake = { setLoop() {}, stinger: (id) => (played.push(id), true), state: () => ({ on: true }) };
  const dir = createMusicDirector({ music: fake, now: () => 0 });
  dir.update({ ph: "live", mode: "gpudrop" });
  dir.secured(); dir.roundEnd(true); dir.roundEnd(false);
  dir.update({ ph: "live" }); // a defusal map again
  dir.roundEnd(true);
  assert.deepEqual(played, ["secure", "blackFriday", "roundLost", "roundWon"]);
});

test("player + director: the buy loop schedules notes in freeze, nothing after the fade once live", () => {
  const ctx = audioCtx();
  ctx.currentTime = 100;
  const player = createMusicPlayer({ timers }), music = createMusic({ player });
  let clock = 0;
  const dir = createMusicDirector({ music, now: () => clock });
  const run = (ph, secs, extra = {}) => {
    for (let i = 0; i < secs * 40; i++) { clock += 0.025; ctx.currentTime += 0.025; dir.update({ ph, ...extra }); player.pump(); }
  };
  started.length = 0;
  run("freeze", 6);
  assert.equal(music.state().loop, "buytime");
  assert.ok(started.length > 30, `freeze scheduled ${started.length} voices`);
  const liveAt = ctx.currentTime;
  started.length = 0;
  run("live", 3);
  assert.equal(music.state().loop, null);
  assert.ok(started.every((s) => s.t <= liveAt + 1.5 + 0.2), "no loop note after the live fade");
  started.length = 0;
  run("live", 10);
  assert.equal(started.length, 0, "silence in live play");
  assert.equal(player.tracks, 0);
  // Stingers do play live: a kill, then nothing again.
  dir.kill({ multi: 2, headshot: true });
  run("live", 4);
  assert.ok(started.length >= 5 && started.length < 20);
  assert.equal(music.state().last, "headshot");
  assert.equal(player.tracks, 0);
  // Music off: no loop and no stingers.
  music.set({ enabled: false });
  started.length = 0;
  run("freeze", 3);
  assert.equal(dir.roundEnd(true), false);
  run("freeze", 3);
  assert.equal(music.state().loop, null);
  assert.equal(started.length, 0);
  music.set({ enabled: true });
  run("freeze", 1);
  assert.equal(music.state().playing, true);
});

test("player: the MVP anthem starts after the round-end stinger and stops at freeze", () => {
  const ctx = audioCtx();
  const player = createMusicPlayer({ timers }), music = createMusic({ player });
  let clock = 0;
  const dir = createMusicDirector({ music, now: () => clock });
  const step = (ph, secs, extra) => { for (let i = 0; i < secs * 40; i++) { clock += 0.025; ctx.currentTime += 0.025; dir.update({ ph, ...extra }); player.pump(); } };
  step("live", 1);
  assert.equal(dir.roundEnd(false), true);
  step("over", 1, { mvp: "alice" });
  assert.equal(music.state().loop, null);
  step("over", 2, { mvp: "alice" });
  assert.equal(music.state().loop, "mvp:alice");
  step("freeze", 1);
  assert.equal(music.state().loop, "buytime");
});

test("instruments: every voice plays a note into the graph (the guitars through their drive)", async () => {
  const { INSTRUMENTS } = await import("./instruments.js");
  const out = audioCtx().createGain();
  for (const [name, play] of Object.entries(INSTRUMENTS)) {
    const before = started.length;
    play(out, 1, 220, 0.4, 0.8, 36);
    assert.ok(started.length > before, `${name} started nothing`);
  }
});
