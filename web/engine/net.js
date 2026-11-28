// Websocket client with auto-reconnect, ping, and remote-player interpolation.
import { gaitFromWire, gaitBetween, poseTime } from "../gen/gait.js";
import { createDeltaDecoder, NET_SCHEMA_HASH } from "./netdelta.js";
import { mulberry32 } from "../gen/rng.js";
import { prof } from "./prof.js";
// The delay on a clean link with a snapshot every tick (100 Hz): two snapshot
// intervals and a tick (cl_interp_ratio 2), no jitter. The adaptive delay
// starts here and has no floor of its own (cl_interp sets one).
export const INTERP_DELAY = 30; // ms behind newest snapshot
// The fake link faults' seed (net_fake*: tests and scenarios repeat exactly).
export const FAULT_SEED = 0x5eed;

// unpackPoses: a pose snapshot's players ([id, x, y, z, yaw, pitch, flags];
// flags 1 crouched, 2 not a body, >>2 the gravity; then for a rider of a
// mover fr, lx, ly, lz: its frame and place in it: server.go posesJSON) as snapshot players.
export function unpackPoses(rows) {
  return rows.map((r) => {
    const p = { id: r[0], p: [r[1], r[2], r[3]], yaw: r[4], pitch: r[5], c: !!(r[6] & 1), nb: !!(r[6] & 2), gv: r[6] >> 2 };
    if (r.length >= 11) { p.fr = r[7]; p.fl = [r[8], r[9], r[10]]; }
    return p;
  });
}

