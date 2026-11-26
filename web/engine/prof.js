// Frame-time profiler (game-agnostic). Named sections, nestable, timed per
// frame; a rolling window (~1.5 s) gives each one its mean, p95/p99, max and
// calls per frame, CPU and (when the renderer measured one) GPU. docs/profiling.md.
//
//   prof.frame()             a frame boundary: what ran since the last one is one frame
//   prof.begin("render") ... prof.end()      or prof.scope("render", fn)
//   prof.cur()               the open section (a handle for gpuSample)
//   prof.gpuSample(node, ms, at)   a GPU time measured for that section (arrives frames later)
//   prof.table()             the window's numbers (pure data: overlay, console, test API)
//
// Off (prof.on false) every call is one boolean check. Turning it on or off
// takes effect at the next frame boundary, so a scope is never half measured.
// Sections opened outside the frame (socket messages, timers) are top-level
// siblings of the frame's own: time between two boundaries that no top-level
// section covers is "idle" (waiting for vsync / the cap, the browser's own work, GC).

const CAP = 2048; // frames kept: the window at up to ~1300 fps
const GCAP = 512; // GPU samples kept per section
const MAX_DEPTH = 32;

export function createProf({ now = () => performance.now(), windowMs = 1500 } = {}) {
  const mkNode = (name, parent) => ({
    name, parent, depth: parent ? parent.depth + 1 : -1, path: parent && parent.path ? `${parent.path}/${name}` : name,
    kids: [], byName: new Map(), sum: 0, calls: 0, ema: 0, gpuEma: null, // ema: ms a frame, smoothed (stats readouts)
    ring: new Float32Array(CAP), callRing: new Uint16Array(CAP),
    gpu: null, // {ms: Float32Array, at: Float64Array, i, n}: GPU samples, by when they were taken
  });
  const root = mkNode("", null);
  const nodes = []; // every section, in creation order
  const stack = new Array(MAX_DEPTH), t0s = new Float64Array(MAX_DEPTH);
  let sp = 0, want = false, last = 0;
  const frameAt = new Float64Array(CAP), interval = new Float32Array(CAP), busy = new Float32Array(CAP);
  let fi = 0, fn = 0; // next slot, frames stored
  const gpuFrame = { ms: new Float32Array(GCAP), at: new Float64Array(GCAP), i: 0, n: 0 }; // whole-frame GPU totals

  const p = {
    on: false,
    get wanted() { return want; },
    // Ask for on/off: applied at the next frame boundary.
    enable(v) { want = !!v; },
    begin(name) {
      if (!p.on || sp >= MAX_DEPTH) return;
      const parent = sp ? stack[sp - 1] : root;
      let n = parent.byName.get(name);
      if (!n) {
        n = mkNode(name, parent);
        parent.byName.set(name, n); parent.kids.push(n); nodes.push(n);
      }
      stack[sp] = n; t0s[sp++] = now();
    },
    end() {
      if (!p.on || !sp) return;
      const n = stack[--sp];
      n.sum += now() - t0s[sp]; n.calls++;
    },
    scope(name, f) {
      if (!p.on) return f();
      p.begin(name);
      try { return f(); } finally { p.end(); }
    },
    cur: () => (p.on && sp ? stack[sp - 1] : null),
    // A GPU time for a section (a handle from cur()), measured at `at` (the frame it belongs to).
    gpuSample(node, ms, at) {
      if (!node) return;
      const g = node.gpu ??= { ms: new Float32Array(GCAP), at: new Float64Array(GCAP), i: 0, n: 0 };
      push(g, ms, at);
      node.gpuEma = node.gpuEma == null ? ms : node.gpuEma * 0.9 + ms * 0.1;
    },
    gpuFrameSample(ms, at) { push(gpuFrame, ms, at); p.gpuFrameEma = p.gpuFrameEma == null ? ms : p.gpuFrameEma * 0.9 + ms * 0.1; },
    gpuFrameEma: null,
    now,
    // A frame boundary (the loop calls it as each tick starts).
    frame(t = now()) {
      if (p.on) {
        while (sp) p.end(); // a section left open (an exception): close it here
        if (last) {
          let b = 0;
          for (const k of root.kids) b += k.sum;
          frameAt[fi] = t; interval[fi] = t - last; busy[fi] = b;
          for (const n of nodes) { n.ring[fi] = n.sum; n.callRing[fi] = n.calls; n.ema = n.ema * 0.9 + n.sum * 0.1; }
          fi = (fi + 1) % CAP; fn = Math.min(CAP, fn + 1);
        }
      }
      if (p.on !== want) { p.on = want; last = 0; }
      if (p.on) {
        for (const n of nodes) { n.sum = 0; n.calls = 0; }
        sp = 0; last = t;
      }
    },
    reset() { // measuring starts afresh at the next frame boundary
      fn = 0; fi = 0; gpuFrame.n = 0; last = 0;
      for (const n of nodes) { n.sum = 0; n.calls = 0; n.ema = 0; n.gpuEma = null; if (n.gpu) n.gpu.n = 0; }
      p.gpuFrameEma = null;
    },
    // The window's numbers (win: its length, ms; Infinity: every frame kept, for benches).
    // rows: depth-first, children by mean (largest first).
    table(t = now(), win = windowMs) {
      const idx = []; // ring slots inside the window, newest first
      for (let k = 1; k <= fn; k++) {
        const i = (fi - k + CAP) % CAP;
        if (t - frameAt[i] > win) break;
        idx.push(i);
      }
      const nF = idx.length;
      const col = (arr) => stat(idx.map((i) => arr[i]));
      const out = { frames: nF, windowMs: win, interval: col(interval), busy: col(busy), idle: stat(idx.map((i) => Math.max(0, interval[i] - busy[i]))), gpu: gpuStat(gpuFrame, t, win), rows: [] };
      const rowOf = (n) => {
        const s = stat(idx.map((i) => n.ring[i]));
        let calls = 0;
        for (const i of idx) calls += n.callRing[i];
        return { path: n.path, name: n.name, depth: n.depth, ...s, calls: nF ? calls / nF : 0, gpu: n.gpu ? gpuStat(n.gpu, t, win) : null };
      };
      const walk = (n) => {
        const rows = n.kids.map((k) => ({ k, r: rowOf(k) })).filter((e) => e.r.calls > 0).sort((a, b) => b.r.mean - a.r.mean);
        for (const { k, r } of rows) {
          out.rows.push(r);
          const at = out.rows.length;
          walk(k);
          let kids = 0;
          for (let j = at; j < out.rows.length; j++) if (out.rows[j].depth === r.depth + 1) kids += out.rows[j].mean;
          r.self = Math.max(0, r.mean - kids);
        }
      };
      walk(root);
      return out;
    },
  };
  return p;
}

