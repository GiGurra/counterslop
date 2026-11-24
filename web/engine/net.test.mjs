import test from "node:test";
import assert from "node:assert/strict";
import { createRemotes, INTERP_DELAY, EXTRAP_MS } from "./net.js";

// A bot walking +x at 3 m/s; a snapshot every tick (10 ms), as the server sends them.
const snap = (tick) => ({ tick, players: [{ id: 2, name: "bot", p: [tick * 0.03, 0, 0], yaw: 0, pitch: 0 }] });
const lag = INTERP_DELAY / 10; // ticks

test("steady stream: drawn INTERP_DELAY behind the newest, positions match the ticks", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick += 1) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40); // 40 ms one-way
  const now = 1000 + 300 * 10 + 40;
  assert.ok(Math.abs(r.viewTick(now) - (300 - lag)) < 1e-6, `view ${r.viewTick(now)}`);
  const [b] = r.sample(now);
  assert.ok(Math.abs(b.p[0] - (300 - lag) * 0.03) < 1e-6, `drawn at x=${b.p[0]}`);
  assert.ok(Math.abs(b.v - 3) < 1e-6, `speed ${b.v}`);
});

test("a stall: the late burst doesn't drag the view back in time", () => {
  const r = createRemotes();
  let tick = 0;
  for (; tick <= 300; tick += 1) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40);
  // 700 ms of nothing, then everything at once (a blocked main thread)
  const burstAt = 1000 + 369 * 10 + 40;
  for (; tick <= 370; tick += 1) r.ingest(snap(tick), 1, burstAt);
  const v = r.viewTick(burstAt);
  // within MAX_TRAIL (3) ticks of the new present: the slewed estimate catches up over ~0.3 s
  assert.ok(v > 369 - lag - 3.5 && v < 369 - lag + 0.5, `view ${v}, want about ${369 - lag} (arrival-time stamping would give ~${300})`);
});

test("latency grows for good: the view follows within a few seconds; jitter alone doesn't move it", () => {
  const r = createRemotes();
  let tick = 0;
  for (; tick <= 300; tick += 1) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40 + (tick % 2 ? 15 : 0));
  const before = 1000 + 300 * 10 + 40;
  const d0 = r.timing(before).viewDelayMs / 10; // 2 snapshots + p95 jitter (1.5 ticks) + 1
  assert.ok(d0 >= lag - 0.01 && d0 <= lag + 1.6, `delay ${d0} ticks`);
  assert.ok(Math.abs(r.viewTick(before) - (300 - d0)) < 0.1, "jitter: the earliest arrivals set the clock");
  for (; tick <= 900; tick += 1) r.ingest(snap(tick), 1, 1000 + tick * 10 + 140); // +100 ms for good
  const now = 1000 + 900 * 10 + 140;
  assert.ok(Math.abs(r.viewTick(now) - (900 - r.timing(now).viewDelayMs / 10)) < 1, `view ${r.viewTick(now)}`);
});

test("starved: the view (what shots are rewound to) holds at the newest snapshot; the drawing goes on EXTRAP_MS, then holds", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick += 1) r.ingest(snap(tick), 1, 1000 + tick * 10);
  assert.equal(r.viewTick(1000 + 300 * 10 + 5000), 300);
  assert.ok(Math.abs(r.sample(1000 + 300 * 10 + 5000)[0].p[0] - (9 + EXTRAP_MS / 10 * 0.03)) < 1e-9);
});

