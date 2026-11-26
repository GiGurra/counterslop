// The cars' combat on the client (race phase 3; the server is internal/game
// carcombat.go): the live shots and drops from every full snapshot ("gear":
// [{id, k: the item, p, v, a: armed}]) drawn where they are (shots run ahead
// by their velocity), and the optional "car" events as effects: a beam's
// line, a blast, a wreck's burst. Placeholder looks (phase 4 does the real
// ones); every effect ages with the frames.
import { chain, translate, scale, rotY, rotX } from "../engine/math.js";
import { PALETTE as P } from "../assets/palette.js";
import { carItem } from "./cars.js";

const BEAM_TIME = 0.12, BOOM_TIME = 0.7, WRECK_TIME = 1.2; // seconds an effect shows

// The boxes of one live piece of gear at p (time: the shader clock, for spin and blink).
export function gearParts(g, p, time) {
  const it = carItem(g.k), kind = it?.kind || "";
  const box = (x, y, z, sx, sy, sz, color, emit = 0, yaw = 0) => ({ m: chain(translate(p[0] + x, p[1] + y, p[2] + z), rotY(yaw), scale(sx, sy, sz)), color, emit });
  const heading = Math.atan2(-(g.v?.[0] || 0), -(g.v?.[2] || 0));
  switch (kind) {
    case "rocket": { // a body and a flame behind it
      const big = it.damage > 45 ? 1.4 : 1;
      return [{ m: chain(translate(p[0], p[1], p[2]), rotY(heading), rotX(time * 20), scale(0.18 * big, 0.18 * big, 0.7 * big)), color: P.steel, emit: 0 },
        { m: chain(translate(p[0], p[1], p[2]), rotY(heading), translate(0, 0, 0.5 * big), scale(0.14 * big, 0.14 * big, 0.35 * big)), color: P.sunGlow, emit: 1 }];
    }
    case "orb": { const s = 0.45 + 0.08 * Math.sin(time * 18 + g.id); return [box(0, 0, 0, s, s, s, P.ledBlue, 1, time * 6), box(0, 0, 0, s * 0.6, s * 0.6, s * 0.6, P.lamp, 1, -time * 9)]; }
    case "spikes": { // a strip of spikes across the road
      const out = [box(0, 0.03, 0, it.radius * 1.6, 0.06, 0.5, P.tyre, 0, heading)];
      for (let i = -3; i <= 3; i++) out.push({ m: chain(translate(p[0], p[1], p[2]), rotY(heading), translate(i * it.radius * 0.22, 0.15, 0), scale(0.08, 0.3, 0.08)), color: P.steel, emit: 0 });
      return out;
    }
    case "oil": return [box(0, 0.02, 0, it.radius * 1.6, 0.03, it.radius * 1.6, P.hazardDark, 0, g.id), box(0.3, 0.025, -0.2, it.radius, 0.03, it.radius * 0.8, P.chip, 0.05, g.id * 2)];
    case "mine": { // a puck and a light that blinks once armed
      const on = g.a && Math.floor(time * 4 + g.id) % 2 === 0;
      return [box(0, 0.1, 0, 0.6, 0.2, 0.6, it.seek ? P.containerYellow : P.rack, 0), box(0, 0.25, 0, 0.15, 0.08, 0.15, g.a ? P.diffDel : P.ledAmber, on || !g.a ? 1 : 0.1)];
    }
  }
  return [box(0, 0.2, 0, 0.4, 0.4, 0.4, P.hazard, 0.5)];
}

// A beam from a to b at age t (s): a thin glowing bar, fading.
export function beamParts(a, b, t) {
  const k = 1 - t / BEAM_TIME, dx = b[0] - a[0], dy = b[1] - a[1], dz = b[2] - a[2], len = Math.hypot(dx, dy, dz);
  if (k <= 0 || len < 1e-6) return [];
  const yaw = Math.atan2(dx, dz), pitch = -Math.asin(dy / len), w = 0.08 * k + 0.02;
  return [{ m: chain(translate((a[0] + b[0]) / 2, (a[1] + b[1]) / 2, (a[2] + b[2]) / 2), rotY(yaw), rotX(pitch), scale(w, w, len)), color: P.ledGreen, emit: 1 }];
}

// A blast of radius r at p, age t: a hot core and flying bits.
export function boomParts(p, r, t, seed = 1) {
  const out = [], k = t / BOOM_TIME;
  const core = Math.max(0, r * 0.8 * (1 - t / 0.3));
  if (core > 0) out.push({ m: chain(translate(p[0], p[1] + core * 0.3, p[2]), scale(core, core, core)), color: P.sunCore, emit: 1 });
  for (let i = 0; i < 16; i++) {
    const a = i * 2.39996 + seed, up = 0.4 + ((i * 7) % 5) * 0.25, sp = r * (1.5 + (i % 3)), s = 0.3 * (1 - k);
    if (s <= 0) break;
    out.push({ m: chain(translate(p[0] + Math.sin(a) * sp * t, p[1] + up * sp * t - 9 * t * t + 0.3, p[2] + Math.cos(a) * sp * t), rotY(a + t * 7), scale(s, s, s)), color: i % 2 ? P.sunGlow : P.hazardDark, emit: i % 2 ? 0.9 : 0 });
  }
  return out;
}

// deps: {sfx, spatial(p)}
export function createCarGear(deps = {}) {
  const { sfx = {}, spatial = () => undefined } = deps;
  let live = new Map(), at = 0;
  const fx = []; // {k: beam|boom|wreck, a, b, p, r, t}
  const st = {
    get list() { return [...live.values()]; },
    get fxCount() { return fx.length; },
    ingest(list, now) { live = new Map((list || []).map((g) => [g.id, g])); at = now; },
    event(m) {
      const sp = m.p ? spatial(m.p) : undefined;
      switch (m.ev) {
        case "beam": if (m.to) fx.push({ k: "beam", a: m.p, b: m.to, t: 0 }); sfx.carBeam?.(sp); break;
        case "fire": sfx.carFire?.(sp, m.k); break;
        case "boom": fx.push({ k: "boom", p: m.p, r: m.r || 3, t: 0 }); sfx.heBlast?.(sp); break;
        case "wreck": fx.push({ k: "wreck", p: m.p, r: 3, t: 0 }); sfx.heBlast?.(sp); break;
        case "ram": sfx.carRam?.(sp); break;
        case "slip": sfx.carSlip?.(sp, m.k); break;
      }
    },
    update(dt) {
      for (let i = fx.length - 1; i >= 0; i--) {
        const f = fx[i];
        f.t += dt;
        if (f.t > (f.k === "beam" ? BEAM_TIME : f.k === "boom" ? BOOM_TIME : WRECK_TIME)) fx.splice(i, 1);
      }
    },
    solids(time, now) {
      const out = [], dt = Math.min(0.15, Math.max(0, (now - at) / 1000));
      for (const g of live.values()) {
        const v = g.v || [0, 0, 0];
        out.push(...gearParts(g, [g.p[0] + v[0] * dt, g.p[1] + v[1] * dt, g.p[2] + v[2] * dt], time));
      }
      for (const f of fx) {
        if (f.k === "beam") out.push(...beamParts(f.a, f.b, f.t));
        else out.push(...boomParts(f.p, f.r, f.k === "wreck" ? f.t * 0.6 : f.t, f.k === "wreck" ? 7 : 1));
      }
      return out;
    },
  };
  return st;
}
