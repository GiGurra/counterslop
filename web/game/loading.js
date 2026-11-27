// A map change's loading screen and our part in it (internal/game/maploading.go).
//
// The server announces a load in every snapshot (round.load: {id, map, step:
// "nav"|"ready", secs, pl: [{id, step, pct}]}) and holds the world frozen until
// it and every player are ready, or sv_load_wait seconds pass. We report how
// far our own build of the map is ({t:"loading", lid, step, pct}): on a change,
// and again every RESEND_MS until the snapshots show it (the network may lose
// any message; the server keeps the newest per load id and drops the rest).
//
// Pure: main.js passes the clock, our progress and the snapshot's state.

export const RESEND_MS = 1000;
export const PCT_EVERY_MS = 250; // a new percent goes out at most this often (a step change at once)

// Our own steps, in order (what the "You" line and the others' rows say).
export const STEPS = ["map", "geometry", "lights", "shaders", "ready"];
export const STEP_TEXT = { "": "connecting", map: "fetching the map", geometry: "building geometry", lights: "building lights", shaders: "compiling shaders", ready: "ready" };
export const SERVER_TEXT = { nav: "building the bots' nav graph", ready: "ready" };

// The report to send now, or null. load: the snapshot's round.load (null: no
// load under way); mine: {step, pct}; myId: our player id.
export function createLoadReporter() {
  let sent = null, sentAt = -Infinity;
  return {
    next(load, mine, myId, now) {
      if (!load || myId < 0) { sent = null; return null; }
      const msg = { t: "loading", lid: load.id, step: mine.step, pct: Math.round(mine.pct || 0) };
      const row = load.pl?.find((r) => r.id === myId);
      if (row && row.step === msg.step && (row.pct || 0) === msg.pct) { sent = msg; sentAt = now; return null; } // the server has it
      const same = sent && sent.lid === msg.lid && sent.step === msg.step;
      if (same && sent.pct === msg.pct && now - sentAt < RESEND_MS) return null; // sent, not shown yet: wait before resending
      if (same && sent.pct !== msg.pct && now - sentAt < PCT_EVERY_MS) return null; // a percent: not every frame
      sent = msg; sentAt = now;
      return msg;
    },
    reset() { sent = null; sentAt = -Infinity; },
  };
}

// What the loading screen shows. load: round.load or null; mine: {step, pct}
// or null (our map is built); names: id -> name; myId; mapName: the map we
// have. Null: no screen.
export function loadView(load, mine, names, myId, mapName) {
  if (!load) return mine ? { map: mapName, own: true, me: line(mine) } : null; // our own build only (joining, a reconnect)
  const others = (load.pl || []).filter((r) => r.id !== myId);
  const waiting = others.filter((r) => r.step !== "ready").map((r) => ({ id: r.id, name: names[r.id] ?? `#${r.id}`, ...line(r) }));
  const ready = others.length - waiting.length;
  const meLine = line(mine || { step: "ready" });
  const server = { step: load.step, text: SERVER_TEXT[load.step] ?? load.step, secs: load.secs || 0 };
  let status;
  if (load.step !== "ready") status = mine ? "Loading with the server…" : "Waiting for the server";
  else if (mine) status = "The server is ready · finishing your load";
  else if (waiting.length) status = `Waiting for ${waiting.map((w) => w.name).join(", ")}`;
  else status = "Starting…";
  return { map: load.map, server, me: meLine, waiting, ready, total: others.length + 1, status, startsIn: load.step === "ready" && waiting.length ? server.secs : null };
}

function line(r) {
  const step = r.step ?? "";
  const pct = step === "ready" ? 100 : Math.max(0, Math.min(100, Math.round(r.pct || 0)));
  return { step, text: STEP_TEXT[step] ?? step, pct, done: step === "ready" };
}

// Our progress on a map, from what main.js knows: due (the map is set but the
// renderer hasn't started on it), the renderer's loadStep() ({step, frac} or
// null when drawn), and whether the map we have is the one being loaded.
export function ownProgress({ have, due, gfxStep }) {
  if (!have) return { step: "map", pct: 0 };
  if (due) return { step: "geometry", pct: 0 };
  if (!gfxStep) return { step: "ready", pct: 100 };
  if (gfxStep.step === "lights") return { step: "lights", pct: Math.floor(gfxStep.frac * 100) };
  return { step: gfxStep.step, pct: 0 };
}