export function createNet(onMsg) {
  let ws = null, name = "", greeted = true, retry = null;
  // Test-only link faults (all off by default):
  // - fakeLag (ms, round trip): net_fakelag, half each way.
  // - faults.jitter (ms): net_fakejitter, a varying delay of 0..jitter on
  //   everything, in order, one queue per direction: a TCP link (websocket over
  //   a VPN tunnel) delays and bunches messages but never reorders or loses them.
  // - faults.reorder (ms): net_fakereorder, each command/snapshot packet delayed
  //   on its own by 0..reorder, so they overtake each other as datagrams would.
  // - faults.loss / dup (0..1): net_fakeloss / net_fakedup, commands and
  //   snapshots only (the unreliable-by-design traffic). The protocol must cope.
  // - delayed.up / .down: messages held back by those faults right now; lost.up / .down:
  //   how many they dropped so far.
  //   The faults draw from seeded generators (FAULT_SEED, one stream per direction),
  //   restarted whenever the fault settings change value: a scripted run (lockstep, virtual
  //   clock) loses, duplicates and delays the same messages every time.
  //   Lockstep cases (cmd/sim -cases) run the page on a virtual clock: while the harness
  //   waits for a command, it moves the clock on as long as one is still held back.
  // codec: the server's snapshot codec (the welcome says; docs/replication.md).
  // delta: binary snapshots decoded here into the json codec's object shape.
  // A missing base or a checksum mismatch drops the snapshot and asks the
  // server for a full one (codecErrors counts them; tests fail on a mismatch).
  // Reconnect state (game/link.js shows it): wanted = we mean to be connected
  // (connect() until a deliberate disconnect()); attempt = failed connections
  // in a row (0 once one opens); retryAt = performance.now() of the next try
  // (0 while one is under way). retryNow() tries at once.
  // cut (tests only, the __cs.cutLink hook): "silent" = the socket stays open
  // but nothing gets through either way (a dead link); "closed" = the socket
  // drops and every reconnect fails until the cut is lifted. A barrier() then has
  // nothing to wait for (it resolves false at once).
  const st = { open: false, ping: null, codec: "json", codecErrors: 0, codecErrorsClean: 0, codecMismatches: 0, codecLastError: "", fullsAsked: 0, fakeLag: 0, faults: { loss: 0, dup: 0, jitter: 0, reorder: 0 }, faultRand: null, delayed: { up: 0, down: 0 }, lost: { up: 0, down: 0 }, connecting: () => ws?.readyState === 0, connect, send, hello, disconnect, barrier,
    wanted: false, attempt: 0, retryAt: 0, cut: null, retryNow, setCut };
  // A fake link fault on (or one that held back a snapshot still due): snapshots may go missing.
  const faked = () => { const f = st.faults; return st.fakeLag > 0 || f.loss > 0 || f.dup > 0 || f.jitter > 0 || f.reorder > 0 || st.delayed.down > 0 || st.lost.down > 0; };
  const later = (fn) => (st.fakeLag > 0 ? setTimeout(fn, st.fakeLag / 2) : fn());
  const UNRELIABLE = new Set(["cmd", "snap", "sack"]);
  // One FIFO per direction, drained by a single timer: separate timers don't
  // reliably fire in deadline order, which would reorder the "in-order" link.
  const fifo = { up: [], down: [] }, pumping = { up: false, down: false };
  function pump(k) {
    const q = fifo[k];
    const now = performance.now();
    while (q.length && q[0].at <= now + 0.5) later(q.shift().fn);
    pumping[k] = q.length > 0;
    if (pumping[k]) setTimeout(() => pump(k), Math.max(0, q[0].at - now));
  }
  // The fault settings: setting different values restarts the seeded draws.
  let faults = st.faults, draw = null;
  const reseed = () => { draw = { up: mulberry32(FAULT_SEED), down: mulberry32(FAULT_SEED + 1) }; };
  delete st.faults;
  Object.defineProperty(st, "faults", {
    enumerable: true,
    get: () => faults,
    set: (f) => {
      const key = (x) => `${x.loss}/${x.dup}/${x.jitter}/${x.reorder}`;
      if (key(f) !== key(faults)) reseed();
      faults = f;
    },
  });
  reseed();
  // The next fault draw in [0, 1) for direction k (see FAULT_SEED above).
  const rand = (k) => (st.faultRand ? st.faultRand() : draw[k]()); // faultRand: tests script the draws
  function ordered(k, fn) {
    const j = st.faults.jitter;
    if (!j && !fifo[k].length) return later(fn);
    const q = fifo[k];
    q.push({ at: Math.max(q.length ? q[q.length - 1].at : 0, performance.now() + rand(k) * j), fn });
    if (!pumping[k]) { pumping[k] = true; setTimeout(() => pump(k), Math.max(0, q[0].at - performance.now())); }
  }
  // k: "up" (to the server) or "down"; t: the message type.
  function faulty(k, t, fn) {
    const f = st.faults;
    if (t === "cmd" && st.dropCmds > 0) { st.dropCmds--; st.lost[k]++; return; } // tests: lose exactly the next N command packets
    const held = () => { st.delayed[k]++; return () => { st.delayed[k]--; fn(); }; }; // counted until it goes
    if (!UNRELIABLE.has(t) || !(f.loss || f.dup || f.reorder)) return ordered(k, held());
    if (rand(k) < f.loss) { st.lost[k]++; return; }
    const copies = rand(k) < f.dup ? 2 : 1;
    for (let i = 0; i < copies; i++) {
      const go = held();
      if (f.reorder) setTimeout(() => ordered(k, go), rand(k) * f.reorder);
      else ordered(k, go);
    }
  }

  const delta = createDeltaDecoder();
  // asked: a full snapshot we asked for and haven't had yet ({at: ms, faulted:
  // asked while a fake fault was on}). Asked again if it doesn't come within
  // reaskMs (it was lost: net_fakeloss), not after a fixed count of snapshots.
  let asked = null;
  // Snapshot acks (delta codec): every command packet carries the newest
  // kept tick we decoded ("sa"; the server keeps one every few as a base),
  // the server bases its deltas on the newest it heard of; with no commands
  // going out (dead, spectating, the menus) a "sack" goes on its own when a
  // newer kept one came and we are SACK_EVERY snapshots past the last ack. Acks are datagrams like the commands: lost, duplicated, reordered.
  // A delta whose base we lack (the server based it on a full we haven't
  // had: still in flight, overtaken, or lost) is answered with an ack of what
  // we have, at once (at most every SACK_MS), so the next ones come from a
  // base we have; a full is asked for only if we aren't in step within
  // reaskMs (gap: when it started). Asking at once made a loop under
  // reordering: each full asked for was overtaken by deltas based on it.
  let ackSent = -1, ackAt = -1, gap = null, sackAt = -Infinity;
  const SACK_EVERY = 4, SACK_MS = 20;
  const reaskMs = () => Math.max(100, 2 * rtt);
  st.fullPending = () => asked !== null;
  st.fullPendingFaulted = () => !!asked?.faulted;
  // The welcome names the codec (none: json). An unknown codec, or a delta
  // schema this client wasn't built with, can't be decoded: refuse it.
  function codecOK(m) {
    st.codec = m.codec || "json";
    let why = "";
    if (st.codec !== "json" && st.codec !== "delta") why = `the server sends snapshots as "${st.codec}", which this client can't read`;
    else if (st.codec === "delta" && m.schema !== NET_SCHEMA_HASH) why = `the server's snapshot schema (${m.schema}) isn't this client's (${NET_SCHEMA_HASH}): reload the page`;
    st.codecLastError = why;
    return !why;
  }
  // The welcome still goes in (a different client build reloads the page on
  // it); then this connection stops.
  function refuse(welcome) {
    console.error(`net: ${st.codecLastError}`);
    onMsg(welcome);
    disconnect();
    onMsg({ t: "notice", text: st.codecLastError });
  }

  function disconnect() {
    clearTimeout(retry);
    st.wanted = false; st.attempt = 0; st.retryAt = 0;
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (st.open) { st.open = false; onMsg({ t: "close" }); }
    dropBarriers();
  }

  // A connection went (or never came): say so once, then try again later.
  // Fast first retries (a deploy restart takes a few seconds), then back off to 3 s.
  function dropped() {
    ws = null;
    const was = st.open;
    st.open = false;
    dropBarriers();
    if (was) onMsg({ t: "close" });
    const wait = Math.min(3000, 250 * 2 ** st.attempt++);
    st.retryAt = performance.now() + wait;
    clearTimeout(retry);
    retry = setTimeout(() => connect(name, greeted), wait);
  }

  // RETRY NOW, or a link that went silent: drop whatever socket we have and
  // connect at once (counts as a failed attempt if the old one was open).
  function retryNow() {
    if (!st.wanted) return;
    if (ws) { ws.onclose = null; ws.close(); ws = null; dropBarriers(); }
    if (st.open) { st.open = false; st.attempt++; onMsg({ t: "close" }); }
    connect(name, greeted);
  }

  function setCut(mode) {
    st.cut = mode || null;
    if (st.cut === "closed" && ws) { ws.onclose = null; ws.close(); dropped(); }
  }

  // greet = false connects as a hidden observer (the menus: the server knows
  // the current map, admin state, and changelevel works before PLAY); hello()
  // joins later on the same socket. A reconnect says hello again only if we had.
  function connect(n, greet = true) {
    name = n;
    greeted = greet;
    st.wanted = true;
    st.retryAt = 0;
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); ws = null; dropBarriers(); } // that socket's barriers never get a pong
    if (st.cut === "closed") { retry = setTimeout(dropped, 0); return; } // tests: the server is unreachable
    const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws";
    ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    delta.reset();
    ackSent = -1; ackAt = -1; gap = null; sackAt = -Infinity; asked = null;
    ws.onopen = () => { st.open = true; st.attempt = 0; if (greeted) send({ t: "hello", name }); onMsg({ t: "open" }); };
    ws.onmessage = (e) => {
      if (st.cut) return;
      prof.begin("net"); // engine/prof.js: socket messages, decode + handlers
      try { onMessage(e); } finally { prof.end(); }
    };
    const onMessage = (e) => {
      if (typeof e.data !== "string") return faulty("down", "snap", () => deliverDelta(e.data));
      prof.begin("decode");
      const m = JSON.parse(e.data);
      prof.end();
      if (m.t === "welcome" && !codecOK(m)) return refuse(m);
      // Test markers skip the fake faults: the barrier's pong, the harness's sync (what
      // those faults hold back is still on its way, on the page's virtual clock).
      if (m.t === "pong" && barriers.has(m.seq)) return barriers.get(m.seq)();
      if (m.t === "sync") return onMsg(m);
      faulty("down", m.t, () => deliver(m));
    };
    const deliverDelta = (buf) => {
      if (st.codec !== "delta") return;
      prof.begin("decode");
      const r = delta.apply(buf);
      prof.end();
      if (r.snap) {
        asked = null; gap = null; // decoded: in step again
        if (delta.kept > ackSent && (ackSent < 0 || delta.tick - ackAt >= SACK_EVERY)) send({ t: "sack" }); // the first at once
        return onMsg(r.snap);
      }
      if (r.stale) return;
      st.codecErrors++;
      if (!faked()) st.codecErrorsClean++; // nothing on the link may lose a snapshot: a codec or server bug (tests fail on it)
      st.codecLastError = r.error;
      if (r.mismatch) { st.codecMismatches++; console.error(`net: delta snapshot ${r.error}`); }
      const now = performance.now();
      if (r.missing) {
        if (delta.kept >= 0 && now - sackAt >= SACK_MS) { sackAt = now; send({ t: "sack" }); }
        gap ??= now;
        if (now - gap < reaskMs()) return;
      }
      // Ask once per gap, again if the full doesn't come within reaskMs (lost).
      if (!asked || now - asked.at >= reaskMs()) { asked = { at: now, faulted: faked() }; st.fullsAsked++; send({ t: "full" }); }
    };
    const deliver = (m) => {
      if (m.t === "pong" && m.seq < 0) return;
      // A measured round trip is never 0 to the server (0 = none yet): on a test's
      // virtual clock the pong can come back at the very ms the ping left.
      if (m.t === "pong") { rtt = Math.max(0.001, performance.now() - m.seq); st.ping = Math.round(rtt); }
      else onMsg(m);
    };
    ws.onclose = dropped;
  }

  // Tests (lockstep): resolves once the server has read everything this page
  // put on its socket before it, and everything the server sent back before
  // the pong has arrived (one socket, in order). The ping and pong skip the
  // fake link faults: a message they hold back is still on its way (virtual
  // time), not yet sent. No game time passes; resolves anyway after 2 s
  // (closed socket). A socket that closes settles the barriers
  // in flight at once (false): on the lockstep virtual clock the 2 s timer
  // never fires while the harness waits (an admin kick hung sim 'admin').
  const barriers = new Map();
  let barrierN = 0;
  function dropBarriers() { for (const done of [...barriers.values()]) done(false); }
  function barrier() {
    if (!st.open || ws?.readyState !== 1 || st.cut) return Promise.resolve(false);
    const seq = -++barrierN;
    return new Promise((r) => {
      const done = (ok) => { barriers.delete(seq); clearTimeout(t); r(ok); };
      const t = setTimeout(() => done(false), 2000);
      barriers.set(seq, (ok = true) => done(ok));
      ws.send(JSON.stringify({ t: "ping", seq }));
    });
  }

  function hello(n) { name = n; greeted = true; send({ t: "hello", name }); }

  function send(o) {
    if ((o.t === "cmd" || o.t === "sack") && st.codec === "delta" && delta.kept >= 0) { o = { ...o, sa: delta.kept }; ackSent = delta.kept; ackAt = delta.tick; }
    const s = JSON.stringify(o);
    faulty("up", o.t, () => { if (ws?.readyState === 1 && !st.cut) ws.send(s); });
  }
  // Each keepalive reports the last round trip (unrounded: never 0 ms on a
  // fast LAN); the server carries it to everyone's scoreboard (players[].ping).
  let rtt = 0;
  setInterval(() => st.open && send({ t: "ping", seq: performance.now(), ...(rtt > 0 && { rtt }) }), 1000);
  return st;
}

