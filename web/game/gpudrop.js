// Client side of GPU Drop (docs/gpu-drop.md; the server is internal/game/gpudrop.go).
// Everything drawn and shown comes from the round state in every snapshot
// (round.gpus [{st, by, p, pr, who}], secured, toWin); the {t:"gpu"} events
// only add sounds, announcer lines and kill-feed text, so a lost one costs
// nothing but the noise.
import { chain, translate, scale } from "../engine/math.js";
import { gpuParts, gpuCarriedParts } from "../assets/models/gpu.js";
import { PALETTE as P } from "../assets/palette.js";

// Grab reach (= content.InGrabReach): within GRAB_RANGE across, the box at most
// REACH_UP above the feet and REACH_DOWN below them.
export const GRAB_RANGE = 1.5, REACH_UP = 1.7, REACH_DOWN = 0.6;
export function inReach(feet, gpu) {
  const dy = gpu[1] - feet[1];
  return Math.hypot(gpu[0] - feet[0], gpu[2] - feet[2]) <= GRAB_RANGE && dy <= REACH_UP && dy >= -REACH_DOWN;
}

// The round-end banner text and the announcer line, by winner and reason.
const END = {
  vibe: { secured: "Sold out: the GPUs left the store", elimination: "The staff went on break. Permanently" },
  artisan: { time: "Closing time: the stock stays on the shelves", elimination: "Every scalper escorted out" },
};
export function endOf(win, why) {
  return { line: win === "vibe" ? "scalpers_win" : "staff_wins", text: END[win]?.[why] ?? "" };
}

// HUD state for the local player `my` ({id, team, pos, dead}): the top-bar
// counter, the grab / restock bar (yours, or a team-mate's: other = their id)
// and the hint. teamOf(id) is a player's team (who is grabbing vs restocking).
export function gpuHud(st, my, map, teamOf = () => "") {
  const gpus = st?.gpus || [];
  const count = (s) => gpus.filter((g) => g.st === s).length;
  const gpu = st?.mode === "gpudrop" && st.ph !== "warmup" ? { secured: st.secured || 0, toWin: st.toWin || 0, stock: count("shelf"), loose: count("carried") + count("dropped") } : null;
  const mine = gpus.find((g) => g.who === my.id && g.pr > 0);
  const mate = !mine && gpus.find((g) => g.who && g.pr > 0 && teamOf(g.who) === my.team);
  const w = mine || mate;
  const work = w ? { kind: workKind(mine ? my.team : teamOf(w.who)), pr: w.pr, ...(mine ? {} : { other: w.who }) } : null;
  const carrying = gpus.some((g) => g.st === "carried" && g.by === my.id) && !my.dead;
  let hint = "";
  if (!mine && st?.ph === "live" && !my.dead && my.pos) {
    if (carrying) {
      const names = (map?.exits || []).map((e) => e.name).join(" or the ");
      hint = `You have an RTX 9090: get it to an exit${names ? ` (the ${names})` : ""}`;
    } else if (my.team === "vibe" && gpus.some((g) => (g.st === "shelf" || g.st === "dropped") && inReach(my.pos, g.p))) {
      hint = "Hold E to grab the RTX 9090";
    } else if (my.team === "artisan" && gpus.some((g) => g.st === "dropped" && inReach(my.pos, g.p))) {
      hint = "Hold E to restock it";
    }
  }
  return { gpu, work, hint, carrying };
}

// The work bar's kind: a Vibe Coder grabs, an Artisan restocks.
export const workKind = (team) => (team === "artisan" ? "restock" : "grab");

// What to draw: every GPU on a shelf or the floor (with its progress bar), on
// carriers' backs (not hideCarrier's: that is you in first person), and the
// exits outlined while you carry one.
export function gpuSolids(st, players, time, hideCarrier, map, myId) {
  const out = [];
  if (st?.mode !== "gpudrop" || !st.gpus || st.ph === "warmup") return out;
  st.gpus.forEach((g, i) => {
    if (g.st === "carried") {
      const c = players.find((p) => p.id === g.by);
      if (c && c.id !== hideCarrier) out.push(...gpuCarriedParts(c.p, c.yaw, !!c.c, time));
    } else if (g.st === "shelf" || g.st === "dropped") {
      const yaw = g.st === "shelf" ? map?.gpus?.[i]?.yaw ?? 0 : 0.7 + i * 1.3;
      out.push(...gpuParts({ pos: g.p, yaw, time, st: g.st, progress: g.pr || 0 }));
    }
  });
  if (st.gpus.some((g) => g.st === "carried" && g.by === myId)) out.push(...exitOutlines(map, time));
  return out;
}

const LINE = 0.18, THICK = 0.03;
const slab = (x0, y, z0, x1, z1, color, emit) => ({ m: chain(translate((x0 + x1) / 2, y + THICK / 2, (z0 + z1) / 2), scale(x1 - x0, THICK, z1 - z0)), color, emit });
// A pulsing lime outline on the floor of every exit (like the buy zones').
export function exitOutlines(map, time) {
  const out = [], e = 0.55 + 0.45 * Math.sin(time * 5);
  for (const z of map?.exits || []) {
    const [x0, y, z0] = z.min, [x1, , z1] = z.max;
    out.push(slab(x0, y, z0, x1, z0 + LINE, P.neonLime, e), slab(x0, y, z1 - LINE, x1, z1, P.neonLime, e),
      slab(x0, y, z0, x0 + LINE, z1, P.neonLime, e), slab(x1 - LINE, y, z0, x1, z1, P.neonLime, e));
  }
  return out;
}

// Sounds, lines and feed text. deps: {sfx, say, spatial(p), feed(text), nameOf(id), me(), music}.
export function createGpuDrop(deps) {
  const { sfx, say, spatial, feed, nameOf, me } = deps;
  let stockKey = ""; // round:stock as of the last snapshot (Out of stock! once when it hits 0)
  return {
    // Per snapshot: the shelves running empty is state, not an event.
    ingest(st) {
      if (st?.mode !== "gpudrop" || st.ph !== "live" || !st.gpus) { stockKey = ""; return; }
      const stock = st.gpus.filter((g) => g.st === "shelf").length;
      const key = `${st.n}:${stock}`;
      if (stockKey && stockKey !== key && stock === 0 && stockKey.startsWith(`${st.n}:`)) say("out_of_stock");
      stockKey = key;
    },
    event(m) {
      const my = me(), who = m.by === my.id ? "You" : nameOf(m.by);
      switch (m.ev) {
        case "grab":
          sfx.gpuGrab?.(spatial(m.p));
          say("gpu_grabbed");
          feed(`${who} grabbed an RTX 9090.`);
          break;
        case "drop":
          feed("An RTX 9090 is on the floor!");
          break;
        case "restock":
          sfx.gpuRestock?.(spatial(m.p));
          say("restocked");
          feed(`${who} restocked an RTX 9090.`);
          break;
        case "secure":
          sfx.gpuSecure?.(spatial(m.p));
          say("gpu_secured");
          deps.music?.secured?.();
          feed(`${who} secured an RTX 9090.`);
          break;
      }
    },
  };
}
