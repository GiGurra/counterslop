// Every sound in the catalog plays through a fake WebAudio graph: each recipe
// builds nodes, connects them to the output and schedules them inside its time
// window, with finite parameters (a NaN frequency or a negative start time
// throws in a browser). Node has no WebAudio, so the fake records the graph.
import test from "node:test";
import assert from "node:assert/strict";

const nodes = [];
const fin = (v, what) => assert.ok(Number.isFinite(v), `${what}: ${v}`);
class Param {
  constructor() { this.value = 0; }
  setValueAtTime(v, t) { fin(v, "value"); fin(t, "time"); }
  exponentialRampToValueAtTime(v, t) { fin(v, "ramp value"); assert.ok(v > 0, `exponential ramp to ${v}`); fin(t, "ramp time"); }
  linearRampToValueAtTime(v, t) { fin(v, "ramp value"); fin(t, "ramp time"); }
}
class Node {
  constructor(kind) { this.kind = kind; this.outs = []; this.gain = new Param(); this.frequency = new Param(); this.Q = new Param(); this.pan = new Param(); nodes.push(this); }
  connect(n) { assert.ok(n, `${this.kind} connects to nothing`); this.outs.push(n); return n; }
  start(t = 0) { fin(t, `${this.kind} start`); assert.ok(t >= 0, `${this.kind} starts at ${t}`); this.started = true; }
  stop(t) { fin(t, `${this.kind} stop`); }
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