test("a late snapshot: drawn on at the last velocity for at most EXTRAP_MS, shots never past real data, the correction eases out", () => {
  const r = createRemotes();
  let tick = 0;
  for (; tick <= 300; tick++) r.ingest(snap(tick), 1, 1000 + tick * 10);
  // 80 ms with nothing; the bot stopped at tick 300 (x 9) meanwhile.
  const frames = [];
  for (let now = 1000 + 300 * 10; now < 1000 + 380 * 10; now += 1000 / 60) frames.push({ now, x: r.sample(now)[0].p[0], vt: r.viewTick(now) });
  const xs = frames.map((f) => f.x);
  assert.ok(Math.max(...xs) <= 9 + EXTRAP_MS / 10 * 0.03 + 1e-9, `drawn up to x=${Math.max(...xs)}`);
  assert.ok(frames.every((f) => f.vt <= 300), "the view tick (shots) went past the newest snapshot");
  assert.ok(Math.max(...xs) > 9 + 0.03, "no extrapolation at all");
  // Data again: it stood still at x 9 since tick 300.
  const stop = (k) => ({ tick: k, players: [{ id: 2, name: "bot", p: [9, 0, 0], yaw: 0, pitch: 0 }] });
  let prev = xs[xs.length - 1], worst = 0;
  for (tick = 301; tick <= 420; tick++) {
    r.ingest(stop(tick), 1, 1000 + 380 * 10 + (tick - 301) * 10);
    const x = r.sample(1000 + 380 * 10 + (tick - 301) * 10)[0].p[0];
    worst = Math.max(worst, Math.abs(x - prev));
    prev = x;
  }
  assert.ok(worst < 0.03, `the drawing jumped ${worst} m in a frame when the data came`);
  assert.ok(Math.abs(prev - 9) < 1e-3, `settled at x=${prev}`);
});

test("reordered snapshots slot in by tick; a restarted server starts over", () => {
  const r = createRemotes();
  for (let k = 0; k <= 20; k++) if (k !== 13) r.ingest(snap(k), 1, 1000 + k * 10);
  r.ingest(snap(13), 1, 1000 + 15 * 10); // two ticks late, after 14 and 15
  for (let k = 21; k <= 30; k++) r.ingest(snap(k), 1, 1000 + k * 10);
  const now = 1000 + 30 * 10 - 160 + INTERP_DELAY; // about render tick 14
  const t = r.timing(now).renderTick;
  assert.ok(t > 12 && t < 16, `render tick ${t}`); // around the reordered tick 13
  const [b] = r.sample(now);
  assert.ok(Math.abs(b.p[0] - t * 0.03) < 1e-6, `x ${b.p[0]} at tick ${t}`);
  r.ingest(snap(5000), 1, 2000);
  r.ingest(snap(3), 1, 3000); // a new server
  assert.ok(Math.abs(r.viewTick(3000) - (3 - lag)) < 0.1, `view ${r.viewTick(3000)}`);
});

// A TCP-like link: a snapshot every 10 ms, one-way 40 ms + 0..jitter ms, in
// order (a delayed one holds back the ones behind it). Returns arrivals [{at, tick}].
import { mulberry32 } from "../gen/rng.js";
function link({ ms, jitter, seed = 7, from = 0 }) {
  const rnd = mulberry32(seed), out = [];
  let last = 0;
  for (let tick = from; tick * 10 < ms; tick += 1) {
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
  // 10% of a 16.7 ms frame is 0.17 ticks. A rare very early arrival (once in
  // 14 s here) moves the raw clock 3.4 ticks at once: past MAX_TRAIL the
  // estimate catches up at the whole slew, it doesn't hop (a hard clamp there
  // moved this frame 0.4 ticks: net-jitter's tickJumps).
  assert.ok(res.worstStep < 0.17 + 1e-6, `a frame moved the timeline ${res.worstStep} ticks off real time`);
  assert.ok(res.delay > 80 && res.delay <= 150, `delay ${res.delay} ms`); // 2 snapshots (20 ms) + p95 lateness (~70) + a tick
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
  try {
    net.faults = { loss: 0.5, dup: 0.5, jitter: 0 };
    net.faultRand = () => 0.9; // keep, no dup
    net.send({ t: "cmd", cmds: [1] });
    t.mock.timers.tick(1);
    assert.equal(ws.sent.filter((m) => m.t === "cmd").length, 1);
    net.faultRand = () => 0.1; // lost
    net.send({ t: "cmd", cmds: [2] });
    net.send({ t: "team", team: "vibe" }); // not unreliable: always goes
    t.mock.timers.tick(1);
    assert.equal(ws.sent.filter((m) => m.t === "cmd").length, 1);
    assert.equal(ws.sent.at(-1).t, "team");
    net.faultRand = (() => { const seq = [0.9, 0.1]; let i = 0; return () => seq[i++ % 2]; })(); // keep, dup
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
  } finally { net.faultRand = null; }
}));