// Keeps a short history per remote player and samples it a little in the
// past, on the server's tick timeline (like Source): every snapshot is placed
// at the tick it shows, not at the moment it happened to arrive.
//
// Server clock: the client estimates the server's current tick (offset from
// its own clock). A snapshot that arrives with less delay than any recent one
// moves the estimate forward at once; later ones pull it back only slowly.
// So after a stall (a long frame, GC, a Wi-Fi hiccup) the burst of late
// snapshots fills in history but the view stays at the present instead of
// replaying the last second, and the view tick sent with shots stays within
// reach of the server's lag compensation.
//
// Interpolation delay: how far behind that estimate the remotes are drawn.
// It adapts to the link (CS: cl_interp_ratio 2 plus a jitter buffer): two
// snapshot intervals plus the 95th percentile of how late snapshots arrived
// over the last few seconds, never less than INTERP_DELAY, at most MAX_DELAY
// (the server rewinds shots up to sv_maxunlag 0.5 s, one-way latency included).
//
// The drawn timeline doesn't hop on jitter: the tick estimate it uses follows
// the raw one, and the delay its target, at up to SLEW of real time between
// them (the remotes run 0.9x..1.1x). Past MAX_TRAIL ticks behind, the estimate
// takes the whole SLEW (the delay holds) to catch up, still without a hop (a
// hard clamp at MAX_TRAIL hopped when an early arrival came on top of a trail
// near it); it never trails by more than HARD_TRAIL. A jittery early arrival
// moves the raw estimate by a tick or two, absorbed smoothly; a real change
// (lower latency, or the lockstep harness running server ticks faster than
// this clock) is followed within HARD_TRAIL, so shots' view ticks stay where
// lag compensation expects them.
export const MAX_DELAY = 250; // ms
// A late snapshot: the remotes go on at their last velocity this long (ms),
// then hold; the difference when the data comes eases out over BLEND_MS.
export const EXTRAP_MS = 50, BLEND_MS = 50;
// A rider going into or out of a mover's frame is eased from where it was
// drawn when the two placings are less than this far apart (m; farther: it
// snaps, as for a teleport).
export const FRAME_BLEND_M = 25;
const SLEW = 0.1, MAX_TRAIL = 3, HARD_TRAIL = 10, LATE_WINDOW_MS = 3000;

