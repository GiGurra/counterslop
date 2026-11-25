import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { readReplay, createReplayCursor, gunzip } from "../engine/replay.js";
import { createReplayer } from "./replayer.js";
import { createRecorder, fileName, EVERY, KEY_EVERY } from "./recorder.js";

// internal/game TestReplayGolden's round, received again by a client: its
// snapshots (as decoded) and events, in tick order.
const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/replay_golden.json", import.meta.url)));
const server = readReplay(new Uint8Array(Buffer.from(golden.raw, "base64")));
const ctx = { map: server.header.map, version: "v1", myId: 2, simHz: 100 };

// What the client got: every snapshot (pose ones in between, as the json
// codec sends), the events where they fell.
function receive(rec, { from = 0 } = {}) {
  let e = 0;
  const w = golden.snaps[0].want; // warmup before it (not recorded): the round's start event opens the file from it
  rec.snap({ ...w, t: "snap", tick: w.tick - 50, round: { ...w.round, ph: "warmup", n: 0 } }, ctx);
  for (const s of golden.snaps.slice(from)) {
    while (e < server.events.length && server.events[e].tick <= s.tick) rec.event(server.events[e++].msg, ctx);
    rec.snap({ pose: true, tick: s.tick - 1, players: [] }, ctx);
    rec.snap({ ...s.want, t: "snap" }, ctx);
  }
  while (e < server.events.length) rec.event(server.events[e++].msg, ctx);
}

test("records a round in the server's format: header, delta snapshots that decode to what was received, the events", () => {
  const rec = createRecorder();
  receive(rec);
  rec.stop();
  const list = rec.list();
  assert.equal(list.length, 1);
  assert.equal(list[0].live, false);
  assert.equal(list[0].round, 1);
  assert.match(list[0].name, /^\d{8}-\d{6}-lab_bhop-round-01\.cstv\.gz$/);
  const rep = readReplay(rec.raw(0));
  assert.equal(rep.complete, true);
  const h = rep.header;
  assert.equal(h.format, 1);
  assert.equal(h.recorder, "client");
  assert.equal(h.pov, 2);
  assert.equal(h.map.name, "lab_bhop");
  assert.equal(h.round, 1);
  assert.equal(h.every, EVERY);
  assert.equal(h.keyEvery, KEY_EVERY);
  assert.deepEqual(h.players.map((p) => p.name).sort(), server.header.players.map((p) => p.name).sort());
  // every received snapshot, at the server's rate (these come every 3 ticks: all of them)
  assert.deepEqual(rep.snaps.map((s) => s.tick), golden.snaps.map((s) => s.tick));
  assert.ok(rep.snaps.filter((s) => s.back === 0).length >= 2, "keyframes");
  const cur = createReplayCursor(rep);
  for (const i of [0, 7, golden.snaps.length - 1, 3, 40]) assert.deepEqual(cur.at(golden.snaps[i].tick), { ...golden.snaps[i].want, t: "snap" }, `snapshot ${i}`);
  // the events, each at the newest snapshot's tick when it came; hits say whose
  assert.deepEqual(rep.events.map((e) => e.msg.t), server.events.map((e) => e.msg.t));
  rep.events.forEach((e, i) => assert.ok(e.tick <= server.events[i].tick + EVERY && e.tick >= server.events[i].tick - EVERY, `event ${i} at ${e.tick}, server ${server.events[i].tick}`));
  for (const e of rep.events.filter((x) => x.msg.t === "hit")) assert.ok(e.msg.by !== undefined);
  // and the replay player plays it: the kill is on the timeline
  const p = createReplayer(rep, { deliver() {}, reset() {} });
  assert.ok(p.markers().some((m) => m.kind === "kill"));
});

test("a new round number starts a new file; warmup isn't recorded; only the last `keep` kept; a live round reads up to now", async () => {
  const rec = createRecorder({ keep: 2 });
  const snap = (tick, n, ph = "live") => ({ t: "snap", time: 1e12 + tick * 10, tick, players: [{ id: 2, name: "me", team: "vibe", p: [0, 0, 0], yaw: 0, pitch: 0, g: [], hp: 100, k: 0, d: 0, ammo: 0, res: 0, color: "#fff" }], round: { ph, n, sc: { vibe: 0, artisan: 0 }, left: 10, max: 24, hist: [] } });
  rec.snap(snap(1, 0, "warmup"), ctx);
  assert.equal(rec.list().length, 0);
  for (let n = 1; n <= 3; n++) {
    for (let t = 0; t < 30; t += 1) rec.snap(snap(n * 1000 + t, n), ctx);
    rec.event({ t: "kill", k: 2, v: 3, w: "prompt" }, ctx);
  }
  const l = rec.list();
  assert.deepEqual(l.map((x) => [x.round, x.live]), [[2, false], [3, true]]);
  assert.equal(l[0].match, l[1].match);
  const rep = readReplay((await gunzip(await rec.file(1))).bytes);
  assert.deepEqual(rep.snaps.map((s) => s.tick), [3000, 3003, 3006, 3009, 3012, 3015, 3018, 3021, 3024, 3027]); // every 3rd tick
  assert.equal(rep.events.length, 1);
  // a round 1 again: a new match (a restart)
  rec.snap(snap(9000, 1), ctx);
  assert.notEqual(rec.list().at(-1).match, "");
  assert.equal(rec.list().at(-1).round, 1);
  assert.equal(fileName({ match: "m", round: 7 }), "m-round-07.cstv.gz");
});

test("a round's start event opens its file before the round's first snapshot", () => {
  const rec = createRecorder();
  const snap = (tick, n) => ({ t: "snap", time: 5, tick, players: [], round: { ph: "live", n, sc: {}, left: 1, max: 24, hist: [] } });
  rec.snap(snap(10, 1), ctx);
  rec.event({ t: "round", ev: "end", win: "vibe", n: 1 }, ctx);
  rec.event({ t: "round", ev: "start", n: 2, sc: { vibe: 1 } }, ctx);
  rec.snap(snap(20, 2), ctx);
  const l = rec.list();
  assert.deepEqual(l.map((x) => x.round), [1, 2]);
  const r2 = readReplay(rec.raw(1));
  assert.equal(r2.events[0].msg.ev, "start");
  assert.equal(r2.header.startTick, 11);
  assert.equal(readReplay(rec.raw(0)).events.at(-1).msg.ev, "end");
});
