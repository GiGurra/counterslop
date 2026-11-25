import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { readReplay } from "../engine/replay.js";
import { NET_SCHEMA_HASH } from "../engine/netdelta.js";
import { createReplayer, replayNotes, roundLabel, SPEEDS } from "./replayer.js";

// internal/game TestReplayGolden's recorded round (lab_bhop, a kill in it).
const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/replay_golden.json", import.meta.url)));
const rep = () => readReplay(new Uint8Array(Buffer.from(golden.raw, "base64")));

function player() {
  const got = [];
  let resets = 0;
  const r = createReplayer(rep(), { deliver: (m) => got.push(m), reset: () => { resets++; got.length = 0; } });
  return { r, got, resets: () => resets };
}
const snapTicks = (got) => got.filter((m) => m.t === "snap").map((m) => m.tick);

test("plays from the first snapshot: snapshots a step ahead of the playhead, events at it", () => {
  const { r, got } = player();
  const hz = r.header.simHz, every = r.header.every;
  assert.equal(r.tick, r.first);
  assert.deepEqual(snapTicks(got), golden.snaps.filter((s) => s.tick <= r.first + every).map((s) => s.tick));
  r.update(0.5); // 1x: half a second
  assert.equal(r.tick, r.first + 0.5 * hz);
  assert.equal(Math.max(...snapTicks(got)) <= r.tick + every, true);
  assert.ok(snapTicks(got).includes(golden.snaps.filter((s) => s.tick <= r.tick).at(-1).tick));
  // every delivered snapshot is the Go decoder's, with t "snap"
  const s = got.find((m) => m.t === "snap" && m.tick === golden.snaps[3].tick);
  assert.deepEqual({ ...s, t: undefined }, { ...golden.snaps[3].want, t: undefined });
  assert.equal(s.t, "snap");
  // the events up to the playhead, in order, none after it
  const evs = got.filter((m) => m.t !== "snap");
  assert.deepEqual(evs, golden.events.filter((e) => e.tick <= r.tick).map((e) => e.msg));
});

test("speed, pause, the end; play at the end starts over", () => {
  const { r } = player();
  const hz = r.header.simHz;
  r.setSpeed(4); r.update(0.1);
  assert.equal(r.speed, 4);
  assert.equal(r.tick, r.first + 0.4 * hz);
  r.setSpeed(0.3); assert.equal(r.speed, 0.25); // the nearest step
  r.stepSpeed(-1); assert.equal(r.speed, 0.25); // clamped
  r.stepSpeed(+1); assert.equal(r.speed, 0.5);
  r.setPaused(true);
  const t = r.tick;
  r.update(1);
  assert.equal(r.tick, t);
  r.togglePause(); r.setSpeed(4);
  for (let i = 0; i < 1000 && !r.ended; i++) r.update(0.1);
  assert.equal(r.tick, r.last);
  assert.ok(r.paused && r.ended);
  assert.equal(r.time, r.length);
  r.togglePause(); // play again: from the start
  assert.equal(r.tick, r.first);
  assert.ok(!r.paused && !r.ended);
  assert.deepEqual(SPEEDS, [0.25, 0.5, 1, 2, 4]);
});

test("seeks both ways: the world resets and is rebuilt from the snapshots before the target; skipped events stay skipped", () => {
  const { r, got, resets } = player();
  const kill = golden.events.find((e) => e.msg.t === "kill");
  r.seek(kill.tick + 10); // forward past the kill
  assert.equal(resets(), 2); // the seek to the start at creation, then this
  assert.ok(!got.some((m) => m.t === "kill"), "a seek doesn't replay the events it jumped over");
  const ticks = snapTicks(got);
  assert.ok(ticks[0] >= kill.tick + 10 - 30 && ticks.includes(golden.snaps.filter((s) => s.tick <= kill.tick + 10).at(-1).tick), `${ticks}`);
  // the snapshot at the target decodes as the Go decoder's
  const at = golden.snaps.filter((s) => s.tick <= r.tick).at(-1);
  assert.deepEqual({ ...got.find((m) => m.tick === at.tick), t: undefined }, { ...at.want, t: undefined });
  // back before the kill, then play through it: it comes
  r.seek(kill.tick - 20);
  assert.equal(resets(), 3);
  r.update(30 / r.header.simHz);
  assert.equal(got.filter((m) => m.t === "kill").length, 1);
  // clamped to the round
  r.skip(-1e6); assert.equal(r.tick, r.first);
  r.skip(1e6); assert.equal(r.tick, r.last); assert.ok(r.ended);
});

test("timeline markers: the kills (who, headshot) and the round's events", () => {
  const { r } = player();
  const m = r.markers();
  const kills = golden.events.filter((e) => e.msg.t === "kill");
  assert.deepEqual(m.filter((x) => x.kind === "kill").map((x) => [x.tick, x.k, x.v]), kills.map((e) => [e.tick, e.msg.k, e.msg.v]));
  assert.ok(m.every((x) => x.tick >= r.first - 100 && x.tick <= r.last + 100));
  assert.ok(m.filter((x) => x.kind === "round").every((x) => ["live", "planted", "defused", "exploded", "end"].includes(x.ev)));
});

test("notes: a cut file, another snapshot layout, another version", () => {
  const h = rep().header;
  assert.deepEqual(replayNotes(h, { complete: true, schemaHash: h.schemaHash, clientVersion: h.version }), []);
  assert.deepEqual(replayNotes(h, { complete: true, schemaHash: h.schemaHash, clientVersion: "?" }), []);
  assert.match(replayNotes(h, { complete: false, schemaHash: h.schemaHash, clientVersion: "?" })[0], /still being recorded/);
  assert.match(replayNotes(h, { complete: true, schemaHash: "deadbeef", clientVersion: "?" })[0], /another snapshot layout/);
  assert.match(replayNotes({ ...h, version: "abcdef0123" }, { complete: true, schemaHash: h.schemaHash, clientVersion: "1234567890" })[0], /recorded by server abcdef0; this is 1234567/);
  assert.equal(h.schemaHash, NET_SCHEMA_HASH, "the golden replay is this build's layout (regenerate it when the schema changes)");
  assert.equal(roundLabel(h), "lab_bhop · round 1");
});