// The page's profiler. Wall time even on a lockstep test page (cmd/sim
// vclock.js keeps the real clock at __real): it reports how long work took.
export const prof = createProf({ now: globalThis.__real?.perfNow ?? (() => performance.now()) });

function push(g, ms, at) {
  g.ms[g.i] = ms; g.at[g.i] = at; g.i = (g.i + 1) % g.ms.length; g.n = Math.min(g.ms.length, g.n + 1);
}

function gpuStat(g, t, windowMs) {
  const v = [];
  for (let k = 1; k <= g.n; k++) {
    const i = (g.i - k + g.ms.length) % g.ms.length;
    if (t - g.at[i] > windowMs) break;
    v.push(g.ms[i]);
  }
  return v.length ? stat(v) : null;
}

// mean, p95, p99, max of a list (nearest-rank percentiles).
export function stat(v) {
  if (!v.length) return { n: 0, mean: 0, p95: 0, p99: 0, max: 0 };
  const s = Float64Array.from(v).sort();
  let sum = 0;
  for (const x of s) sum += x;
  const rank = (q) => s[Math.min(s.length - 1, Math.max(0, Math.ceil(q * s.length) - 1))];
  return { n: s.length, mean: sum / s.length, p95: rank(0.95), p99: rank(0.99), max: s[s.length - 1] };
}

