// The player figure: every box comes from player.json, which is also the hit
// geometry the server ray-traces (posed by gen/boxmodel.js here and its Go twin
// there). Only the muzzle flash is drawn outside the model: it isn't hittable.
// Returns render parts [{m, color, emit}] for the renderer's solid pass.
import { PALETTE as P } from "../palette.js";
import { scaleRgb, hue } from "../../gen/color.js";
import { partsFor, partMatrix, partBox, partFrame, seeded } from "../../gen/boxmodel.js";
import { gaitPose, newPose } from "../../gen/gait.js";
import { chain, translate, rotX, rotY, rotZ } from "../../engine/math.js";
import { rng } from "../../gen/rng.js";
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

// ---- Silly animation (draw only) ----
// The gait (gen/gait.js) moves groups of parts on top of the replicated pose:
// the stumpy feet step, the legs block waddles, everything above rolls and
// leans on it, the head wobbles on top. The hit pose (partFrame, hitParts,
// rayModel, the server's twin) never sees any of it.
const FEET = { footL: "footL", footR: "footR" };
const groupOf = (pt) => FEET[pt.name] || (pt.name === "legs" ? "legs" : pt.region === "head" ? "head" : "upper");
// Joint heights (standing, crouched): top of the legs block, top of the body.
const HIP = [0.45, 0.3], NECK = [1.35, 0.75];

// Per-player silliness, seeded by id: how wide this one swings, waddles, wobbles.
const sillyCache = new Map();
export function silly(id) {
  const key = id | 0;
  let k = sillyCache.get(key);
  if (!k) {
    const r = rng((7000 + key) >>> 0);
    sillyCache.set(key, k = { swing: r.range(0.85, 1.2), waddle: r.range(0.6, 1.5), bob: r.range(0.75, 1.3), wobble: r.range(0.5, 1.6), lean: r.range(0.7, 1.3), offset: r.range(0, Math.PI * 2) });
  }
  return k;
}

const pose = newPose(), ZERO = [0, 0, 0];
// The group transforms (player frame) for this gait at alpha.
function groupMatrices(gait, alpha, id, crouched) {
  gaitPose(gait, alpha, silly(id), pose);
  const hip = HIP[crouched ? 1 : 0], neck = NECK[crouched ? 1 : 0];
  const legs = chain(translate(0, pose.body.y, 0), rotZ(pose.legsRoll));
  const upper = chain(legs, translate(0, hip, 0), rotZ(pose.body.roll - pose.legsRoll), rotX(pose.body.pitch), translate(0, -hip, 0));
  const head = chain(upper, translate(0, neck, 0), rotZ(pose.head.roll), rotX(pose.head.pitch), translate(0, -neck, 0));
  const foot = (f, pt) => {
    const c = pt ? partBox(pt, crouched).c : ZERO;
    return chain(translate(f.x + c[0], f.y + c[1], f.z + c[2]), rotX(f.rx), translate(-c[0], -c[1], -c[2]));
  };
  return { legs, upper, head, footL: foot(pose.footL, FOOT_PARTS.footL), footR: foot(pose.footR, FOOT_PARTS.footR) };
}
const FOOT_PARTS = { footL: PLAYER.parts.find((pt) => pt.name === "footL"), footR: PLAYER.parts.find((pt) => pt.name === "footR") };

// p: {p:[x,y,z], yaw, pitch, rgb, crouch: bool (replicated state), team?, id?, time?, flash,
//     gait?: gen/gait state (draw-only animation), alpha?: 0..1 between its ticks}
export function playerParts(p) {
  const st = { p: p.p, yaw: p.yaw, pitch: p.pitch, crouch: !!p.crouch, team: p.team, id: p.id };
  const { vars } = seeded(PLAYER, p.id);
  let draw = (pt) => partMatrix(pt, st);
  if (p.gait) {
    const g = groupMatrices(p.gait, p.alpha ?? 1, p.id, st.crouch);
    const base = chain(translate(...st.p), rotY(st.yaw)), local = { ...st, p: ZERO, yaw: 0 };
    for (const k in g) g[k] = chain(base, g[k]);
    draw = (pt) => chain(g[groupOf(pt)], partMatrix(pt, local));
  }
  const out = partsFor(PLAYER, st).map((pt) => ({ m: draw(pt), color: colorOf(pt.color, p, vars), emit: pt.emit || 0, name: pt.name }));
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

// The carrier frame for things worn on the torso (the Big Diff, a GPU on the back):
// the upper-body group's world transform, feet at the origin, facing -z, so an
// attachment placed in it sways, bobs and leans with the torso it hangs on.
// p: the same draw state as playerParts (gait/alpha optional: without them it is
// the unanimated hit pose's frame).
export function torsoFrame(p) {
  const base = chain(translate(...p.p), rotY(p.yaw));
  if (!p.gait) return base;
  return chain(base, groupMatrices(p.gait, p.alpha ?? 1, p.id, !!p.crouch).upper);
}

// The hittable parts (cl_showhitboxes outlines): [{m: frame, c, s, region}].
export function hitParts(st) {
  return partsFor(PLAYER, st).filter((pt) => pt.hit !== false).map((pt) => ({ ...partFrame(pt, st), region: pt.region }));
}
