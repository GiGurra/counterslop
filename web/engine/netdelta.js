// The delta codec's decoder (--net-codec delta; internal/game/netdelta*.go,
// docs/replication.md). A snapshot is a binary delta from the last one the
// server sent on this connection (or from the zero state: a full one), laid
// out by NET_SCHEMA, which cmd/gojs generates from the server's net tags.
// apply() turns each into the same object JSON.parse gives for the json
// codec's full snapshot, so nothing above net.js knows the codec.
//
// Message: 0xD5, uvarint tick, uvarint ticks back to the base (0: full),
// uint32 LE checksum (FNV-1a of the whole decoded state), the root struct.
import { NET_SCHEMA, NET_SCHEMA_HASH } from "./netschema.gen.js";

export { NET_SCHEMA_HASH };
export const DELTA_MAGIC = 0xd5;

const utf8 = new TextDecoder();
const entity = (n) => (n.e.k === "ptr" ? n.e.e : n.e);

// The zero value of node n (a full snapshot is a delta from it).
export function zeroNV(n) {
  switch (n.k) {
    case "vec3": return { i: 0, kids: [{ i: 0 }, { i: 0 }, { i: 0 }] };
    case "exact3": return { i: 0, kids: [{ i: 0n }, { i: 0n }, { i: 0n }] };
    case "exact": return { i: 0n };
    case "struct": return { i: 0, kids: n.f.map((f) => zeroNV(f.t)) };
    case "str": return { i: 0, s: "" };
    case "gait": return { i: 0, kids: [] };
  }
  return { i: 0 };
}

function reader(bytes) {
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const r = {
    off: 0,
    byte() { if (r.off >= bytes.length) throw new Error("short message"); return bytes[r.off++]; },
    uv() { // a uvarint as a Number (exact up to 2^53)
      let v = 0, mul = 1;
      for (;;) {
        const c = r.byte();
        v += (c & 0x7f) * mul;
        if (c < 0x80) return v;
        mul *= 128;
      }
    },
    uvBig() {
      let v = 0n, sh = 0n;
      for (;;) {
        const c = r.byte();
        v |= BigInt(c & 0x7f) << sh;
        if (c < 0x80) return v;
        sh += 7n;
      }
    },
    zz() { const u = r.uv(); return u % 2 === 0 ? u / 2 : -(u + 1) / 2; },
    u64() { if (r.off + 8 > bytes.length) throw new Error("short message"); const v = dv.getBigUint64(r.off, true); r.off += 8; return v; },
    u32() { if (r.off + 4 > bytes.length) throw new Error("short message"); const v = dv.getUint32(r.off, true); r.off += 4; return v; },
    str() {
      const l = r.uv();
      if (r.off + l > bytes.length) throw new Error("short string");
      const s = utf8.decode(bytes.subarray(r.off, r.off + l));
      r.off += l;
      return s;
    },
  };
  return r;
}

const num = (r, base, hot) => (hot ? base + r.zz() : r.zz());
const exact = (r, base, hot) => (hot ? base ^ r.uvBig() : r.u64());
const gaitAt = (v, j) => (j < v.kids.length ? v.kids[j].i : 0);

