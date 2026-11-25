// The replay player's clock and feed (docs/replays.md): plays a read replay
// (engine/replay.js) into the same message handler the server's socket feeds,
// at a playhead that runs at 0.25-4x, pauses, and seeks both ways. Pure: no
// DOM, runs in Node (replayer.test.mjs); main.js wires it to the game.
//
//   deliver(msg)  a snapshot ({t: "snap", ...}) or an event message, in tick order
//   reset()       the world is about to jump (a seek): forget remotes, effects, the feed
import { createReplayCursor } from "../engine/replay.js";

export const SPEEDS = [0.25, 0.5, 1, 2, 4];
// A seek feeds the snapshots this far before the target first, so the remotes
// have a pair to interpolate (and a velocity) at once.
const SEEK_LEAD = 30;
// Round events shown on the timeline (with kills).
const MARK_ROUND = new Set(["live", "planted", "defused", "exploded", "end"]);

export function createReplayer(rep, { deliver, reset }) {
  const cur = createReplayCursor(rep);
  const hz = rep.header.simHz || 100, every = rep.header.every || 3;
  const snaps = rep.snaps, events = rep.events;
  const first = cur.first, last = cur.last;
  let tick = first, speed = 1, paused = false, ended = false;
  let nextSnap = 0, nextEv = 0; // the first snapshot / event not delivered yet

  const lowerBound = (arr, t) => { // the first index with .tick >= t
    let lo = 0, hi = arr.length;
    while (lo < hi) { const m = (lo + hi) >> 1; if (arr[m].tick < t) lo = m + 1; else hi = m; }
    return lo;
  };
  // Snapshots up to t (the one after the playhead too: something to
  // interpolate toward), events up to the playhead.
  function feed(withEvents = true) {
    const ahead = tick + every;
    while (nextSnap < snaps.length && snaps[nextSnap].tick <= ahead) {
      const s = cur.at(snaps[nextSnap].tick);
      nextSnap++;
      if (s) deliver({ ...s, t: "snap" });
    }
    if (!withEvents) { nextEv = lowerBound(events, Math.floor(tick) + 1); return; }
    while (nextEv < events.length && events[nextEv].tick <= tick) deliver(events[nextEv++].msg);
  }

  const r = {
    header: rep.header,
    complete: rep.complete,
    first, last,
    get tick() { return tick; },
    get speed() { return speed; },
    get paused() { return paused; },
    get ended() { return ended; },
    // seconds into the round, and its length
    get time() { return (tick - first) / hz; },
    get length() { return (last - first) / hz; },
    // Advance by dt seconds of wall time (nothing while paused); stops at the end.
    update(dt) {
      if (!paused) {
        tick += dt * hz * speed;
        if (tick >= last) { tick = last; paused = ended = true; }
      }
      feed();
    },
    // Jump to tick t (either way): the world is rebuilt from the snapshots
    // before it; the events skipped are not replayed (the kill feed starts over).
    seek(t) {
      tick = Math.max(first, Math.min(last, t));
      ended = false;
      if (tick >= last) paused = ended = true;
      reset();
      nextSnap = lowerBound(snaps, tick - SEEK_LEAD);
      nextEv = 0;
      feed(tick <= first); // from the start: the round's opening events (before its first snapshot) too
    },
    // Seek by seconds (negative: back).
    skip(s) { r.seek(tick + s * hz); },
    setPaused(p) {
      if (!p && ended) { r.seek(first); ended = false; } // play at the end: from the start
      paused = !!p;
    },
    togglePause() { r.setPaused(!paused); },
    setSpeed(x) { speed = SPEEDS.reduce((a, b) => (Math.abs(b - x) < Math.abs(a - x) ? b : a)); },
    // One step up or down the SPEEDS list.
    stepSpeed(dir) { const i = SPEEDS.indexOf(speed); speed = SPEEDS[Math.max(0, Math.min(SPEEDS.length - 1, i + dir))]; },
    // The timeline's marks: kills ({tick, kind: "kill", k, v, hs, w}) and
    // round events ({tick, kind: "round", ev, win}).
    markers() {
      const out = [];
      for (const e of events) {
        const m = e.msg;
        if (m.t === "kill") out.push({ tick: e.tick, kind: "kill", k: m.k, v: m.v, hs: !!m.hs, w: m.w });
        else if (m.t === "round" && MARK_ROUND.has(m.ev)) out.push({ tick: e.tick, kind: "round", ev: m.ev, win: m.win || "" });
      }
      return out;
    },
  };
  r.seek(first);
  return r;
}

// replayNotes: what to tell the viewer about a replay before it plays: a
// cut-short file (the round was still running), and a version mismatch (the
// snapshot layout changed: it decodes with the file's own schema, but what
// this client draws from newer fields is missing). clientVersion "" or "?":
// unknown (no note).
export function replayNotes(header, { complete, schemaHash, clientVersion }) {
  const notes = [];
  if (!complete) notes.push("this round was still being recorded: it ends where the file does");
  const v = (x) => String(x || "?").slice(0, 7);
  if (header.schemaHash !== schemaHash) notes.push(`recorded by server ${v(header.version)} with another snapshot layout than this client's: it plays, but some things may not be drawn`);
  else if (clientVersion && clientVersion !== "?" && header.version && header.version !== clientVersion) notes.push(`recorded by server ${v(header.version)}; this is ${v(clientVersion)}`);
  return notes;
}

// roundLabel: "de_slop · round 3" for lists and the player bar.
export const roundLabel = (h) => `${h.map?.name ?? "?"} · round ${h.round ?? "?"}`;
