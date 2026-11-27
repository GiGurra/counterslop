// The ghost of your best run on a race map (docs/racing.md "Ghost"): while
// you race, your own run is sampled on the race clock (where you were, which
// way you faced, on foot, in a car or a boat); when you finish faster than
// the best run kept for that map (or there is none), the run becomes the
// map's best, kept in localStorage (per browser, per map). In the next race
// the best run plays back on the race clock: a translucent ghost racing
// alongside (main.js draws it; cl_ghost turns it off). Client-only: the
// server never sees it, so it can't touch gameplay.

import { playerParts } from "../assets/models/player.js";
import { carParts } from "../assets/models/car.js";
import { boatParts } from "../assets/models/boat.js";
import { newGait, resetGait, gaitStep } from "../gen/gait.js";

export const GHOST_HZ = 10; // samples a second of race clock
export const GHOST_V = 1; // the stored format's version (another one is ignored)
const KEY = "counterslop.ghost."; // + the map's name
const MAX_SAMPLES = GHOST_HZ * 60 * 20; // a 20 minute run at most (the stored run stays ~1 MB or less)

// Sample kinds: on foot (bit 4: on the ground), in a car, in a boat.
export const FOOT = 0, CAR = 1, BOAT = 2, GROUND = 4;

const r2 = (v) => Math.round(v * 100) / 100, r3 = (v) => Math.round(v * 1000) / 1000;

// storage: localStorage when there is one and it works, else nothing (a
// private window, blocked site data, Node): every access in try/catch.
export function browserStore() {
  try {
    const s = globalThis.localStorage;
    if (!s) return null;
    return {
      get: (k) => { try { return s.getItem(k); } catch { return null; } },
      set: (k, v) => { try { s.setItem(k, v); return true; } catch { return false; } },
    };
  } catch { return null; }
}

// A stored run, checked: {v, map, t: the finish time, s: samples [t, x, y, z, yaw, kind, car?]} or null.
export function parseRun(text, map) {
  try {
    const r = JSON.parse(text);
    if (r?.v !== GHOST_V || r.map !== map || !(r.t > 0) || !Array.isArray(r.s) || r.s.length < 2) return null;
    for (const s of r.s) if (!Array.isArray(s) || s.length < 6 || !s.slice(0, 5).every(Number.isFinite)) return null;
    return r;
  } catch { return null; }
}

const lerpAngle = (a, b, k) => a + Math.atan2(Math.sin(b - a), Math.cos(b - a)) * k;

// The pose of a run at race-clock time t (s): {pos, yaw, kind, car, ground},
// or null before its first sample or after its finish.
export function poseAt(run, t) {
  const s = run?.s;
  if (!s?.length || t < s[0][0] || t > run.t) return null;
  let lo = 0, hi = s.length - 1;
  if (t >= s[hi][0]) lo = hi;
  else while (hi - lo > 1) { const m = (lo + hi) >> 1; if (s[m][0] <= t) lo = m; else hi = m; }
  const a = s[lo], b = s[Math.min(lo + 1, s.length - 1)];
  const span = b[0] - a[0], k = span > 0 ? Math.min(1, Math.max(0, (t - a[0]) / span)) : 0;
  // A jump in kind (into a car, a respawn far away) is not blended across.
  const far = Math.hypot(b[1] - a[1], b[2] - a[2], b[3] - a[3]) > 40 * Math.max(span, 0.1);
  const kk = (a[5] & 3) !== (b[5] & 3) || far ? 0 : k;
  return {
    pos: [a[1] + (b[1] - a[1]) * kk, a[2] + (b[2] - a[2]) * kk, a[3] + (b[3] - a[3]) * kk],
    yaw: lerpAngle(a[4], b[4], kk), kind: a[5] & 3, ground: !!(a[5] & GROUND), car: a[6] || "",
  };
}

