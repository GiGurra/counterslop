// The player figure: every box comes from player.json, which is also the hit
// geometry the server ray-traces (posed by gen/boxmodel.js here and its Go twin
// there). Only the muzzle flash is drawn outside the model: it isn't hittable.
// Returns render parts [{m, color, emit}] for the renderer's solid pass.
import { PALETTE as P } from "../palette.js";
import { scaleRgb, hue } from "../../gen/color.js";
import { partsFor, partMatrix, partBox, partFrame, seeded, poseFrames } from "../../gen/boxmodel.js";
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

// ---- Silly animation ----
// The gait (Go gait_shared.go via gen/gait.js) moves the parts in groups: the
// stumpy feet step, the legs block waddles, everything above rolls and leans
// on it, the head wobbles on top. It is part of the hit pose: the server
// traces shots against the same frames (Model.Frames), and cl_showhitboxes
// draws them (hitParts).
// p: {p:[x,y,z], yaw, pitch, rgb, crouch: bool (replicated state), team?, id?, time?, flash,
//     gait?: the replicated walk cycle (gen/gait.js), t?: its pose time (s)}
// The pose state the server traces: the draw state minus the looks.
const poseState = (p) => ({ p: p.p, yaw: p.yaw, pitch: p.pitch, crouch: !!p.crouch, team: p.team, id: p.id, gait: p.gait || null, t: p.t || 0, gv: p.gv | 0 });

// The parts a player has (partsFor: seeded by its id, by team) and their colours
// (its personal colour, the seeded vars) don't change from frame to frame: kept
// per player instead of rebuilt for every player every frame. A colour that
// cycles with time ("rgb") is null here and worked out per frame.
const MAX_CACHED = 256;
const partsCache = new Map(), colorCache = new Map();
const remember = (map, key, make) => {
  let v = map.get(key);
  if (v === undefined) {
    if (map.size >= MAX_CACHED) map.clear();
    map.set(key, v = make());
  }
  return v;
};
const baseOf = (spec, team) => String((spec && typeof spec === "object" ? spec[team] ?? spec["*"] : spec) ?? "").split("*")[0];
const partsOf = (st) => remember(partsCache, `${st.id | 0}|${st.team || ""}`, () => partsFor(PLAYER, st));
const colorsOf = (p, parts, vars) => remember(colorCache, `${p.id | 0}|${p.team}|${p.rgb}`,
  () => parts.map((pt) => (baseOf(pt.color, p.team) === "rgb" ? null : colorOf(pt.color, p, vars))));

export function playerParts(p) {
  const st = poseState(p);
  const { vars } = seeded(PLAYER, p.id);
  const frames = poseFrames(PLAYER, st);
  const draw = (pt) => partMatrix(pt, st, frames);
  const parts = partsOf(st), colors = colorsOf(p, parts, vars);
  const out = parts.map((pt, i) => ({ m: draw(pt), color: colors[i] ?? colorOf(pt.color, p, vars), emit: pt.emit || 0, name: pt.name }));
  if (p.flash > 0) {
    const tip = PLAYER.parts.find((pt) => pt.name === "gunTip");
    if (tip) {
      const b = partBox(tip, st.crouch);
      const m = draw({ ...tip, c: [b.c[0], b.c[1], b.c[2] - 0.12], s: [0.22, 0.22, 0.22], crouch: tip.crouch && { pivot: tip.crouch.pivot } });
      out.push({ m, color: P.flash, emit: 1 });
    }
  }
  return out;
}

// The carrier frame for things worn on the torso (the Big Diff, a GPU on the
// back): the upper-body group's world transform (feet at the origin, facing
// -z), so an attachment sways, bobs and leans with the torso. p: the draw state
// of playerParts; without a gait it is translate(p) rotY(yaw).
export function torsoFrame(p) {
  const f = poseFrames(PLAYER, poseState(p));
  return f.groups ? f.groups[1] : f.base; // 1: Go GroupUpper
}

// The hittable parts (cl_showhitboxes outlines): [{m: frame, c, s, region}].
export function hitParts(st) {
  const frames = poseFrames(PLAYER, st);
  return partsOf(st).filter((pt) => pt.hit !== false).map((pt) => ({ ...partFrame(pt, st, frames), region: pt.region }));
}
