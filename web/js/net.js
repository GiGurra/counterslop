// Websocket client with auto-reconnect, ping, and remote-player interpolation.
const INTERP_DELAY = 100; // ms behind newest snapshot

export function createNet(onMsg) {
  let ws = null, name = "", retry = null;
  const st = { open: false, ping: null, connect, send };

  function connect(n) {
    name = n;
    clearTimeout(retry);
    if (ws) { ws.onclose = null; ws.close(); }
    const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws";
    ws = new WebSocket(url);
    ws.onopen = () => { st.open = true; send({ t: "hello", name }); onMsg({ t: "open" }); };
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.t === "pong") st.ping = Math.round(performance.now() - m.seq);
      else onMsg(m);
    };
    ws.onclose = () => {
      const was = st.open;
      st.open = false;
      if (was) onMsg({ t: "close" });
      retry = setTimeout(() => connect(name), 1000);
    };
  }

  function send(o) { if (ws?.readyState === 1) ws.send(JSON.stringify(o)); }
  setInterval(() => st.open && send({ t: "ping", seq: performance.now() }), 1000);
  return st;
}

// Keeps a short history per remote player and samples it INTERP_DELAY ms in the past.
export function createRemotes() {
  const hist = new Map(); // id -> {info, samples: [{t, p, yaw, pitch}]}

  function ingest(snap, myId, now) {
    const seen = new Set();
    for (const p of snap.players) {
      if (p.id === myId) continue;
      seen.add(p.id);
      let h = hist.get(p.id);
      if (!h) hist.set(p.id, (h = { samples: [] }));
      h.info = p;
      h.samples.push({ t: now, p: p.p, yaw: p.yaw, pitch: p.pitch });
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
      out.push({
        id, name: h.info.name, color: h.info.color,
        p: a.p.map((v, i) => v + (b.p[i] - v) * k),
        yaw: lerpAngle(a.yaw, b.yaw, k),
        pitch: a.pitch + (b.pitch - a.pitch) * k,
      });
    }
    return out;
  }

  return { ingest, sample, clear: () => hist.clear() };
}

function lerpAngle(a, b, t) {
  let d = b - a;
  while (d > Math.PI) d -= 2 * Math.PI;
  while (d < -Math.PI) d += 2 * Math.PI;
  return a + d * t;
}
