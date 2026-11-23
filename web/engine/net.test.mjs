import test from "node:test";
import assert from "node:assert/strict";
import { createRemotes, INTERP_DELAY } from "./net.js";

// A bot walking +x at 3 m/s; snapshots every 3 ticks (30 ms).
const snap = (tick) => ({ tick, players: [{ id: 2, name: "bot", p: [tick * 0.03, 0, 0], yaw: 0, pitch: 0 }] });
const lag = INTERP_DELAY / 10; // ticks

test("steady stream: drawn INTERP_DELAY behind the newest, positions match the ticks", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick += 3) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40); // 40 ms one-way
  const now = 1000 + 300 * 10 + 40;
  assert.ok(Math.abs(r.viewTick(now) - (300 - lag)) < 1e-6, `view ${r.viewTick(now)}`);
  const [b] = r.sample(now);
  assert.ok(Math.abs(b.p[0] - (300 - lag) * 0.03) < 1e-6, `drawn at x=${b.p[0]}`);
  assert.ok(Math.abs(b.v - 3) < 1e-6, `speed ${b.v}`);
});

test("a stall: the late burst doesn't drag the view back in time", () => {
  const r = createRemotes();
  let tick = 0;
  for (; tick <= 300; tick += 3) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40);
  // 700 ms of nothing, then everything at once (a blocked main thread)
  const burstAt = 1000 + 369 * 10 + 40;
  for (; tick <= 370; tick += 3) r.ingest(snap(tick), 1, burstAt);
  const v = r.viewTick(burstAt);
  // within MAX_TRAIL (3) ticks of the new present: the slewed estimate catches up over ~0.3 s
  assert.ok(v > 369 - lag - 3.5 && v < 369 - lag + 0.5, `view ${v}, want about ${369 - lag} (arrival-time stamping would give ~${300})`);
});

test("latency grows for good: the view follows within a few seconds; jitter alone doesn't move it", () => {
  const r = createRemotes();
  let tick = 0;
  for (; tick <= 300; tick += 3) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40 + (tick % 2 ? 15 : 0));
  const before = 1000 + 300 * 10 + 40;
  assert.ok(Math.abs(r.viewTick(before) - (300 - lag)) < 0.1, "jitter: the earliest arrivals set the clock");
  for (; tick <= 900; tick += 3) r.ingest(snap(tick), 1, 1000 + tick * 10 + 140); // +100 ms for good
  const now = 1000 + 900 * 10 + 140;
  assert.ok(Math.abs(r.viewTick(now) - (900 - lag)) < 1, `view ${r.viewTick(now)}`);
});

test("starved: the view holds at the newest snapshot", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick += 3) r.ingest(snap(tick), 1, 1000 + tick * 10);
  assert.equal(r.viewTick(1000 + 300 * 10 + 5000), 300);
  assert.ok(Math.abs(r.sample(1000 + 300 * 10 + 5000)[0].p[0] - 9) < 1e-9);
});

test("reordered snapshots slot in by tick; a restarted server starts over", () => {
  const r = createRemotes();
  r.ingest(snap(0), 1, 1000); r.ingest(snap(6), 1, 1060); r.ingest(snap(3), 1, 1065);
  r.ingest(snap(9), 1, 1090);
  const now = 1090 + 10 * 4.5 + INTERP_DELAY - 90; // about render tick 4.5 (the late tick 3 grows the buffer a little)
  const t = r.timing(now).renderTick;
  assert.ok(t > 3 && t < 6, `render tick ${t}`); // between the reordered tick 3 and tick 6
  const [b] = r.sample(now);
  assert.ok(Math.abs(b.p[0] - t * 0.03) < 1e-6, `x ${b.p[0]} at tick ${t}`);
  r.ingest(snap(5000), 1, 2000);
  r.ingest(snap(3), 1, 3000); // a new server
  assert.ok(Math.abs(r.viewTick(3000) - (3 - lag)) < 1e-6, `view ${r.viewTick(3000)}`);
});

