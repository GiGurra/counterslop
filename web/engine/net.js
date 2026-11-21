// Websocket client with auto-reconnect, ping, and remote-player interpolation.
export const INTERP_DELAY = 100; // ms behind newest snapshot

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
  const st = { open: false, ping: null, fakeLag: 0, faults: { loss: 0, dup: 0, jitter: 0, reorder: 0 }, connect, send, hello, disconnect };
  const later = (fn) => (st.fakeLag > 0 ? setTimeout(fn, st.fakeLag / 2) : fn());
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
    q.push({ at: Math.max(q.length ? q[q.length - 1].at : 0, performance.now() + Math.random() * j), fn });
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
      if (f.reorder) setTimeout(() => ordered(k, fn), Math.random() * f.reorder);
      else ordered(k, fn);
    }
  }

  function disconnect() {
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (st.open) { st.open = false; onMsg({ t: "close" }); }
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
      if (m.t === "pong") st.ping = Math.round(performance.now() - m.seq);
      else onMsg(m);
    };
    ws.onclose = () => {
      const was = st.open;
      st.open = false;
      if (was) onMsg({ t: "close" });
      // Fast first retries (a deploy restart takes a few seconds), then back off to 3 s.
      retry = setTimeout(() => connect(name, greeted), Math.min(3000, 250 * 2 ** attempt++));
    };
  }

  function hello(n) { name = n; greeted = true; send({ t: "hello", name }); }

  function send(o) {
    const s = JSON.stringify(o);
    faulty("up", o.t, () => { if (ws?.readyState === 1) ws.send(s); });
  }
  setInterval(() => st.open && send({ t: "ping", seq: performance.now() }), 1000);
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
// The drawn timeline never jumps: it slews toward (estimate - delay) at up to
// SLEW of real time (it runs 0.9x..1.1x), so a new early arrival or a growing
// jitter buffer is absorbed over a second or so instead of hopping every
// remote player. Only a big discontinuity (first snapshot, a restarted
// server, half a second or more) snaps.
export const MAX_DELAY = 250; // ms
const SLEW = 0.1, SNAP_MS = 500, LATE_WINDOW_MS = 3000;

export function createRemotes({ tickHz = 100 } = {}) {
  const hist = new Map(); // id -> {info, samples: [{tick, p, yaw, pitch, c}]}
  let offset = null; // estimated server tick - now * tickHz/1000 (the least-delayed arrival)
  let view = null, viewAt = 0; // drawn tick - now * tickHz/1000, as of time viewAt (slewed)
  let newest = -Infinity, prevTick = null, interval = 3; // snapshot spacing in ticks
  const late = []; // [{t, l}] how late each snapshot arrived vs the estimate (ticks)
  const perMs = tickHz / 1000;
  const SETTLE = 0.02; // per snapshot, toward a later (slower) arrival

  // The interpolation delay (ticks) the link needs now.
  function delayTicks() {
    const ls = late.map((x) => x.l).sort((a, b) => a - b);
    const p95 = ls.length ? ls[Math.min(ls.length - 1, Math.floor(ls.length * 0.95))] : 0;
    return Math.min(MAX_DELAY * perMs, Math.max(INTERP_DELAY * perMs, 2 * interval + p95 + 1));
  }
  const target = () => offset - delayTicks();
  // Move the drawn timeline toward its target, at most SLEW of the time passed.
  function slew(now) {
    if (offset === null) return;
    const tg = target();
    if (view === null || Math.abs(tg - view) >= SNAP_MS * perMs) { view = tg; viewAt = now; return; }
    const dt = now - viewAt;
    if (dt <= 0) return;
    viewAt = now;
    const step = SLEW * dt * perMs;
    view += Math.max(-step, Math.min(step, tg - view));
  }

  function ingest(snap, myId, now) {
    const tick = snap.tick;
    if (tick === undefined) return;
    slew(now); // up to now on the old target, then take the new snapshot in
    const o = tick - now * perMs;
    const restarted = tick < newest - 10 * tickHz;
    if (restarted) { hist.clear(); late.length = 0; newest = -Infinity; prevTick = null; view = null; }
    if (offset === null || o > offset || restarted) offset = o; // first, early, or a new server
    else offset += (o - offset) * SETTLE;
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
      h.info = p;
      const s = h.samples;
      const sm = { tick, p: p.p, yaw: p.yaw, pitch: p.pitch, c: !!p.c };
      // keep them in tick order (a reordered snapshot slots in)
      let at = s.length;
      while (at > 0 && s[at - 1].tick > tick) at--;
      if (at > 0 && s[at - 1].tick === tick) s[at - 1] = sm;
      else s.splice(at, 0, sm);
      if (s.length > 40) s.shift();
    }
    for (const id of hist.keys()) if (!seen.has(id)) hist.delete(id);
  }

  // The server tick drawn now: the estimated current tick minus the
  // interpolation delay, slewed (see above).
  const renderTick = (now) => { if (offset === null) return 0; slew(now); return now * perMs + view; };

  function sample(now) {
    const t = renderTick(now), out = [];
    for (const [id, h] of hist) {
      const s = h.samples;
      let a = s[0], b = s[0];
      for (let i = 1; i < s.length; i++) {
        if (s[i].tick >= t) { a = s[i - 1]; b = s[i]; break; }
        a = b = s[i];
      }
      const k = b.tick > a.tick ? Math.min(1, Math.max(0, (t - a.tick) / (b.tick - a.tick))) : 1;
      // Ground speed from the snapshot pair itself, not from per-frame deltas.
      const speed = b.tick > a.tick ? Math.hypot(b.p[0] - a.p[0], b.p[2] - a.p[2]) / ((b.tick - a.tick) / tickHz) : 0;
      out.push({ v: speed,
        id, name: h.info.name, color: h.info.color, team: h.info.team,
        // latest replicated status (not interpolated): weapon in hand, HP, ammo
        w: h.info.w, hp: h.info.hp, dead: !!h.info.dead, ammo: h.info.ammo, res: h.info.res, bot: !!h.info.bot, ai: !!h.info.ai,
        p: a.p.map((v, i) => v + (b.p[i] - v) * k),
        yaw: lerpAngle(a.yaw, b.yaw, k),
        pitch: a.pitch + (b.pitch - a.pitch) * k,
        c: b.c,
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
  const timing = (now) => ({ renderTick: renderTick(now), newest, offset, delayMs: offset === null ? INTERP_DELAY : delayTicks() / perMs, viewDelayMs: offset === null || view === null ? INTERP_DELAY : (offset - view) / perMs });
  return { ingest, sample, viewTick, timing, clear: () => { hist.clear(); offset = null; view = null; newest = -Infinity; prevTick = null; late.length = 0; } };
}

function lerpAngle(a, b, t) {
  let d = b - a;
  while (d > Math.PI) d -= 2 * Math.PI;
  while (d < -Math.PI) d += 2 * Math.PI;
  return a + d * t;
}
