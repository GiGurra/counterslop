// Websocket client with auto-reconnect, ping, and remote-player interpolation.
// The delay on a clean link with a snapshot every tick (100 Hz): two snapshot
// intervals and a tick (cl_interp_ratio 2), no jitter. The adaptive delay
// starts here and has no floor of its own (cl_interp sets one).
export const INTERP_DELAY = 30; // ms behind newest snapshot

// unpackPoses: a pose snapshot's players ([id, x, y, z, yaw, pitch, flags];
// flags 1 crouched, 2 not a body: server.go posesJSON) as snapshot players.
export function unpackPoses(rows) {
  return rows.map((r) => ({ id: r[0], p: [r[1], r[2], r[3]], yaw: r[4], pitch: r[5], c: !!(r[6] & 1), nb: !!(r[6] & 2) }));
}

export function createNet(onMsg) {
  let ws = null, name = "", greeted = true, retry = null, attempt = 0;
  // Test-only link faults (all off by default):
  // - fakeLag (ms, round trip): net_fakelag, half each way.
  // - faults.jitter (ms): net_fakejitter, a varying delay of 0..jitter on
  //   everything, in order, one queue per direction: a TCP link (websocket over
  //   a VPN tunnel) delays and bunches messages but never reorders or loses them.
  // - faults.reorder (ms): net_fakereorder, each command/snapshot packet delayed
  //   on its own by 0..reorder, so they overtake each other as datagrams would.
  // - faults.loss / dup (0..1): net_fakeloss / net_fakedup, commands and
  //   snapshots only (the unreliable-by-design traffic). The protocol must cope.
  // - faultScale: every fake delay above runs this many times as long in wall time. Lockstep
  //   cases (cmd/sim -cases) set it below 1: their server ticks only when the harness steps it,
  //   so a delay's wall time is just waiting (the same as on a faster machine).
  const st = { open: false, ping: null, fakeLag: 0, faults: { loss: 0, dup: 0, jitter: 0, reorder: 0 }, faultScale: 1, connect, send, hello, disconnect, barrier };
  const later = (fn) => (st.fakeLag > 0 ? setTimeout(fn, (st.fakeLag / 2) * st.faultScale) : fn());
  const UNRELIABLE = new Set(["cmd", "snap"]);
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
  function ordered(k, fn) {
    const j = st.faults.jitter;
    if (!j && !fifo[k].length) return later(fn);
    const q = fifo[k];
    q.push({ at: Math.max(q.length ? q[q.length - 1].at : 0, performance.now() + Math.random() * j * st.faultScale), fn });
    if (!pumping[k]) { pumping[k] = true; setTimeout(() => pump(k), Math.max(0, q[0].at - performance.now())); }
  }
  // k: "up" (to the server) or "down"; t: the message type.
  function faulty(k, t, fn) {
    const f = st.faults;
    if (t === "cmd" && st.dropCmds > 0) { st.dropCmds--; return; } // tests: lose exactly the next N command packets
    if (!UNRELIABLE.has(t) || !(f.loss || f.dup || f.reorder)) return ordered(k, fn);
    if (Math.random() < f.loss) return;
    const copies = Math.random() < f.dup ? 2 : 1;
    for (let i = 0; i < copies; i++) {
      if (f.reorder) setTimeout(() => ordered(k, fn), Math.random() * f.reorder * st.faultScale);
      else ordered(k, fn);
    }
  }

  function disconnect() {
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (st.open) { st.open = false; onMsg({ t: "close" }); }
    dropBarriers();
  }

  // greet = false connects as a hidden observer (the menus: the server knows
  // the current map, admin state, and changelevel works before PLAY); hello()
  // joins later on the same socket. A reconnect says hello again only if we had.
  function connect(n, greet = true) {
    name = n;
    greeted = greet;
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); }
    const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws";
    ws = new WebSocket(url);
    ws.onopen = () => { st.open = true; attempt = 0; if (greeted) send({ t: "hello", name }); onMsg({ t: "open" }); };
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      faulty("down", m.t, () => deliver(m));
    };
    const deliver = (m) => {
      if (m.t === "pong" && m.seq < 0) barriers.get(m.seq)?.();
      else if (m.t === "pong") { rtt = performance.now() - m.seq; st.ping = Math.round(rtt); }
      else onMsg(m);
    };
    ws.onclose = () => {
      const was = st.open;
      st.open = false;
      dropBarriers();
      if (was) onMsg({ t: "close" });
      // Fast first retries (a deploy restart takes a few seconds), then back off to 3 s.
      retry = setTimeout(() => connect(name, greeted), Math.min(3000, 250 * 2 ** attempt++));
    };
  }

  // Tests (lockstep): resolves once the server has read everything sent before
  // it and everything it sent back before the pong is here (one socket, in
  // order). No game time passes; resolves anyway after 2 s of wall time
  // (closed socket, fake faults). A socket that closes settles the barriers
  // in flight at once (false): on the lockstep virtual clock the 2 s timer
  // never fires while the harness waits (an admin kick hung sim 'admin').
  const barriers = new Map();
  let barrierN = 0;
  function dropBarriers() { for (const done of [...barriers.values()]) done(false); }
  function barrier() {
    if (!st.open) return Promise.resolve(false);
    const seq = -++barrierN;
    return new Promise((r) => {
      const done = (ok) => { barriers.delete(seq); clearTimeout(t); r(ok); };
      const t = setTimeout(() => done(false), 2000);
      barriers.set(seq, (ok = true) => done(ok));
      send({ t: "ping", seq });
    });
  }

  function hello(n) { name = n; greeted = true; send({ t: "hello", name }); }

  function send(o) {
    const s = JSON.stringify(o);
    faulty("up", o.t, () => { if (ws?.readyState === 1) ws.send(s); });
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
// them (the remotes run 0.9x..1.1x), but the estimate never trails the raw
// one by more than MAX_TRAIL ticks. A jittery early arrival moves the raw
// estimate by a tick or two, absorbed smoothly; a real change (lower latency,
// or the lockstep harness running server ticks faster than this clock) is
// followed within MAX_TRAIL, so shots' view ticks stay where lag compensation
// expects them.
export const MAX_DELAY = 250; // ms
// A late snapshot: the remotes go on at their last velocity this long (ms),
// then hold; the difference when the data comes eases out over BLEND_MS.
export const EXTRAP_MS = 50, BLEND_MS = 50;
const SLEW = 0.1, MAX_TRAIL = 3, LATE_WINDOW_MS = 3000;

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
    const step = SLEW / 2 * Math.max(0, now - slewAt) * perMs; // half each: together within SLEW
    slewAt = Math.max(slewAt, now);
    est = Math.max(offset - MAX_TRAIL, Math.min(offset + MAX_TRAIL, toward(est, offset, step)));
    delay = toward(delay, delayTicks(), step);
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
      const sm = { tick, p: p.p, yaw: p.yaw, pitch: p.pitch, c: !!p.c, s: snap.pose ? !p.nb : !p.dead && p.team !== "spec" };
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
  const renderTick = (now) => { if (offset === null) return 0; slew(now); return now * perMs + est - delay; };

  // What is drawn now: sampleAt(renderTick(now)), and where a snapshot is
  // late (the view is past a remote's newest data) the remote goes on at its
  // last velocity for at most EXTRAP_MS (Source's cl_extrapolate), then holds.
  // When its data arrives, the jump from the guess eases out over BLEND_MS
  // instead of snapping. Drawing only: what shots and collisions are rewound
  // to (viewTick, sampleAt) never goes past the newest real snapshot.
  let drawnAt = null;
  function sample(now) {
    const t = renderTick(now);
    const out = sampleAt(t);
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
      if (h.err) p = [p[0] + h.err[0], p[1] + h.err[1], p[2] + h.err[2]];
      h.guessed = guess > 0; h.guessTick = last.tick; h.drawn = p;
      r.p = p; r.extrap = guess;
    }
    return out;
  }
  // A remote's velocity (m per server tick) between its last two snapshots;
  // null without two, across a teleport, or if it was not a body in both.
  function velocity(id) {
    const s = hist.get(id)?.samples;
    if (!s || s.length < 2) return null;
    const a = s[s.length - 2], b = s[s.length - 1], n = b.tick - a.tick;
    if (n <= 0 || !a.s || !b.s || teleported(a.p, b.p, n)) return null;
    return [(b.p[0] - a.p[0]) / n, (b.p[1] - a.p[1]) / n, (b.p[2] - a.p[2]) / n];
  }
  const HIST_TICKS = 150; // the server's history (lagcomp.go histLen x snapEvery) and then some

  // Every remote at server tick t (fractional), interpolated between the two
  // snapshots around it (clamped to the oldest / newest): what is drawn at t.
  // = internal/game History.At, which the server rewinds collisions and shots to.
  // gone: also those that left since, while they were still in a snapshot at
  // or after t (History.At: in the later frame).
  function sampleAt(t, gone = false) {
    const out = [];
    for (const [id, h] of hist) {
      const s = h.samples;
      if (h.gone && !(gone && s[s.length - 1].tick >= t)) continue;
      if (!h.full && !gone) continue; // seen in pose snapshots only so far: nothing to draw it with (collisions have it)
      let a = s[0], b = s[0];
      for (let i = 1; i < s.length; i++) {
        if (s[i].tick >= t) { a = s[i - 1]; b = s[i]; break; }
        a = b = s[i];
      }
      const k = b.tick > a.tick ? Math.min(1, Math.max(0, (t - a.tick) / (b.tick - a.tick))) : 1;
      const tp = teleported(a.p, b.p, b.tick - a.tick) || (b.s && !a.s); // or just became a body (= History.At: missing in the earlier frame)
      // Ground speed from the snapshot pair itself, not from per-frame deltas.
      const speed = b.tick > a.tick ? Math.hypot(b.p[0] - a.p[0], b.p[2] - a.p[2]) / ((b.tick - a.tick) / tickHz) : 0;
      out.push({ v: tp ? 0 : speed,
        id, name: h.info.name, color: h.info.color, team: h.info.team,
        // latest replicated status (not interpolated): weapon in hand, HP, ammo
        dk: h.info.dk || 0, fy: h.info.fy || 0, dv: !!h.info.dv, // the corpse (web/game/corpse.js)
        w: h.info.w, hp: h.info.hp, dead: !!h.info.dead, ammo: h.info.ammo, res: h.info.res, bot: !!h.info.bot, ai: !!h.info.ai,
        // a respawn or teleport: the later pose, no sliding across the map
        p: tp ? b.p : a.p.map((v, i) => v + (b.p[i] - v) * k),
        yaw: tp ? b.yaw : lerpAngle(a.yaw, b.yaw, k),
        pitch: tp ? b.pitch : a.pitch + (b.pitch - a.pitch) * k,
        c: b.c,
        solid: b.s, // a body at t (History.At has it): collisions rewound to t bump into it
        gone: !!h.gone,
      });
    }
    return out;
  }

  // The server tick the remotes are drawn at (fractional, exactly what
  // sample() interpolates to, held at the newest snapshot if we're starved).
  // Fire commands send it so the server traces the shot against what was on
  // screen (lag compensation).
  function viewTick(now) {
    if (offset === null) return 0;
    return Math.min(renderTick(now), newest);
  }

  // Where the view stands on the snapshot timeline (net debug, tests).
  const timing = (now) => ({ renderTick: renderTick(now), newest, offset, delayMs: offset === null ? INTERP_DELAY : delayTicks() / perMs, viewDelayMs: delay === null ? INTERP_DELAY : delay / perMs });
  // forget(): drop every remote's samples but keep the timing (a fresh world:
  // nothing drawn or rewound from before it).
  // pinDelay(ms) (tests): draw this far back, whatever the arrivals say; null: adaptive again.
  function pinDelay(ms) { pinned = ms === null ? null : ms * perMs; if (pinned !== null) delay = pinned; }
  return { ingest, sample, sampleAt, velocity, viewTick, pinDelay, timing, setInterp, forget: () => hist.clear(), clear: () => { hist.clear(); offset = null; est = null; newest = -Infinity; prevTick = null; late.length = 0; } };
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
