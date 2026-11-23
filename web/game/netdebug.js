// cl_netdebug: a live readout of the command/snapshot pipeline (top left,
// monospace). It shows what the client sent vs what the server has run, the
// server's queue, tick drift, ping and snapshot age, prediction health and the
// fate of the last shot, plus a 10 s sparkline of the backlog.
const TICK_MS = 10;

// interpDelay: () => the current remote interpolation delay in ms (adaptive).
export function createNetDebug(interpDelay) {
  const el = document.createElement("div");
  el.className = "netdebug";
  const text = document.createElement("pre");
  const spark = document.createElement("canvas");
  spark.width = 200; spark.height = 28;
  el.append(text, spark);
  document.body.append(el);

  const st = {
    sent: 0, acked: 0, queue: 0, dropped: 0,
    serverTick: 0, lastSnapAt: 0, drift: [], // [{t, d}] server tick − our seq, over the last minute
    shots: new Map(), // seq -> fire time (ms)
    lastShot: null, // {seq, result, ms}
    backlog: [], // [{t, v}] for the sparkline
  };

  function sent(seq, fire, now) {
    st.sent = seq;
    if (fire && !st.shots.has(seq)) st.shots.set(seq, now);
    if (st.shots.size > 200) st.shots.delete(st.shots.keys().next().value);
  }

  function snap(m, now) {
    st.lastSnapAt = now;
    if (m.tick !== undefined) st.serverTick = m.tick;
    if (m.me) { st.acked = m.me.ack; st.queue = m.me.q ?? 0; st.dropped = m.me.drop ?? 0; }
    if (st.sent) {
      st.drift.push({ t: now, d: st.serverTick - st.sent });
      while (st.drift.length && now - st.drift[0].t > 60000) st.drift.shift();
    }
    st.backlog.push({ t: now, v: Math.max(0, st.sent - st.acked) });
    while (st.backlog.length && now - st.backlog[0].t > 10000) st.backlog.shift();
  }

  function fired(m, now) {
    const t0 = st.shots.get(m.s);
    st.lastShot = { seq: m.s, result: m.r, ms: t0 === undefined ? null : Math.round(now - t0) };
  }

  function driftPerMin() {
    const d = st.drift;
    if (d.length < 2 || d[d.length - 1].t - d[0].t < 5000) return null;
    return ((d[d.length - 1].d - d[0].d) / (d[d.length - 1].t - d[0].t)) * 60000;
  }

  const f = (v, n = 3) => (v === null || v === undefined ? "–" : (+v).toFixed(n));

  function render(now, { ping, prediction, audio }) {
    const backlog = Math.max(0, st.sent - st.acked);
    const level = backlog > 10 ? "bad" : backlog > 3 ? "warn" : "";
    const dr = driftPerMin();
    const ls = st.lastShot;
    text.innerHTML = [
      `cmd sent     ${st.sent}`,
      `cmd acked    ${st.acked}`,
      `<span class="${level}">backlog      ${backlog} ticks  ${backlog * TICK_MS} ms</span>`,
      `server queue ${st.queue}  dropped ${st.dropped}`,
      `tick srv/us  ${st.serverTick} / ${st.sent}  drift ${dr === null ? "–" : dr.toFixed(1)} ticks/min`,
      `ping         ${ping ?? "–"} ms  snap age ${st.lastSnapAt ? Math.round(now - st.lastSnapAt) : "–"} ms  interp ${Math.round(interpDelay())} ms`,
      `prediction   pending ${prediction.pending}  corrections ${prediction.corrections}  re-syncs ${prediction.resyncs ?? 0}`,
      `             last error ${f(prediction.lastError)} m  srv-vs-us ${f(prediction.ackDelta)} m`,
      `audio        ${audio}`,
      `last shot    ${ls ? `#${ls.seq} ${esc(ls.result || "?")} ${ls.ms === null ? "" : `in ${ls.ms} ms`}` : "–"}`,
    ].join("\n");
    drawSpark(now);
  }

  function drawSpark(now) {
    const c = spark.getContext("2d"), w = spark.width, h = spark.height;
    c.clearRect(0, 0, w, h);
    const max = Math.max(12, ...st.backlog.map((b) => b.v));
    c.strokeStyle = "rgba(255,255,255,0.25)";
    for (const lim of [3, 10]) { const y = h - (lim / max) * h; c.beginPath(); c.moveTo(0, y); c.lineTo(w, y); c.stroke(); }
    c.strokeStyle = "#9cf59a";
    c.beginPath();
    st.backlog.forEach((b, i) => {
      const x = w - ((now - b.t) / 10000) * w, y = h - (b.v / max) * h;
      i ? c.lineTo(x, y) : c.moveTo(x, y);
    });
    c.stroke();
  }

  return {
    sent, snap, fired, render, state: st,
    show(on) { el.style.display = on ? "" : "none"; },
  };
}

const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]);