// decNV: a value of node n read as a change from base.
function decNV(r, n, base, hot) {
  switch (n.k) {
    case "int": case "angle": return { i: num(r, base.i, hot) };
    case "bool": return { i: r.byte() };
    case "str": return { i: 0, s: r.str() };
    case "exact": return { i: exact(r, base.i, hot) };
    case "vec3": return { i: 0, kids: [0, 1, 2].map((j) => ({ i: num(r, base.kids[j].i, hot) })) };
    case "exact3": {
      const m = r.byte();
      return { i: 0, kids: [0, 1, 2].map((j) => (m & (1 << j) ? { i: exact(r, base.kids[j].i, hot) } : base.kids[j])) };
    }
    case "gait": {
      const h = r.byte(), kids = [];
      for (let j = 0; j < h >> 4; j++) kids.push({ i: gaitAt(base, j) + (h & (1 << j) ? r.zz() : 0) });
      return { i: 0, kids };
    }
    case "struct": {
      let mask = r.uv();
      const kids = new Array(n.f.length);
      for (let j = 0; j < n.f.length; j++) {
        const bit = mask % 2 === 1;
        mask = Math.floor(mask / 2);
        kids[j] = bit ? decNV(r, n.f[j].t, base.kids[j], !!n.f[j].h) : base.kids[j];
      }
      return { i: 0, kids };
    }
    case "ptr":
      if (r.byte() === 0) return { i: 0 };
      return { i: 1, kids: [decNV(r, n.e, base.i ? base.kids[0] : zeroNV(n.e), hot)] };
    case "list": {
      const flags = r.byte();
      if (flags === 0) return { i: 0 };
      if (n.id !== undefined) {
        const en = entity(n);
        let bases;
        if (!(flags & 2)) bases = base.kids;
        else {
          const byId = new Map();
          if (base.i) for (const b of base.kids) byId.set(b.kids[n.id].i, b);
          bases = [];
          let prev = 0;
          for (let c = r.uv(); c > 0; c--) { prev += r.zz(); bases.push(byId.get(prev) || zeroNV(en)); }
        }
        return { i: 1, kids: bases.map((b) => decNV(r, en, b, hot)) };
      }
      const kids = [];
      for (let j = 0, l = r.uv(); j < l; j++) kids.push(decNV(r, n.e, base.i && j < base.kids.length ? base.kids[j] : zeroNV(n.e), hot));
      return { i: 1, kids };
    }
    case "map": {
      const flags = r.byte();
      if (flags === 0) return { i: 0 };
      let keys = base.keys;
      if (flags & 2) { keys = []; for (let c = r.uv(); c > 0; c--) keys.push(r.str()); }
      const kids = keys.map((k) => {
        const x = base.i ? base.keys.indexOf(k) : -1;
        return decNV(r, n.e, x >= 0 ? base.kids[x] : zeroNV(n.e), hot);
      });
      return { i: 1, keys, kids };
    }
  }
  throw new Error(`netdelta: unknown kind ${n.k}`);
}

// FNV-1a (32 bits) over the canonical bytes of a value (hashNV in Go).
function hasher() {
  let h = 0x811c9dc5;
  const enc = new TextEncoder(), b8 = new DataView(new ArrayBuffer(8));
  const H = {
    byte(c) { h = Math.imul(h ^ c, 16777619) >>> 0; },
    uv(v) { while (v >= 128) { H.byte((v % 128) | 128); v = Math.floor(v / 128); } H.byte(v); },
    zz(v) { H.uv(v >= 0 ? v * 2 : -v * 2 - 1); },
    u64(v) { b8.setBigUint64(0, v, true); for (let j = 0; j < 8; j++) H.byte(b8.getUint8(j)); },
    str(s) { const b = enc.encode(s); H.uv(b.length); for (const c of b) H.byte(c); },
    sum: () => h,
  };
  return H;
}

function hashNV(H, n, v) {
  switch (n.k) {
    case "int": case "angle": return H.zz(v.i);
    case "bool": return H.byte(v.i);
    case "str": return H.str(v.s);
    case "exact": return H.u64(v.i);
    case "vec3": case "gait": H.uv(v.kids.length); for (const k of v.kids) H.zz(k.i); return;
    case "exact3": for (const k of v.kids) H.u64(k.i); return;
    case "struct": n.f.forEach((f, j) => hashNV(H, f.t, v.kids[j])); return;
    case "ptr": if (!v.i) return H.byte(0); H.byte(1); return hashNV(H, n.e, v.kids[0]);
    case "list": case "map": {
      if (!v.i) return H.byte(0);
      H.byte(1); H.uv(v.kids.length);
      const en = n.k === "list" && n.id !== undefined ? entity(n) : n.e;
      v.kids.forEach((k, j) => { if (n.k === "map") H.str(v.keys[j]); hashNV(H, en, k); });
      return;
    }
  }
}

export function checksum(n, v) { const H = hasher(); hashNV(H, n, v); return H.sum(); }

const f64 = new DataView(new ArrayBuffer(8));
const bitsToFloat = (b) => { f64.setBigUint64(0, BigInt.asUintN(64, b), true); return f64.getFloat64(0, true); };