test("net: fakejitter delays but keeps order (a TCP link); fakereorder lets packets overtake (datagrams)", (t) => withNet(t, (net, got) => {
  net.connect("bob");
  const ws = FakeWS.all[0];
  ws.open();
  const pnow = performance.now;
  let clock = 1000; // the link times deadlines on performance.now: advance it with the mocked timers
  performance.now = () => clock;
  const tick = (ms) => { for (let i = 0; i < ms; i++) { clock++; t.mock.timers.tick(1); } };
  const seq = (v) => { let i = 0; return () => v[i++ % v.length]; };
  try {
    const cmds = () => ws.sent.filter((m) => m.t === "cmd").map((m) => m.cmds[0]);
    net.faults = { loss: 0, dup: 0, jitter: 100, reorder: 0 };
    net.faultRand = seq([0.9, 0.1, 0.5]); // 90, 10, 50 ms: in order, all once the first is due
    for (const s of [1, 2, 3]) net.send({ t: "cmd", cmds: [s] });
    net.send({ t: "buy", item: "x" }); // reliable traffic shares the ordered link
    tick(89);
    assert.deepEqual(cmds(), []);
    tick(2);
    assert.deepEqual(cmds(), [1, 2, 3]);
    assert.equal(ws.sent.at(-1).t, "buy");
    net.faultRand = seq([0.9, 0.1]); // the same downstream: snapshots arrive in tick order
    ws.recv({ t: "snap", tick: 1 }); ws.recv({ t: "snap", tick: 2 });
    tick(100);
    assert.deepEqual(got.filter((m) => m.t === "snap").map((m) => m.tick), [1, 2]);

    net.faults = { loss: 0, dup: 0, jitter: 0, reorder: 100 };
    net.faultRand = seq([0.99, 0.99, 0.9, 0.99, 0.99, 0.1, 0.99, 0.99, 0.5]); // per packet: kept, no dup, delay 90 / 10 / 50 ms
    for (const s of [4, 5, 6]) net.send({ t: "cmd", cmds: [s] });
    tick(100);
    assert.deepEqual(cmds().slice(3), [5, 6, 4]);
  } finally { net.faultRand = null; performance.now = pnow; }
}));

test("net: the fake faults are seeded: the same messages are lost and duplicated every run, and again after the settings change", (t) => withNet(t, (net, got) => {
  // One link: send 200 commands and receive 200 snapshots; what got through.
  const pattern = (net, got) => {
    net.connect("bob");
    const ws = FakeWS.all.at(-1);
    ws.open();
    got.length = 0;
    for (let s = 0; s < 200; s++) { net.send({ t: "cmd", cmds: [s] }); ws.recv({ t: "snap", tick: s }); }
    t.mock.timers.tick(1);
    const r = { up: ws.sent.filter((m) => m.t === "cmd").map((m) => m.cmds[0]), down: got.filter((m) => m.t === "snap").map((m) => m.tick) };
    net.disconnect();
    return r;
  };
  net.faults = { loss: 0.2, dup: 0.2, jitter: 0, reorder: 0 };
  const a = pattern(net, got);
  assert.ok(a.up.length > 100 && new Set(a.up).size < 190 && a.down.length > 100, JSON.stringify(a).slice(0, 200)); // some lost
  assert.ok(new Set(a.up).size < a.up.length && new Set(a.down).size < a.down.length, "some duplicated");
  net.faults = { loss: 0.3, dup: 0.2, jitter: 0, reorder: 0 };
  net.faults = { loss: 0.2, dup: 0.2, jitter: 0, reorder: 0 }; // changed and back: the streams start over
  assert.deepEqual(pattern(net, got), a);
  const got2 = [], other = createNet((m) => got2.push(m)); // another page, the same settings
  other.faults = { loss: 0.2, dup: 0.2, jitter: 0, reorder: 0 };
  assert.deepEqual(pattern(other, got2), a);
}));

test("cl_interp: a floor under the delay that applies at once, capped at MAX_DELAY; forget() keeps the timing", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick += 1) r.ingest(snap(tick), 1, 1000 + tick * 10 + 40);
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

test("net: a reconnect (retry) settles the old socket's barriers (false); a barrier on no open socket settles at once", async (t) => {
  let p, q;
  withNet(t, (net) => {
    net.connect("bob");
    FakeWS.all[0].open();
    p = net.barrier();
    net.connect("bob"); // retry: the old socket is closed with its onclose detached
    q = net.barrier(); // the new socket isn't open yet
  });
  const pending = new Promise((r) => setImmediate(() => r("pending")));
  assert.deepEqual(await Promise.all([Promise.race([p, pending]), Promise.race([q, pending])]), [false, false]);
});