// A TCP-like link: snapshots every 30 ms, one-way 40 ms + 0..jitter ms, in
// order (a delayed one holds back the ones behind it). Returns arrivals [{at, tick}].
import { mulberry32 } from "../gen/rng.js";
function link({ ms, jitter, seed = 7, from = 0 }) {
  const rnd = mulberry32(seed), out = [];
  let last = 0;
  for (let tick = from; tick * 10 < ms; tick += 3) {
    last = Math.max(last, 1000 + tick * 10 + 40 + rnd() * jitter);
    out.push({ at: last, tick });
  }
  return out;
}
// Plays arrivals into r and samples it at 60 fps from `t0`: under-runs (drawn
// past the newest snapshot), the worst per-frame deviation of the drawn tick
// from real time, and the delay at the end.
function play(r, arrivals, t0, t1) {
  let i = 0, under = 0, worstStep = 0, prev = null;
  for (let now = 1000; now < t1; now += 1000 / 60) {
    while (i < arrivals.length && arrivals[i].at <= now) { const a = arrivals[i++]; r.ingest(snap(a.tick), 1, a.at); }
    const tm = r.timing(now);
    if (now >= t0) {
      if (tm.renderTick > tm.newest) under++;
      if (prev) worstStep = Math.max(worstStep, Math.abs((tm.renderTick - prev.t) - (now - prev.now) / 10));
      prev = { t: tm.renderTick, now };
    }
  }
  return { under, worstStep, delay: r.timing(t1).viewDelayMs };
}

test("±100 ms jitter (in order): once adapted, no under-runs and the timeline never jumps", () => {
  const r = createRemotes();
  const res = play(r, link({ ms: 20000, jitter: 100 }), 6000, 20000);
  assert.equal(res.under, 0, JSON.stringify(res));
  assert.ok(res.worstStep < 0.2, `a frame moved the timeline ${res.worstStep} ticks off real time`); // 10% of a 16.7 ms frame is 0.17
  assert.ok(res.delay > 120 && res.delay <= 250, `delay ${res.delay} ms`);
});

test("a fixed 100 ms buffer under-runs on that link (what the adaptive delay fixes)", () => {
  const arr = link({ ms: 20000, jitter: 100 });
  let under = 0, i = 0, best = -Infinity, newest = -Infinity;
  for (let now = 1000; now < 21000; now += 1000 / 60) {
    while (i < arr.length && arr[i].at <= now) { best = Math.max(best, arr[i].tick - arr[i].at / 10); newest = arr[i].tick; i++; }
    if (now >= 6000 && now / 10 + best - 10 > newest) under++;
  }
  assert.ok(under > 20, `under-runs ${under}`);
});

test("the jitter goes away: the delay shrinks back to INTERP_DELAY within a few seconds", () => {
  const r = createRemotes();
  const a = link({ ms: 10000, jitter: 100 }), b = link({ ms: 25000, jitter: 0, from: 1002 });
  const last = a[a.length - 1].at;
  play(r, [...a, ...b.filter((x) => x.at > last)], 30000, 26000);
  const d = r.timing(26000).viewDelayMs;
  assert.ok(Math.abs(d - INTERP_DELAY) < 2, `delay ${d} ms`);
});

// ---- createNet with a fake WebSocket and mocked timers ----
import { createNet } from "./net.js";

class FakeWS {
  static all = [];
  constructor(url) { this.url = url; this.sent = []; this.readyState = 0; FakeWS.all.push(this); }
  send(s) { this.sent.push(JSON.parse(s)); }
  close() { this.readyState = 3; }
  open() { this.readyState = 1; this.onopen(); }
  recv(m) { this.onmessage({ data: JSON.stringify(m) }); }
}

function withNet(t, fn) {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
  globalThis.WebSocket = FakeWS;
  globalThis.location = { protocol: "https:", host: "example.test" };
  FakeWS.all = [];
  const got = [];
  const net = createNet((m) => got.push(m));
  try { fn(net, got); } finally { net.disconnect(); }
}

test("net: connects, says hello, delivers messages, answers pings, reconnects with backoff", (t) => withNet(t, (net, got) => {
  net.connect("alice");
  const ws = FakeWS.all[0];
  assert.equal(ws.url, "wss://example.test/ws");
  ws.open();
  assert.ok(net.open);
  assert.deepEqual(ws.sent[0], { t: "hello", name: "alice" });
  assert.equal(got[0].t, "open");
  ws.recv({ t: "snap", tick: 3 });
  assert.equal(got.at(-1).tick, 3);
  t.mock.timers.tick(1000); // the ping
  assert.equal(ws.sent.at(-1).t, "ping");
  ws.recv({ t: "pong", seq: performance.now() - 42 });
  assert.ok(net.ping >= 42 && net.ping < 60, `ping ${net.ping}`);
  ws.onclose();
  assert.equal(got.at(-1).t, "close");
  assert.equal(net.open, false);
  t.mock.timers.tick(250); // first retry
  assert.equal(FakeWS.all.length, 2);
}));

