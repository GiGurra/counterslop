// Replay files (server recordings, internal/game/tv.go; docs/replays.md): read
// one into its header, snapshots and events, and decode snapshots at any tick
// with the schema the file carries (so an old replay decodes after the
// snapshot schema changed). Pure: runs in the browser and in Node.
//
// File (gzip): "CSTV" | uvarint n | header JSON (n bytes) | records:
//   'S' | uvarint n | a delta-codec snapshot (netdelta.js)
//   'E' | uvarint tick | uvarint n | an event message's JSON
import { createDeltaDecoder } from "./netdelta.js";

export const REPLAY_EXT = ".cstv.gz";
const MAGIC = [0x43, 0x53, 0x54, 0x56]; // "CSTV"
const utf8 = new TextDecoder();

// gunzip: the decompressed bytes of a .cstv.gz (DecompressionStream, browser
// and Node). A file still being recorded ends mid-stream: what decompressed
// before the cut is kept ({bytes, complete: false}).
export async function gunzip(gz) {
  const reader = new Blob([gz]).stream().pipeThrough(new DecompressionStream("gzip")).getReader();
  const parts = [];
  let complete = true, n = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      parts.push(value); n += value.length;
    }
  } catch {
    complete = false;
  }
  const bytes = new Uint8Array(n);
  let o = 0;
  for (const p of parts) { bytes.set(p, o); o += p.length; }
  return { bytes, complete };
}

function uvarint(b, at) {
  let x = 0, s = 1;
  for (let i = at.off; i < b.length; i++) {
    const c = b[i];
    x += (c & 0x7f) * s;
    if (c < 0x80) { at.off = i + 1; return x; }
    s *= 128;
  }
  return -1; // cut short
}

// readReplay: the decompressed file into {header, snaps, events, complete}.
// snaps: [{tick, back (0: a keyframe), bytes}], in order; events: [{tick, msg}].
// complete is false when the last record is cut (a live file); the records
// before it are kept.
export function readReplay(bytes) {
  if (bytes.length < 5 || MAGIC.some((c, i) => bytes[i] !== c)) throw new Error("not a counterslop replay");
  const at = { off: 4 };
  const hn = uvarint(bytes, at);
  if (hn < 0 || at.off + hn > bytes.length) throw new Error("replay header cut short");
  const header = JSON.parse(utf8.decode(bytes.subarray(at.off, at.off + hn)));
  at.off += hn;
  if (header.format !== 1) throw new Error(`replay format ${header.format}: this client reads 1`);
  const snaps = [], events = [];
  let complete = true;
  while (at.off < bytes.length) {
    const kind = bytes[at.off++];
    let tick = 0;
    if (kind === 0x45) tick = uvarint(bytes, at); // 'E'
    else if (kind !== 0x53) throw new Error(`bad replay record ${kind}`);
    const n = uvarint(bytes, at);
    if (tick < 0 || n < 0 || at.off + n > bytes.length) { complete = false; break; }
    const body = bytes.subarray(at.off, at.off + n);
    at.off += n;
    if (kind === 0x45) events.push({ tick, msg: JSON.parse(utf8.decode(body)) });
    else {
      const h = { off: 1 };
      const t = uvarint(body, h), back = uvarint(body, h);
      snaps.push({ tick: t, back, bytes: body });
    }
  }
  return { header, snaps, events, complete };
}

// createReplayCursor: snapshots of a read replay at any tick, forward or
// back. at(tick) is the newest snapshot at or before tick (null before the
// first): decoded from the keyframe before it, or onward from the last one
// decoded when that is on the way.
export function createReplayCursor(rep) {
  const dec = createDeltaDecoder(rep.header.schema);
  let cur = -1, snap = null; // index of the snapshot decoded last, and it
  const indexAt = (tick) => {
    let lo = 0, hi = rep.snaps.length - 1, i = -1;
    while (lo <= hi) { const m = (lo + hi) >> 1; if (rep.snaps[m].tick <= tick) { i = m; lo = m + 1; } else hi = m - 1; }
    return i;
  };
  const keyBefore = (i) => { while (i > 0 && rep.snaps[i].back !== 0) i--; return i; };
  return {
    get first() { return rep.snaps[0]?.tick ?? 0; },
    get last() { return rep.snaps.at(-1)?.tick ?? 0; },
    // the keyframes' ticks (seek points, one a second)
    keyframes: () => rep.snaps.filter((s) => s.back === 0).map((s) => s.tick),
    at(tick) {
      const i = indexAt(tick);
      if (i < 0) return null;
      if (i === cur) return snap;
      let from = keyBefore(i);
      if (cur >= from && cur < i) from = cur + 1; // forward from where we are
      else dec.reset();
      for (let j = from; j <= i; j++) {
        const r = dec.apply(rep.snaps[j].bytes);
        if (r.error) throw new Error(`replay snapshot at tick ${rep.snaps[j].tick}: ${r.error}`);
        snap = r.snap; cur = j;
      }
      return snap;
    },
    // the events in (from, to]: what happened since the last frame drawn
    eventsBetween: (from, to) => rep.events.filter((e) => e.tick > from && e.tick <= to),
  };
}
