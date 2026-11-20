// The player figure: every box comes from player.json, which is also the hit
// geometry the server ray-traces (posed by gen/boxmodel.js here and its Go twin
// there). Only the muzzle flash is drawn outside the model: it isn't hittable.
// Returns render parts [{m, color, emit}] for the renderer's solid pass.
import { PALETTE as P } from "../palette.js";
import { scaleRgb } from "../../gen/color.js";
import { partsFor, partMatrix, partBox, partFrame } from "../../gen/boxmodel.js";
import PLAYER from "./player.json" with { type: "json" };

export { PLAYER };

function colorOf(name, team) {
  if (name === "team") return team;
  const m = /^team\*([\d.]+)$/.exec(name);
  if (m) return scaleRgb(team, parseFloat(m[1]));
  return P[name] || [1, 0, 1];
}

// p: {p:[x,y,z], yaw, pitch, rgb, crouch: bool (replicated state), team?, flash}
export function playerParts(p) {
  const st = { p: p.p, yaw: p.yaw, pitch: p.pitch, crouch: !!p.crouch, team: p.team };
  const out = partsFor(PLAYER, st).map((pt) => ({ m: partMatrix(pt, st), color: colorOf(pt.color, p.rgb), emit: pt.emit || 0 }));
  if (p.flash > 0) {
    const tip = PLAYER.parts.find((pt) => pt.name === "gunTip");
    if (tip) {
      const b = partBox(tip, st.crouch);
      const m = partMatrix({ ...tip, c: [b.c[0], b.c[1], b.c[2] - 0.12], s: [0.22, 0.22, 0.22], crouch: tip.crouch && { pivot: tip.crouch.pivot } }, st);
      out.push({ m, color: P.flash, emit: 1 });
    }
  }
  return out;
}

// The hittable parts (cl_showhitboxes outlines): [{m: frame, c, s, region}].
export function hitParts(st) {
  return partsFor(PLAYER, st).filter((pt) => pt.hit !== false).map((pt) => ({ ...partFrame(pt, st), region: pt.region }));
}
