// Frame scheduling. Two modes:
//
// raf (vsync): one tick per requestAnimationFrame, i.e. per display refresh.
//   The default "even" limiter only caps at integer divisors of the refresh
//   (tick every n-th vsync), because rAF is tied to the compositor's vsync:
//   anything else beats against the refresh and drops a frame every so often.
//   The opt-in "latency" limiter is the original one: any cap (incl. auto =
//   refresh − 3) by skipping vsyncs, and it may micro-stutter.
//
// unlocked (vsync off, experimental low latency): ticks are paced evenly at an
//   exact interval (fps_max_unlocked, 0 = uncapped): a timer sleeps until ~1.5 ms
//   before the deadline, then a short spin finishes it. The spin yields to the
//   browser on every step (each step is its own task), and no tick runs in the
//   last ~1.5 ms before the next vsync, so compositing, input and websocket
//   messages are never starved. The browser still shows one frame per vsync;
//   ticking faster only makes the shown frame fresher.
//
// Stats separate ticks (simulate + render) from presented frames (vsyncs whose
// frame had new content), since only the latter reach the screen.
// `settings` is any store with get() -> {fps_limit, fps_max, fps_max_unlocked, cl_loop, cl_showfps} and subscribe(fn).

const RING = 240;
const VSYNC_GUARD_MS = 1.5;

export function createLoop(tick, settings) {
  const st = {
    refreshMs: 1000 / 60, // measured display interval (median of rAF deltas)
    frameTimes: new Float32Array(RING), fi: 0, frames: 0,    // tick-to-tick ms
    presentTimes: new Float32Array(RING), pi: 0, presents: 0, // presented-frame intervals, ms
    capMs: 0, divisor: 1,
  };
  const rafDeltas = [];
  let lastRaf = 0, lastTick = 0, lastPresent = 0, tickedSinceRaf = false, rafCount = 0;
  const unlocked = () => settings.get().cl_loop === "unlocked";

  function runTick(now) {
    if (lastTick) {
      st.frameTimes[st.fi] = now - lastTick;
      st.fi = (st.fi + 1) % RING;
      st.frames++;
    }
    lastTick = now;
    tickedSinceRaf = true;
    tick(now);
  }

  // rAF mode, even limiter: every n-th vsync, n = refresh / cap rounded (cap snapped to refresh/n).
  function divisor() {
    const s = settings.get();
    if (s.fps_limit !== "custom" || !(s.fps_max > 0)) return 1;
    const hz = 1000 / st.refreshMs;
    return Math.max(1, Math.round(hz / s.fps_max));
  }

  // rAF mode, latency limiter (opt-in, the original one): any cap, incl. auto =
  // refresh − 3, by skipping vsyncs on accumulated time. Lowest latency under
  // VRR in native games; here it beats against the refresh and may micro-stutter.
  let acc = 0, lastSeen = 0;
  function latencyCap() {
    const s = settings.get();
    if (s.fps_limit === "custom" && s.fps_max > 0) return 1000 / s.fps_max;
    if (s.fps_limit === "auto") return 1000 / Math.max(20, 1000 / st.refreshMs - 3);
    return 0;
  }
  function latencyTick(now) {
    st.capMs = latencyCap();
    const dt = lastSeen ? now - lastSeen : 0;
    lastSeen = now;
    if (st.capMs > 0) {
      acc += dt;
      if (acc + 0.25 < st.capMs) return;
      acc -= st.capMs;
      if (acc > st.capMs) acc = 0;
    } else acc = 0;
    runTick(now);
  }

  function raf(now) {
    requestAnimationFrame(raf);
    if (lastRaf) {
      const d = now - lastRaf;
      if (d > 2 && d < 100) {
        rafDeltas.push(d);
        if (rafDeltas.length > 120) rafDeltas.shift();
        if (rafDeltas.length % 15 === 0 && !st.pinnedRefresh) st.refreshMs = median(rafDeltas);
      }
    }
    // The previous vsync interval showed a new frame if anything ticked in it.
    if (tickedSinceRaf) {
      if (lastPresent) {
        st.presentTimes[st.pi] = now - lastPresent;
        st.pi = (st.pi + 1) % RING;
        st.presents++;
      }
      lastPresent = now;
    }
    tickedSinceRaf = false;
    lastRaf = now;
    rafCount++; st.rafs = rafCount;
    if (unlocked()) { wake(); return; }
    if (settings.get().fps_limiter === "latency") { st.divisor = 1; latencyTick(now); return; }
    st.divisor = divisor();
    st.capMs = st.divisor > 1 ? st.divisor * st.refreshMs : 0;
    if (rafCount % st.divisor === 0) runTick(now);
  }
  requestAnimationFrame(raf);

  // ---- unlocked: even pacing with timer + yielding spin ----
  const ch = new MessageChannel();
  let next = 0, timer = 0, spinning = false, active = false;
  const interval = () => {
    const f = settings.get().fps_max_unlocked;
    return f > 0 ? 1000 / f : 0;
  };
  // Inside the last moments before the next vsync: leave the main thread to the compositor.
  // Returns ms until the window ends, or 0 outside it.
  const guardLeft = (now) => {
    if (!lastRaf) return 0;
    const v = lastRaf + st.refreshMs;
    return now > v - VSYNC_GUARD_MS && now < v + 1 ? v + 1 - now : 0;
  };

  function step() {
    timer = 0; spinning = false;
    if (!unlocked() || document.hidden) { active = false; return; }
    const now = performance.now(), iv = interval();
    st.capMs = iv;
    const g = guardLeft(now);
    if (g > 0) { timer = setTimeout(step, g); return; } // the next rAF may wake us sooner
    if (iv > 0) {
      const wait = next - now;
      if (wait > 2) { timer = setTimeout(step, wait - VSYNC_GUARD_MS); return; }
      if (wait > 0) { spin(); return; }
      next = Math.max(next + iv, now); // even spacing; after a stall restart, don't burst
    }
    runTick(now);
    spin();
  }
  function spin() { spinning = true; ch.port2.postMessage(0); }
  ch.port1.onmessage = step;
  // Start or resume the unlocked loop (rAF calls this every vsync).
  function wake() {
    if (!unlocked() || document.hidden) return;
    if (!active) { active = true; if (!next) next = performance.now(); }
    if (timer && guardLeft(performance.now()) === 0) { clearTimeout(timer); timer = 0; }
    if (!timer && !spinning) spin();
  }
  settings.subscribe(() => { next = 0; acc = 0; wake(); });
  document.addEventListener("visibilitychange", wake);

  // Summary over the last ~second: ticks and presented frames.
  const summary = (ring, idx, count) => {
    const n = Math.min(count, RING);
    let sum = 0, max = 0, cnt = 0, sq = 0;
    for (let i = 1; i <= n && sum < 1000; i++) {
      const v = ring[(idx - i + RING) % RING];
      sum += v; sq += v * v; max = Math.max(max, v); cnt++;
    }
    const avg = cnt ? sum / cnt : 0;
    return { fps: cnt ? (cnt * 1000) / sum : 0, avg, max, sd: cnt ? Math.sqrt(Math.max(0, sq / cnt - avg * avg)) : 0, n: cnt };
  };
  st.stats = () => {
    const t = summary(st.frameTimes, st.fi, st.frames), p = summary(st.presentTimes, st.pi, st.presents);
    return { ...t, presented: p, hz: 1000 / st.refreshMs, capMs: st.capMs, divisor: st.divisor, mode: unlocked() ? "unlocked" : "vsync" };
  };
  return st;
}

