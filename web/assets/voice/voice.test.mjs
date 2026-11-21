// Announcer playback (voice/index.js) and the synth FX it uses (gen/synth/fx.js)
// on a fake WebAudio graph: node has no WebAudio, so the fake records nodes.
import test from "node:test";
import assert from "node:assert/strict";

const nodes = [];
class Param { constructor() { this.value = 0; } setValueAtTime() {} linearRampToValueAtTime() {} exponentialRampToValueAtTime() {} }
class Node {
  constructor(kind) { this.kind = kind; this.outs = []; this.gain = new Param(); this.frequency = new Param(); this.Q = new Param(); this.pan = new Param(); nodes.push(this); }
  connect(n) { this.outs.push(n); return n; }
  start() { this.started = true; }
  stop() {}
}
class FakeContext {
  constructor() { this.state = "running"; this.currentTime = 1; this.sampleRate = 8000; this.destination = new Node("destination"); }
  resume() {}
  createDynamicsCompressor() { return new Node("compressor"); }
  createGain() { return new Node("gain"); }
  createStereoPanner() { return new Node("panner"); }
  createOscillator() { return new Node("osc"); }
  createBufferSource() { return new Node("source"); }
  createBiquadFilter() { return new Node("filter"); }
  createConvolver() { return new Node("convolver"); }
  createBuffer(ch, len) { const d = Array.from({ length: ch }, () => new Float32Array(len)); return { length: len, copyToChannel: (s, c) => d[c].set(s), getChannelData: (c) => d[c] }; }
}
globalThis.AudioContext = FakeContext;
globalThis.window = { requestIdleCallback: (f) => setTimeout(f, 0) };

const { initAudio, master } = await import("../../gen/synth/core.js");
const { reverbIR, reverb, playSamples } = await import("../../gen/synth/fx.js");
const voice = await import("./index.js");

test("reverbIR: seeded stereo noise that decays", () => {
  const a = reverbIR(8000, { seconds: 0.5, seed: 3 }), b = reverbIR(8000, { seconds: 0.5, seed: 3 });
  assert.equal(a.length, 2);
  assert.equal(a[0].length, 4000);
  assert.deepEqual(a[0], b[0], "same seed, same IR");
  assert.notDeepEqual(reverbIR(8000, { seconds: 0.5, seed: 4 })[0], a[0]);
  const energy = (ch, i0, i1) => ch.slice(i0, i1).reduce((s, x) => s + x * x, 0);
  assert.ok(energy(a[0], 0, 1000) > 10 * energy(a[0], 3000, 4000), "the tail is much quieter");
});

test("playSamples: a source into the destination, with one shared convolver per reverb and destination", () => {
  initAudio();
  const before = nodes.length;
  const src = playSamples(new Float32Array(100), 8000, { rev: { seconds: 0.2, decay: 2, seed: 1 }, wet: 0.3 });
  assert.ok(src.started);
  const made = nodes.slice(before);
  assert.equal(made.filter((n) => n.kind === "convolver").length, 1);
  const c = reverb({ seconds: 0.2, decay: 2, seed: 1 }, master());
  assert.equal(reverb({ seconds: 0.2, decay: 2, seed: 1 }, master()), c, "built once");
  const dry = nodes.length;
  playSamples(new Float32Array(10), 8000);
  assert.equal(nodes.slice(dry).filter((n) => n.kind === "convolver").length, 0, "no send without wet");
});

test("say: one line at a time, at most two waiting (the latest), captions per line", async () => {
  const shown = [];
  voice.setAnnouncer({ volume: 0.5, enabled: true, onLine: (l) => shown.push(l.id) });
  assert.throws(() => voice.say("no_such_line"));
  const ids = Object.keys(voice.LINES);
  const before = nodes.length;
  voice.say(ids[0]);
  voice.say(ids[1]); voice.say(ids[2]); voice.say(ids[3]); // ids[1] is dropped
  assert.deepEqual(shown, [ids[0]]);
  const playing = nodes.slice(before).filter((n) => n.kind === "source");
  assert.equal(playing.length, 1, "one source while the first line plays");
  playing[0].onended();
  assert.deepEqual(shown, [ids[0], ids[2]], "then the waiting ones, latest two");
  nodes.filter((n) => n.kind === "source").at(-1).onended();
  assert.deepEqual(shown, [ids[0], ids[2], ids[3]]);
  nodes.filter((n) => n.kind === "source").at(-1).onended();
});

test("say with the voice off still runs captions on the line's own timing", async () => {
  const shown = [];
  voice.setAnnouncer({ enabled: false, onLine: (l) => shown.push(l) });
  const ids = Object.keys(voice.LINES);
  voice.say(ids[0]);
  voice.say(ids[1]);
  assert.equal(shown.length, 1);
  assert.ok(shown[0].dur > 0 && shown[0].text);
  await new Promise((r) => setTimeout(r, shown[0].dur * 1000 + 50));
  assert.equal(shown.length, 2, "the next line once the first one's time is up");
  await new Promise((r) => setTimeout(r, shown[1].dur * 1000 + 50));
  voice.setAnnouncer({ enabled: true, onLine: null });
});

test("renderVoice caches; mul scales and set replaces voice numbers; sayPhonemes plays now", () => {
  const a = voice.renderVoice("HH AY1", "announcer");
  assert.equal(voice.renderVoice("HH AY1", "announcer"), a, "cached");
  const slow = voice.renderVoice("HH AY1", "announcer", { rate: 0.5 });
  assert.ok(slow.samples.length > a.samples.length, "rate 0.5 is longer");
  assert.notEqual(voice.renderVoice("HH AY1", "announcer", {}, { robot: 1 }), a);
  const r = voice.sayPhonemes("HH AY1");
  assert.ok(r.src?.started);
});

test("warmup renders every line in idle slots", async () => {
  voice.warmup();
  await new Promise((r) => setTimeout(r, 200));
  const id = Object.keys(voice.LINES).at(-1), l = voice.LINES[id];
  const r = voice.renderVoice(l.ph, l.voice);
  assert.ok(r.samples.length > 0);
});

test("store PA lines ring the chime first and speak after it; captions count the chime", async () => {
  const { PA_CHIME_SECONDS } = await import("../sounds/store.js");
  for (const id of ["gpu_grabbed", "gpu_secured", "out_of_stock", "restocked"]) assert.ok(voice.LINES[id].chime && voice.LINES[id].voice === "pa", id);
  for (const id of ["scalpers_win", "staff_wins"]) assert.ok(voice.LINES[id].banner, id);
  const shown = [];
  voice.setAnnouncer({ enabled: true, onLine: (l) => shown.push(l) });
  const before = nodes.length;
  voice.say("gpu_secured");
  assert.equal(shown[0].id, "gpu_secured");
  assert.ok(shown[0].dur > PA_CHIME_SECONDS);
  const made = () => nodes.slice(before);
  assert.ok(made().some((n) => n.kind === "osc"), "the chime");
  assert.equal(made().filter((n) => n.kind === "source").length, 0, "no voice yet");
  await new Promise((r) => setTimeout(r, PA_CHIME_SECONDS * 1000 + 30));
  const src = made().filter((n) => n.kind === "source");
  assert.equal(src.length, 1, "the voice after the chime");
  src[0].onended();
  voice.setAnnouncer({ onLine: null });
});
