import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createDeltaDecoder, NET_SCHEMA_HASH } from "./netdelta.js";

// internal/game TestNetDeltaGolden records one client's delta snapshots from
// the Go encoder and what each decodes to (the Go decoder, checked against
// the json codec's snapshots there).
const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/netdelta_golden.json", import.meta.url)));
const bytes = (b64) => new Uint8Array(Buffer.from(b64, "base64"));

test("the golden stream was recorded with this schema", () => {
  assert.equal(golden.hash, NET_SCHEMA_HASH);
});

test("the client decodes the Go encoder's stream to the same snapshots", () => {
  const d = createDeltaDecoder();
  assert.ok(golden.msgs.length > 40);
  golden.msgs.forEach((m, i) => {
    const r = d.apply(bytes(m.b));
    assert.equal(r.error, undefined, `message ${i}: ${r.error}`);
    assert.deepEqual(r.snap, m.want, `message ${i}`);
  });
});

test("a lost delta: the next one's base is missing", () => {
  const d = createDeltaDecoder();
  d.apply(bytes(golden.msgs[0].b));
  const r = d.apply(bytes(golden.msgs[2].b));
  assert.match(r.error, /missing base/);
  assert.equal(d.tick, JSON.parse(JSON.stringify(golden.msgs[0].want)).tick, "the state stays where it was");
});

test("a duplicate or an older snapshot is stale", () => {
  const d = createDeltaDecoder();
  d.apply(bytes(golden.msgs[0].b));
  d.apply(bytes(golden.msgs[1].b));
  assert.deepEqual(d.apply(bytes(golden.msgs[1].b)), { stale: true });
  assert.deepEqual(d.apply(bytes(golden.msgs[0].b)), { stale: true });
});

test("a corrupted snapshot fails its checksum (or its framing), never applies", () => {
  for (let i = 1; i < 6; i++) {
    const d = createDeltaDecoder();
    d.apply(bytes(golden.msgs[0].b));
    const b = bytes(golden.msgs[1].b);
    b[b.length - i] ^= 0x01; // a state byte, from the end
    const r = d.apply(b);
    assert.ok(r.error, `flipped byte -${i}: ${JSON.stringify(r).slice(0, 80)}`);
    assert.equal(d.tick, golden.msgs[0].want.tick);
  }
  const d = createDeltaDecoder();
  const b = bytes(golden.msgs[0].b);
  b[3] ^= 0x10; // the checksum itself
  assert.equal(d.apply(b).mismatch, true);
});

test("a full snapshot puts a decoder with a lost base right", () => {
  const fullAt = golden.msgs.findIndex((m, i) => i > 0 && bytes(m.b)[2] === 0 && bytes(m.b)[1] < 128);
  assert.ok(fullAt > 0, "the golden stream has a requested full");
  const d = createDeltaDecoder();
  d.apply(bytes(golden.msgs[0].b));
  assert.ok(d.apply(bytes(golden.msgs[fullAt - 1].b)).error);
  assert.deepEqual(d.apply(bytes(golden.msgs[fullAt].b)).snap, golden.msgs[fullAt].want);
  assert.deepEqual(d.apply(bytes(golden.msgs[fullAt + 1].b)).snap, golden.msgs[fullAt + 1].want);
});