export function createRemotes({ tickHz = 100 } = {}) {
  const hist = new Map(); // id -> {info, samples: [{tick, p, yaw, pitch, c}]}
  let offset = null; // estimated server tick - now * tickHz/1000 (the least-delayed arrival)
  let est = null, delay = null, slewAt = 0; // the slewed estimate and delay (ticks), as of time slewAt
  let newest = -Infinity, prevTick = null, interval = 1; // snapshot spacing in ticks (every tick)
  const late = []; // [{t, l}] how late each snapshot arrived vs the estimate (ticks)
  const perMs = tickHz / 1000;
  const SETTLE = 0.02; // per 30 ms of snapshots, toward a later (slower) arrival
  let floor = 0; // cl_interp: the delay never goes below this (ticks)
  let pinned = null; // tests (lockstep scenarios, whose stepping is no link): the delay, fixed (ticks)
  let playhead = null; // a replay (game/replayer.js): the tick drawn, set by the player, not by arrivals

  // The interpolation delay (ticks) the link needs now.
  function delayTicks() {
    const ls = late.map((x) => x.l).sort((a, b) => a - b);
    const p95 = ls.length ? ls[Math.min(ls.length - 1, Math.floor(ls.length * 0.95))] : 0;
    if (pinned !== null) return pinned;
    return Math.min(MAX_DELAY * perMs, Math.max(floor, 2 * interval + p95 + 1));
  }
  const toward = (v, tg, step) => v + Math.max(-step, Math.min(step, tg - v));
  // Move the estimate and the delay toward their targets, at most SLEW of the
  // time passed; the estimate never trails the raw one by more than MAX_TRAIL.
  function slew(now) {
    if (offset === null) return;
    if (est === null) { est = offset; delay = delayTicks(); slewAt = now; return; }
    const step = SLEW * Math.max(0, now - slewAt) * perMs; // together within SLEW
    slewAt = Math.max(slewAt, now);
    // Within MAX_TRAIL the estimate and the delay get half the slew each; past it
    // the estimate gets all of it (the delay holds) to catch up without a hop.
    // Only past HARD_TRAIL (a real step: latency dropped, a server burst) does it snap.
    const far = Math.abs(offset - est) > MAX_TRAIL;
    est = Math.max(offset - HARD_TRAIL, Math.min(offset + HARD_TRAIL, toward(est, offset, far ? step : step / 2)));
    if (!far) delay = toward(delay, delayTicks(), step / 2);
  }

  function ingest(snap, myId, now) {
    const tick = snap.tick;
    if (tick === undefined) return;
    slew(now); // up to now on the old target, then take the new snapshot in
    const o = tick - now * perMs;
    const restarted = tick < newest - 10 * tickHz;
    if (restarted) { hist.clear(); late.length = 0; newest = -Infinity; prevTick = null; est = null; }
    if (offset === null || o > offset || restarted) offset = o; // first, early, or a new server
    else offset += (o - offset) * (1 - Math.pow(1 - SETTLE, Math.min(30, Math.max(1, prevTick === null ? 3 : tick - prevTick)) / 3)); // SETTLE per 3 ticks of snapshots, however often they come
    late.push({ t: now, l: offset - o });
    while (late.length && now - late[0].t > LATE_WINDOW_MS) late.shift();
    if (prevTick !== null && tick > prevTick && tick - prevTick <= 30) interval += (tick - prevTick - interval) * 0.1;
    if (tick > newest) prevTick = tick;
    newest = Math.max(newest, tick);
    slew(now);
    const seen = new Set();
    for (const p of snap.players) {
      if (p.id === myId) continue;
      seen.add(p.id);
      let h = hist.get(p.id);
      if (!h) hist.set(p.id, (h = { samples: [] }));
      // A pose snapshot updates the pose only; the rest (name, team, weapon,
      // HP...) is the newest full snapshot's. full: it has had one (drawn only then).
      if (snap.pose) h.info = { ...h.info, ...p };
      else { h.info = p; h.full = true; }
      const s = h.samples;
      // s: a body then (alive, on a team; a pose snapshot says nb when not): the
      // server's lag-compensation history keeps only those (lagcomp.go
      // recordHistory), and so do collisions here.
      // fr, fl: riding a mover, its frame (1-based mover) and place in it (Player.Frame, Local)
      const sm = { tick, p: p.p, yaw: p.yaw, pitch: p.pitch, c: !!p.c, s: snap.pose ? !p.nb : !p.dead && p.team !== "spec", g: gaitFromWire(p.g), fr: (p.fl && p.fr) || 0, fl: p.fr ? p.fl : null };
      // keep them in tick order (a reordered snapshot slots in)
      let at = s.length;
      while (at > 0 && s[at - 1].tick > tick) at--;
      if (at > 0 && s[at - 1].tick === tick) s[at - 1] = sm;
      else s.splice(at, 0, sm);
      if (s.length > HIST_TICKS) s.shift(); // every tick's: as far back as the server rewinds
    }
    // Gone from the snapshot (left, kicked): not drawn any more, but its samples
    // stay until they age out: a command drawn at an older tick still bumps
    // into it there, as the server's history has it (sampleAt(t, true)).
    for (const [id, h] of hist) {
      if (seen.has(id)) h.gone = false;
      else if (!h.gone) h.gone = true;
      if (h.gone && h.samples[h.samples.length - 1].tick < newest - HIST_TICKS) hist.delete(id);
    }
  }

  // cl_interp (ms): a floor under the delay. A setting, not jitter: it applies at once.
  function setInterp(ms) {
    floor = Math.min(MAX_DELAY, Math.max(0, ms)) * perMs;
    if (delay !== null && delay < floor) delay = floor;
  }

  // The server tick drawn now: the estimated current tick minus the
  // interpolation delay, slewed (see above).
  const renderTick = (now) => { if (playhead !== null) return playhead; if (offset === null) return 0; slew(now); return now * perMs + est - delay; };

  // What is drawn now: sampleAt(renderTick(now)), and where a snapshot is
  // late (the view is past a remote's newest data) the remote goes on at its
  // last velocity for at most EXTRAP_MS (Source's cl_extrapolate), then holds.
  // When its data arrives, the jump from the guess eases out over BLEND_MS
  // instead of snapping. Drawing only: what shots and collisions are rewound
  // to (viewTick, sampleAt) never goes past the newest real snapshot.
  // ft: the movers' clock time to place riders of movers at (frames below);
  // by default frames.time(now), the time the movers are drawn at.
  let drawnAt = null;
  function sample(now, ft = frames?.time?.(now) ?? null) {
    const t = renderTick(now);
    const out = sampleAt(t, false, ft);
    const fade = drawnAt === null ? 0 : Math.exp(-Math.max(0, now - drawnAt) / BLEND_MS);
    drawnAt = now;
    for (const r of out) {
      const h = hist.get(r.id), s = h.samples, last = s[s.length - 1];
      let p = r.p, guess = 0;
      const v = t > last.tick ? velocity(r.id) : null;
      if (v && (v[0] || v[1] || v[2])) {
        guess = Math.min(t - last.tick, EXTRAP_MS * perMs);
        p = [p[0] + v[0] * guess, p[1] + v[1] * guess, p[2] + v[2] * guess];
      }
      if (h.err) h.err = h.err.map((e) => e * fade);
      if (h.guessed && last.tick !== h.guessTick && h.drawn && !teleported(h.drawn, p, 1)) {
        h.err = [h.drawn[0] - p[0], h.drawn[1] - p[1], h.drawn[2] - p[2]]; // data came after a guess: stay where it was drawn, then ease
      }
      // Into or out of a mover's frame: placed in the frame on our clock, in the
      // world on the snapshots' (or the other way round): the gap between the
      // two eases out too (a rider stepping off a moving lift doesn't hop).
      if (h.drawn && h.drawnFr !== undefined && h.drawnFr !== r.fr) {
        const d = [h.drawn[0] - p[0], h.drawn[1] - p[1], h.drawn[2] - p[2]];
        if (Math.hypot(...d) < FRAME_BLEND_M) h.err = d;
      }
      if (h.err) p = [p[0] + h.err[0], p[1] + h.err[1], p[2] + h.err[2]];
      h.guessed = guess > 0; h.guessTick = last.tick; h.drawn = p; h.drawnFr = r.fr;
      r.p = p; r.extrap = guess;
    }
    return out;
  }
  // A remote's velocity (m per server tick) between its last two snapshots;
  // null without two, across a teleport, or if it was not a body in both.
  // A rider of a mover in the same frame in both: its velocity in the frame
  // (what to add to a placed position: the frame's own motion is the placing's).
  function velocity(id) {
    const s = hist.get(id)?.samples;
    if (!s || s.length < 2) return null;
    const a = s[s.length - 2], b = s[s.length - 1], n = b.tick - a.tick;
    const [pa, pb] = sameFrame(a, b) ? [a.fl, b.fl] : [a.p, b.p];
    if (n <= 0 || !a.s || !b.s || teleported(pa, pb, n)) return null;
    return [(pb[0] - pa[0]) / n, (pb[1] - pa[1]) / n, (pb[2] - pa[2]) / n];
  }
  // frames: {offset(fr, t): where frame fr (a 1-based mover) is at time t on
  // the movers' clock, [x, y, z], or null (no such mover); time(now): the
  // movers' clock time things are drawn at now} (setFrames: the map's movers).
  let frames = null;
  const sameFrame = (a, b) => !!(frames && a.fr && a.fr === b.fr && a.fl && b.fl);
  const place = (fr, l, ft) => {
    const o = ft === null ? null : frames?.offset(fr, ft);
    return o ? [l[0] + o[0], l[1] + o[1], l[2] + o[2]] : null;
  };
  const HIST_TICKS = 150; // the server's history (lagcomp.go histLen x snapEvery) and then some

  // Every remote at server tick t (fractional), interpolated between the two
  // snapshots around it (clamped to the oldest / newest): what is drawn at t.
  // = internal/game History.At, which the server rewinds collisions and shots to.
  // gone: also those that left since, while they were still in a snapshot at
  // or after t (History.At: in the later frame).
  // ft (frames of reference, docs/physics.md): riders of a mover are placed
  // where the mover is at ft on the movers' clock (where we draw it, or the
  // command's time for collisions): one in the same frame in both snapshots
  // is interpolated in that frame (its place in it) and placed; one going
  // into or out of a frame between them is interpolated in the world, the
  // later pose placed after a teleport. = History.AtIn. ft null: the world
  // positions as sent (History.At).
  function sampleAt(t, gone = false, ft = null) {
    const out = [];
    for (const [id, h] of hist) {
      const s = h.samples;
      if (h.gone && !(gone && s[s.length - 1].tick >= t)) continue;
      if (!h.full && !gone) continue; // seen in pose snapshots only so far: nothing to draw it with (collisions have it)
      let a = s[0], b = s[0], bi = s.length; // bi: the first sample at or after t
      for (let i = 1; i < s.length; i++) {
        if (s[i].tick >= t) { a = s[i - 1]; b = s[i]; bi = i; break; }
        a = b = s[i];
      }
      if (s[0].tick >= t) bi = 0;
      const k = b.tick > a.tick ? Math.min(1, Math.max(0, (t - a.tick) / (b.tick - a.tick))) : 1;
      const framed = ft !== null && sameFrame(a, b) && !(b.s && !a.s); // = AtIn's same (both bodies: both in its history)
      const tp = (framed ? teleported(a.fl, b.fl, b.tick - a.tick) : teleported(a.p, b.p, b.tick - a.tick)) || (b.s && !a.s); // or just became a body (= History.At: missing in the earlier frame)
      // where it is: in its frame (placed at ft) or in the world (= AtIn)
      let p;
      if (tp) p = (ft !== null && b.fr && b.fl && place(b.fr, b.fl, ft)) || b.p;
      else if (framed) p = place(b.fr, a.fl.map((v, i) => v + (b.fl[i] - v) * k), ft) || b.p;
      else p = a.p.map((v, i) => v + (b.p[i] - v) * k);
      // Ground speed from the snapshot pair itself, not from per-frame deltas.
      const speed = b.tick > a.tick ? Math.hypot(b.p[0] - a.p[0], b.p[2] - a.p[2]) / ((b.tick - a.tick) / tickHz) : 0;
      out.push({ v: tp ? 0 : speed,
        id, name: h.info.name, color: h.info.color, team: h.info.team,
        // latest replicated status (not interpolated): weapon in hand, HP, ammo
        dk: h.info.dk || 0, fy: h.info.fy || 0, dv: !!h.info.dv, // the corpse (web/game/corpse.js)
        gv: h.info.gv || 0, // the gravity it stands in (the boots): drawn and collided with turned
        hk: h.info.hk || null, // where its hook is latched (full snapshots: the rope)
        w: h.info.w, hp: h.info.hp, dead: !!h.info.dead, ammo: h.info.ammo, res: h.info.res, bot: !!h.info.bot, ai: !!h.info.ai,
        // a respawn or teleport: the later pose, no sliding across the map
        p, tp, fr: framed || (tp && ft !== null && b.fr) ? b.fr : 0,
        yaw: tp ? b.yaw : lerpAngle(a.yaw, b.yaw, k),
        pitch: tp ? b.pitch : a.pitch + (b.pitch - a.pitch) * k,
        c: b.c,
        solid: b.s, // a body at t (History.At has it): collisions rewound to t bump into it
        gone: !!h.gone,
        // the walk cycle (Go LerpGait) and the pose time, as History.At on the server
        g: gaitAt(s, t, bi), t: poseTime(a.tick, b.tick, k),
      });
    }
    return out;
  }

  // The gait at t: it comes in full snapshots only, so between the nearest
  // samples around t that have one (and were a body) = History.gaitAt.
  function gaitAt(s, t, bi) {
    let a = null, b = null;
    for (let j = Math.min(bi, s.length) - 1; j >= 0; j--) if (s[j].g && s[j].s) { a = s[j]; break; }
    for (let j = Math.max(bi, 0); j < s.length; j++) if (s[j].g && s[j].s) { b = s[j]; break; }
    return gaitBetween(a ? a.tick : 0, a ? a.g : null, b ? b.tick : 0, b ? b.g : null, t);
  }

  // The server tick the remotes are drawn at (fractional, exactly what
  // sample() interpolates to, held at the newest snapshot if we're starved).
  // Fire commands send it so the server traces the shot against what was on
  // screen (lag compensation).
  function viewTick(now) {
    if (playhead !== null) return Math.min(playhead, newest);
    if (offset === null) return 0;
    return Math.min(renderTick(now), newest);
  }

  // Where the view stands on the snapshot timeline (net debug, tests).
  const timing = (now) => ({ renderTick: renderTick(now), newest, offset, delayMs: offset === null ? INTERP_DELAY : delayTicks() / perMs, viewDelayMs: delay === null ? INTERP_DELAY : delay / perMs });
  // forget(): drop every remote's samples but keep the timing (a fresh world:
  // nothing drawn or rewound from before it).
  // pinDelay(ms) (tests): draw this far back, whatever the arrivals say; null: adaptive again.
  function pinDelay(ms) { pinned = ms === null ? null : ms * perMs; if (pinned !== null) delay = pinned; }
  // setPlayhead(tick) (a replay): draw exactly this tick, whatever arrived
  // when; null: the network's clock again.
  const setPlayhead = (t) => { playhead = t; };
  return { ingest, sample, sampleAt, velocity, viewTick, pinDelay, setPlayhead, timing, setInterp, setFrames: (f) => { frames = f; }, place: (fr, l, ft) => place(fr, l, ft), forget: () => hist.clear(), clear: () => { hist.clear(); offset = null; est = null; newest = -Infinity; prevTick = null; late.length = 0; } };
}