// toJSON: v as JSON.parse of the json codec's snapshot would have it.
function toJSON(n, v) {
  switch (n.k) {
    case "int": return v.i;
    case "bool": return v.i !== 0;
    case "str": return v.s;
    case "exact": return bitsToFloat(v.i);
    case "angle": return v.i / 1e4;
    case "vec3": return v.kids.map((k) => k.i / 1000);
    case "exact3": return v.kids.map((k) => bitsToFloat(k.i));
    case "gait": return v.kids.map((k) => k.i);
    case "struct": { const o = {}; structJSON(n, v, o); return o; }
    case "ptr": return v.i ? toJSON(n.e, v.kids[0]) : null;
    case "list": {
      if (!v.i) return null;
      const en = n.id !== undefined ? entity(n) : n.e;
      return v.kids.map((k) => toJSON(en, k));
    }
    case "map": {
      if (!v.i) return null;
      const o = {};
      v.keys.forEach((k, j) => { o[k] = toJSON(n.e, v.kids[j]); });
      return o;
    }
  }
}

function structJSON(n, v, o) {
  n.f.forEach((f, j) => {
    const c = v.kids[j];
    if (f.i) { if (c.i) structJSON(f.t.e, c.kids[0], o); return; }
    if (f.o && empty(f.t, c)) return;
    o[f.n] = toJSON(f.t, c);
  });
}

// encoding/json's omitempty.
function empty(n, v) {
  switch (n.k) {
    case "int": case "bool": case "angle": return v.i === 0;
    case "exact": return bitsToFloat(v.i) === 0;
    case "str": return v.s === "";
    case "ptr": return !v.i;
    case "list": case "map": return !v.i || v.kids.length === 0;
  }
  return false;
}

// createDeltaDecoder: one connection's decoder. apply(bytes) returns
// { snap } (the decoded snapshot, a fresh object), { stale: true } (a tick
// not newer than the state's: a duplicate or a late one, drop it) or
// { error } (a missing base or a checksum mismatch: ask the server for a full one).
export function createDeltaDecoder(schema = NET_SCHEMA) {
  let state = null, tick = -1;
  const root0 = zeroNV(schema);
  return {
    get tick() { return tick; },
    reset() { state = null; tick = -1; },
    apply(buf) {
      const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
      const r = reader(bytes);
      try {
        if (r.byte() !== DELTA_MAGIC) return { error: "not a delta snapshot" };
        const t = r.uv(), back = r.uv(), sum = r.u32();
        if (state && t <= tick) return { stale: true };
        let base = root0;
        if (back > 0) {
          if (!state || tick !== t - back) return { error: `missing base ${t - back} (have ${tick})`, tick: t };
          base = state;
        }
        const v = decNV(r, schema, base, false);
        if (r.off !== bytes.length) return { error: `${bytes.length - r.off} bytes left over`, tick: t };
        const got = checksum(schema, v);
        if (got !== sum) return { error: `checksum mismatch at tick ${t} (${got.toString(16)} != ${sum.toString(16)})`, mismatch: true, tick: t };
        state = v; tick = t;
        return { snap: toJSON(schema, v) };
      } catch (e) {
        return { error: String(e.message || e) };
      }
    },
  };
}

// ---- the encoder (client-side recording, game/recorder.js) ----
// The same bytes rules as the server's encNV (internal/game/netdelta.go):
// a snapshot object (as the json codec or the decoder gives it) is read into
// a value tree (fromJSON: the server's valueNV, its quantisation included),
// then written as a change from the last one (or from zero: a keyframe).

// Halves away from zero (Go's math.Round; Math.round sends -0.5 to 0).
const roundAway = (x) => { const t = Math.trunc(x); return Math.abs(x - t) >= 0.5 ? t + Math.sign(x) : t; };
const floatBits = (x) => { f64.setFloat64(0, Number(x) || 0, true); return f64.getBigUint64(0, true); };