// createGhost: one per client. update() every frame with the map's name, the
// race (rounds' raceHud: null outside a race) and our own pose; best/at for drawing.
export function createGhost({ store = browserStore() } = {}) {
  let map = "", best = null, run = null, lastT = -1, saved = false, finishedSeen = false;

  function load(name) {
    map = name; best = null; run = null; lastT = -1; saved = false; finishedSeen = false;
    const text = name && store?.get(KEY + name);
    if (text) best = parseRun(text, name);
  }

  const g = {
    get best() { return best; },
    get recording() { return run ? run.length : 0; },
    key: (name) => KEY + name,

    // race: {live, clock, finished (our place, 0: not yet), finishTime, racing (we are a racer)}
    // pose: {pos, yaw, kind, car?, ground?} or null (dead, spectating)
    update(name, race, pose) {
      if (name !== map) load(name);
      if (!race?.live || !race.racing) {
        if (!race?.live) { run = null; lastT = -1; saved = false; finishedSeen = false; }
        return;
      }
      if (race.clock + 0.5 < lastT) { run = null; lastT = -1; saved = false; finishedSeen = false; } // a new race: the clock went back
      if (race.finished) {
        if (!finishedSeen && run && run.length >= 2 && !saved) g.finish(race.finishTime);
        finishedSeen = true;
        return;
      }
      if (!pose || (run && Math.floor(race.clock * GHOST_HZ) <= Math.floor(lastT * GHOST_HZ)) || (run && run.length >= MAX_SAMPLES)) return; // one sample per 1/GHOST_HZ of race clock
      run ||= [];
      lastT = race.clock;
      const s = [r3(race.clock), r2(pose.pos[0]), r2(pose.pos[1]), r2(pose.pos[2]), r3(pose.yaw), (pose.kind & 3) | (pose.ground ? GROUND : 0)];
      if (pose.kind === CAR && pose.car) s.push(pose.car);
      run.push(s);
    },

    // finish: the run ended in time t; kept if it beats the best (true).
    finish(t) {
      if (!run || run.length < 2 || !(t > 0)) return false;
      saved = true;
      if (best && best.t <= t) return false;
      best = { v: GHOST_V, map, t: r3(t), s: run };
      store?.set(KEY + map, JSON.stringify(best));
      return true;
    },

    // at: the best run's pose at race-clock time t (null: no best run, or not on the course then).
    at: (t) => poseAt(best, t),

    // adopt: run r ({v, map, t, s}) as the map's best, if it checks out (the test API's ghostRun).
    adopt(r) {
      const ok = parseRun(JSON.stringify(r), map);
      if (!ok) return false;
      best = ok;
      store?.set(KEY + map, JSON.stringify(ok));
      return true;
    },

    // forget the map's best run.
    clear() {
      best = null;
      store?.set(KEY + map, "");
    },
  };
  return g;
}

// The ghost's looks: every part of the model it was in (a player, a car, a
// boat) washed towards a pale cyan, glowing a little, see-through (the
// renderer draws alpha < 1 last, blended: engine/gl.js drawSolids).
export const GHOST_ID = 0x6857; // a fixed id: the same seeded looks and walk every race
export const GHOST_TINT = [0.55, 0.9, 1], GHOST_ALPHA = 0.35;
const gait = newGait();
let gaitAt = null; // where the ghost's walk cycle last stepped from

// ghostParts: the solids of the ghost at pose g ({pos, yaw, kind, car, ground}).
// carSpec: id -> the car's spec (cars.js).
export function ghostParts(g, { time = 0, carSpec = null } = {}) {
  let parts;
  if (g.kind === CAR && carSpec) {
    gaitAt = null;
    parts = carParts({ spec: carSpec(g.car || "buggy"), pos: g.pos, yaw: g.yaw, time, id: GHOST_ID });
  } else if (g.kind === BOAT) {
    gaitAt = null;
    parts = boatParts({ pos: g.pos, yaw: g.yaw, time, id: GHOST_ID });
  } else {
    if (!gaitAt || Math.hypot(g.pos[0] - gaitAt[0], g.pos[1] - gaitAt[1], g.pos[2] - gaitAt[2]) > 5) resetGait(gait); // a respawn: no stride across it
    else gaitStep(gait, g.pos[0] - gaitAt[0], g.pos[1] - gaitAt[1], g.pos[2] - gaitAt[2], { onGround: g.ground, id: GHOST_ID });
    gaitAt = g.pos.slice();
    parts = playerParts({ id: GHOST_ID, team: "", p: g.pos, yaw: g.yaw, pitch: 0, rgb: GHOST_TINT, time, crouch: 0, gait, t: time, gv: 0 });
  }
  return parts.map((pt) => ({ ...pt, color: pt.color.map((c, i) => c * 0.4 + GHOST_TINT[i] * 0.6), emit: Math.min(0.9, (pt.emit || 0) + 0.5), alpha: GHOST_ALPHA }));
}
