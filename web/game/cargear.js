// The cars' combat on the client (race phase 3; the server is internal/game
// carcombat.go): the live shots and drops from every full snapshot ("gear":
// [{id, k: the item, p, v, a: armed}]) drawn where they are (shots run ahead
// by their velocity), and the optional "car" events as effects: a beam's
// line, a blast, a wreck's burst. Placeholder looks (phase 4 does the real
// ones); every effect ages with the frames.
import { chain, translate, scale, rotY, rotX, rotZ } from "../engine/math.js";
import { PALETTE as P } from "../assets/palette.js";
import { carItem } from "./cars.js";

const BEAM_TIME = 0.12, BOOM_TIME = 0.7, WRECK_TIME = 1.6, FLASH_TIME = 0.12; // seconds an effect shows

// The boxes of one live piece of gear at p (time: the shader clock, for spin and blink).
export function gearParts(g, p, time) {
  const it = carItem(g.k), kind = it?.kind || "";
  const box = (x, y, z, sx, sy, sz, color, emit = 0, yaw = 0) => ({ m: chain(translate(p[0] + x, p[1] + y, p[2] + z), rotY(yaw), scale(sx, sy, sz)), color, emit });
  const heading = Math.atan2(-(g.v?.[0] || 0), -(g.v?.[2] || 0));
  switch (kind) {
    case "rocket": { // a body with a warhead and fins, spinning; a flame and a trail of smoke behind it
      const big = it.damage > 45 ? 1.4 : 1, f = chain(translate(p[0], p[1], p[2]), rotY(heading));
      const out = [{ m: chain(f, rotZ(time * 20), scale(0.18 * big, 0.18 * big, 0.7 * big)), color: P.steel, emit: 0 },
        { m: chain(f, translate(0, 0, -0.4 * big), rotZ(time * 20), scale(0.13 * big, 0.13 * big, 0.14 * big)), color: big > 1 ? P.diffDel : P.hazard, emit: 0.3 },
        { m: chain(f, translate(0, 0, 0.28 * big), rotZ(time * 20), scale(0.5 * big, 0.04, 0.18 * big)), color: P.gunmetal, emit: 0 },
        { m: chain(f, translate(0, 0, 0.28 * big), rotZ(time * 20), scale(0.04, 0.5 * big, 0.18 * big)), color: P.gunmetal, emit: 0 },
        { m: chain(f, translate(0, 0, 0.5 * big), scale(0.14 * big, 0.14 * big, (0.3 + 0.1 * Math.sin(time * 50 + g.id)) * big)), color: P.sunGlow, emit: 1 }];
      for (let i = 1; i <= 4; i++) { const s = (0.15 + i * 0.09) * big; out.push({ m: chain(f, translate(Math.sin(i * 2.3 + g.id) * 0.06 * i, 0, (0.55 + i * 0.55) * big), rotY(i), scale(s, s, s)), color: P.smoke, emit: 0 }); }
      return out;
    }
    case "orb": { // a glowing core and two satellites orbiting it
      const s = 0.45 + 0.08 * Math.sin(time * 18 + g.id), out = [box(0, 0, 0, s, s, s, P.ledBlue, 1, time * 6), box(0, 0, 0, s * 0.6, s * 0.6, s * 0.6, P.lamp, 1, -time * 9)];
      for (const k of [0, Math.PI]) { const a = time * 7 + k; out.push(box(Math.sin(a) * 0.6, Math.cos(a * 0.7) * 0.25, Math.cos(a) * 0.6, 0.12, 0.12, 0.12, P.neonMagenta, 1, a)); }
      return out;
    }
    case "spikes": { // a strip of spikes across the road
      const out = [box(0, 0.03, 0, it.radius * 1.6, 0.06, 0.5, P.tyre, 0, heading)];
      for (let i = -3; i <= 3; i++) out.push({ m: chain(translate(p[0], p[1], p[2]), rotY(heading), translate(i * it.radius * 0.22, 0.15, 0), scale(0.08, 0.3, 0.08)), color: P.steel, emit: 0 });
      return out;
    }
    case "oil": { // a black puddle, a spaghetti-coloured swirl, a slow rainbow sheen
      const sh = (time * 0.4 + g.id * 0.3) % 1, sheen = [P.neonMagenta, P.neonCyan, P.neonLime][Math.floor(sh * 3)];
      return [box(0, 0.02, 0, it.radius * 1.6, 0.03, it.radius * 1.6, P.hazardDark, 0, g.id), box(0.3, 0.025, -0.2, it.radius, 0.03, it.radius * 0.8, P.chip, 0.05, g.id * 2),
        box(-0.4, 0.03, 0.3, it.radius * 0.5, 0.02, it.radius * 0.3, sheen, 0.25, g.id + time * 0.3)];
    }
    case "mine": { // a puck with a light that blinks once armed and a ring pulsing round it; Scope Creep scuttles on four legs
      const on = g.a && Math.floor(time * 4 + g.id) % 2 === 0;
      const out = [box(0, 0.1, 0, 0.6, 0.2, 0.6, it.seek ? P.containerYellow : P.rack, 0), box(0, 0.25, 0, 0.15, 0.08, 0.15, g.a ? P.diffDel : P.ledAmber, on || !g.a ? 1 : 0.1)];
      if (g.a) { const r = (it.trigger || 1.8) * ((time * 0.8 + g.id * 0.21) % 1); out.push(box(0, 0.03, 0, r * 2, 0.02, 0.06, P.diffDel, 0.6), box(0, 0.03, 0, 0.06, 0.02, r * 2, P.diffDel, 0.6)); }
      if (it.seek) for (let k = 0; k < 4; k++) { const a = k * Math.PI / 2 + Math.PI / 4, w = 0.25 * Math.sin(time * 16 + k * 1.6); out.push(box(Math.sin(a) * 0.42, 0.06 + Math.max(0, w) * 0.2, Math.cos(a) * 0.42, 0.06, 0.06, 0.32, P.gunmetal, 0, a + w)); }
      return out;
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

// A muzzle flash or a ram's spark at p (size r), age t: a hot star, gone in a blink.
export function flashParts(p, r, t) {
  const k = 1 - t / FLASH_TIME;
  if (k <= 0) return [];
  const s = r * k;
  return [{ m: chain(translate(p[0], p[1], p[2]), rotY(t * 30), scale(s, s * 0.5, s)), color: P.sunCore, emit: 1 },
    { m: chain(translate(p[0], p[1], p[2]), rotY(t * 30 + 0.8), scale(s * 0.5, s, s * 0.5)), color: P.sunGlow, emit: 1 }];
}

// A car exploding at p, age t: a fireball that swells and goes dark, a
// shockwave ring along the ground, panels and a wheel flung up, black smoke.
export function wreckParts(p, t) {
  const out = [], k = t / WRECK_TIME;
  if (t < 0.5) { // the fireball: white-hot, then orange, then sooty
    const s = 1.2 + 5 * Math.sqrt(t / 0.5), c = p[1] + 1 + t * 2, hot = t < 0.3;
    for (const [k, a] of [[1, 0], [0.92, Math.PI / 4]]) // two boxes 45 degrees apart: a rounder ball
      out.push({ m: chain(translate(p[0], c, p[2]), rotY(t * 3 + a), rotX(a * 0.5), scale(s * k, s * 0.8 * k, s * k)), color: hot ? P.sunGlow : P.hazardDark, emit: hot ? 0.9 : 0 });
    if (t < 0.25) { const k = s * 0.6 * (1 - t / 0.25); out.push({ m: chain(translate(p[0], c, p[2]), rotY(-t * 5), scale(k, k, k)), color: P.sunCore, emit: 1 }); }
  }
  if (t < 0.6) { // the shockwave: a flat ring of eight slabs running out
    const R = 1 + 14 * t, th = 0.3 * (1 - t / 0.6);
    for (let i = 0; i < 8; i++) { const a = (i / 8) * Math.PI * 2; out.push({ m: chain(translate(p[0] + Math.sin(a) * R, p[1] + 0.15, p[2] + Math.cos(a) * R), rotY(a), scale(R * 0.8, th, 0.25)), color: P.sunGlow, emit: 0.6 }); }
  }
  for (let i = 0; i < 10; i++) { // panels, a wheel: up, out, tumbling, down
    const a = i * 2.39996, sp = 4 + (i % 4) * 2, up = 6 + (i % 3) * 3, y = p[1] + 0.6 + up * t - 9.8 * t * t;
    if (y < p[1] - 0.2) continue;
    const s = i === 0 ? 0.7 : 0.35 + (i % 3) * 0.15;
    out.push({ m: chain(translate(p[0] + Math.sin(a) * sp * t, y, p[2] + Math.cos(a) * sp * t), rotX(t * (5 + i)), rotY(a + t * 4), scale(s, i === 0 ? s : s * 0.3, s)), color: i === 0 ? P.tyre : i % 2 ? P.gunmetal : P.hazardDark, emit: 0 });
  }
  for (let i = 0; i < 6; i++) { // the smoke column rising after it
    const age = Math.max(0, t - 0.2 - i * 0.12);
    if (age <= 0) continue;
    const s = 1 + age * 2.5 * (1 - k * 0.3);
    out.push({ m: chain(translate(p[0] + Math.sin(i * 1.7) * 0.8, p[1] + 1.5 + age * 4, p[2] + Math.cos(i * 1.7) * 0.8), rotY(i), scale(s, s * 0.8, s)), color: P.smokeShade.map((v) => v * 0.3), emit: 0 });
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
        case "fire": { // a shot off the front or a drop off the back, by the item's kind
          const it = carItem(m.k), kind = it?.kind || "";
          if (kind === "rocket") { sfx.carRocket?.(sp, (it.damage || 0) > 45); fx.push({ k: "flash", p: m.p, r: 0.8, t: 0 }); }
          else if (kind === "orb") sfx.carOrb?.(sp);
          else if (kind === "spikes" || kind === "oil" || kind === "mine") sfx.carDrop?.(sp, kind);
          break;
        }
        case "boom": fx.push({ k: "boom", p: m.p, r: m.r || 3, t: 0 }); sfx.carBoom?.(sp); break;
        case "wreck": fx.push({ k: "wreck", p: m.p, r: 3, t: 0 }); sfx.carWreck?.(sp); break;
        case "ram": fx.push({ k: "flash", p: m.p, r: 0.6, t: 0 }); sfx.carRam?.(sp); break;
        case "slip": sfx.carSkid?.(sp); break;
        case "pickup": fx.push({ k: "flash", p: m.p, r: 1.4, t: 0 }); sfx.carPickup?.(sp, m.k); break; // a power-up taken (pickups.go)
      }
    },
    update(dt) {
      for (let i = fx.length - 1; i >= 0; i--) {
        const f = fx[i];
        f.t += dt;
        if (f.t > (f.k === "beam" ? BEAM_TIME : f.k === "flash" ? FLASH_TIME : f.k === "boom" ? BOOM_TIME : WRECK_TIME)) fx.splice(i, 1);
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
        else if (f.k === "flash") out.push(...flashParts(f.p, f.r, f.t));
        else if (f.k === "wreck") out.push(...wreckParts(f.p, f.t));
        else out.push(...boomParts(f.p, f.r, f.t, 1));
      }
      return out;
    },
  };
  return st;
}