function median(a) {
  const s = [...a].sort((x, y) => x - y);
  return s[s.length >> 1];
}

// Fps / frame-time overlay; plain DOM so it costs nothing in Preact. The graph
// shows presented frame intervals (what reaches the screen) as bars and tick
// intervals as a line. extra() returns more text (e.g. the renderer's pass cost).
export function createOverlay(loop, settings, getPing, extra = () => "") {
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
    const cap = s.mode === "vsync"
      ? (settings.get().fps_limiter === "latency" ? (s.capMs ? `cap ${(1000 / s.capMs).toFixed(0)} (latency limiter)` : "cap off")
        : s.divisor > 1 ? `cap ${(s.hz / s.divisor).toFixed(0)} (1/${s.divisor})` : "cap off")
      : (s.capMs ? `cap ${(1000 / s.capMs).toFixed(0)}` : "uncapped");
    const more = extra();
    text.textContent =
      `${s.presented.fps.toFixed(0)} fps shown · ${s.fps.toFixed(0)} ticks/s · ${s.avg.toFixed(2)} ms (max ${s.max.toFixed(1)})\n` +
      `display ${s.hz.toFixed(0)} Hz · ${cap} · ${s.mode} · ping ${ping ?? "–"}` + (more ? `\n${more}` : "");
    // 0..2× refresh; dashed line = refresh
    const top = Math.max(2 * (1000 / s.hz), s.capMs * 1.5, 20);
    const y = (ms) => cv.height - Math.min(cv.height, (ms / top) * cv.height);
    g.clearRect(0, 0, cv.width, cv.height);
    g.fillStyle = "#8bff4f";
    for (let x = 0; x < RING; x++) {
      const v = loop.presentTimes[(loop.pi - RING + x + RING) % RING];
      g.fillRect(x, y(v), 1, cv.height - y(v));
    }
    g.strokeStyle = "#ffd84f"; g.setLineDash([]); g.beginPath();
    for (let x = 0; x < RING; x++) {
      const v = loop.frameTimes[(loop.fi - RING + x + RING) % RING];
      x ? g.lineTo(x, y(v)) : g.moveTo(x, y(v));
    }
    g.stroke();
    g.strokeStyle = "#7cc4ff"; g.setLineDash([4, 3]); g.beginPath();
    g.moveTo(0, y(1000 / s.hz)); g.lineTo(cv.width, y(1000 / s.hz)); g.stroke();
  }, 250);
}
