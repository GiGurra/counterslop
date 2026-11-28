// The map previews' work queue (pure: no DOM, no timers of its own): the cards
// ask for a preview when they scroll into view and take it back when they
// scroll away; one preview is worked on at a time, the lowest priority number
// first (the vote's options before any card, cards in page order), and each
// one is handed to onDone the moment it is ready, before the next one starts.
//
// work(name) -> Promise<result | null> (a cache read or a render);
// onDone(name, result, src) puts it on its card; yieldFn() runs between two
// previews (main.js: a macrotask, so the page paints the finished one first).
export function createPreviewQueue(work, onDone, { yieldFn = () => Promise.resolve(), onStart = null } = {}) {
  const wanted = new Map(); // name -> priority
  const done = new Set();
  let busy = null, running = false;
  const log = []; // ["start" | "done" | "fail", name] in order (tests)

  function next() {
    let best = null, bp = Infinity;
    for (const [n, p] of wanted) if (p < bp) { best = n; bp = p; }
    return best;
  }

  async function run() {
    if (running) return;
    running = true;
    try {
      await yieldFn(); // a batch of wants (the cards coming into view together) is in before the first pick
      for (let n = next(); n != null; n = next()) {
        wanted.delete(n);
        busy = n;
        log.push(["start", n]);
        onStart?.(n);
        let r = null;
        try { r = await work(n); } catch { r = null; }
        busy = null;
        if (r != null) { done.add(n); log.push(["done", n]); onDone(n, r); } else log.push(["fail", n]);
        await yieldFn();
      }
    } finally { running = false; }
  }

  return {
    // Ask for name's preview (again: a better priority wins). Does nothing once it's done.
    want(name, prio = 0) {
      if (done.has(name) || busy === name) return;
      if (!wanted.has(name) || prio < wanted.get(name)) wanted.set(name, prio);
      run();
    },
    // Scrolled away before its turn: not drawn (the one being drawn finishes).
    drop(name) { wanted.delete(name); },
    // Forget a preview (a custom map changed): the next want draws it again.
    forget(name) { done.delete(name); },
    pending: () => [...wanted.entries()].sort((a, b) => a[1] - b[1]).map(([n]) => n),
    busy: () => busy,
    idle: () => !busy && !wanted.size,
    log,
  };
}

// Resolves after the browser has painted a frame: a requestAnimationFrame
// callback runs just before the paint, a timer queued from it just after. A
// bare setTimeout(0) between two previews can run before any paint, so where
// the previews block the page's thread (no worker) every card changed at
// once at the end instead of one by one.
export const afterPaint = (raf = globalThis.requestAnimationFrame) => new Promise((r) =>
  typeof raf === "function" ? raf(() => setTimeout(r, 0)) : setTimeout(r, 0));