test("net: the barrier and the harness's sync marker skip the fake link faults (still in flight on the page's clock is not sent)", async (t) => {
  let p, got;
  withNet(t, (net, g) => {
    got = g;
    net.connect("bob");
    const ws = FakeWS.all[0];
    ws.open();
    net.fakeLag = 200;
    net.send({ t: "cmd", seq: 1 });
    p = net.barrier();
    const ping = ws.sent.find((m) => m.t === "ping" && m.seq < 0);
    assert.ok(ping, "the ping went out at once");
    assert.ok(!ws.sent.some((m) => m.t === "cmd"), "the lagged command is still held back");
    ws.recv({ t: "sync", n: 7 });
    ws.recv({ t: "pong", seq: ping.seq });
  });
  assert.equal(await p, true);
  assert.ok(got.some((m) => m.t === "sync" && m.n === 7));
});

test("net: a pong back at the very ms its ping left still counts as measured (next keepalive reports rtt > 0)", (t) => withNet(t, (net) => {
  t.mock.method(performance, "now", () => 5000); // a virtual clock: no time passes on its own
  net.connect("bob");
  const ws = FakeWS.all[0];
  ws.open();
  t.mock.timers.tick(1000);
  const ping = ws.sent.findLast((m) => m.t === "ping" && m.seq >= 0);
  ws.recv({ t: "pong", seq: ping.seq }); // no time passed (a virtual clock)
  t.mock.timers.tick(1000);
  const next = ws.sent.findLast((m) => m.t === "ping" && m.seq >= 0);
  assert.ok(next.rtt > 0, `rtt ${next.rtt}`);
}));

test("a respawn or teleport (faster than TELEPORT_SPEED between snapshots) jumps to the later pose: no sliding across the map", () => {
  const r = createRemotes();
  r.ingest({ tick: 30, players: [{ id: 2, p: [0, 0, 0], yaw: 0.5, pitch: 0.1 }] }, 1, 1000);
  r.ingest({ tick: 33, players: [{ id: 2, p: [40, 0, 10], yaw: 2, pitch: 0 }] }, 1, 1030);
  const [b] = r.sampleAt(31);
  assert.deepEqual([b.p, b.yaw, b.pitch, b.v], [[40, 0, 10], 2, 0, 0]);
  r.ingest({ tick: 36, players: [{ id: 2, p: [40.3, 0, 10], yaw: 2, pitch: 0 }] }, 1, 1060);
  assert.ok(Math.abs(r.sampleAt(34)[0].p[0] - 40.1) < 1e-9); // a walk still interpolates
});

test("delayed counts the messages a fake fault holds back until they go (lost ones never count)", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
  const sent = [];
  const prev = { WebSocket: globalThis.WebSocket, location: globalThis.location };
  globalThis.location = { protocol: "http:", host: "test" };
  globalThis.WebSocket = class { constructor() { this.readyState = 1; } send(s) { sent.push(JSON.parse(s)); } close() {} };
  t.after(() => Object.assign(globalThis, prev));
  const net = createNet(() => {});
  net.connect("p", false);
  net.send({ t: "cmd", seq: 1 });
  assert.deepEqual([sent.length, net.delayed.up], [1, 0], "no faults: sent at once");
  net.fakeLag = 100; // 50 ms each way
  net.send({ t: "cmd", seq: 2 });
  assert.equal(net.delayed.up, 1);
  t.mock.timers.tick(49);
  assert.deepEqual([sent.length, net.delayed.up], [1, 1], "still held back");
  t.mock.timers.tick(1);
  assert.deepEqual([sent.map((m) => m.seq), net.delayed.up], [[1, 2], 0]);
  net.faults.loss = 1;
  net.send({ t: "cmd", seq: 3 });
  assert.deepEqual([net.delayed.up, net.lost.up], [0, 1], "lost: nothing held back, one lost");
});