test("net: faults drop, duplicate and delay only commands and snapshots; dropCmds loses exactly n", (t) => withNet(t, (net, got) => {
  net.connect("bob");
  const ws = FakeWS.all[0];
  ws.open();
  const rnd = Math.random;
  try {
    net.faults = { loss: 0.5, dup: 0.5, jitter: 0 };
    Math.random = () => 0.9; // keep, no dup
    net.send({ t: "cmd", cmds: [1] });
    t.mock.timers.tick(1);
    assert.equal(ws.sent.filter((m) => m.t === "cmd").length, 1);
    Math.random = () => 0.1; // lost
    net.send({ t: "cmd", cmds: [2] });
    net.send({ t: "team", team: "vibe" }); // not unreliable: always goes
    t.mock.timers.tick(1);
    assert.equal(ws.sent.filter((m) => m.t === "cmd").length, 1);
    assert.equal(ws.sent.at(-1).t, "team");
    Math.random = (() => { const seq = [0.9, 0.1]; let i = 0; return () => seq[i++ % 2]; })(); // keep, dup
    ws.recv({ t: "snap", tick: 9 });
    t.mock.timers.tick(1);
    assert.equal(got.filter((m) => m.tick === 9).length, 2);
    net.faults = { loss: 0, dup: 0, jitter: 0 };
    net.dropCmds = 2;
    for (const s of [3, 4, 5]) net.send({ t: "cmd", cmds: [s] });
    assert.deepEqual(ws.sent.filter((m) => m.t === "cmd").map((m) => m.cmds[0]), [1, 5]);
    net.fakeLag = 100;
    net.send({ t: "cmd", cmds: [6] });
    assert.equal(ws.sent.at(-1).cmds?.[0], 5);
    t.mock.timers.tick(50);
    assert.equal(ws.sent.at(-1).cmds[0], 6);
  } finally { Math.random = rnd; }
}));

test("net: fakejitter delays but keeps order (a TCP link); fakereorder lets packets overtake (datagrams)", (t) => withNet(t, (net, got) => {
  net.connect("bob");
  const ws = FakeWS.all[0];
  ws.open();
  const rnd = Math.random, pnow = performance.now;
  let clock = 1000; // the link times deadlines on performance.now: advance it with the mocked timers
  performance.now = () => clock;
  const tick = (ms) => { for (let i = 0; i < ms; i++) { clock++; t.mock.timers.tick(1); } };
  const seq = (v) => { let i = 0; return () => v[i++ % v.length]; };
  try {
    const cmds = () => ws.sent.filter((m) => m.t === "cmd").map((m) => m.cmds[0]);
    net.faults = { loss: 0, dup: 0, jitter: 100, reorder: 0 };
    Math.random = seq([0.9, 0.1, 0.5]); // 90, 10, 50 ms: in order, all once the first is due
    for (const s of [1, 2, 3]) net.send({ t: "cmd", cmds: [s] });
    net.send({ t: "buy", item: "x" }); // reliable traffic shares the ordered link
    tick(89);
    assert.deepEqual(cmds(), []);
    tick(2);
    assert.deepEqual(cmds(), [1, 2, 3]);
    assert.equal(ws.sent.at(-1).t, "buy");
    Math.random = seq([0.9, 0.1]); // the same downstream: snapshots arrive in tick order
    ws.recv({ t: "snap", tick: 1 }); ws.recv({ t: "snap", tick: 2 });
    tick(100);
    assert.deepEqual(got.filter((m) => m.t === "snap").map((m) => m.tick), [1, 2]);

    net.faults = { loss: 0, dup: 0, jitter: 0, reorder: 100 };
    Math.random = seq([0.99, 0.99, 0.9, 0.99, 0.99, 0.1, 0.99, 0.99, 0.5]); // per packet: kept, no dup, delay 90 / 10 / 50 ms
    for (const s of [4, 5, 6]) net.send({ t: "cmd", cmds: [s] });
    tick(100);
    assert.deepEqual(cmds().slice(3), [5, 6, 4]);
  } finally { Math.random = rnd; performance.now = pnow; }
}));

