import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createDeltaDecoder, checksum, NET_SCHEMA_HASH } from "./netdelta.js";

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

// The checksum's fast paths (ASCII strings straight from their char codes,
// a float's bits a byte at a time) hash the same bytes as hashNV in Go:
// FNV-1a over uvarint(len) + UTF-8, and the 8 bits bytes little-endian.
test("the checksum hashes strings and float bits as Go does", () => {
  const fnv = (bs) => bs.reduce((h, c) => Math.imul(h ^ c, 16777619) >>> 0, 0x811c9dc5);
  const uv = (v) => { const o = []; while (v >= 128) { o.push((v % 128) | 128); v = Math.floor(v / 128); } o.push(v); return o; };
  for (const s of ["", "alice", "Target Practice (bot 2)", "åsa ☕ 🚀", "x".repeat(200)]) {
    const b = [...new TextEncoder().encode(s)];
    assert.equal(checksum({ k: "str" }, { s }), fnv([...uv(b.length), ...b]), s);
  }
  for (const f of [0, -0, 1.5, -123.456, 1e300, Number.MIN_VALUE]) {
    const dv = new DataView(new ArrayBuffer(8));
    dv.setFloat64(0, f, true);
    const bits = dv.getBigUint64(0, true);
    assert.equal(checksum({ k: "exact" }, { i: bits }), fnv([...new Uint8Array(dv.buffer)]), String(f));
  }
});