// fromJSON: x (node n's JSON) as a value tree, like decNV builds.
// omitempty: an empty list or map is nil (encoding/json leaves both out).
export function fromJSON(n, x, omitempty = false) {
  switch (n.k) {
    case "int": return { i: Math.trunc(Number(x) || 0) };
    case "bool": return { i: x ? 1 : 0 };
    case "str": return { i: 0, s: x == null ? "" : String(x) };
    case "exact": return { i: floatBits(x) };
    case "angle": return { i: roundAway((Number(x) || 0) * 1e4) };
    case "vec3": return { i: 0, kids: [0, 1, 2].map((j) => ({ i: roundAway((Number(x?.[j]) || 0) * 1000) })) };
    case "exact3": return { i: 0, kids: [0, 1, 2].map((j) => ({ i: floatBits(x?.[j]) })) };
    case "gait": return { i: 0, kids: (x || []).map((v) => ({ i: Math.trunc(v) })) };
    case "struct": return { i: 0, kids: n.f.map((f) => structFieldJSON(f, x || {})) };
    case "ptr": return x == null ? { i: 0 } : { i: 1, kids: [fromJSON(n.e, x)] };
    case "list": {
      if (x == null || (omitempty && !x.length)) return { i: 0 };
      const en = n.id !== undefined ? entity(n) : n.e;
      return { i: 1, kids: x.map((e) => (n.id !== undefined && e == null ? zeroNV(en) : fromJSON(en, e))) };
    }
    case "map": {
      const keys = x == null ? [] : Object.keys(x).sort(); // Go sorts the keys (byte order = UTF-16 order for these)
      if (x == null || (omitempty && !keys.length)) return { i: 0 };
      return { i: 1, keys, kids: keys.map((k) => fromJSON(n.e, x[k])) };
    }
  }
  throw new Error(`netdelta: unknown kind ${n.k}`);
}
// A struct field from its JSON object: an embedded one (f.i) is there when
// any of its own fields is (its JSON is flattened into the parent's).
function structFieldJSON(f, o) {
  if (f.i) {
    const sub = f.t.e;
    return sub.f.some((g) => g.n in o) ? { i: 1, kids: [fromJSON(sub, o)] } : { i: 0 };
  }
  return f.n in o ? fromJSON(f.t, o[f.n], !!f.o) : zeroNV(f.t);
}

function eqNV(a, b) {
  if (a.i !== b.i || (a.s ?? "") !== (b.s ?? "")) return false;
  const ak = a.kids || [], bk = b.kids || [];
  if (ak.length !== bk.length) return false;
  const ks = a.keys || [], ls = b.keys || [];
  if (ks.length !== ls.length || ks.some((k, j) => k !== ls[j])) return false;
  for (let j = 0; j < ak.length; j++) if (!eqNV(ak[j], bk[j])) return false;
  return true;
}

function writer() {
  let buf = new Uint8Array(256), n = 0;
  const utf8e = new TextEncoder();
  const room = (k) => { if (n + k > buf.length) { const b = new Uint8Array(Math.max(buf.length * 2, n + k)); b.set(buf.subarray(0, n)); buf = b; } };
  const w = {
    byte(c) { room(1); buf[n++] = c; },
    uv(v) { // a Number (exact to 2^53) or a BigInt
      if (typeof v === "bigint") { while (v >= 128n) { w.byte(Number(v & 127n) | 128); v >>= 7n; } w.byte(Number(v)); return; }
      while (v >= 128) { w.byte((v % 128) | 128); v = Math.floor(v / 128); }
      w.byte(v);
    },
    zz(v) { w.uv(v >= 0 ? v * 2 : -v * 2 - 1); },
    u64(v) { room(8); new DataView(buf.buffer).setBigUint64(n, v, true); n += 8; },
    u32(v) { room(4); new DataView(buf.buffer).setUint32(n, v, true); n += 4; },
    str(s) { const b = utf8e.encode(s); w.uv(b.length); room(b.length); buf.set(b, n); n += b.length; },
    bytes: () => buf.slice(0, n),
  };
  return w;
}

const putNum = (w, cur, base, hot) => w.zz(hot ? cur - base : cur);
const putExact = (w, cur, base, hot) => (hot ? w.uv(cur ^ base) : w.u64(cur));