test("cl_interp: a floor under the delay that applies at once, capped at MAX_DELAY; forget() keeps the timing", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick += 3) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40);
  const now = 1000 + 300 * 10 + 40;
  assert.ok(Math.abs(r.viewTick(now) - (300 - lag)) < 1e-6);
  r.setInterp(200);
  assert.ok(Math.abs(r.viewTick(now) - 280) < 1e-6, `200 ms floor: view ${r.viewTick(now)}`);
  r.setInterp(900);
  assert.ok(Math.abs(r.viewTick(now) - 275) < 1e-6, `capped at 250 ms: view ${r.viewTick(now)}`);
  r.forget();
  assert.equal(r.sample(now).length, 0, "nobody drawn from before");
  assert.ok(Math.abs(r.viewTick(now) - 275) < 1e-6, "the clock is kept");
  r.ingest(snap(303), 1, now + 30);
  const [b] = r.sample(now + 30);
  assert.ok(Math.abs(b.p[0] - 303 * 0.03) < 1e-6, `one sample: drawn there (${b.p[0]})`);
});

// Lockstep pages run on a virtual clock (cmd/sim/vclock.js): the barrier's 2 s
// fallback timer never fires while the harness waits on it. A socket that
// closes with a barrier in flight (an admin kick) must settle it at once, or
// the harness's settle hangs (sim 'admin': "settle: Runtime.evaluate: timeout").
test("net: a socket closing with a barrier in flight resolves it (false), no timer needed", async (t) => {
  let p;
  withNet(t, (net) => {
    net.connect("bob");
    const ws = FakeWS.all[0];
    ws.open();
    p = net.barrier();
    assert.ok(ws.sent.some((m) => m.t === "ping" && m.seq < 0));
    ws.readyState = 3;
    ws.onclose();
  });
  const pending = new Promise((r) => setImmediate(() => r("pending")));
  assert.equal(await Promise.race([p, pending]), false);
});

test("a respawn or teleport (faster than TELEPORT_SPEED between snapshots) jumps to the later pose: no sliding across the map", () => {
  const r = createRemotes();
  r.ingest({ tick: 30, players: [{ id: 2, p: [0, 0, 0], yaw: 0.5, pitch: 0.1 }] }, 1, 1000);
  r.ingest({ tick: 33, players: [{ id: 2, p: [40, 0, 10], yaw: 2, pitch: 0 }] }, 1, 1030);
  const [b] = r.sampleAt(31);
  assert.deepEqual([b.p, b.yaw, b.pitch, b.v], [[40, 0, 10], 2, 0, 0]);
  r.ingest({ tick: 36, players: [{ id: 2, p: [40.3, 0, 10], yaw: 2, pitch: 0 }] }, 1, 1060);
  assert.ok(Math.abs(r.sampleAt(34)[0].p[0] - 40.1) < 1e-9); // a walk still interpolates
});

test("faultScale: a fake delay takes that fraction of its wall time (lockstep cases)", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
  const sent = [];
  const prev = { WebSocket: globalThis.WebSocket, location: globalThis.location };
  globalThis.location = { protocol: "http:", host: "test" };
  globalThis.WebSocket = class { constructor() { this.readyState = 1; } send(s) { sent.push(JSON.parse(s)); } close() {} };
  t.after(() => Object.assign(globalThis, prev));
  const net = createNet(() => {});
  net.connect("p", false);
  net.fakeLag = 100; // 50 ms each way
  net.send({ t: "cmd", seq: 1 });
  t.mock.timers.tick(49);
  assert.equal(sent.length, 0, "unscaled: still on its way after 49 ms");
  t.mock.timers.tick(1);
  assert.equal(sent.length, 1);
  net.faultScale = 0.1;
  net.send({ t: "cmd", seq: 2 });
  t.mock.timers.tick(4);
  assert.equal(sent.length, 1, "scaled: not before 5 ms");
  t.mock.timers.tick(1);
  assert.deepEqual(sent.map((m) => m.seq), [1, 2]);
});
