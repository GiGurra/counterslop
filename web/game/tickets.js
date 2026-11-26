// Client side of the tickets mode (Star Slop, docs/starslop.md; the server is
// internal/game/tickets.go). Everything shown comes from state: the round in
// every snapshot (tk: each team's ticket backlog, tkMax, pts: the capture
// points [{id, own, by, pr, v, a, cont}]) and the self-snapshot (spk: our
// spawn point choice, kt: our kit, spAt: where the next spawn goes, rsp:
// seconds to the respawn). Our choice goes out as {t:"deploy", pt, kit} and is
// sent again until the self-snapshot shows it (a lost message costs a resend).
import econ from "../assets/economy.json" with { type: "json" };

export const KITS = econ.tickets?.kits || [];
const RESEND_MS = 250;

// The round-end banner text, by winner and reason.
const END = {
  vibe: { tickets: "The Moon shipped everything: its backlog is empty", time: "Time's up: Earth has the shorter backlog" },
  artisan: { tickets: "Earth shipped everything: its backlog is empty", time: "Time's up: the Moon held" },
};
export const endText = (win, why) => END[win]?.[why] ?? "";

// HUD state for the tickets mode, or null in any other mode / in warmup.
// st: the round state; my: {team, dead}; me: the last self-snapshot; map: the
// map (points' names and positions); want: our choice {pt, kit}.
export function ticketsHud(st, my, me, map, want = { pt: "auto", kit: "" }) {
  if (st?.mode !== "tickets" || !st.tk || st.ph === "warmup") return null;
  const defs = map?.points || [];
  const points = (st.pts || []).map((p, i) => {
    const d = defs[i] || {};
    return { id: p.id, name: d.name || p.id, own: p.own || "", by: p.by || "", pr: Math.round((p.pr || 0) * 50) / 50,
      cont: !!p.cont, locked: !!d.locked, spawn: !!d.spawn, pos: d.pos || [0, 0, 0], r: d.r || 0 };
  });
  const tickets = { vibe: st.tk.vibe ?? 0, artisan: st.tk.artisan ?? 0, max: st.tkMax || 0 };
  let spawn = null;
  if (my.dead && st.ph === "live" && (my.team === "vibe" || my.team === "artisan")) {
    const enemy = my.team === "vibe" ? "a" : "v";
    const ok = (p) => p.spawn && p.own === my.team && (p.locked || !(st.pts.find((q) => q.id === p.id)?.[enemy] > 0));
    spawn = {
      left: Math.ceil(me?.rsp || 0), at: me?.spAt || "", pick: want.pt || "auto",
      kit: want.kit || me?.kt || KITS[0]?.id || "",
      points: points.filter((p) => !p.locked || p.own === my.team).map((p) => ({ id: p.id, name: p.name, ok: ok(p) })),
    };
  }
  return { tickets, points, spawn };
}

// What to put on the map overview: every point at its place on a w x h
// picture of the map's floor plan (x right, z down), radius scaled alike.
export function overview(points, map, w, h, pad = 8) {
  let lo = [Infinity, Infinity], hi = [-Infinity, -Infinity];
  for (const b of map?.boxes || []) {
    lo = [Math.min(lo[0], b.min[0]), Math.min(lo[1], b.min[2])];
    hi = [Math.max(hi[0], b.max[0]), Math.max(hi[1], b.max[2])];
  }
  if (!(hi[0] > lo[0])) { lo = [-1, -1]; hi = [1, 1]; }
  const k = Math.min((w - 2 * pad) / (hi[0] - lo[0]), (h - 2 * pad) / (hi[1] - lo[1]));
  const ox = (w - k * (hi[0] - lo[0])) / 2, oy = (h - k * (hi[1] - lo[1])) / 2;
  return {
    k, field: { x: ox, y: oy, w: k * (hi[0] - lo[0]), h: k * (hi[1] - lo[1]) },
    points: points.map((p) => ({ ...p, x: ox + k * (p.pos[0] - lo[0]), y: oy + k * (p.pos[2] - lo[1]), rr: Math.max(6, k * p.r) })),
  };
}

// The deploy choice: kept here, sent until the server's self-snapshot shows it.
// deps: {send(msg), now() -> ms, store (optional: localStorage-like, the kit is remembered)}
export function createDeploy({ send, now, store }) {
  let kit = "";
  try { kit = store?.getItem("cs.kit") || ""; } catch {}
  if (!KITS.some((k) => k.id === kit)) kit = "";
  const want = { pt: "auto", kit };
  let sentAt = -Infinity;
  return {
    want,
    choose(pt) { want.pt = pt || "auto"; sentAt = -Infinity; },
    setKit(id) {
      if (!KITS.some((k) => k.id === id)) return false;
      want.kit = id; sentAt = -Infinity;
      try { store?.setItem("cs.kit", id); } catch {}
      return true;
    },
    // Every self-snapshot in tickets mode: resend while it doesn't show our choice.
    sync(me) {
      if (!me) return;
      const pt = me.spk || "auto", kt = me.kt || "";
      if (pt === want.pt && (!want.kit || kt === want.kit)) return;
      const t = now();
      if (t - sentAt < RESEND_MS) return;
      sentAt = t;
      send({ t: "deploy", pt: want.pt, kit: want.kit });
    },
  };
}
