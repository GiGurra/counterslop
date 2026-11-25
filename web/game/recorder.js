// Client-side recording (docs/replays.md, part 4): the rounds this client
// plays, recorded from what it received, in the server recorder's file format
// (internal/game/tv.go; engine/replay.js reads both). The snapshots (full
// ones, every EVERY ticks: the server's rate) are re-encoded with the delta
// codec (engine/netdelta.js createDeltaEncoder, a keyframe every KEY_EVERY
// ticks); the events (kills, shots, grenades, round events...) go in as JSON
// at the tick of the newest snapshot. A file per round, kept in memory (the
// last `keep`), gzipped when saved. Pure: runs in Node (recorder.test.mjs).
//
// What a client recording has that the server's doesn't: the self-snapshot
// (me) and the team's spotted list, as this player got them; header.pov is
// the recording player's id (the replay player follows it).
import { createDeltaEncoder, NET_SCHEMA_HASH } from "../engine/netdelta.js";
import { NET_SCHEMA } from "../engine/netschema.gen.js";

export const EVERY = 3, KEY_EVERY = 100, KEEP = 30;
// The events a replay plays (as the server records them).
const EVENTS = new Set(["kill", "shot", "nade", "round", "gpu", "notice", "hit"]);
const utf8 = new TextEncoder();

function uvarint(v) {
  const out = [];
  while (v >= 128) { out.push((v % 128) | 128); v = Math.floor(v / 128); }
  out.push(v);
  return out;
}
const concat = (parts) => {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let o = 0;
  for (const p of parts) { out.set(p, o); o += p.length; }
  return out;
};
// The server's match id: UTC yyyymmdd-hhmmss-<map>.
const matchID = (ms, map) => new Date(ms).toISOString().replace(/[-:]/g, "").replace("T", "-").slice(0, 15) + "-" + map;

// gzip (CompressionStream, browser and Node): the file as saved.
export async function gzip(bytes) {
  const s = new Blob([bytes]).stream().pipeThrough(new CompressionStream("gzip"));
  return new Uint8Array(await new Response(s).arrayBuffer());
}

export function createRecorder({ keep = KEEP } = {}) {
  const rounds = []; // finished and the live one, oldest first
  let cur = null, enc = createDeltaEncoder();
  let last = null; // the newest full snapshot and its context: a round started by its event begins from it
  let tickSeen = -1, match = "", matchMap = "", matchN = 0;

  const put = (parts) => { const b = concat(parts); cur.chunks.push(b); cur.size += b.length; };
  function begin(n, snap, ctx, startTick) {
    finish();
    const map = ctx.map;
    if (!match || matchMap !== map.name || n <= matchN) { match = matchID(snap.time || 0, map.name); matchMap = map.name; }
    matchN = n;
    const header = {
      format: 1, version: ctx.version || "", match, round: n, map, schemaHash: NET_SCHEMA_HASH, schema: NET_SCHEMA,
      simHz: ctx.simHz || 100, every: EVERY, keyEvery: KEY_EVERY, startTick, started: snap.time || 0,
      players: (snap.players || []).map((p) => ({ id: p.id, name: p.name, team: p.team, ...(p.bot || p.ai ? { bot: true } : {}) })),
      score: snap.round?.sc || {}, pov: ctx.myId || 0, recorder: "client",
    };
    const h = utf8.encode(JSON.stringify(header));
    cur = { match, round: n, map: map.name, live: true, header, chunks: [], size: 0, lastSnap: -1, lastKey: -1, started: snap.time || 0 };
    enc = createDeltaEncoder();
    put([new Uint8Array([0x43, 0x53, 0x54, 0x56]), new Uint8Array(uvarint(h.length)), h]);
    rounds.push(cur);
    while (rounds.length > keep) rounds.shift();
  }
  function finish() { if (cur) { cur.live = false; cur = null; } }
  function record(snap) {
    const key = cur.lastKey < 0 || snap.tick - cur.lastKey >= KEY_EVERY;
    const msg = enc.encode(snap, snap.tick, key);
    put([new Uint8Array([0x53]), new Uint8Array(uvarint(msg.length)), msg]);
    cur.lastSnap = snap.tick;
    if (key) cur.lastKey = snap.tick;
  }

  const r = {
    // Every snapshot received (ctx: {map, version, myId, simHz}): pose ones
    // only move the clock; a full one outside warmup is recorded (a new
    // round number starts a new file).
    snap(m, ctx) {
      if (m.tick !== undefined) tickSeen = Math.max(tickSeen, m.tick);
      if (m.pose || m.tick === undefined || !ctx?.map) return;
      const rd = m.round;
      last = { snap: m, ctx }; // warmup's too: round 1's start event opens its file from it
      if (!rd || rd.ph === "warmup") { finish(); return; } // warmup isn't recorded (the server's neither)
      if (!cur || cur.round !== rd.n || cur.map !== ctx.map.name) begin(rd.n, m, ctx, m.tick);
      if (cur.lastSnap >= 0 && m.tick - cur.lastSnap < EVERY) return;
      if (m.tick <= cur.lastSnap) return; // an old one (reordered): the file goes forward only
      record(m);
    },
    // An event message received. A round's start event opens its file (it
    // comes before that round's first snapshot), from the last snapshot seen.
    event(m, ctx) {
      if (!EVENTS.has(m.t)) return;
      if (m.t === "round" && m.ev === "start" && last) begin(m.n, last.snap, { ...last.ctx, ...ctx }, tickSeen + 1);
      if (!cur) return;
      const msg = m.t === "hit" && ctx?.myId ? { by: ctx.myId, ...m } : m; // the server's file says whose hit it was
      const b = utf8.encode(JSON.stringify(msg));
      put([new Uint8Array([0x45]), new Uint8Array(uvarint(Math.max(0, tickSeen))), new Uint8Array(uvarint(b.length)), b]);
    },
    // The connection ended or the map changed: the round's file ends here.
    stop() { finish(); last = null; },
    // The recordings, oldest first: {match, round, map, live, size, started, name}.
    list: () => rounds.map((x) => ({ match: x.match, round: x.round, map: x.map, live: x.live, size: x.size, started: x.started, name: fileName(x) })),
    // A round's file (gzip, as the server stores them); a live one as far as it has got.
    async file(i) { const x = rounds[i]; if (!x) throw new Error(`no recording ${i}`); return gzip(concat(x.chunks)); },
    // The raw (uncompressed) bytes: tests.
    raw: (i) => concat(rounds[i].chunks),
    clear() { rounds.length = 0; cur = null; last = null; },
  };
  return r;
}

// The saved file's name: <match>-round-NN.cstv.gz (as the server's downloads).
export const fileName = (x) => `${x.match}-round-${String(x.round).padStart(2, "0")}.cstv.gz`;
