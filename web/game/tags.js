// Which name tags to draw, CS-style (pure; main.js renders them).
//   - Playing: teammates always (fading with distance); an enemy only while your
//     crosshair is on them and nothing in the map or a smoke is in between,
//     so names never show through walls or smokes.
//   - Observing (spectating or dead): everyone.
//   - Either way, de-cluttered: a tag that overlaps a nearer one is dropped.
import { frameOf, unturn, above } from "../engine/gravity.js";
import * as G from "./shared.gen.js";
import { basis } from "../engine/math.js";
import { RaySolid } from "../engine/sim.gen.js";

export const TAG_FAR = 60;      // metres: no tag beyond this
const FADE_FROM = 20, FADE_MIN = 0.35;
const AIM_RADIUS = 0.45;        // metres around the body's axis that count as "on them"
const TAG_H = 16, CHAR_W = 7;   // approximate tag box in px, for overlap pruning

// Does the segment a->b pass through any solid ({min, max, ramp?}: a box or a
// wedge, RaySolid from internal/sim)?
export function segmentBlocked(boxes, a, b) {
  const d = [b[0] - a[0], b[1] - a[1], b[2] - a[2]];
  for (const bx of boxes) {
    const [t0] = RaySolid(a, d, bx);
    if (t0 >= 0 && t0 <= 1) return true;
  }
  return false;
}

// Distance from point c to the segment a-b (Go segPointDist, generated).
export const segPointDist = (a, b, c) => G.segPointDist(a, b, c);

// Is the view ray (eye, fwd) on a player standing at p (feet), height h?
// The distance between the ray and the body's vertical axis, in front of you.
export function aimedAt(eye, fwd, p, h) {
  // closest points between the ray eye + s*fwd (s >= 0) and the segment p..p+h*up
  const w = [eye[0] - p[0], eye[1] - p[1], eye[2] - p[2]];
  const b = fwd[1], d = fwd[0] * w[0] + fwd[1] * w[1] + fwd[2] * w[2], e = w[1];
  const den = 1 - b * b;
  let s = den > 1e-9 ? (b * e - d) / den : 0, t = den > 1e-9 ? (e - b * d) / den : e;
  t = Math.min(h, Math.max(0, t));
  s = Math.max(0, b * t - d); // re-project with t clamped
  const q = [eye[0] + fwd[0] * s - p[0], eye[1] + fwd[1] * s - p[1] - t, eye[2] + fwd[2] * s - p[2]];
  return Math.hypot(...q) <= AIM_RADIUS;
}

// tagAlpha: full up close, fading to FADE_MIN at TAG_FAR.
export const tagAlpha = (dist) => (dist <= FADE_FROM ? 1 : Math.max(FADE_MIN, 1 - (1 - FADE_MIN) * (dist - FADE_FROM) / (TAG_FAR - FADE_FROM)));

// pickTags: the tags to draw this frame, nearest first.
//   view: { eye, yaw, pitch, frame?, team, observing }   (frame: engine/gravity.js)
//   players: [{ id, name, team, p, crouch, gv? }]   (remote players; gv: their gravity)
//   boxes: the map's solid boxes; project(world) -> [x, y, depth] | null
//   smokes: standing smoke cores [{ c, r }] (nades.smokeCores): no enemy tag through one
// Returns [{ id, name, team, x, y, alpha }].
export function pickTags(view, players, boxes, project, smokes = []) {
  const { fwd } = basis(view.yaw, view.pitch, view.frame);
  const cand = [];
  for (const pl of players) {
    const h = pl.crouch ? 1.2 : 1.8;
    const g = pl.gv | 0, gm = frameOf(g); // a player in a gravity of its own: the tag over its head, out of its wall
    const head = above(pl.p, g, 2.15 - (pl.crouch ? 0.6 : 0));
    const dist = Math.hypot(pl.p[0] - view.eye[0], pl.p[1] - view.eye[1], pl.p[2] - view.eye[2]);
    if (dist > TAG_FAR) continue;
    if (!view.observing && pl.team !== view.team) {
      const rel = unturn(gm, [view.eye[0] - pl.p[0], view.eye[1] - pl.p[1], view.eye[2] - pl.p[2]]); // its frame: it stands up the y axis
      if (!aimedAt(rel, unturn(gm, fwd), [0, 0, 0], h)) continue;
      const chest = above(pl.p, g, h * 0.75);
      if (segmentBlocked(boxes, view.eye, chest)) continue;
      if (smokes.some((s) => segPointDist(view.eye, chest, s.c) < s.r)) continue;
    }
    const s = project(head);
    if (!s) continue;
    cand.push({ id: pl.id, name: pl.name, team: pl.team, x: s[0], y: s[1], dist, alpha: tagAlpha(dist) });
  }
  cand.sort((a, b) => a.dist - b.dist);
  const kept = [];
  for (const c of cand) {
    const w = c.name.length * CHAR_W;
    const over = kept.some((k) => Math.abs(k.x - c.x) * 2 < (w + k.name.length * CHAR_W) && Math.abs(k.y - c.y) < TAG_H);
    if (!over) kept.push(c);
  }
  return kept.map(({ dist, ...t }) => t);
}
