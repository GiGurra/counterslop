// Every sound in the catalog plays through a fake WebAudio graph: each recipe
// builds nodes, connects them to the output and schedules them inside its time
// window, with finite parameters (a NaN frequency or a negative start time
// throws in a browser). Node has no WebAudio, so the fake records the graph.
import test from "node:test";
import assert from "node:assert/strict";

const nodes = [];
const fin = (v, what) => assert.ok(Number.isFinite(v), `${what}: ${v}`);
class Param {
  constructor() { this.value = 0; this.events = []; } // events: what is scheduled ([kind, time, value]), cancel drops those at/after its time
  setValueAtTime(v, t) { fin(v, "value"); fin(t, "time"); this.events.push(["set", t, v]); }
  exponentialRampToValueAtTime(v, t) { fin(v, "ramp value"); assert.ok(v > 0, `exponential ramp to ${v}`); fin(t, "ramp time"); this.events.push(["ramp", t, v]); }
  linearRampToValueAtTime(v, t) { fin(v, "ramp value"); fin(t, "ramp time"); this.events.push(["ramp", t, v]); }
  setTargetAtTime(v, t, k) { fin(v, "target"); fin(t, "target time"); assert.ok(k > 0, `time constant ${k}`); this.events.push(["target", t, v]); }
  cancelScheduledValues(t) { fin(t, "cancel time"); this.events = this.events.filter((e) => e[1] < t); }
}
class Node {
  constructor(kind) { this.kind = kind; this.outs = []; this.gain = new Param(); this.frequency = new Param(); this.Q = new Param(); this.pan = new Param(); nodes.push(this); }
  connect(n) { assert.ok(n, `${this.kind} connects to nothing`); this.outs.push(n); return n; }
  start(t = 0) { fin(t, `${this.kind} start`); assert.ok(t >= 0, `${this.kind} starts at ${t}`); this.started = true; }
  stop(t) { fin(t, `${this.kind} stop`); this.stopAt = Math.min(this.stopAt ?? Infinity, t); }
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
  createBuffer(ch, len) { const d = new Float32Array(len); return { getChannelData: () => d, length: len }; }
}
globalThis.AudioContext = FakeContext;

const { CATALOG, initAudio } = await import("./index.js");
initAudio();
// Reaches the destination through the graph? (A modulator connected into an
// AudioParam, e.g. a vibrato LFO, counts: it is heard through that node.)
const reaches = (n, seen = new Set()) => n.kind === "destination" || n instanceof Param ||
  (!seen.has(n) && (seen.add(n), (n.outs || []).some((o) => reaches(o, seen))));

for (const [name, play] of Object.entries(CATALOG)) {
  test(`sound ${name} builds a graph that reaches the output`, () => {
    const before = nodes.length;
    play();
    const made = nodes.slice(before);
    const sources = made.filter((n) => n.kind === "osc" || n.kind === "source");
    assert.ok(sources.length > 0, `${name} makes no sound sources`);
    for (const s of sources) {
      assert.ok(s.started, `${name}: a ${s.kind} never starts`);
      assert.ok(reaches(s), `${name}: a ${s.kind} is not connected to the output`);
    }
  });
}

test("positional big diff sounds take {pan, gain}", async () => {
  const { ciBeep, mergeBoom, plantClick } = await import("./bigdiff.js");
  for (const f of [() => ciBeep({ pan: -0.5, gain: 0.3 }, 1), () => mergeBoom({ pan: 0.2, gain: 1 }), () => plantClick({ pan: 0, gain: 0.5 })]) {
    const before = nodes.length;
    f();
    assert.ok(nodes.slice(before).some((n) => n.kind === "panner"), "panned");
  }
});

test("store sounds: positional, and the escalator hum is a handle that follows and stops", async () => {
  const { gpuGrab, gpuRestock, gpuSecure, escalatorHum } = await import("./store.js");
  for (const f of [gpuGrab, gpuRestock, gpuSecure]) {
    const before = nodes.length;
    f({ pan: 0.3, gain: 0.5 });
    assert.ok(nodes.slice(before).some((n) => n.kind === "panner"), `${f.name} panned`);
  }
  const before = nodes.length;
  const h = escalatorHum({ pan: -1, gain: 0.2 });
  const made = nodes.slice(before);
  assert.ok(made.some((n) => n.kind === "panner"));
  for (const s of made.filter((n) => n.kind === "osc" || n.kind === "source")) assert.ok(s.started && reaches(s));
  const src = made.filter((n) => n.kind === "source");
  assert.ok(src.length === 1 && src[0].loop === true, "the belt noise loops (the shared buffer is 0.5 s)");
  // every frame calls update: only real changes reach the AudioParams
  const gainNode = made.find((n) => n.kind === "gain" && n.outs.some((o) => o.kind === "panner"));
  let events = 0;
  gainNode.gain.setTargetAtTime = () => { events++; };
  for (let i = 0; i < 100; i++) h.update({ pan: -1, gain: 0.2 + i * 1e-5 });
  assert.equal(events, 0, "no events for inaudible changes");
  h.update({ pan: 0.5, gain: 0.8 });
  assert.equal(events, 1);
  h.stop(0.3);
});

// A loop's handle (engine, outboard, escalator hum) stopped goes quiet: every
// source it started stops (the looped noise too: the "sssh" that went on after
// a race reset), and a stop during its fade-in leaves no ramp back up after
// the fade out (the last thing scheduled on its output gain is the fade to 0).
test("looped sounds stop all their sources and stay faded out", async () => {
  const { carEngine } = await import("./cars.js");
  const { outboard } = await import("./boat.js");
  const { escalatorHum } = await import("./store.js");
  for (const [name, make] of [["carEngine hovercraft", () => carEngine(undefined, "hovercraft")], ["carEngine buggy", () => carEngine()], ["outboard", () => outboard()], ["escalatorHum", () => escalatorHum()]]) {
    const before = nodes.length;
    const h = make();
    const made = nodes.slice(before);
    h.stop(); // at once: inside the 0.3-0.4 s fade-in
    for (const n of made.filter((n) => n.kind === "osc" || n.kind === "source")) assert.ok(n.stopAt < 10, `${name}: a ${n.kind} keeps playing after stop() (until ${n.stopAt})`);
    const out = made.find((n) => n.kind === "gain" && n.outs[0]?.kind === "panner");
    const last = out.gain.events.reduce((a, e) => (e[1] >= a[1] ? e : a)); // the latest in time (AudioParam events run in time order)
    assert.ok(last[0] === "target" && last[2] <= 0.0001, `${name}: after stop() the gain's last event is ${JSON.stringify(last)} (a fade-in ramp back up)`);
  }
});
