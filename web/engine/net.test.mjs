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
  assert.ok(Math.abs(v - (369 - lag)) < 0.5, `view ${v}, want about ${369 - lag} (arrival-time stamping would give ~${300})`);
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
  const [b] = r.sample(1090 + 10 * 4.5 + INTERP_DELAY - 90); // render tick 4.5
  assert.ok(Math.abs(b.p[0] - 4.5 * 0.03) < 1e-6, `x ${b.p[0]}`);
  r.ingest(snap(5000), 1, 2000);
  r.ingest(snap(3), 1, 3000); // a new server
  assert.ok(Math.abs(r.viewTick(3000) - (3 - lag)) < 1e-6, `view ${r.viewTick(3000)}`);
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