// Two snapshots of a player farther apart than this many metres per tick
// between them (30 m/s: no run, jump or fall gets near) are a respawn or a
// teleport: the pose jumps to the later one (= internal/game lagcomp.go
// teleportSpeed, which collisions and shots are rewound with).
export const TELEPORT_SPEED = 0.3;
function teleported(a, b, ticks) {
  const dx = b[0] - a[0], dy = b[1] - a[1], dz = b[2] - a[2], g = TELEPORT_SPEED * Math.max(ticks, 1);
  return dx * dx + dy * dy + dz * dz > g * g;
}

function lerpAngle(a, b, t) {
  let d = b - a;
  while (d > Math.PI) d -= 2 * Math.PI;
  while (d < -Math.PI) d += 2 * Math.PI;
  return a + d * t;
}

// The server's tick rate as this client sees it (cl_netdebug): server ticks
// advanced per second of our clock, from each snapshot's tick and arrival
// time, over the last WINDOW ms, with the min and max over its 1 s windows.
// A server clock running at another rate than ours (WSL drifts) shows here as
// a rate off 100; arrival jitter only moves the 1 s windows a little.
export function createTickRate({ window = 5000, step = 1000 } = {}) {
  const s = []; // [{t, tick}], newest last, ticks increasing
  // The newest sample at or before time t (null: none).
  const at = (t) => { let r = null; for (const x of s) { if (x.t > t) break; r = x; } return r; };
  return {
    add(tick, now) {
      if (tick === undefined || (s.length && tick <= s[s.length - 1].tick)) return; // a duplicate or a late one
      s.push({ t: now, tick });
      while (s.length > 2 && now - s[1].t >= window) s.shift();
    },
    // {rate, min, max} in ticks per second (null: under a second of samples).
    read() {
      if (s.length < 2) return null;
      const last = s[s.length - 1], first = s[0];
      if (last.t - first.t < step) return null;
      let min = Infinity, max = -Infinity;
      for (let end = last.t; end - step >= first.t; end -= step) {
        const a = at(end - step), b = at(end); // the server's tick as known at each end (a stall: no new one)
        if (!a || !b) continue;
        const r = ((b.tick - a.tick) * 1000) / step;
        min = Math.min(min, r); max = Math.max(max, r);
      }
      return { rate: ((last.tick - first.tick) * 1000) / (last.t - first.t), min, max };
    },
  };
}

// "100.0 tick/s (96.1-103.8)", or "–" without a second of snapshots.
export function fmtTickRate(r) {
  return r ? `${r.rate.toFixed(1)} tick/s (${r.min.toFixed(1)}-${r.max.toFixed(1)})` : "–";
}
