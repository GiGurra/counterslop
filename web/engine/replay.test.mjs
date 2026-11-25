import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { gzipSync } from "node:zlib";
import { readReplay, createReplayCursor, gunzip } from "./replay.js";

// internal/game TestReplayGolden: a recorded round, ungzipped, and what each
// snapshot decodes to with the Go decoder.
const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/replay_golden.json", import.meta.url)));
const raw = new Uint8Array(Buffer.from(golden.raw, "base64"));

test("reads the Go recorder's file: header, snapshots, events", () => {
  const rep = readReplay(raw);
  assert.equal(rep.complete, true);
  assert.equal(rep.header.format, 1);
  assert.equal(rep.header.map.name, "lab_bhop");
  assert.equal(rep.header.round, 1);
  assert.equal(rep.header.players.length, 2);
  assert.deepEqual(rep.snaps.map((s) => [s.tick, s.back === 0]), golden.snaps.map((s) => [s.tick, s.key]));
  assert.deepEqual(rep.events, golden.events);
  assert.equal(rep.events[0].msg.ev, "start");
  assert.ok(rep.events.some((e) => e.msg.t === "kill"));
});

test("decodes every snapshot to what the Go decoder got, forward, and seeking back and forth", () => {
  const rep = readReplay(raw);
  const c = createReplayCursor(rep);
  golden.snaps.forEach((s) => assert.deepEqual(c.at(s.tick), s.want, `tick ${s.tick}`));
  // between snapshots: the one before
  const s5 = golden.snaps[5];
  assert.deepEqual(c.at(s5.tick + 1), s5.want);
  // back, then forward again, then a jump past a keyframe
  for (const i of [30, 2, 3, 40, 0, golden.snaps.length - 1, 17]) assert.deepEqual(c.at(golden.snaps[i].tick), golden.snaps[i].want, `seek to ${i}`);
  assert.equal(c.at(golden.snaps[0].tick - 1), null);
  assert.ok(c.keyframes().length >= 2);
  assert.equal(c.first, golden.snaps[0].tick);
  assert.equal(c.last, golden.snaps.at(-1).tick);
  const k = golden.events.find((e) => e.msg.t === "kill");
  assert.deepEqual(c.eventsBetween(k.tick - 1, k.tick).map((e) => e.msg.t), ["kill"]);
});

test("a file still being recorded: a cut gzip and a cut last record read up to the cut", async () => {
  const gz = gzipSync(raw);
  const whole = await gunzip(gz);
  assert.equal(whole.complete, true);
  assert.deepEqual(whole.bytes, raw);
  const cut = await gunzip(gz.subarray(0, gz.length - 20)); // no gzip trailer (and the last bytes)
  assert.equal(cut.complete, false);
  const rep = readReplay(raw.subarray(0, raw.length - 7)); // the last record cut
  assert.equal(rep.complete, false);
  assert.ok(rep.snaps.length + rep.events.length < golden.snaps.length + golden.events.length);
  const c = createReplayCursor(rep);
  assert.deepEqual(c.at(rep.snaps.at(-1).tick), golden.snaps[rep.snaps.length - 1].want);
});

test("not a replay, or a newer format", () => {
  assert.throws(() => readReplay(new Uint8Array([1, 2, 3, 4, 5, 6])), /not a counterslop replay/);
  const h = new TextEncoder().encode(JSON.stringify({ format: 2 }));
  assert.throws(() => readReplay(new Uint8Array([0x43, 0x53, 0x54, 0x56, h.length, ...h])), /format 2/);
});