// encNV: cur as a change from base (Go encNV, byte for byte).
function encNV(w, n, cur, base, hot) {
  switch (n.k) {
    case "int": case "angle": return putNum(w, cur.i, base.i, hot);
    case "bool": return w.byte(cur.i);
    case "str": return w.str(cur.s);
    case "exact": return putExact(w, cur.i, base.i, hot);
    case "vec3": for (let j = 0; j < 3; j++) putNum(w, cur.kids[j].i, base.kids[j].i, hot); return;
    case "exact3": {
      let m = 0;
      for (let j = 0; j < 3; j++) if (cur.kids[j].i !== base.kids[j].i) m |= 1 << j;
      w.byte(m);
      for (let j = 0; j < 3; j++) if (m & (1 << j)) putExact(w, cur.kids[j].i, base.kids[j].i, hot);
      return;
    }
    case "gait": {
      let m = 0;
      cur.kids.forEach((k, j) => { if (k.i !== gaitAt(base, j)) m |= 1 << j; });
      w.byte((cur.kids.length << 4) | m);
      cur.kids.forEach((k, j) => { if (m & (1 << j)) w.zz(k.i - gaitAt(base, j)); });
      return;
    }
    case "struct": {
      let mask = 0;
      n.f.forEach((f, j) => { if (!eqNV(cur.kids[j], base.kids[j])) mask += 2 ** j; });
      w.uv(mask);
      n.f.forEach((f, j) => { if (Math.floor(mask / 2 ** j) % 2) encNV(w, f.t, cur.kids[j], base.kids[j], !!f.h); });
      return;
    }
    case "ptr":
      if (!cur.i) return w.byte(0);
      w.byte(1);
      return encNV(w, n.e, cur.kids[0], base.i ? base.kids[0] : zeroNV(n.e), hot);
    case "list": {
      if (!cur.i) return w.byte(0);
      if (n.id !== undefined) {
        const en = entity(n), idOf = (v) => v.kids[n.id].i;
        const same = base.i && base.kids.length === cur.kids.length && cur.kids.every((k, j) => idOf(k) === idOf(base.kids[j]));
        if (same) { w.byte(1); cur.kids.forEach((k, j) => encNV(w, en, k, base.kids[j], hot)); return; }
        w.byte(3); w.uv(cur.kids.length);
        let prev = 0;
        for (const k of cur.kids) { w.zz(idOf(k) - prev); prev = idOf(k); }
        const byId = new Map();
        if (base.i) for (const b of base.kids) byId.set(idOf(b), b);
        for (const k of cur.kids) encNV(w, en, k, byId.get(idOf(k)) || zeroNV(en), hot);
        return;
      }
      w.byte(1); w.uv(cur.kids.length);
      cur.kids.forEach((k, j) => encNV(w, n.e, k, base.i && j < base.kids.length ? base.kids[j] : zeroNV(n.e), hot));
      return;
    }
    case "map": {
      if (!cur.i) return w.byte(0);
      if (base.i && base.keys.length === cur.keys.length && cur.keys.every((k, j) => k === base.keys[j])) w.byte(1);
      else { w.byte(3); w.uv(cur.keys.length); for (const k of cur.keys) w.str(k); }
      cur.keys.forEach((k, j) => {
        const x = base.i ? base.keys.indexOf(k) : -1;
        encNV(w, n.e, cur.kids[j], x >= 0 ? base.kids[x] : zeroNV(n.e), hot);
      });
      return;
    }
  }
  throw new Error(`netdelta: unknown kind ${n.k}`);
}

// createDeltaEncoder: snapshots in, delta messages out (createDeltaDecoder
// reads them back). encode(snap, tick, key): key (or no base yet) writes a
// keyframe (back 0); otherwise the change from the last one encoded.
export function createDeltaEncoder(schema = NET_SCHEMA) {
  let base = null, baseTick = -1;
  const root0 = zeroNV(schema);
  return {
    reset() { base = null; baseTick = -1; },
    encode(snap, tick, key = false) {
      const cur = fromJSON(schema, snap);
      const full = key || !base || tick <= baseTick;
      const w = writer();
      w.byte(DELTA_MAGIC); w.uv(tick); w.uv(full ? 0 : tick - baseTick); w.u32(checksum(schema, cur));
      encNV(w, schema, cur, full ? root0 : base, false);
      base = cur; baseTick = tick;
      return w.bytes();
    },
  };
}