// What limits the frame rate, in plain words. Pure: t is a table(), env what
// the loop knows: {refreshMs (the display's interval), capMs (the fps cap's
// interval, 0: none), hidden (a background tab), gpuTimer (the GPU can be timed)}.
// kind: gpu | cpu | vsync | cap | throttled | background | unknown.
export const WAIT = "gpu-wait"; // a top-level section that waits for the GPU (benches: gl.finish / readPixels)
export function verdict(t, env = {}) {
  const { refreshMs = 1000 / 60, capMs = 0, hidden = false, gpuTimer = t.gpu != null } = env;
  if (hidden) return { kind: "background", text: "Background tab: the browser throttles it" };
  if (!t.frames) return { kind: "unknown", text: "Measuring…" };
  // A bench waits for the GPU on the main thread (section gpu-wait, WAIT): that is GPU time, not CPU work.
  const wait = t.rows.find((r) => r.path === WAIT)?.mean ?? 0;
  // GPU time is per drawn frame: frames that drew nothing (r_gpu_fence, stepped test frames) scale it down.
  const gpuRoot = t.rows.find((r) => r.gpu && !t.rows.some((q) => q.gpu && r.path.startsWith(q.path + "/"))); // the renderer's frame
  const draws = Math.min(1, gpuRoot?.calls ?? 1);
  const timed = t.gpu ? t.gpu.mean * draws : null;
  const frame = t.interval.mean, cpu = t.busy.mean - wait, gpu = timed == null && !wait ? null : Math.max(timed ?? 0, wait);
  const fps = (ms) => (ms > 0 ? Math.round(1000 / ms) : 0);
  const top = topCpu(t), topG = topGpu(t);
  const pct = (a) => Math.round((100 * a) / frame);
  if (gpu != null && gpu >= 0.8 * frame) return { kind: "gpu", text: `GPU-bound: the GPU needs ${gpu.toFixed(1)} ms of the ${frame.toFixed(1)} ms frame${topG ? ` (most: ${topG.name} ${topG.ms.toFixed(1)} ms)` : ""}`, top: topG?.name };
  if (cpu >= 0.8 * frame) return { kind: "cpu", text: `CPU-bound: the main thread works ${cpu.toFixed(1)} ms of the ${frame.toFixed(1)} ms frame${top ? ` (most: ${top.path} ${top.ms.toFixed(1)} ms)` : ""}`, top: top?.path };
  const near = (a, b) => Math.abs(a - b) <= 0.1 * b;
  const spare = `${pct(Math.max(cpu, gpu ?? 0))}% of the frame used`;
  if (capMs > 0 && near(frame, capMs)) return { kind: "cap", text: `Capped by fps_max at ${fps(capMs)} fps: ${spare}` };
  if (near(frame, refreshMs)) return { kind: "vsync", text: `Holding the display's ${fps(refreshMs)} Hz (vsync): ${spare}` };
  if (frame > 2.5 * Math.max(refreshMs, capMs) && cpu < 0.5 * frame && (gpu ?? 0) < 0.5 * frame) {
    return { kind: "throttled", text: `Throttled by the browser: ${fps(frame)} fps with the CPU${gpu != null ? " and GPU" : ""} mostly idle (power saving, another window?)` };
  }
  if (!gpuTimer) return { kind: "gpu", text: `Probably GPU-bound: ${fps(frame)} fps, the CPU idle ${pct(frame - cpu)}% of the frame (this browser can't time the GPU)`, guess: true };
  return { kind: "unknown", text: `${fps(frame)} fps: CPU ${cpu.toFixed(1)} ms${gpu != null ? `, GPU ${gpu.toFixed(1)} ms` : ""} of ${frame.toFixed(1)} ms (the browser's own work, compositing?)` };
}

// The leaf (or self time) costing the most CPU.
function topCpu(t) {
  let best = null;
  for (const r of t.rows) if (r.path !== WAIT && r.self > (best?.ms ?? 0)) best = { path: r.path, ms: r.self };
  return best;
}
function topGpu(t) {
  let best = null;
  for (const r of t.rows) if (r.gpu && !t.rows.some((k) => k.gpu && k.path.startsWith(r.path + "/")) && r.gpu.mean > (best?.ms ?? 0)) best = { name: r.path.split("/").pop(), ms: r.gpu.mean };
  return best;
}

const f2 = (v) => (v == null ? "  n/a" : v.toFixed(2).padStart(5));
// The overlay / prof_dump text. level 1: the verdict and the top sections;
// level 2: the whole tree with p95/p99/max, calls and the GPU column.
// info: {gpu (renderer string), res ("1920×1080 @2"), hz, verdict}.
export function formatProf(t, level, info = {}) {
  const v = info.verdict ?? verdict(t, info);
  const lines = [v.text];
  const ctx = [info.gpu, info.res, info.hz ? `${Math.round(info.hz)} Hz` : ""].filter(Boolean).join(" · ");
  if (ctx) lines.push(ctx);
  const g = t.gpu ? `${t.gpu.mean.toFixed(2)} ms (p95 ${t.gpu.p95.toFixed(2)})` : "n/a";
  lines.push(`frame ${t.interval.mean.toFixed(2)} ms (p95 ${t.interval.p95.toFixed(2)}, max ${t.interval.max.toFixed(1)}) · cpu ${t.busy.mean.toFixed(2)} · idle ${t.idle.mean.toFixed(2)} · gpu ${g}`);
  if (level <= 1) {
    const top = [...t.rows].sort((a, b) => (b.self ?? 0) - (a.self ?? 0)).slice(0, 6);
    lines.push("cpu self ms (gpu ms)");
    for (const r of top) lines.push(`  ${r.path.padEnd(22)} ${f2(r.self)}${r.gpu ? `  (${r.gpu.mean.toFixed(2)})` : ""}`);
    return lines.join("\n");
  }
  lines.push(`${"section".padEnd(26)}  cpu:  mean  self   p95   p99   max calls │ gpu:  mean   p95`);
  for (const r of t.rows) {
    const name = ("  ".repeat(r.depth) + r.name).padEnd(26).slice(0, 26);
    lines.push(`${name}       ${f2(r.mean)} ${f2(r.self)} ${f2(r.p95)} ${f2(r.p99)} ${f2(r.max)} ${r.calls.toFixed(1).padStart(5)} │      ${r.gpu ? `${f2(r.gpu.mean)} ${f2(r.gpu.p95)}` : "    –"}`);
  }
  return lines.join("\n");
}
