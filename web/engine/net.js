// Websocket client with auto-reconnect, ping, and remote-player interpolation.
export const INTERP_DELAY = 100; // ms behind newest snapshot

export function createNet(onMsg) {
  let ws = null, name = "", retry = null, attempt = 0;
  // fakeLag (ms, round trip): net_fakelag, for testing lag compensation.
  // faults: net_fakeloss/dup (0..1) and net_fakejitter (ms), applied to the
  // unreliable-by-design traffic (commands up, snapshots down) as a datagram
  // transport would: lost, duplicated, reordered. The protocol must cope.
  const st = { open: false, ping: null, fakeLag: 0, faults: { loss: 0, dup: 0, jitter: 0 }, connect, send, disconnect };
  const later = (fn) => (st.fakeLag > 0 ? setTimeout(fn, st.fakeLag / 2) : fn());
  const UNRELIABLE = new Set(["cmd", "snap"]);
  function faulty(t, fn) {
    const f = st.faults;
    if (t === "cmd" && st.dropCmds > 0) { st.dropCmds--; return; } // tests: lose exactly the next N command packets
    if (!UNRELIABLE.has(t) || !(f.loss || f.dup || f.jitter)) return later(fn);
    if (Math.random() < f.loss) return;
    const copies = Math.random() < f.dup ? 2 : 1;
    for (let i = 0; i < copies; i++) setTimeout(() => later(fn), f.jitter ? Math.random() * f.jitter : 0);
  }

  function disconnect() {
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    if (st.open) { st.open = false; onMsg({ t: "close" }); }
  }

  function connect(n) {
    name = n;
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); }
    const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws";
    ws = new WebSocket(url);
    ws.onopen = () => { st.open = true; attempt = 0; send({ t: "hello", name }); onMsg({ t: "open" }); };
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      faulty(m.t, () => deliver(m));
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
      retry = setTimeout(() => connect(name), Math.min(3000, 250 * 2 ** attempt++));
    };
  }

  function send(o) {
    const s = JSON.stringify(o);
    faulty(o.t, () => { if (ws?.readyState === 1) ws.send(s); });
  }
  setInterval(() => st.open && send({ t: "ping", seq: performance.now() }), 1000);
  return st;
}

// Keeps a short history per remote player and samples it INTERP_DELAY ms in the past.
export function createRemotes() {
  const hist = new Map(); // id -> {info, samples: [{t, p, yaw, pitch}]}
  const ticks = []; // [{t, tick}]: when each snapshot arrived and which server tick it shows

  function ingest(snap, myId, now) {
    if (snap.tick !== undefined) { ticks.push({ t: now, tick: snap.tick }); if (ticks.length > 30) ticks.shift(); }
    const seen = new Set();
    for (const p of snap.players) {
      if (p.id === myId) continue;
      seen.add(p.id);
      let h = hist.get(p.id);
      if (!h) hist.set(p.id, (h = { samples: [] }));
      h.info = p;
      h.samples.push({ t: now, p: p.p, yaw: p.yaw, pitch: p.pitch, c: !!p.c });
      if (h.samples.length > 30) h.samples.shift();
    }
    for (const id of hist.keys()) if (!seen.has(id)) hist.delete(id);
  }

  function sample(now) {
    const t = now - INTERP_DELAY, out = [];
    for (const [id, h] of hist) {
      const s = h.samples;
      let a = s[0], b = s[0];
      for (let i = 1; i < s.length; i++) {
        if (s[i].t >= t) { a = s[i - 1]; b = s[i]; break; }
        a = b = s[i];
      }
      const k = b.t > a.t ? Math.min(1, Math.max(0, (t - a.t) / (b.t - a.t))) : 1;
      // Ground speed from the snapshot pair itself, not from per-frame deltas.
      const speed = b.t > a.t ? Math.hypot(b.p[0] - a.p[0], b.p[2] - a.p[2]) / ((b.t - a.t) / 1000) : 0;
      out.push({ v: speed,
        id, name: h.info.name, color: h.info.color, team: h.info.team,
        // latest replicated status (not interpolated): weapon in hand, HP, ammo
        w: h.info.w, hp: h.info.hp, dead: !!h.info.dead, ammo: h.info.ammo, res: h.info.res, bot: !!h.info.bot,
        p: a.p.map((v, i) => v + (b.p[i] - v) * k),
        yaw: lerpAngle(a.yaw, b.yaw, k),
        pitch: a.pitch + (b.pitch - a.pitch) * k,
        c: b.c,
      });
    }
    return out;
  }

  // The server tick the remotes are drawn at (fractional, interpolated exactly
  // like the players in sample()). Fire commands send it so the server can
  // trace the shot against what was on screen (lag compensation).
  function viewTick(now) {
    if (!ticks.length) return 0;
    const t = now - INTERP_DELAY;
    let a = ticks[0], b = ticks[0];
    for (let i = 1; i < ticks.length; i++) {
      if (ticks[i].t >= t) { a = ticks[i - 1]; b = ticks[i]; break; }
      a = b = ticks[i];
    }
    const k = b.t > a.t ? Math.min(1, Math.max(0, (t - a.t) / (b.t - a.t))) : 1;
    return a.tick + (b.tick - a.tick) * k;
  }

  return { ingest, sample, viewTick, clear: () => { hist.clear(); ticks.length = 0; } };
}

function lerpAngle(a, b, t) {
  let d = b - a;
  while (d > Math.PI) d -= 2 * Math.PI;
  while (d < -Math.PI) d += 2 * Math.PI;
  return a + d * t;
}
