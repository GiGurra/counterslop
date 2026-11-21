// Grenades on the client: the server's live grenades (snapshot "nades") drawn
// in flight and as standing smoke clouds, their events (throw, bounce, he,
// flash, smoke) as sounds and effects, and the screen whiteout when the
// server says we're flashed (me.fl / me.fld in every self-snapshot).
import { chain, translate, scale, rotX, rotY, rotZ } from "../engine/math.js";
import { PALETTE as P } from "../assets/palette.js";
import { MODELS } from "../assets/models/weapons.js";
import { WEAPONS } from "../assets/weapons/index.js";
import { mulberry32 } from "../gen/rng.js";

const GRAVITY = 22, HE_TIME = 0.9, BULB_TIME = 0.25, SMOKE_GROW = 1.2, SMOKE_FADE = 2;

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
  let blindUntil = 0, blindDur = 0;
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
      }
    },
    // Effects age with the frames (so a stepped frame shows them as a played one would).
    update(dt) {
      for (let i = blasts.length - 1; i >= 0; i--) {
        const b = blasts[i];
        b.t += dt;
        if (b.t > (b.k === "he" ? HE_TIME : BULB_TIME)) blasts.splice(i, 1);
      }
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

// Stack Trace: a white bulb that flares and is gone.
export function bulbParts(p, t) {
  const s = 0.6 + t * 6, f = 1 - t / BULB_TIME;
  if (f <= 0) return [];
  return [{ m: chain(translate(...p), scale(s * f, s * f, s * f)), color: P.flash, emit: 1 }];
}
