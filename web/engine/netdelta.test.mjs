import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createDeltaDecoder, NET_SCHEMA_HASH, BASE_RING } from "./netdelta.js";

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

test("a delta whose base never came: missing base, the state stays where it was", () => {
  const d = createDeltaDecoder();
  d.apply(bytes(golden.msgs[0].b));
  const r = d.apply(bytes(golden.msgs[2].b));
  assert.match(r.error, /missing base/);
  assert.equal(d.tick, JSON.parse(JSON.stringify(golden.msgs[0].want)).tick, "the state stays where it was");
});

// internal/game TestNetDeltaLossGolden: what one client read over a lossy
// link (10% loss and duplicates, reordering, its acks late and lost, a 300 ms
// stall), in arrival order. Deltas name older acked bases (kept ones); the
// client misses what the Go one missed (around the stall's fallback full).
const lossy = JSON.parse(readFileSync(new URL("../../internal/game/testdata/netdelta_loss_golden.json", import.meta.url)));

test("over a lossy link: deltas from older acked (kept) bases decode, late and duplicate ones are stale, exactly as the Go decoder", () => {
  assert.equal(lossy.hash, NET_SCHEMA_HASH);
  const d = createDeltaDecoder();
  let older = 0, compared = 0;
  lossy.msgs.forEach((m, i) => {
    const b = bytes(m.b);
    const r = d.apply(b);
    if (m.want === "stale") return assert.deepEqual(r, { stale: true }, `message ${i}`);
    if (typeof m.want === "string" && m.want !== "ok") return assert.equal(r.error, m.want, `message ${i}: the Go decoder's error`); // after the stall's fallback full
    assert.equal(r.error, undefined, `message ${i}: ${r.error}`);
    if (m.want !== "ok") { assert.deepEqual(r.snap, m.want, `message ${i}`); compared++; }
    if (b[2] > 1 && b[2] < 128) older++; // ticks back to the base (one-byte varint): not the last one
  });
  assert.ok(older > 50, `${older} deltas from a base older than the last tick`);
  assert.ok(compared >= 10);
});

test("the decoder keeps BASE_RING kept snapshots as bases (Go baseRing)", () => {
  const go = readFileSync(new URL("../../internal/game/netdelta_codec.go", import.meta.url), "utf8");
  assert.equal(Number(go.match(/const baseRing = (\d+)/)[1]), BASE_RING);
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
