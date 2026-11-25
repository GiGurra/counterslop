// Grenades on the client: the server's live grenades (snapshot "nades") drawn
// in flight, as standing smoke clouds and as fire pools (flames on the
// server's flame points, n.fl), their events (throw, bounce, he, flash,
// smoke, fire, out) as sounds and effects, and the screen whiteout when the
// server says we're flashed (me.fl / me.fld in every self-snapshot).
import { chain, translate, scale, rotX, rotY, rotZ } from "../engine/math.js";
import { PALETTE as P } from "../assets/palette.js";
import { MODELS } from "../assets/models/weapons.js";
import { WEAPONS } from "../assets/weapons/index.js";
import { mulberry32 } from "../gen/rng.js";

const GRAVITY = 22, HE_TIME = 0.9, BULB_TIME = 0.25, SMOKE_GROW = 1.2, SMOKE_FADE = 2;
const FIRE_GROW = 0.35, FIRE_FADE = 1.2, CRACKLE_EVERY = 0.45; // seconds

// The boxes of a smoke cloud, seeded by the grenade id so every client sees the same one.
export function smokeCloud(id, radius) {
  const r = mulberry32(0x5e0e + id), out = [];
  for (let i = 0; i < 48; i++) {
    const a = r() * Math.PI * 2, d = Math.sqrt(r()) * radius * 0.8, y = 0.4 + r() * radius * 0.5;
    out.push({ x: Math.sin(a) * d, y, z: Math.cos(a) * d, s: 1.1 + r() * 1.2, spin: (r() - 0.5) * 0.3, tilt: r() * 3, shade: r() < 0.4 });
  }
  return out;
}

// How far into the cloud a point is: 1 inside the core, 0 outside.
export function smokeDensity(pt, centre, radius, grow) {
  const dx = pt[0] - centre[0], dy = (pt[1] - centre[1] - radius * 0.3) * 1.4, dz = pt[2] - centre[2];
  const d = Math.hypot(dx, dy, dz) / Math.max(0.01, radius * grow);
  return Math.max(0, Math.min(1, (1 - d) * 3));
}

// deps: {sfx, spatial(p), overlay: an element for the whiteout (optional)}
export function createNades(deps) {
  const { sfx, spatial } = deps;
  let live = new Map(), at = 0; // id -> server nade; time of the snapshot
  const blasts = []; // {k, p, t0}
  let blindUntil = 0, blindDur = 0, crackle = 0;
  const clouds = new Map(); // id -> boxes

  const spec = (w) => WEAPONS[w]?.grenade || { radius: 4.5, duration: 18 };
  const st = {
    get list() { return [...live.values()]; },
    get blastCount() { return blasts.length; },
    events: 0, log: [],
    get blind() { return { until: blindUntil, dur: blindDur }; },
    ingest(list, now) {
      live = new Map((list || []).map((n) => [n.id, n]));
      at = now;
      for (const id of clouds.keys()) if (!live.has(id)) clouds.delete(id);
    },
    // Our own flash state from the self-snapshot (seconds left, out of how many).
    flashed(left, dur, now) {
      const until = now + (left || 0) * 1000;
      if (left > 0 && until > blindUntil + 300) sfx.flashRing?.(Math.min(left, 4)); // a new flash
      blindUntil = left > 0 ? until : Math.min(blindUntil, now);
      blindDur = dur || 0;
    },
    event(m, now) {
      st.events++;
      st.log.push(m.ev); if (st.log.length > 12) st.log.shift();
      const sp = m.p ? spatial(m.p) : undefined;
      switch (m.ev) {
        case "throw": if (m.by !== deps.myId?.()) sfx.nadeThrow?.(sp); break;
        case "bounce": sfx.nadeBounce?.(sp); break;
        case "he": sfx.heBlast?.(sp); blasts.push({ k: "he", p: m.p, t: 0 }); break;
        case "flash": sfx.flashBang?.(sp); blasts.push({ k: "flash", p: m.p, t: 0 }); break;
        case "smoke": sfx.smokePop?.(sp); break;
        case "fire": sfx.fireBurst?.(sp); break;
        case "out": sfx.fireOut?.(sp); break;
      }
    },
    // Effects age with the frames (so a stepped frame shows them as a played one would).
    update(dt) {
      crackle -= dt;
      if (crackle <= 0) { // a crackle from each burning pool (spatial: the far ones are quiet)
        crackle = CRACKLE_EVERY;
        for (const n of live.values()) if (n.st === "fire") sfx.fireCrackle?.(spatial(n.p));
      }
      for (let i = blasts.length - 1; i >= 0; i--) {
        const b = blasts[i];
        b.t += dt;
        if (b.t > (b.k === "he" ? HE_TIME : BULB_TIME)) blasts.splice(i, 1);
      }
    },
    // Standing smokes' cores, as the server's smokeBetween sees them (radar
    // spotting): a sight line within r of c is blocked. Name tags use it.
    smokeCores() {
      const out = [];
      for (const n of live.values()) {
        if (n.st !== "smoke") continue;
        const r = spec(n.w).radius;
        out.push({ c: [n.p[0], n.p[1] + r * 0.3, n.p[2]], r: r * 0.75 });
      }
      return out;
    },
    // 0..1: how much smoke the eye is in.
    smokeAt(eye) {
      let a = 0;
      for (const n of live.values()) {
        if (n.st !== "smoke") continue;
        const g = spec(n.w), age = g.duration - n.left;
        a = Math.max(a, smokeDensity(eye, n.p, g.radius, grow(age, n.left)));
      }
      return a;
    },
    // 0..1 whiteout now.
    whiteout(now) {
      const left = (blindUntil - now) / 1000;
      return left > 0 ? Math.min(1, left / 1.5) : 0;
    },
    solids(time, now) {
      const out = [], dt = Math.min(0.1, Math.max(0, (now - at) / 1000));
      for (const n of live.values()) {
        if (n.st === "smoke") { out.push(...cloudParts(n, time)); continue; }
        if (n.st === "fire") { out.push(...fireParts(n.fl, n.id, time, fireGrow(spec(n.w).duration - n.left, n.left))); continue; }
        const p = [n.p[0] + n.v[0] * dt, n.p[1] + n.v[1] * dt - 0.5 * GRAVITY * dt * dt, n.p[2] + n.v[2] * dt];
        const spin = Math.hypot(n.v[0], n.v[2]) > 0.2 ? time * 9 + n.id : 0;
        const base = chain(translate(p[0], p[1] + 0.05, p[2]), rotY(n.id), rotX(spin), scale(1.2, 1.2, 1.2));
        for (const [x, y, z, sx, sy, sz, col, emit = 0] of MODELS[n.w] || []) {
          out.push({ m: chain(base, translate(x, y, z), scale(sx, sy, sz)), color: P[col] || P.gunmetal, emit });
        }
      }
      for (const b of blasts) out.push(...(b.k === "he" ? heParts(b.p, b.t) : bulbParts(b.p, b.t)));
      return out;
    },
  };
  const fireGrow = (age, left) => Math.min(1, Math.max(0.05, age / FIRE_GROW)) * Math.min(1, Math.max(0, left / FIRE_FADE));
  const grow = (age, left) => Math.min(1, Math.max(0.05, age / SMOKE_GROW)) * Math.min(1, Math.max(0, left / SMOKE_FADE));
  function cloudParts(n, time) {
    const g = spec(n.w), k = grow(g.duration - n.left, n.left);
    if (!clouds.has(n.id)) clouds.set(n.id, smokeCloud(n.id, g.radius));
    return clouds.get(n.id).map((b) => ({
      m: chain(translate(n.p[0] + b.x * k, n.p[1] + b.y * k, n.p[2] + b.z * k), rotY(b.tilt + time * b.spin), rotZ(b.tilt), scale(b.s * k, b.s * k * 0.8, b.s * k)),
      color: b.shade ? P.smokeShade : P.smoke, emit: 0.7,
    }));
  }
  return st;
}