test("pinDelay (tests): the delay is fixed whatever the arrivals say, null makes it adaptive again", () => {
  const r = createRemotes();
  for (let tick = 0; tick <= 300; tick++) r.ingest(snap(tick), 1, 1000 + tick * 10 + (tick % 2 ? 40 : 0)); // jittery: adaptive > 30 ms
  const now = 1000 + 300 * 10;
  assert.ok(r.timing(now).viewDelayMs > 50);
  r.pinDelay(30);
  assert.equal(r.timing(now).viewDelayMs, 30);
  r.pinDelay(null);
  for (let tick = 301; tick <= 400; tick++) r.ingest(snap(tick), 1, 1000 + tick * 10 + (tick % 2 ? 40 : 0));
  assert.ok(r.timing(1000 + 400 * 10).viewDelayMs > 30);
});

test("the gait comes in full snapshots only: between them it is interpolated (= server History.gaitAt)", () => {
  const r = createRemotes();
  const full = (tick, x, g) => r.ingest({ tick, players: [{ id: 2, team: "vibe", p: [x, 0, 0], yaw: 0, pitch: 0, g }] }, 1, tick * 10);
  const pose = (tick, x) => r.ingest({ tick, pose: true, players: [{ id: 2, p: [x, 0, 0], yaw: 0, pitch: 0, c: false, nb: false }] }, 1, tick * 10);
  full(30, 0, [100, 50]); pose(31, 0.03); pose(32, 0.06); full(33, 0.09, [130, 50]);
  const at = (t) => r.sampleAt(t)[0];
  assert.ok(Math.abs(at(31).g.phase - 2.2) < 1e-9, `phase ${at(31).g.phase}`); // 2 -> 2.6 rad, a third of the way
  assert.equal(at(33).g.phase, 2.6);
  assert.equal(at(31).t, 0.31);
  pose(34, 0.12);
  assert.equal(at(34).g.phase, 2.6, "past the newest full snapshot: held at it");
});

// ---- the delta codec's resync (a missing base: ask for a full, once per gap) ----
import { readFileSync } from "node:fs";
import { NET_SCHEMA_HASH } from "./netdelta.js";

test("delta: a missing base asks for a full once; a lost full is asked again after reaskMs, not 25 snapshots later; clean-link errors are counted", (t) => withNet(t, (net, got) => {
  const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/netdelta_golden.json", import.meta.url)));
  const msg = (i) => { const b = Buffer.from(golden.msgs[i].b, "base64"); return b.buffer.slice(b.byteOffset, b.byteOffset + b.length); };
  const realNow = performance.now.bind(performance);
  let clock = 1000;
  performance.now = () => clock;
  t.after(() => { performance.now = realNow; });
  net.connect("dee");
  const ws = FakeWS.all[0];
  ws.open();
  ws.recv({ t: "welcome", codec: "delta", schema: NET_SCHEMA_HASH });
  const bin = (i) => ws.onmessage({ data: msg(i) });
  const asks = () => ws.sent.filter((m) => m.t === "full").length;
  bin(0);
  assert.equal(got.at(-1).t, "snap");
  bin(2); // its base (1) never came
  assert.deepEqual([net.codecErrors, net.codecErrorsClean, asks(), net.fullPending()], [1, 1, 1, true]);
  clock += 50;
  bin(3);
  assert.equal(asks(), 1, "asked once per gap");
  clock += 60; // the full we asked for never came (lost): ask again, ~100 ms on
  bin(4);
  assert.equal(asks(), 2);
  net.faults = { loss: 0, dup: 0, jitter: 10, reorder: 0 };
  bin(5);
  clock += 20; t.mock.timers.tick(20); // through the jitter queue
  assert.deepEqual([net.codecErrors, net.codecErrorsClean], [4, 3], "under a fake fault an undecodable snapshot isn't a clean-link error");
  assert.equal(net.fullPendingFaulted(), false, "asked on a clean link");
}));

import { createTickRate, fmtTickRate } from "./net.js";

