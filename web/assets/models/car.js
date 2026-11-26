// Placeholder car models (docs/racing.md phase 1; phase 4 makes the real,
// animated ones): one box-built body per type of web/assets/cars.json, in its
// size, with wheels (buggy, racecar), tracks (tank) or an air skirt
// (hovercraft) drawn from the replicated state: the front wheels turn with
// the steering, every wheel spins with the speed and rides its suspension
// (Comp), the skirt floats at the ride height. Model space: the footprint's
// centre on the ground at the origin, the nose towards -z (as a player faces).
//   carParts({spec, pos, yaw, pitch, roll, steer, fwd, comp, contact, time, id})
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";

// The look of each type: body, trim and accent colours (palette names).
export const CAR_LOOK = {
  buggy: { body: "toyYellow", trim: "caseBlack", accent: "neonLime" },
  racecar: { body: "carRed", trim: "steel", accent: "toyPink" },
  tank: { body: "containerGreen", trim: "gunmetal", accent: "hazard" },
  hovercraft: { body: "neonCyan", trim: "caseBlack", accent: "neonMagenta" },
};

// comp: the four wheels' suspension packed a byte each (internal/game cars.go
// CarComp) or an array of four 0..1 values (our own predicted car). Returns 0..1 per wheel.
export function unpackComp(comp) {
  if (Array.isArray(comp)) return comp;
  const n = comp | 0;
  return [0, 1, 2, 3].map((i) => ((n >>> (8 * i)) & 255) / 255);
}

// spec: the car's cars.json entry; the rest is its state (snapshot or predicted).
export function carParts({ spec, pos, yaw = 0, pitch = 0, roll = 0, steer = 0, fwd = 0, comp = 0, contact = 15, time = 0, id = 1 }) {
  const look = CAR_LOOK[spec.id] || CAR_LOOK.buggy;
  const col = (n) => P[n] || P.gunmetal;
  const [w, h, l] = spec.size, ride = spec.ride, travel = spec.travel;
  const hover = spec.drive === "hover", tracks = spec.drive === "tracks";
  const bob = hover ? 0.04 * Math.sin(time * 5 + id) : 0;
  const base = chain(translate(pos[0], pos[1], pos[2]), rotY(yaw), rotX(pitch), rotZ(-roll));
  const box = (x, y, z, sx, sy, sz, c, emit = 0, extra = null) => ({ m: chain(base, translate(x, y, z), ...(extra ? [extra] : []), scale(sx, sy, sz)), color: col(c), emit });
  const out = [];
  const bodyH = Math.max(0.2, h - ride), bodyY = ride + bob + bodyH / 2;
  out.push(box(0, bodyY - bodyH * 0.15, 0, w * 0.92, bodyH * 0.7, l, look.body)); // the hull
  out.push(box(0, bodyY + bodyH * 0.3, l * 0.08, w * 0.7, bodyH * 0.3, l * 0.45, look.trim)); // the cockpit
  out.push(box(0, bodyY - bodyH * 0.1, -l / 2 - 0.02, w * 0.6, bodyH * 0.18, 0.05, look.accent, 0.4)); // the nose stripe
  out.push(box(0, bodyY, l / 2 + 0.02, w * 0.8, 0.12, 0.05, "reviewRed", 0.6)); // tail lights
  const comps = unpackComp(comp);
  if (hover) { // the air skirt, all round, with a fan at the back
    out.push(box(0, ride * 0.5 + bob, 0, w, ride, l * 0.96, "caseBlack"));
    out.push(box(0, bodyY + bodyH * 0.6, l / 2 - 0.35, w * 0.6, 0.6, 0.12, "gunmetal", 0, rotZ(time * 12 * (0.3 + Math.min(1, Math.abs(fwd) / 20)))));
    return out;
  }
  const r = Math.max(0.15, ride * 0.9 + travel * 0.4); // wheel radius
  const spin = (time * fwd) / r; // (drawn: the speed now times the time, as a phase)
  const xs = [-spec.track / 2, spec.track / 2, -spec.track / 2, spec.track / 2], zs = [-spec.wheelbase / 2, -spec.wheelbase / 2, spec.wheelbase / 2, spec.wheelbase / 2];
  if (tracks) { // a track down each side, the road wheels riding their suspension
    for (const s of [-1, 1]) {
      out.push(box(s * (w / 2 - 0.25), r, 0, 0.5, 2 * r, l * 0.95, "tyre"));
      for (let k = 0; k < 5; k++) out.push(box(s * (w / 2 - 0.25), r + 0.01, -l * 0.42 + (k + ((spin * 0.2) % 1 + 1) % 1) * l * 0.17, 0.52, 2 * r + 0.02, 0.06, "gunmetal")); // the treads, scrolling with the speed
    }
    out.push(box(0, bodyY + bodyH * 0.55, -l * 0.25, 0.18, 0.18, l * 0.6, "gunmetal")); // the barrel (the rockets come in phase 3)
    return out;
  }
  for (let i = 0; i < 4; i++) {
    const c = (contact >> i) & 1 ? comps[i] : 0, y = r + (c - 0.5) * travel; // compressed: the wheel rides up
    const turn = i < 2 ? -steer * 0.45 : 0;
    out.push(box(xs[i] * 1.05, y, zs[i], 0.3, 2 * r, 2 * r, "tyre", 0, chain(rotY(turn), rotX(-spin))));
    out.push(box(xs[i] * 1.05 + Math.sign(xs[i]) * 0.16, y, zs[i], 0.02, r, r, "steel", 0, chain(rotY(turn), rotX(-spin)))); // the hub (shows the spin)
  }
  return out;
}
