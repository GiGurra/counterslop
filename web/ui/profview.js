// The profiler overlay (cl_showprof 1|2), under the fps overlay: plain DOM,
// its text rebuilt 4 times a second, never per frame. docs/profiling.md.
import { formatProf, verdict } from "../engine/prof.js";
import { overlayColumn } from "../engine/loop.js";

// env() -> what the verdict and the header need: {refreshMs, capMs, hidden, gpuTimer, gpu, res, hz}.
export function createProfOverlay(prof, settings, env) {
  const el = document.createElement("div");
  el.className = "prof";
  el.style.display = "none";
  const head = document.createElement("div");
  head.className = "verdict";
  const body = document.createElement("div");
  el.append(head, body);
  overlayColumn().appendChild(el);
  const view = { text: "", verdict: null };
  function update() {
    const level = settings.get().cl_showprof | 0;
    el.style.display = level ? "" : "none";
    if (!level) return;
    const e = env(), t = prof.table();
    const v = verdict(t, e);
    const text = formatProf(t, level, { ...e, verdict: v });
    const nl = text.indexOf("\n");
    head.textContent = nl < 0 ? text : text.slice(0, nl);
    body.textContent = nl < 0 ? "" : text.slice(nl + 1);
    view.text = text; view.verdict = v;
  }
  setInterval(update, 250);
  settings.subscribe(update); // a level change shows at once
  return { el, update, view };
}
