import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createDeltaEncoder, createDeltaDecoder, fromJSON, zeroNV, NET_SCHEMA_HASH } from "./netdelta.js";
import { NET_SCHEMA } from "./netschema.gen.js";
import { readReplay } from "./replay.js";

// internal/game TestReplayGolden: a recorded round's snapshots as the Go decoder gave them.
const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/replay_golden.json", import.meta.url)));

test("the encoder's deltas decode to the snapshots it was given (the Go decoder's, a round of them), keyframes and deltas", () => {
  const enc = createDeltaEncoder(), dec = createDeltaDecoder();
  let bytesKey = 0, bytesDelta = 0, nKey = 0;
  golden.snaps.forEach((s, i) => {
    const key = i % 30 === 0;
    const b = enc.encode(s.want, s.tick, key);
    if (key) { bytesKey += b.length; nKey++; } else bytesDelta += b.length;
    const r = dec.apply(b);
    assert.ok(!r.error, `tick ${s.tick}: ${r.error}`);
    assert.deepEqual(r.snap, s.want, `tick ${s.tick}`);
  });
  // deltas are much smaller than keyframes
  assert.ok(bytesDelta / (golden.snaps.length - nKey) < (bytesKey / nKey) / 2, `${bytesDelta} ${bytesKey}`);
});

test("byte for byte the server's encoding: a Go keyframe re-encodes to the same message", () => {
  const raw = new Uint8Array(Buffer.from(golden.raw, "base64"));
  const dec = createDeltaDecoder();
  const goMsg = readReplay(raw).snaps[0].bytes; // the round's first snapshot: a Go keyframe
  const snap = dec.apply(goMsg).snap;
  assert.deepEqual([...createDeltaEncoder().encode(snap, golden.snaps[0].tick, true)], [...goMsg]);
});

test("the json codec's shapes: raw floats quantised like the server (mm, 1e-4 rad, halves away from zero), omitted fields, nil lists", () => {
  const players = NET_SCHEMA.f.find((f) => f.n === "players").t;
  const pl = (x) => fromJSON(players, x);
  const v = pl([{ id: 3, name: "a", p: [1.0005, -0.0005, 2.4994], yaw: -0.00005, pitch: 0.12344, g: [1, 2], hp: 100, team: "vibe" }]);
  const pe = v.kids[0];
  const f = (name) => entityFields(players).findIndex((x) => x.n === name);
  assert.deepEqual(pe.kids[f("p")].kids.map((k) => k.i), [1001, -1, 2499]);
  assert.equal(pe.kids[f("yaw")].i, -1);
  assert.equal(pe.kids[f("pitch")].i, 1234);
  assert.deepEqual(pe.kids[f("dead")], { i: 0 });
  assert.deepEqual(fromJSON(players, null), { i: 0 });
  // a whole snapshot with fields missing round-trips through the decoder as the json codec has it
  const snap = { t: "snap", time: 5, tick: 9, players: [{ id: 1, name: "x", color: "#fff", p: [0, 0, 0], yaw: 0, pitch: 0, c: false, g: [], ammo: 0, res: 0, hp: 100, k: 0, d: 0, team: "spec" }], items: [] };
  const r = createDeltaDecoder().apply(createDeltaEncoder().encode(snap, 9));
  assert.ok(!r.error, r.error);
  assert.deepEqual(r.snap.players, snap.players);
  assert.equal(r.snap.tick, 9);
  assert.deepEqual(zeroNV(NET_SCHEMA).kids.length, NET_SCHEMA.f.length);
  assert.equal(typeof NET_SCHEMA_HASH, "string");
});

function entityFields(list) { return (list.e.k === "ptr" ? list.e.e : list.e).f; }