test("tick rate: server ticks per second of our clock over 5 s, min/max of its 1 s windows; a slow server clock shows", () => {
  const r = createTickRate();
  assert.equal(r.read(), null);
  assert.equal(fmtTickRate(null), "–");
  // 10 s of snapshots every 10 ms of our clock, the server's clock 5% slow, with arrival jitter.
  let tick = 0, acc = 0;
  for (let ms = 0; ms <= 10000; ms += 10) {
    acc += 0.95;
    while (acc >= 1) { acc--; tick++; }
    r.add(tick, ms + ((ms / 10) % 3) * 2);
    r.add(tick, ms + 1); // a duplicate: ignored
  }
  const got = r.read();
  assert.ok(Math.abs(got.rate - 95) < 0.5, `rate ${got.rate}`);
  assert.ok(got.min >= 93 && got.max <= 97 && got.min <= got.max, `${got.min}-${got.max}`);
  assert.match(fmtTickRate(got), /^9[45]\.\d tick\/s \(9\d\.\d-9\d\.\d\)$/);
  // A step: the server stalls 0.5 s, then catches up at once: the 1 s windows show it.
  const r2 = createTickRate();
  for (let ms = 0, t = 0; ms <= 6000; ms += 10) { if (ms < 2700 || ms >= 3200) t += ms === 3200 ? 51 : 1; r2.add(t, ms); }
  const g2 = r2.read();
  assert.ok(Math.abs(g2.rate - 100) < 1, `rate ${g2.rate}`);
  assert.ok(g2.min < 80 && g2.max > 120, `${g2.min}-${g2.max}`);
});
// ---- reconnect state (game/link.js reads it: wanted, attempt, retryAt) ----
test("net: a drop counts failed attempts until one opens; a deliberate disconnect isn't wanted", (t) => withNet(t, (net, got) => {
  assert.equal(net.wanted, false);
  net.connect("a");
  assert.equal(net.wanted, true);
  FakeWS.all[0].open();
  assert.deepEqual([net.open, net.attempt], [true, 0]);
  FakeWS.all[0].onclose(); // the server went away
  assert.deepEqual([net.open, net.attempt, got.at(-1).t], [false, 1, "close"]);
  assert.ok(net.retryAt > 0, "a retry is scheduled");
  t.mock.timers.tick(250); // the retry: a new socket, which fails too
  assert.equal(FakeWS.all.length, 2);
  assert.equal(net.retryAt, 0, "under way");
  FakeWS.all[1].onclose();
  assert.equal(net.attempt, 2);
  net.retryNow(); // RETRY NOW: at once, no timer
  assert.equal(FakeWS.all.length, 3);
  t.mock.timers.tick(5000);
  assert.equal(FakeWS.all.length, 3, "the pending retry was cancelled");
  FakeWS.all[2].open();
  assert.deepEqual([net.open, net.attempt], [true, 0]);
  net.disconnect();
  assert.deepEqual([net.wanted, net.open, net.attempt], [false, false, 0]);
  net.retryNow();
  assert.equal(FakeWS.all.length, 3, "nothing to retry after a deliberate disconnect");
}));

test("net: retryNow on a silent open socket drops it (one failed attempt) and reconnects", (t) => withNet(t, (net) => {
  net.connect("a");
  FakeWS.all[0].open();
  net.retryNow();
  assert.equal(FakeWS.all.length, 2);
  assert.deepEqual([net.open, net.attempt], [false, 1]);
}));

test("net: the test cut: silent drops messages both ways; closed refuses reconnects until lifted", (t) => withNet(t, (net, got) => {
  net.connect("a");
  const ws = FakeWS.all[0];
  ws.open();
  net.setCut("silent");
  ws.recv({ t: "snap", tick: 1 });
  net.send({ t: "team", team: "vibe" });
  assert.ok(!got.some((m) => m.t === "snap"));
  assert.ok(!ws.sent.some((m) => m.t === "team"));
  net.setCut("closed");
  assert.deepEqual([net.open, net.attempt], [false, 1]);
  t.mock.timers.tick(250); // the retry fails without a socket
  assert.equal(FakeWS.all.length, 1);
  assert.equal(net.attempt, 2);
  net.setCut(null);
  t.mock.timers.tick(500);
  assert.equal(FakeWS.all.length, 2, "healed: the next retry opens a socket");
}));

test("net: a barrier under a cut resolves false at once (the pong can't come; lockstep would hang)", async (t) => {
  let net;
  withNet(t, (n) => { net = n; n.connect("a"); FakeWS.all[0].open(); n.setCut("silent"); assert.equal(FakeWS.all[0].sent.length, 1, "just the hello"); });
  // withNet disconnected it: reconnect on the cut and ask
  net.connect("a"); FakeWS.all.at(-1).open(); net.setCut("silent");
  assert.equal(await net.barrier(), false);
  assert.ok(!FakeWS.all.at(-1).sent.some((m) => m.t === "ping"), "no ping sent into the dead link");
  net.disconnect();
});
