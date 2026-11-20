// Frame scheduling: vsync'd rAF or an experimental unlocked loop, with an
// optional fps cap applied by skipping frames on accumulated time (no setTimeout jitter).
import { settings } from "./settings.js";

const RING = 240;

export function createLoop(tick) {
  const st = {
    refreshMs: 1000 / 60,   // measured display interval (median of rAF deltas)
    frameTimes: new Float32Array(RING), fi: 0, frames: 0, // tick-to-tick ms
    capMs: 0,
  };
  const rafDeltas = [];
  let lastRaf = 0, lastTick = 0, acc = 0, lastSeen = 0;

  // The interval we aim for, or 0 when uncapped.
  function capInterval() {
    const s = settings.get();
    if (s.fps_limit === "custom" && s.fps_max > 0) return 1000 / s.fps_max;
    if (s.fps_limit === "auto") {
      const hz = 1000 / st.refreshMs;
      // In rAF mode a cap is only meaningful below refresh; elsewhere we still cap at refresh-3.
      return 1000 / Math.max(20, hz - 3);
    }
    return 0;
  }

  function maybeTick(now) {
    st.capMs = capInterval();
    const dt = lastSeen ? now - lastSeen : 0;
    lastSeen = now;
    if (st.capMs > 0) {
      acc += dt;
      // A quarter-ms tolerance absorbs timestamp jitter; the accumulator keeps the long-run rate exact.
      if (acc + 0.25 < st.capMs) return;
      acc -= st.capMs;
      if (acc > st.capMs) acc = 0; // fell far behind (tab hidden, hitch): don't burst
    } else acc = 0;
    if (lastTick) {
      st.frameTimes[st.fi] = now - lastTick;
      st.fi = (st.fi + 1) % RING;
      st.frames++;
    }
    lastTick = now;
    tick(now);
  }

  function raf(now) {
    requestAnimationFrame(raf);
    if (lastRaf) {
      const d = now - lastRaf;
      if (d > 2 && d < 100) {
        rafDeltas.push(d);
        if (rafDeltas.length > 120) rafDeltas.shift();
        if (rafDeltas.length % 15 === 0) st.refreshMs = median(rafDeltas);
      }
    }
    lastRaf = now;
    if (settings.get().cl_loop !== "unlocked") maybeTick(now);
  }
  requestAnimationFrame(raf);

  // Unlocked: a MessageChannel ping-pong runs as fast as the event loop allows.
  // The browser still presents at compositor (display) rate; this only lowers
  // the age of the frame that gets presented.
  const ch = new MessageChannel();
  let spinning = false;
  ch.port1.onmessage = () => {
    if (settings.get().cl_loop !== "unlocked" || document.hidden) { spinning = false; return; }
    maybeTick(performance.now());
    ch.port2.postMessage(0);
  };
  const kick = () => {
    if (!spinning && settings.get().cl_loop === "unlocked" && !document.hidden) { spinning = true; ch.port2.postMessage(0); }
  };
  settings.subscribe(() => { acc = 0; kick(); });
  document.addEventListener("visibilitychange", kick);
  kick();

  // Summary over the last ~second of frames.
  st.stats = () => {
    const n = Math.min(st.frames, RING);
    let sum = 0, max = 0, cnt = 0;
    for (let i = 1; i <= n && sum < 1000; i++) {
      const v = st.frameTimes[(st.fi - i + RING) % RING];
      sum += v; max = Math.max(max, v); cnt++;
    }
    return { fps: cnt ? (cnt * 1000) / sum : 0, avg: cnt ? sum / cnt : 0, max, hz: 1000 / st.refreshMs, capMs: st.capMs };
  };
  return st;
}

function median(a) {
  const s = [...a].sort((x, y) => x - y);
  return s[s.length >> 1];
}

// Fps / frame-time overlay with a sparkline; plain DOM so it costs nothing in Preact.
export function createOverlay(loop, getPing) {
  const el = document.createElement("div");
  el.className = "perf";
  const text = document.createElement("div");
  const cv = document.createElement("canvas");
  cv.width = 240; cv.height = 60;
  el.append(text, cv);
  document.body.appendChild(el);
  const g = cv.getContext("2d");

  setInterval(() => {
    const on = !!settings.get().cl_showfps;
    el.style.display = on ? "" : "none";
    if (!on) return;
    const s = loop.stats(), ping = getPing();
    const mode = settings.get().cl_loop === "unlocked" ? "unlocked" : "vsync";
    text.textContent =
      `${s.fps.toFixed(0)} fps · ${s.avg.toFixed(2)} ms (max ${s.max.toFixed(1)})\n` +
      `display ${s.hz.toFixed(0)} Hz · cap ${s.capMs ? (1000 / s.capMs).toFixed(0) : "off"} · ${mode} · ping ${ping ?? "–"}`;
    // sparkline: frame times, 0..2× refresh; dashed line = cap
    const top = Math.max(2 * (1000 / s.hz), s.capMs * 1.5, 20);
    g.clearRect(0, 0, cv.width, cv.height);
    g.fillStyle = "#8bff4f";
    for (let x = 0; x < RING; x++) {
      const v = loop.frameTimes[(loop.fi - RING + x + RING) % RING];
      const h = Math.min(cv.height, (v / top) * cv.height);
      g.fillRect(x, cv.height - h, 1, h);
    }
    const line = (ms, color) => {
      const y = cv.height - (ms / top) * cv.height;
      g.strokeStyle = color; g.setLineDash([4, 3]); g.beginPath(); g.moveTo(0, y); g.lineTo(cv.width, y); g.stroke();
    };
    line(1000 / s.hz, "#7cc4ff");
    if (s.capMs) line(s.capMs, "#ffd84f");
  }, 250);
}
