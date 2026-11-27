// A car's kit, as the garage shows it (game/garage.js, docs/racing.md "The
// garage"): its front and rear items as mounted parts and its upgrades as
// visible bits, box-built on top of models/car.js carParts:
//   front item  a lens emitter (beam), a rocket pod (rockets; the heavier ones
//               bigger), an orb cradle (orbs), a ram plate across the nose
//   rear item   a spike hopper, an oil drum, a mine rack (the sticky ones on legs)
//   plating     armour panels down each side, one more row a tier, hazard edged
//   engine      a blower on the engine deck that grows a tier, an exhaust stack a tier
//   tyres       fatter treads round the wheels (wider a tier); a tank: grousers
//   boost tank  a cylinder on the flank, longer a tier, a cyan gauge band
//   charges     ammo crates by the front mount, one a tier
// Model space as carParts (nose towards -z, the footprint's centre on the
// ground); every part carries the slot it belongs to (game/garage.js SLOTS),
// so the garage can light up the one you point at.
//   kitAnchors(spec)                where each slot is on the car (the click targets)
//   kitParts(spec, fit, frame, {time, glow})  the parts; fit: garage.js fitOf; glow: a slot id
import { chain, translate, scale, rotX, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";
import { wheelRadius } from "./car.js";

// Mount heights per type (m): the front and rear item's centre, the engine deck.
const AT = {
  buggy: { mount: 0.62, deck: 0.86, side: 0.55 },
  racecar: { mount: 0.42, deck: 0.78, side: 0.45 },
  tank: { mount: 1.05, deck: 1.36, side: 0.95 },
  hovercraft: { mount: 0.7, deck: 0.76, side: 0.55 },
};
const atOf = (spec) => AT[spec.id] || { mount: spec.ride + 0.3, deck: spec.size[1] * 0.75, side: spec.size[1] * 0.45 };

// Where each slot sits on the car, in model space.
export function kitAnchors(spec) {
  const [w, , l] = spec.size, a = atOf(spec), r = wheelRadius(spec);
  const tyre = spec.drive === "hover" ? [-w / 2, spec.ride * 0.5, -l * 0.25]
    : spec.drive === "tracks" ? [-(w / 2 - 0.25), r, -l * 0.3] : [-spec.track / 2 * (spec.id === "racecar" ? 1.08 : 1), r, -spec.wheelbase / 2];
  return {
    front: [0, a.mount, -l / 2 - 0.2],
    rear: [0, a.mount, l / 2 + 0.25],
    tyres: tyre,
    engine: [0, a.deck + 0.2, l * 0.34],
    plating: [w / 2 + 0.05, a.side, 0],
    tank: [-w / 2 - 0.12, a.side + 0.15, l * 0.12],
    charges: [w * 0.3, a.mount + 0.3, -l / 2 + 0.35],
  };
}

// fit: {front: {kind, id}, rear: {kind, id}, tiers: {charges, tank, plating,
// engine, tyres}}; frame: the car's base matrix (where it stands, its yaw).
export function kitParts(spec, fit, frame, { time = 0, glow = "" } = {}) {
  const [w, h, l] = spec.size, at = atOf(spec), A = kitAnchors(spec), t = fit.tiers || {};
  const out = [];
  let slot = "";
  const lift = (c) => (glow && slot === glow ? c.map((v) => Math.min(1, v * 1.25 + 0.12)) : c);
  const add = (x, y, z, sx, sy, sz, c, emit = 0, extra = null) => {
    const col = typeof c === "string" ? P[c] || P.gunmetal : c;
    out.push({ m: chain(frame, translate(x, y, z), ...(extra ? [extra] : []), scale(sx, sy, sz)), color: lift(col), emit: glow && slot === glow ? Math.max(emit, 0.35) : emit, slot });
  };
  const pulse = 0.6 + 0.4 * Math.sin(time * 4);

  // The front item on a bracket off the nose.
  slot = "front";
  {
    const [x, y, z] = A.front, k = fit.front?.kind || "";
    add(x, y - 0.12, z + 0.15, 0.3, 0.06, 0.4, "gunmetal"); // the bracket
    if (k === "beam") {
      add(x, y, z, 0.26, 0.22, 0.4, "caseBlack");
      add(x, y, z - 0.21, 0.16, 0.14, 0.03, "neonLime", 0.6 + 0.4 * pulse); // the lens
    } else if (k === "rocket") {
      const big = (fit.front?.id || "").startsWith("rollback"), s = big ? 1.35 : 1;
      add(x, y, z + 0.05, 0.5 * s, 0.3 * s, 0.6 * s, "gunmetal");
      for (const u of [-1, 1]) for (const v of [-1, 1]) add(x + u * 0.12 * s, y + v * 0.07 * s, z - 0.26 * s, 0.1 * s, 0.1 * s, 0.03, "hazardDark", 0.25);
      add(x, y + 0.16 * s, z + 0.05, 0.52 * s, 0.03, 0.62 * s, "hazard");
    } else if (k === "orb") {
      for (const u of [-1, 1]) add(x + u * 0.2, y, z + 0.05, 0.05, 0.4, 0.3, "steel"); // the cradle
      add(x, y, z, 0.26, 0.26, 0.26, "neonMagenta", 0.8 * pulse, rotX(time * 1.3));
      add(x, y, z, 0.26, 0.26, 0.26, "neonMagenta", 0.5, rotX(time * 1.3 + Math.PI / 4));
    } else if (k === "ram") {
      add(x, y, z - 0.05, w * 1.05, 0.5, 0.1, "steel");
      for (let i = 0; i < 5; i++) add(x - w * 0.42 + i * w * 0.21, y, z - 0.12, 0.08, 0.46, 0.04, "hazard");
    }
  }

  // The rear item on a hitch off the tail.
  slot = "rear";
  {
    const [x, y, z] = A.rear, k = fit.rear?.kind || "";
    add(x, y - 0.12, z - 0.15, 0.3, 0.06, 0.4, "gunmetal"); // the hitch
    if (k === "spikes") {
      add(x, y, z, 0.6, 0.3, 0.35, "gunmetal");
      for (let i = 0; i < 4; i++) add(x - 0.22 + i * 0.15, y + 0.22, z, 0.04, 0.16, 0.04, "steel", 0, rotZ(0.3 * (i % 2 ? 1 : -1)));
    } else if (k === "oil") {
      add(x, y, z, 0.42, 0.5, 0.42, "caseBlack");
      add(x, y + 0.1, z, 0.44, 0.04, 0.44, "toyPink", 0.2);
      add(x, y - 0.1, z, 0.44, 0.04, 0.44, "toyPink", 0.2);
    } else if (k === "mine") {
      const sticky = (fit.rear?.id || "").startsWith("sticky");
      add(x, y - 0.05, z, 0.6, 0.06, 0.4, "steel"); // the rack
      for (const u of [-1, 0, 1]) {
        add(x + u * 0.18, y + 0.04, z, 0.16, 0.08, 0.16, sticky ? "toyYellow" : "containerGreen");
        add(x + u * 0.18, y + 0.1, z, 0.05, 0.04, 0.05, "reviewRed", 0.8 * pulse);
        if (sticky) for (const v of [-1, 1]) add(x + u * 0.18 + v * 0.07, y - 0.02, z + 0.08, 0.02, 0.1, 0.02, "caseBlack");
      }
    }
  }

  // Plating: armour panels down each flank, one more row a tier.
  slot = "plating";
  for (let k = 0; k < (t.plating | 0); k++) {
    for (const s of [-1, 1]) {
      const y = at.side - 0.18 + k * 0.2, x = s * (w / 2 + 0.03 + k * 0.025);
      add(x, y, 0, 0.05, 0.18, l * (0.72 - k * 0.08), "steel");
      add(x + s * 0.03, y + 0.08, 0, 0.02, 0.03, l * (0.72 - k * 0.08), "hazard");
      for (const z of [-0.3, 0, 0.3]) add(x + s * 0.03, y, z * l * 0.7, 0.03, 0.05, 0.05, "gunmetal"); // rivets
    }
  }

  // Engine: a blower on the deck, bigger a tier, an exhaust stack a tier.
  slot = "engine";
  if (t.engine > 0) {
    const [x, y, z] = A.engine, s = 0.8 + 0.25 * t.engine;
    add(x, y, z, 0.4 * s, 0.3 * s, 0.45 * s, "steel");
    add(x, y + 0.17 * s, z, 0.3 * s, 0.06, 0.35 * s, "gunmetal"); // the scoop
    add(x, y + 0.21 * s, z - 0.05, 0.2 * s, 0.05, 0.12, "sunGlow", 0.3 * pulse);
    for (let k = 0; k < t.engine; k++) {
      for (const sd of [-1, 1]) add(x + sd * (0.28 * s + 0.05), y + 0.1 + k * 0.05, z + 0.1 + k * 0.12, 0.07, 0.4 + k * 0.1, 0.07, "steel", 0, rotX(-0.35));
    }
  }

  // Tyres: fatter treads round each wheel (a tank: grousers on the tracks).
  slot = "tyres";
  if (t.tyres > 0 && spec.drive !== "hover") {
    const r = wheelRadius(spec), extra = 0.05 * t.tyres;
    if (spec.drive === "tracks") {
      for (const s of [-1, 1]) for (let k = 0; k < 4 + 2 * t.tyres; k++) {
        add(s * (w / 2 - 0.25 + 0.27 + extra / 2), r * 0.3, -l * 0.43 + k * (l * 0.86) / (3 + 2 * t.tyres), 0.05 + extra, 0.08, 0.1, "hazardDark");
      }
    } else {
      const k = spec.id === "racecar" ? 1.08 : 1, wide = spec.id === "buggy" ? 0.24 : 0.42;
      for (const [xs, zs] of [[-1, -1], [1, -1], [-1, 1], [1, 1]]) {
        const x = xs * spec.track / 2 * k, z = zs * spec.wheelbase / 2;
        add(x, r, z, wide + extra, 2 * r * 0.9 + extra, 2 * r * 0.55, "tyre");
        add(x, r, z, wide + extra, 2 * r * 0.55, 2 * r * 0.9 + extra, "tyre");
        add(x + xs * (wide + extra) / 2, r, z, 0.02, r * 0.5, r * 0.5, "neonLime", 0.3); // the tier's hub cap
      }
    }
  }

  // Boost tank: a cylinder on the flank (an octagon prism), longer a tier.
  slot = "tank";
  {
    const [x, y, z] = A.tank, len = 0.5 + 0.3 * (t.tank | 0), d = 0.22 + 0.04 * (t.tank | 0);
    add(x, y, z, d, d * 0.7, len, "steel");
    add(x, y, z, d * 0.7, d, len, "steel");
    add(x - 0.01, y, z - len * 0.2, d * 1.02, d * 0.75, 0.05, "neonCyan", 0.5 + 0.3 * pulse); // the gauge band
    add(x, y, z + len / 2 + 0.04, 0.08, 0.08, 0.08, "gunmetal"); // the valve
  }

  // Charges: ammo crates by the front mount, one a tier.
  slot = "charges";
  for (let k = 0; k < (t.charges | 0); k++) {
    const [x, y, z] = A.charges;
    add(x, y - 0.12 + k * 0.17, z, 0.3, 0.15, 0.22, "containerGreen");
    add(x, y - 0.12 + k * 0.17, z - 0.115, 0.12, 0.05, 0.01, "hazard", 0.2);
  }
  void h;
  return out;
}
