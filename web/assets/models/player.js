// The player figure: every box comes from player.json, which is also the hit
// geometry the server ray-traces (posed by gen/boxmodel.js here and its Go twin
// there). Only the muzzle flash is drawn outside the model: it isn't hittable.
// Returns render parts [{m, color, emit}] for the renderer's solid pass.
import { PALETTE as P } from "../palette.js";
import { scaleRgb, hue } from "../../gen/color.js";
import { partsFor, partMatrix, partBox, partFrame, seeded } from "../../gen/boxmodel.js";
import PLAYER from "./player.json" with { type: "json" };

export { PLAYER };

// A part's colour (render-only; the server ignores colours):
//   {vibe: spec, artisan: spec, "*": spec}  per team ("*" = anyone else)
//   spec = base["*" factor]...  base: a palette name, "team"/"player" (the
//   player's personal colour) or "rgb" (gamer RGB, cycling with time); a
//   factor is a number or a seeded var name ("artisanWool*wool*0.85").
function colorOf(spec, p, vars) {
  if (spec && typeof spec === "object") spec = spec[p.team] ?? spec["*"];
  const [base, ...factors] = String(spec ?? "").split("*");
  let c = base === "team" || base === "player" ? p.rgb
    : base === "rgb" ? hue((((p.time || 0) * 0.25 + (p.id || 0) * 0.17) % 1 + 1) % 1)
    : P[base] || [1, 0, 1];
  for (const f of factors) c = scaleRgb(c, Number.isFinite(+f) ? +f : (vars?.[f] ?? 1));
  return c;
}

// p: {p:[x,y,z], yaw, pitch, rgb, crouch: bool (replicated state), team?, id?, time?, flash}
export function playerParts(p) {
  const st = { p: p.p, yaw: p.yaw, pitch: p.pitch, crouch: !!p.crouch, team: p.team, id: p.id };
  const { vars } = seeded(PLAYER, p.id);
  const out = partsFor(PLAYER, st).map((pt) => ({ m: partMatrix(pt, st), color: colorOf(pt.color, p, vars), emit: pt.emit || 0, name: pt.name }));
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
