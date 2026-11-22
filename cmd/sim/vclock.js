// Virtual clock for lockstep scenarios (cmd/sim lockstep.go). The harness
// injects this into each page before any of the page's own scripts run
// (Page.addScriptToEvaluateOnNewDocument), and only for lockstep runs: it
// lives here, not under web/, so no served page can load it.
//
// The page's clocks (performance.now, Date.now) and timers (setTimeout,
// setInterval, requestAnimationFrame) then run on game time: they move only
// when the harness advances them (a frame: 1/60 s, a wait: its ms, a server
// tick inside a call: 10 ms). So captions, fades, the HUD's timers and the
// frame clock read the same at the same step however slow the machine is.
// The real timers stay at window.__real for the harness's own polling.
(function install(w) {
  if (w.__vclock) return;
  const real = {
    setTimeout: w.setTimeout.bind(w), clearTimeout: w.clearTimeout.bind(w),
    setInterval: w.setInterval.bind(w), clearInterval: w.clearInterval.bind(w),
    requestAnimationFrame: w.requestAnimationFrame?.bind(w), cancelAnimationFrame: w.cancelAnimationFrame?.bind(w),
    perfNow: w.performance.now.bind(w.performance), dateNow: Date.now,
  };
  const FRAME = 1000 / 60;
  const t0 = real.perfNow(), d0 = real.dateNow();
  let now = 0; // ms of virtual time since install
  let nextId = 1, seq = 0;
  const timers = new Map(); // id -> {at, seq, fn, args, every}
  let rafs = new Map(); // id -> fn

  const add = (fn, ms, args, every) => {
    const id = nextId++;
    const d = Math.max(0, +ms || 0);
    timers.set(id, { at: now + d, seq: seq++, fn, args, every: every ? Math.max(1, d) : 0 });
    return id;
  };
  const due = (limit) => {
    let best = null, bid = 0;
    for (const [id, t] of timers) if (t.at <= limit && (!best || t.at < best.at || (t.at === best.at && t.seq < best.seq))) { best = t; bid = id; }
    return best ? [bid, best] : null;
  };
  const call = (fn, args) => { try { typeof fn === "function" ? fn(...(args || [])) : (0, eval)(String(fn)); } catch (e) { real.setTimeout(() => { throw e; }); } };
  const flushMicrotasks = () => new Promise((r) => real.setTimeout(r, 0));
  const runRafs = async () => {
    const fns = rafs; rafs = new Map();
    for (const fn of fns.values()) call(fn, [t0 + now]);
    await flushMicrotasks();
  };

  w.setTimeout = (fn, ms, ...args) => add(fn, ms, args, false);
  w.setInterval = (fn, ms, ...args) => add(fn, ms, args, true);
  w.clearTimeout = w.clearInterval = (id) => { timers.delete(id); };
  w.requestAnimationFrame = (fn) => { const id = nextId++; rafs.set(id, fn); return id; };
  w.cancelAnimationFrame = (id) => { rafs.delete(id); };
  w.performance.now = () => t0 + now;
  Date.now = () => d0 + Math.round(now);

  w.__real = real;
  w.__vclock = {
    now: () => now,
    // Move the clock ms forward, firing every timer that falls due in order
    // (each at its own time, microtasks after each) and the animation frames
    // at every 1/60 s boundary crossed.
    async advance(ms) {
      const end = now + Math.max(0, ms);
      for (;;) {
        const nextFrame = (Math.floor(now / FRAME + 1e-9) + 1) * FRAME;
        const d = due(Math.min(end, nextFrame));
        if (d) {
          const [id, t] = d;
          now = Math.max(now, t.at);
          if (t.every) t.at += t.every; else timers.delete(id);
          call(t.fn, t.args);
          await flushMicrotasks();
          continue;
        }
        if (nextFrame <= end) { now = nextFrame; await runRafs(); continue; }
        now = end;
        return now;
      }
    },
    // Run what is due now (zero-delay timers, pending animation frames)
    // without moving the clock: Preact effects after a UI call.
    async settle() {
      for (let i = 0; i < 100; i++) {
        const d = due(now);
        if (!d) break;
        const [id, t] = d;
        if (t.every) t.at += t.every; else timers.delete(id);
        call(t.fn, t.args);
        await flushMicrotasks();
      }
      await runRafs();
      return true;
    },
    pending: () => ({ timers: timers.size, rafs: rafs.size }),
  };
})(globalThis);
