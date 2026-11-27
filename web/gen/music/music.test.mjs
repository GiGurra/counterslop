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
const { createMusic, LOOPS, STINGERS, RACE_LOOP_IDS, RACE_TRACKS, RACE_MAP_TRACKS, loopTheme, mvpLoop, catalog } = await import("../../assets/music/index.js");
const { createMusicDirector, loopFor, raceLift, MVP_DELAY } = await import("../../game/music.js");
initAudio();
const timers = { setInterval: () => 1, clearInterval() {}, setTimeout() {} }; // the test pumps by hand

test("themes: every loop loops and every stinger is a few seconds", () => {
  for (const id of [...Object.keys(LOOPS), ...RACE_LOOP_IDS, "mvp:alice"]) {
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

const RACE_MAPS = ["rc_ci_pipeline", "rc_merge_train", "rc_hotfix_highway", "lab_race"];
const lapRace = (lap, laps = 3, extra = {}) => ({ laps, lap, gates: 6, gate: lap * 6, p2p: false, countdown: 0, finished: 0, ...extra });
const course = (gate, gates = 10, extra = {}) => ({ laps: 1, lap: 0, gates, gate, p2p: true, countdown: 0, finished: 0, ...extra });

test("director: each race map plays its own track while the race is on; grid and results as ever", () => {
  const tracks = RACE_MAPS.map((map) => loopFor({ ph: "live", mode: "race", n: 1, map, race: lapRace(1) }));
  assert.deepEqual(tracks, ["pipeline:1", "merge:1", "hotfix:1", "friday:1"]);
  assert.equal(new Set(tracks.map((id) => loopTheme(id).name)).size, 4, "four tracks, four names");
  assert.equal(loopFor({ ph: "live", mode: "race", map: "rc_someone_elses" }), "friday:1", "an unknown race map in cars: the default");
  assert.equal(loopFor({ ph: "live", mode: "race" }), "friday:1");
  assert.equal(loopFor({ ph: "freeze", mode: "race", n: 2, map: "rc_merge_train" }), "buytime");
  assert.equal(loopFor({ ph: "over", mode: "race", n: 2, map: "rc_merge_train" }), null);
  for (const t of ["merge", "hotfix", "friday", "pipeline"]) {
    const p = loopTheme(`${t}:1`).pattern;
    assert.ok(p.events.length > 100, t);
    assert.ok(p.events.some((e) => e.voice === "gtr") && p.events.some((e) => e.voice === "lead"), `${t}: guitars`);
    assert.deepEqual(p, loopTheme(`${t}:1`).pattern, "seeded: the same on every client");
  }
  for (const id of Object.values(RACE_MAP_TRACKS)) assert.ok(RACE_TRACKS[id], id);
});

test("director: races 2 and 3 of a match play variations of the map's track (then round again)", () => {
  const at = (n) => loopFor({ ph: "live", mode: "race", n, map: "rc_hotfix_highway", race: lapRace(1) });
  assert.deepEqual([1, 2, 3, 4].map(at), ["hotfix:1", "hotfix:2", "hotfix:3", "hotfix:1"]);
  const [a, b, c] = [1, 2, 3].map((v) => loopTheme(`hotfix:${v}`).pattern);
  const lead = (p) => p.events.filter((e) => e.voice === "lead").map((e) => e.midi).join();
  assert.notEqual(lead(a), lead(b)); assert.notEqual(lead(b), lead(c)); assert.notEqual(lead(a), lead(c));
  assert.notDeepEqual(a.progression, b.progression);
  assert.equal(a.root, b.root, "the same key: the map's track, varied");
  assert.equal(a.bpm, c.bpm);
});

test("director: the final lap lifts the track: faster, a whole step up, the heaviest drums and an extra arp", () => {
  const at = (lap, laps = 3) => loopFor({ ph: "live", mode: "race", n: 2, map: "rc_merge_train", race: lapRace(lap, laps) });
  assert.equal(at(0), "merge:2"); assert.equal(at(1), "merge:2"); assert.equal(at(2), "merge:2");
  assert.equal(at(3), "merge:2:lift");
  assert.equal(at(1, 1), "merge:2", "a race of one lap never lifts");
  assert.equal(raceLift(null), false);
  for (const t of Object.keys(RACE_TRACKS)) {
    const base = loopTheme(`${t}:2`).pattern, up = loopTheme(`${t}:2:lift`).pattern;
    const r = up.bpm / base.bpm;
    assert.ok(r >= 1.05 && r <= 1.09, `${t}: tempo x${r.toFixed(3)}`);
    assert.equal(up.root, base.root + 2, `${t}: a whole step up`);
    const kicks = (p) => p.events.filter((e) => e.voice === "kit" && e.midi === 36).length;
    assert.ok(kicks(up) > kicks(base), `${t}: heavier drums`);
    assert.ok(up.events.some((e) => e.voice === "kit" && e.midi === 49), `${t}: crashes`);
    assert.ok(up.events.filter((e) => e.voice === "chip").length > base.events.filter((e) => e.voice === "chip").length, `${t}: an extra arp`);
    // The same tune: the lead's notes a whole step up, beat for beat.
    const lead = (p, k) => p.events.filter((e) => e.voice === "lead" || e.voice === "pulse").map((e) => `${e.beat}:${e.midi + k}`).join();
    assert.equal(lead(up, 0), lead(base, 2), `${t}: the lifted melody is the same melody`);
  }
});

test("director: parkour (pk_ maps, lab_parkour, any course on foot) plays its own non-rock track, lifted on the last stretch", () => {
  for (const map of ["pk_happy_path", "lab_parkour", "pk_somebody_new"]) assert.equal(loopFor({ ph: "live", mode: "race", map, race: course(1) }), "parkour:1", map);
  assert.equal(loopFor({ ph: "live", mode: "race", map: "custom", race: course(1) }), "parkour:1", "a point-to-point course: parkour");
  const p = loopTheme("parkour:1").pattern;
  assert.ok(!p.events.some((e) => e.voice === "gtr" || e.voice === "lead"), "no guitars");
  assert.ok(p.events.some((e) => e.voice === "acid") && p.events.some((e) => e.voice === "chip"), "acid bass and chip arp");
  assert.ok(p.events.some((e) => e.voice === "kit" && e.midi === 38 && e.vel < 0.3), "a breakbeat's ghost snares");
  const at = (gate, n = 1) => loopFor({ ph: "live", mode: "race", n, map: "pk_happy_path", race: course(gate) });
  assert.equal(at(8), "parkour:1");
  assert.equal(at(9), "parkour:1:lift", "past the second-to-last gate: the finish next");
  assert.equal(at(10), "parkour:1:lift", "finished: still lifted until the results");
  assert.equal(at(9, 3), "parkour:3:lift");
});

test("director: race stingers fire once per crossing: a lap, the final lap, the finish by place; never on a repeat", () => {
  const played = [];
  const fake = { setLoop() {}, stinger: (id, arg) => (played.push(arg === undefined ? id : `${id}:${arg}`), true), state: () => ({ on: true }) };
  const dir = createMusicDirector({ music: fake, now: () => 0 });
  const go = (race, ph = "live") => { for (let i = 0; i < 3; i++) dir.update({ ph, mode: "race", n: 1, map: "rc_ci_pipeline", race }); }; // every snapshot thrice: duplicates
  go(lapRace(0, 4, { countdown: 3 }), "freeze");
  go(lapRace(0, 4)); go(lapRace(1, 4)); // off the grid, over the start line: no stinger
  assert.deepEqual(played, []);
  go(lapRace(2, 4)); go(lapRace(3, 4)); go(lapRace(4, 4));
  assert.deepEqual(played, ["lap:2", "lap:3", "finalLap"]);
  go(lapRace(4, 4, { finished: 2 }));
  go(lapRace(4, 4, { finished: 2 }));
  assert.deepEqual(played, ["lap:2", "lap:3", "finalLap", "finish:2"]);
  assert.deepEqual(dir.state().raceSaid, played);
  go(lapRace(4, 4, { finished: 2, results: true }), "over");
  // The next race: joining mid-race fires nothing until something changes.
  played.length = 0;
  go(lapRace(3, 4)); go(lapRace(3, 4));
  assert.deepEqual(played, []);
  go(lapRace(4, 4)); // a lost snapshot or two in between: still the one stinger
  go(lapRace(4, 4, { finished: 5 }));
  assert.deepEqual(played, ["finalLap", "finish:5"]);
  // A course: the last stretch, the finish.
  played.length = 0;
  go(course(0, 10, { countdown: 2 }), "freeze");
  for (let g = 0; g <= 9; g++) go(course(g));
  go(course(10, 10, { finished: 1 }));
  assert.deepEqual(played, ["finalLap", "finish:1"]);
  // Other modes: none of it.
  played.length = 0;
  dir.update({ ph: "live", mode: "", race: lapRace(2) }); dir.update({ ph: "live", mode: "", race: lapRace(3) });
  assert.deepEqual(played, []);
});

test("themes: the lap stinger is short; final lap and finish (podium or not) are their own", () => {
  for (const l of [2, 3, 7]) { const s = STINGERS.lap(l); assert.ok(s.delay + seconds(s.pattern) <= 1.5, `lap ${l}: ${seconds(s.pattern)} s`); }
  assert.ok(STINGERS.lap(3).pattern.events[0].midi > STINGERS.lap(2).pattern.events[0].midi, "a step up a lap");
  const notes = (s) => s.pattern.events.map((e) => `${e.voice}${e.midi}`).join();
  assert.notEqual(notes(STINGERS.finalLap()), notes(STINGERS.lap(2)));
  assert.notEqual(notes(STINGERS.finish(1)), notes(STINGERS.finish(2)), "the winner gets the bells");
  assert.notEqual(notes(STINGERS.finish(3)), notes(STINGERS.finish(4)), "podium or not");
  assert.equal(notes(STINGERS.finish(4)), notes(STINGERS.finish(8)));
});

test("player: the final-lap lift cuts in at the old track's next bar line, in time, no gap", () => {
  const ctx = audioCtx();
  ctx.currentTime = 500;
  const player = createMusicPlayer({ timers }), music = createMusic({ player });
  const got = [];
  const pumpFor = (secs) => { for (let i = 0; i < secs * 40; i++) { ctx.currentTime += 0.025; started.length = 0; player.pump(); got.push(...started.map((s) => s.t)); } };
  music.setLoop("merge:1");
  pumpFor(3.3);
  const bar = (4 * 60) / 168, t0 = 500 + 0.05; // started 50 ms on
  const cut = ctx.currentTime;
  music.setLoop("merge:1:lift");
  assert.equal(music.state().segues, 1);
  assert.equal(player.tracks, 2, "the old track plays on to the bar line");
  const line = t0 + Math.ceil((cut + 0.2 - t0) / bar) * bar; // past what was already scheduled
  got.length = 0;
  pumpFor(4);
  assert.ok(got.some((t) => Math.abs(t - line) < 1e-6), "the lift's first notes on the bar line");
  const ts = [...new Set(got.filter((t) => t >= cut && t <= line + 1).map((t) => +t.toFixed(6)))].sort((x, y) => x - y);
  for (let i = 1; i < ts.length; i++) assert.ok(ts[i] - ts[i - 1] <= 60 / 168 / 2 + 1e-6, `no gap at the cut: ${ts[i - 1]} -> ${ts[i]}`);
  assert.equal(player.tracks, 1, "the old track is gone");
  music.setLoop("hotfix:1");
  assert.equal(music.state().segues, 1, "another track (another map) fades, no segue");
  music.setLoop(null);
  pumpFor(3);
  assert.equal(music.state().segues, 1, "a stop is a fade, not a segue");
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

test("director: Star Slop (tickets) plays the overture in warmup and freeze, the liftoff theme aboard in flight, a launch stinger as the doors shut", () => {
  assert.equal(loopFor({ ph: "freeze", mode: "tickets" }), "overture");
  assert.equal(loopFor({ ph: "warmup", buyMenu: true, mode: "tickets" }), "overture");
  assert.equal(loopFor({ ph: "live", mode: "tickets" }), null);
  assert.equal(loopFor({ ph: "live", mode: "tickets", ride: true }), "liftoff");
  assert.equal(loopFor({ ph: "live", ride: true }), null); // only in tickets mode
  assert.equal(loopFor({ ph: "over", mode: "tickets", ride: true }), null);
  for (const id of ["overture", "liftoff"]) {
    const t = loopTheme(id);
    assert.ok(t.name && t.pattern.events.length > 60, id);
    assert.ok(t.pattern.events.some((e) => e.voice === "brass") && t.pattern.events.some((e) => e.voice === "timp"), `${id}: brass and timpani`);
    assert.deepEqual(t.pattern, loopTheme(id).pattern, "seeded: the same on every client");
  }
  const played = [];
  const fake = { setLoop() {}, stinger: (id) => (played.push(id), true), state: () => ({ on: true }) };
  const dir = createMusicDirector({ music: fake, now: () => 0 });
  dir.update({ ph: "live", mode: "tickets" });
  dir.update({ ph: "live", mode: "tickets", ride: true });
  dir.update({ ph: "live", mode: "tickets", ride: true }); // once per launch
  dir.update({ ph: "live", mode: "tickets" });
  dir.update({ ph: "live", mode: "tickets", ride: true });
  assert.deepEqual(played, ["launch", "launch"]);
  assert.ok(STINGERS.launch().pattern.events.length > 5);
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