// Merge Conflict: a fireball of green and red chunks (ours and theirs) around a hot core.
export function heParts(p, t) {
  const out = [], r = mulberry32(0x4e), k = t / HE_TIME;
  const core = Math.max(0, 2.4 * (1 - t / 0.35));
  if (core > 0) out.push({ m: chain(translate(p[0], p[1] + core * 0.4, p[2]), scale(core, core, core)), color: P.flashHot, emit: 1 });
  for (let i = 0; i < 28; i++) {
    const a = r() * Math.PI * 2, up = r() * 1.4, sp = 3 + r() * 6, s = (0.25 + r() * 0.35) * (1 - k);
    const d = sp * t;
    out.push({ m: chain(translate(p[0] + Math.sin(a) * d, p[1] + up * d + 0.3 - 6 * t * t, p[2] + Math.cos(a) * d), rotY(a + t * 5), scale(s, s, s)), color: i % 2 ? P.reviewGreen : P.reviewRed, emit: 0.9 });
  }
  return out;
}

// A fire pool's flames (Hot Reload, Burndown Chart): on each of the server's
// flame points a scorch tile and three tongues (edge red, mid orange, a hot
// core) that flicker up and down, seeded by the pool id and the point so every
// client sees the same fire. k: 0..1, the pool catching and dying down.
export function fireParts(points, id, time, k = 1) {
  const out = [];
  (points || []).forEach((f, i) => {
    const r = mulberry32((id * 977 + i * 131) >>> 0), ph = r() * 6.28, sp = 7 + r() * 6;
    const flick = 0.75 + 0.25 * Math.sin(time * sp + ph) + 0.12 * Math.sin(time * sp * 2.3 + ph * 1.7);
    const jx = (r() - 0.5) * 0.3, jz = (r() - 0.5) * 0.3, tall = (0.45 + r() * 0.5) * k * flick;
    out.push({ m: chain(translate(f[0], f[1] + 0.01, f[2]), scale(0.72, 0.02, 0.72)), color: P.scorch, emit: 0 });
    if (tall < 0.03) return;
    const base = chain(translate(f[0] + jx, f[1], f[2] + jz), rotY(ph + time * 0.6));
    out.push({ m: chain(base, translate(0, tall * 0.35, 0), scale(0.38 * k, tall * 0.7, 0.38 * k)), color: P.flameEdge, emit: 1 });
    out.push({ m: chain(base, translate(0.04, tall * 0.55, -0.03), rotY(0.7), scale(0.24 * k, tall * 1.05, 0.24 * k)), color: P.flameMid, emit: 1 });
    out.push({ m: chain(base, translate(-0.03, tall * 0.4, 0.02), scale(0.12 * k, tall * 0.75, 0.12 * k)), color: P.flameCore, emit: 1 });
  });
  return out;
}

// Stack Trace: a white bulb that flares and is gone.
export function bulbParts(p, t) {
  const s = 0.6 + t * 6, f = 1 - t / BULB_TIME;
  if (f <= 0) return [];
  return [{ m: chain(translate(...p), scale(s * f, s * f, s * f)), color: P.flash, emit: 1 }];
}
